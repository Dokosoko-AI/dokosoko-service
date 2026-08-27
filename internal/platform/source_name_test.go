package platform_test

import (
	"context"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestCreateSourceDerivesNameFromLocation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		kind     string
		location string
		want     string
	}{
		{name: "website path", kind: "website", location: "https://docs.example.com/guides/", want: "docs.example.com/guides"},
		{name: "website root", kind: "website", location: "https://docs.example.com/", want: "docs.example.com"},
		{name: "Git repository", kind: "git", location: "https://github.com/vendor/docs.git", want: "vendor/docs"},
		{name: "OpenAPI document", kind: "openapi", location: "https://api.example.com/openapi.yaml", want: "api.example.com/openapi.yaml"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := platform.New(store.NewMemory())
			source, err := service.CreateSource(context.Background(), "org_acme", "prod_acme", "", test.kind, test.location, platform.Actor{ID: "user_root"})
			if err != nil {
				t.Fatal(err)
			}
			if source.Name != test.want {
				t.Fatalf("source name = %q, want %q", source.Name, test.want)
			}
		})
	}
}
