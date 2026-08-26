package platform

import (
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMinimumToolAuthorizationUsesFirstClassEffect(t *testing.T) {
	for _, test := range []struct {
		tool model.Tool
		want string
	}{
		{tool: model.Tool{BackendKind: "native", HTTPMethod: "NATIVE", Effect: "read"}, want: "read"},
		{tool: model.Tool{BackendKind: "native", HTTPMethod: "NATIVE", Effect: "write"}, want: "write"},
		{tool: model.Tool{BackendKind: "mcp", HTTPMethod: "MCP", Effect: "destructive"}, want: "destructive"},
		{tool: model.Tool{BackendKind: "native", HTTPMethod: "NATIVE"}, want: "destructive"},
		{tool: model.Tool{BackendKind: "http", HTTPMethod: "GET"}, want: "read"},
	} {
		got, classified := minimumToolAuthorizationAction(test.tool)
		if !classified || got != test.want {
			t.Fatalf("minimumToolAuthorizationAction(%#v) = %q, %v", test.tool, got, classified)
		}
	}
}
