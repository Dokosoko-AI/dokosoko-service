package tools

import (
	"fmt"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestRuntimeBoundsPerConnectionAndGlobalUpstreamConcurrency(t *testing.T) {
	runtime := NewRuntime(nil, nil, nil)
	first := model.Tool{ID: "tool-1", APIConnectionID: "connection-1"}
	for index := 0; index < maxConnectionConcurrency; index++ {
		if !runtime.acquireUpstreamSlot("product-1", first) {
			t.Fatalf("per-connection slot %d was unexpectedly rejected", index+1)
		}
	}
	if runtime.acquireUpstreamSlot("product-1", first) {
		t.Fatal("per-connection concurrency limit was not enforced")
	}
	for index := 0; index < maxConnectionConcurrency; index++ {
		runtime.releaseUpstreamSlot("product-1", first)
	}
	if len(runtime.connectionInFlight) != 0 || runtime.globalInFlight != 0 {
		t.Fatalf("released slots were retained: global=%d connections=%#v", runtime.globalInFlight, runtime.connectionInFlight)
	}

	tools := make([]model.Tool, 0, maxUpstreamConcurrency)
	for index := 0; index < maxUpstreamConcurrency; index++ {
		tool := model.Tool{ID: fmt.Sprintf("tool-%d", index), APIConnectionID: fmt.Sprintf("connection-%d", index)}
		if !runtime.acquireUpstreamSlot("product-1", tool) {
			t.Fatalf("global slot %d was unexpectedly rejected", index+1)
		}
		tools = append(tools, tool)
	}
	if runtime.acquireUpstreamSlot("product-1", model.Tool{ID: "overflow", APIConnectionID: "overflow"}) {
		t.Fatal("global upstream concurrency limit was not enforced")
	}
	for _, tool := range tools {
		runtime.releaseUpstreamSlot("product-1", tool)
	}
}
