package config

import (
	"os"
	"path/filepath"
	"testing"
)

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestLoadDefaults(t *testing.T) {
	value, err := LoadWithOptions(Options{LookupEnv: lookup(nil), WorkingDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if value.Listen != ":8080" || value.UploadMaxBytes != 5_000_000 || value.Crawler.MaxPages != 500 {
		t.Fatalf("unexpected defaults: %#v", value)
	}
	if value.Status.Version != 1 || len(value.Status.Items) == 0 {
		t.Fatalf("missing effective configuration status: %#v", value.Status)
	}
}

func TestLoadFileAndEnvironmentPrecedenceWithRedactedSecrets(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "dokosoko.json")
	masterKeyPath := filepath.Join(directory, "master-key")
	if err := os.WriteFile(masterKeyPath, []byte("file-master-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	contents := `{
  "$schema": "./dokosoko.config.schema.json",
  "version": 1,
  "server": {"listen": ":9090", "ui_directory": "ui"},
  "database": {"url": {"env": "DATABASE_SECRET"}},
  "security": {"master_key": {"file": "master-key"}},
  "ai": {"provider": "openai", "api_key": {"env": "AI_SECRET"}, "analysis": {"model": "gpt-file"}},
  "control_plane": {
    "organisation": {"name": "File Organisation", "slug": "file-organisation"},
    "deployment": {"name": "File Deployment", "slug": "file-deployment", "description": ""},
    "environments": [{"name": "Production", "slug": "production", "is_production": true}]
  },
  "crawler": {"max_pages": 250, "localhost_ports": [8080]}
}`
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := LoadWithOptions(Options{
		LookupEnv: lookup(map[string]string{
			"DOKOSOKO_CONFIG_FILE":     configPath,
			"DOKOSOKO_LISTEN":          ":7070",
			"DOKOSOKO_DEPLOYMENT_NAME": "Environment Deployment",
			"DATABASE_SECRET":          "postgres://file",
			"AI_SECRET":                "api-secret",
		}),
		WorkingDirectory: directory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if value.Listen != ":7070" || value.DatabaseURL != "postgres://file" || value.MasterKey != "file-master-key" || value.AI.Analysis.Model != "gpt-file" {
		t.Fatalf("unexpected merged configuration: %#v", value)
	}
	if value.UIDirectory != filepath.Join(directory, "ui") {
		t.Fatalf("file-relative UI directory = %q", value.UIDirectory)
	}
	if value.ControlPlane.Organisation.Slug == nil || *value.ControlPlane.Organisation.Slug != "file-organisation" || value.ControlPlane.Deployment.Name == nil || *value.ControlPlane.Deployment.Name != "Environment Deployment" {
		t.Fatalf("unexpected control-plane merge: %#v", value.ControlPlane)
	}
	if value.ControlPlane.Deployment.Description == nil || *value.ControlPlane.Deployment.Description != "" {
		t.Fatalf("explicit empty managed description was not preserved: %#v", value.ControlPlane.Deployment.Description)
	}
	if value.ControlPlane.Environments == nil || len(*value.ControlPlane.Environments) != 1 || !(*value.ControlPlane.Environments)[0].IsProduction {
		t.Fatalf("unexpected configured environments: %#v", value.ControlPlane.Environments)
	}
	for _, item := range value.Status.Items {
		if item.Sensitive && item.Value != "" {
			t.Fatalf("sensitive configuration leaked through status: %#v", item)
		}
		if item.Key == "server.listen" && item.Source != SourceEnvironment {
			t.Fatalf("listen source = %q", item.Source)
		}
		if item.Key == "control_plane.deployment.name" && item.Source != SourceEnvironment {
			t.Fatalf("deployment name source = %q", item.Source)
		}
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "dokosoko.json")
	if err := os.WriteFile(configPath, []byte(`{"version":1,"server":{"mystery":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithOptions(Options{LookupEnv: lookup(map[string]string{"DOKOSOKO_CONFIG_FILE": configPath}), WorkingDirectory: directory}); err == nil {
		t.Fatal("expected an unknown-field error")
	}
}

func TestLoadRejectsAmbiguousSecretEnvironment(t *testing.T) {
	if _, err := LoadWithOptions(Options{LookupEnv: lookup(map[string]string{
		"DOKOSOKO_MASTER_KEY":      "direct",
		"DOKOSOKO_MASTER_KEY_FILE": "file",
	}), WorkingDirectory: t.TempDir()}); err == nil {
		t.Fatal("expected ambiguous secret environment error")
	}
}

func TestLoadRejectsDuplicateControlPlaneEnvironmentSlugs(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "dokosoko.json")
	contents := `{"version":1,"control_plane":{"environments":[{"name":"Production","slug":"production","is_production":true},{"name":"Duplicate","slug":"production","is_production":false}]}}`
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithOptions(Options{LookupEnv: lookup(map[string]string{"DOKOSOKO_CONFIG_FILE": configPath}), WorkingDirectory: directory}); err == nil {
		t.Fatal("expected duplicate control-plane environment error")
	}
}
