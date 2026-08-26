package nativeplugins

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

func TestManagedHTTPHasNoEnvironmentProxyAndRequiresDeclaredHTTPS443(t *testing.T) {
	client, err := newManagedHTTPClient(nativeplugin.Manifest{Network: []nativeplugin.NetworkClaim{{Host: "api.example.com"}}}, nativeplugin.Config{})
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.MaxResponseHeaderBytes != 64<<10 {
		t.Fatalf("managed transport = %#v", transport)
	}
	for raw, allowed := range map[string]bool{
		"https://api.example.com/v1":     true,
		"https://api.example.com:443/v1": true,
		"https://api.example.com:8443":   false,
		"http://api.example.com":         false,
		"https://other.example.com":      false,
	} {
		parsed, _ := url.Parse(raw)
		if got := client.validateURL(parsed) == nil; got != allowed {
			t.Fatalf("validateURL(%q) = %v", raw, got)
		}
	}
}

func TestLimitedBodyAllowsExactLimitAndRejectsOverflow(t *testing.T) {
	read := func(value string, limit int64) error {
		body := &limitedBody{reader: strings.NewReader(value), closer: io.NopCloser(strings.NewReader("")), remaining: limit, message: "too large"}
		_, err := io.ReadAll(body)
		return err
	}
	if err := read("1234", 4); err != nil {
		t.Fatalf("exact limit: %v", err)
	}
	if err := read("12345", 4); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("overflow error = %v", err)
	}
	if err := read("", 0); err != nil {
		t.Fatalf("empty body: %v", err)
	}
}

func TestManagedHTTPValueRequestRejectsUnsafeMethodsAndHeaders(t *testing.T) {
	client, err := newManagedHTTPClient(nativeplugin.Manifest{Network: []nativeplugin.NetworkClaim{{Host: "api.example.com"}}}, nativeplugin.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(context.Background(), nativeplugin.HTTPRequest{Method: http.MethodConnect, URL: "https://api.example.com"}); err == nil {
		t.Fatal("CONNECT method was accepted")
	}
	for header, allowed := range map[string]bool{"Authorization": true, "Content-Type": true, "Host": false, "Connection": false, "Proxy-Authorization": false, "User-Agent": false} {
		if got := allowedPluginHeader(header); got != allowed {
			t.Fatalf("allowedPluginHeader(%q) = %v", header, got)
		}
	}
}
