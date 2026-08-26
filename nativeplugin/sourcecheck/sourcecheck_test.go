package sourcecheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckRejectsEnvironmentAndInit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plugin.go")
	if err := os.WriteFile(path, []byte("package bad\nimport \"os\"\nfunc init() { _ = os.Getenv(\"SECRET\") }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	findings, err := Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestCheckAcceptsOrdinarySource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plugin.go"), []byte("package good\nimport \"context\"\nvar _ = context.Background\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	findings, err := Check(root)
	if err != nil || len(findings) != 0 {
		t.Fatalf("findings=%#v err=%v", findings, err)
	}
}

func TestCheckRejectsInternalImportsAndNonReadableArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plugin.go"), []byte("package bad\nimport _ \"github.com/dokosoko/dokosoko-service/internal/model\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "payload.dat"), []byte{'h', 'i', 0, 'x'}, 0o600); err != nil {
		t.Fatal(err)
	}
	findings, err := Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %#v", findings)
	}
}
