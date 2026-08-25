package tools

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestPinnedPerCallTransportCannotRetainIdleConnections(t *testing.T) {
	runtime := NewRuntime(nil, nil, nil)
	parsed, err := url.Parse("http://api.vendor.localhost/status")
	if err != nil {
		t.Fatal(err)
	}
	client, ok := runtime.client(parsed, net.ParseIP("127.0.0.1"), time.Second).(*http.Client)
	if !ok {
		t.Fatalf("client type = %T", runtime.client(parsed, net.ParseIP("127.0.0.1"), time.Second))
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || !transport.DisableKeepAlives {
		t.Fatalf("per-call pinned transport can retain an idle connection: %#v", client.Transport)
	}
}

func TestUpstreamIdempotencyKeyIsStableAndNamespaced(t *testing.T) {
	tool := model.Tool{ID: "tool_one", Revision: 4}
	principal := Principal{Issuer: "https://issuer.example", Subject: "user-a", CustomerAccountID: "customer-a", InstallationID: "install-a", RequestID: "request-one", IdempotencyKey: "client-invocation-0001"}
	first := upstreamIdempotencyKey("product-a", tool, principal)
	principal.RequestID = "request-two"
	if retry := upstreamIdempotencyKey("product-a", tool, principal); retry != first {
		t.Fatalf("retry key changed with request ID: %q != %q", retry, first)
	}
	if first == principal.IdempotencyKey || !strings.HasPrefix(first, "doko_") || len(first) != 69 || !validIdempotencyKey(first) {
		t.Fatalf("upstream key is not a bounded opaque namespace: %q", first)
	}
	variants := []struct {
		product   string
		tool      model.Tool
		principal Principal
	}{
		{product: "product-b", tool: tool, principal: principal},
		{product: "product-a", tool: model.Tool{ID: "tool_two", Revision: tool.Revision}, principal: principal},
		{product: "product-a", tool: model.Tool{ID: tool.ID, Revision: tool.Revision + 1}, principal: principal},
		{product: "product-a", tool: tool, principal: func() Principal { value := principal; value.Issuer = "https://other.example"; return value }()},
		{product: "product-a", tool: tool, principal: func() Principal { value := principal; value.Subject = "user-b"; return value }()},
		{product: "product-a", tool: tool, principal: func() Principal { value := principal; value.CustomerAccountID = "customer-b"; return value }()},
		{product: "product-a", tool: tool, principal: func() Principal { value := principal; value.InstallationID = "install-b"; return value }()},
	}
	for index, variant := range variants {
		if candidate := upstreamIdempotencyKey(variant.product, variant.tool, variant.principal); candidate == first {
			t.Fatalf("namespace variant %d collided with %q", index, first)
		}
	}
}

func TestUnsafeIPRejectsIPv6TransitionAndLocalUseRanges(t *testing.T) {
	for _, raw := range []string{"64:ff9b::c000:201", "64:ff9b:1::c000:201", "fec0::1", "2002:c000:0201::1", "2001::1", "::192.0.2.1"} {
		if !unsafeIP(net.ParseIP(raw)) {
			t.Fatalf("special IPv6 address %s was accepted", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if unsafeIP(net.ParseIP(raw)) {
			t.Fatalf("public address %s was rejected", raw)
		}
	}
}

func TestPerProcessUpstreamLimiterIsBoundedPrunedAndFailClosed(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	runtime := NewRuntime(nil, nil, nil)
	runtime.now = func() time.Time { return now }
	for index := 0; index < maxUpstreamRateWindows; index++ {
		tool := model.Tool{ID: fmt.Sprintf("tool-%04d", index)}
		if !runtime.allowUpstreamConnection("product", tool) {
			t.Fatalf("active window %d was rejected before the cap", index)
		}
	}
	if len(runtime.rates) != maxUpstreamRateWindows {
		t.Fatalf("rate window map size=%d", len(runtime.rates))
	}
	if runtime.allowUpstreamConnection("product", model.Tool{ID: "overflow"}) {
		t.Fatal("new window was admitted after the per-process cap")
	}
	if len(runtime.rates) != maxUpstreamRateWindows {
		t.Fatalf("failed admission changed map size=%d", len(runtime.rates))
	}
	now = now.Add(upstreamConnectionWindow)
	if !runtime.allowUpstreamConnection("product", model.Tool{ID: "after-expiry"}) {
		t.Fatal("new window was rejected after expired windows should have been pruned")
	}
	if len(runtime.rates) != 1 {
		t.Fatalf("expired windows were not pruned: size=%d", len(runtime.rates))
	}
}
