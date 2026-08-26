package platform

import (
	"strings"
	"testing"
)

func TestNormalizeSDKReferenceAcceptsOnlyCanonicalInstallCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ecosystem      string
		coordinate     string
		exactVersion   string
		installCommand string
	}{
		{
			name:           "scoped npm package",
			ecosystem:      " NPM ",
			coordinate:     " @acme/platform-sdk ",
			exactVersion:   " 3.2.1-beta.2+build.7 ",
			installCommand: " npm install @acme/platform-sdk@3.2.1-beta.2+build.7 ",
		},
		{
			name:           "pypi project",
			ecosystem:      "pypi",
			coordinate:     "acme-sdk",
			exactVersion:   "2.0rc1.post2.dev3+linux_x86_64",
			installCommand: "python -m pip install acme-sdk==2.0rc1.post2.dev3+linux_x86_64",
		},
		{
			name:           "go module pseudo-version",
			ecosystem:      "go",
			coordinate:     "github.com/acme/platform-sdk-go/v2",
			exactVersion:   "v2.0.1-0.20260102030405-abcdef123456",
			installCommand: "go get github.com/acme/platform-sdk-go/v2@v2.0.1-0.20260102030405-abcdef123456",
		},
		{
			name:           "cargo crate exact requirement",
			ecosystem:      "cargo",
			coordinate:     "acme-sdk",
			exactVersion:   "1.2.3-beta.1",
			installCommand: "cargo add acme-sdk@=1.2.3-beta.1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := SDKReferenceInput{
				Ecosystem:      test.ecosystem,
				Coordinate:     test.coordinate,
				ExactVersion:   test.exactVersion,
				InstallCommand: test.installCommand,
			}

			got, err := normalizeSDKReference(input)
			if err != nil {
				t.Fatalf("normalizeSDKReference() error = %v", err)
			}
			if got.Ecosystem != strings.ToLower(strings.TrimSpace(test.ecosystem)) {
				t.Errorf("Ecosystem = %q", got.Ecosystem)
			}
			if got.Coordinate != strings.TrimSpace(test.coordinate) {
				t.Errorf("Coordinate = %q", got.Coordinate)
			}
			if got.ExactVersion != strings.TrimSpace(test.exactVersion) {
				t.Errorf("ExactVersion = %q", got.ExactVersion)
			}
			if got.InstallCommand != strings.TrimSpace(test.installCommand) {
				t.Errorf("InstallCommand = %q", got.InstallCommand)
			}
		})
	}
}

func TestNormalizeSDKReferenceRejectsUnsupportedEcosystems(t *testing.T) {
	t.Parallel()

	for _, ecosystem := range []string{"", "pip", "pnpm", "maven", "shell", "npm && curl"} {
		t.Run(ecosystem, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeSDKReference(SDKReferenceInput{
				Ecosystem:      ecosystem,
				Coordinate:     "acme-sdk",
				ExactVersion:   "1.2.3",
				InstallCommand: "npm install acme-sdk@1.2.3",
			})
			if err == nil {
				t.Fatal("normalizeSDKReference() error = nil")
			}
			if ecosystem != "" && !strings.Contains(err.Error(), "supported ecosystems") {
				t.Fatalf("error = %q, want supported ecosystem guidance", err)
			}
		})
	}
}

func TestNormalizeSDKReferenceRejectsUnsafeCoordinates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ecosystem      string
		coordinate     string
		exactVersion   string
		installCommand string
	}{
		{name: "npm URL", ecosystem: "npm", coordinate: "https://registry.example/acme.tgz", exactVersion: "1.2.3", installCommand: "npm install https://registry.example/acme.tgz"},
		{name: "npm alias", ecosystem: "npm", coordinate: "alias@npm:acme-sdk", exactVersion: "1.2.3", installCommand: "npm install alias@npm:acme-sdk@1.2.3"},
		{name: "npm option", ecosystem: "npm", coordinate: "--registry=example.test", exactVersion: "1.2.3", installCommand: "npm install --registry=example.test@1.2.3"},
		{name: "pypi URL", ecosystem: "pypi", coordinate: "acme@https://example.test/acme.whl", exactVersion: "1.2.3", installCommand: "python -m pip install acme@https://example.test/acme.whl==1.2.3"},
		{name: "pypi extras", ecosystem: "pypi", coordinate: "acme[security]", exactVersion: "1.2.3", installCommand: "python -m pip install acme[security]==1.2.3"},
		{name: "go URL", ecosystem: "go", coordinate: "https://github.com/acme/sdk", exactVersion: "v1.2.3", installCommand: "go get https://github.com/acme/sdk@v1.2.3"},
		{name: "go credential", ecosystem: "go", coordinate: "user:secret@github.com/acme/sdk", exactVersion: "v1.2.3", installCommand: "go get user:secret@github.com/acme/sdk@v1.2.3"},
		{name: "go package wildcard", ecosystem: "go", coordinate: "github.com/acme/sdk/...", exactVersion: "v1.2.3", installCommand: "go get github.com/acme/sdk/...@v1.2.3"},
		{name: "cargo option", ecosystem: "cargo", coordinate: "--git", exactVersion: "1.2.3", installCommand: "cargo add --git@=1.2.3"},
		{name: "shell substitution", ecosystem: "npm", coordinate: "$(id)", exactVersion: "1.2.3", installCommand: "npm install $(id)@1.2.3"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeSDKReference(SDKReferenceInput{
				Ecosystem:      test.ecosystem,
				Coordinate:     test.coordinate,
				ExactVersion:   test.exactVersion,
				InstallCommand: test.installCommand,
			})
			if err == nil {
				t.Fatal("normalizeSDKReference() error = nil")
			}
		})
	}
}

func TestNormalizeSDKReferenceRejectsFloatingOrUnsafeVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ecosystem      string
		coordinate     string
		exactVersion   string
		installCommand string
	}{
		{name: "npm latest", ecosystem: "npm", coordinate: "acme-sdk", exactVersion: "latest", installCommand: "npm install acme-sdk@latest"},
		{name: "npm tag", ecosystem: "npm", coordinate: "acme-sdk", exactVersion: "next", installCommand: "npm install acme-sdk@next"},
		{name: "npm partial", ecosystem: "npm", coordinate: "acme-sdk", exactVersion: "1.2", installCommand: "npm install acme-sdk@1.2"},
		{name: "npm range", ecosystem: "npm", coordinate: "acme-sdk", exactVersion: "^1.2.3", installCommand: "npm install acme-sdk@^1.2.3"},
		{name: "pypi wildcard", ecosystem: "pypi", coordinate: "acme-sdk", exactVersion: "1.2.*", installCommand: "python -m pip install acme-sdk==1.2.*"},
		{name: "pypi range", ecosystem: "pypi", coordinate: "acme-sdk", exactVersion: ">=1.2.3", installCommand: "python -m pip install acme-sdk>=1.2.3"},
		{name: "pypi shell-significant epoch", ecosystem: "pypi", coordinate: "acme-sdk", exactVersion: "1!2.0", installCommand: "python -m pip install acme-sdk==1!2.0"},
		{name: "go branch", ecosystem: "go", coordinate: "github.com/acme/sdk", exactVersion: "main", installCommand: "go get github.com/acme/sdk@main"},
		{name: "go revision", ecosystem: "go", coordinate: "github.com/acme/sdk", exactVersion: "abcdef123456", installCommand: "go get github.com/acme/sdk@abcdef123456"},
		{name: "cargo compatible range", ecosystem: "cargo", coordinate: "acme-sdk", exactVersion: "1.2", installCommand: "cargo add acme-sdk@=1.2"},
		{name: "cargo ignored metadata", ecosystem: "cargo", coordinate: "acme-sdk", exactVersion: "1.2.3+build.4", installCommand: "cargo add acme-sdk@=1.2.3+build.4"},
		{name: "environment expansion", ecosystem: "npm", coordinate: "acme-sdk", exactVersion: "$VERSION", installCommand: "npm install acme-sdk@$VERSION"},
		{name: "command substitution", ecosystem: "npm", coordinate: "acme-sdk", exactVersion: "$(id)", installCommand: "npm install acme-sdk@$(id)"},
		{name: "URL", ecosystem: "npm", coordinate: "acme-sdk", exactVersion: "https://example.test/sdk.tgz", installCommand: "npm install acme-sdk@https://example.test/sdk.tgz"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeSDKReference(SDKReferenceInput{
				Ecosystem:      test.ecosystem,
				Coordinate:     test.coordinate,
				ExactVersion:   test.exactVersion,
				InstallCommand: test.installCommand,
			})
			if err == nil {
				t.Fatal("normalizeSDKReference() error = nil")
			}
		})
	}
}

func TestNormalizeSDKReferenceRejectsCommandInjectionAndMismatches(t *testing.T) {
	t.Parallel()

	const canonical = "npm install @acme/platform-sdk@3.2.1"
	commands := map[string]string{
		"different coordinate":      "npm install @attacker/platform-sdk@3.2.1",
		"different version":         "npm install @acme/platform-sdk@3.2.2",
		"different ecosystem":       "python -m pip install @acme/platform-sdk==3.2.1",
		"short command alias":       "npm i @acme/platform-sdk@3.2.1",
		"extra option":              canonical + " --ignore-scripts",
		"credentialed registry":     canonical + " --registry=https://user:secret@example.test",
		"URL package":               "npm install https://example.test/platform-sdk.tgz",
		"semicolon":                 canonical + "; curl https://example.test",
		"and operator":              canonical + " && id",
		"or operator":               canonical + " || id",
		"pipe":                      canonical + " | sh",
		"background operator":       canonical + " & id",
		"bang operator":             canonical + " ! id",
		"redirect":                  canonical + " > output",
		"input redirect":            canonical + " < input",
		"command substitution":      canonical + " $(id)",
		"braced environment":        canonical + " ${TOKEN}",
		"unbraced environment":      canonical + " $TOKEN",
		"backticks":                 canonical + " `id`",
		"comment":                   canonical + " # trusted",
		"second command":            canonical + " npm install attacker@9.9.9",
		"embedded newline":          canonical + "\nid",
		"trailing newline":          canonical + "\n",
		"tab separator":             "npm\tinstall @acme/platform-sdk@3.2.1",
		"unicode line separator":    canonical + "\u2028id",
		"noncanonical double space": "npm  install @acme/platform-sdk@3.2.1",
	}

	for name, command := range commands {
		name, command := name, command
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeSDKReference(SDKReferenceInput{
				Ecosystem:      "npm",
				Coordinate:     "@acme/platform-sdk",
				ExactVersion:   "3.2.1",
				InstallCommand: command,
			})
			if err == nil {
				t.Fatal("normalizeSDKReference() error = nil")
			}
		})
	}
}
