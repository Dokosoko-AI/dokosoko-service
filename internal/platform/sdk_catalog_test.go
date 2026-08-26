package platform

import "testing"

func TestCanonicalSDKInstallCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		ecosystem  string
		coordinate string
		version    string
		want       string
	}{
		{name: "npm", ecosystem: "npm", coordinate: "@acme/platform-sdk", version: "3.2.1-beta.2+build.7", want: "npm install @acme/platform-sdk@3.2.1-beta.2+build.7"},
		{name: "pypi", ecosystem: "pypi", coordinate: "acme-sdk", version: "2.0rc1.post2.dev3+linux_x86_64", want: "python -m pip install acme-sdk==2.0rc1.post2.dev3+linux_x86_64"},
		{name: "go", ecosystem: "go", coordinate: "github.com/acme/platform-sdk-go/v2", version: "v2.0.1-0.20260102030405-abcdef123456", want: "go get github.com/acme/platform-sdk-go/v2@v2.0.1-0.20260102030405-abcdef123456"},
		{name: "cargo", ecosystem: "cargo", coordinate: "acme-sdk", version: "1.2.3-beta.1", want: "cargo add acme-sdk@=1.2.3-beta.1"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := canonicalSDKInstallCommand(test.ecosystem, test.coordinate, test.version)
			if err != nil {
				t.Fatalf("canonicalSDKInstallCommand() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("canonicalSDKInstallCommand() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCanonicalSDKInstallCommandRejectsUnsafeOrFloatingIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		ecosystem  string
		coordinate string
		version    string
	}{
		{name: "unsupported ecosystem", ecosystem: "maven", coordinate: "acme-sdk", version: "1.2.3"},
		{name: "npm URL", ecosystem: "npm", coordinate: "https://registry.example/acme.tgz", version: "1.2.3"},
		{name: "npm option", ecosystem: "npm", coordinate: "--registry=example.test", version: "1.2.3"},
		{name: "npm floating tag", ecosystem: "npm", coordinate: "acme-sdk", version: "latest"},
		{name: "npm range", ecosystem: "npm", coordinate: "acme-sdk", version: "^1.2.3"},
		{name: "pypi extras", ecosystem: "pypi", coordinate: "acme[security]", version: "1.2.3"},
		{name: "pypi range", ecosystem: "pypi", coordinate: "acme-sdk", version: ">=1.2.3"},
		{name: "go URL", ecosystem: "go", coordinate: "https://github.com/acme/sdk", version: "v1.2.3"},
		{name: "go branch", ecosystem: "go", coordinate: "github.com/acme/sdk", version: "main"},
		{name: "cargo option", ecosystem: "cargo", coordinate: "--git", version: "1.2.3"},
		{name: "cargo range", ecosystem: "cargo", coordinate: "acme-sdk", version: "1.2"},
		{name: "shell substitution", ecosystem: "npm", coordinate: "$(id)", version: "1.2.3"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := canonicalSDKInstallCommand(test.ecosystem, test.coordinate, test.version); err == nil {
				t.Fatal("canonicalSDKInstallCommand() error = nil")
			}
		})
	}
}
