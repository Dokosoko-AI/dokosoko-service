package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageSchemaStartsWithTheDownloadContract(t *testing.T) {
	t.Parallel()
	directory := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		all.Write(content)
	}
	schema := all.String()
	for _, legacy := range []string{"fetch_hook", "package-fetch", "'fetch'", "RENAME VALUE"} {
		if strings.Contains(schema, legacy) {
			t.Fatalf("migration baseline still contains legacy package contract token %q", legacy)
		}
	}
	initial, err := os.ReadFile(filepath.Join(directory, "0001_initial.sql"))
	if err != nil {
		t.Fatal(err)
	}
	baseline := string(initial)
	for _, required := range []string{
		"CREATE TYPE package_mode AS ENUM ('public', 'proxy', 'download')",
		"external_package_id text",
		"download_url text",
		"/v1/package/download",
		"CONSTRAINT packages_delivery_configuration",
	} {
		if !strings.Contains(baseline, required) {
			t.Fatalf("initial package schema is missing %q", required)
		}
	}
}
