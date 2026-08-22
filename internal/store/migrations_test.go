package store

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationFilesAreImmutable(t *testing.T) {
	t.Parallel()

	directory := filepath.Join("..", "..", "migrations")
	manifestPath := filepath.Join(directory, "checksums.sha256")
	manifest, err := os.Open(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	expected := make(map[string]string)
	sequences := make(map[string]string)
	scanner := bufio.NewScanner(manifest)
	for line := 1; scanner.Scan(); line++ {
		value := strings.TrimSpace(scanner.Text())
		if value == "" || strings.HasPrefix(value, "#") {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) != 2 {
			t.Fatalf("%s:%d: expected '<sha256> <filename>'", manifestPath, line)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil || len(fields[0]) != sha256.Size*2 {
			t.Fatalf("%s:%d: invalid SHA-256 checksum %q", manifestPath, line, fields[0])
		}
		if filepath.Base(fields[1]) != fields[1] || !strings.HasSuffix(fields[1], ".sql") {
			t.Fatalf("%s:%d: invalid migration filename %q", manifestPath, line, fields[1])
		}
		if _, exists := expected[fields[1]]; exists {
			t.Fatalf("%s:%d: duplicate migration %q", manifestPath, line, fields[1])
		}
		sequence, _, validName := strings.Cut(fields[1], "_")
		if !validName || len(sequence) != 4 || strings.Trim(sequence, "0123456789") != "" {
			t.Fatalf("%s:%d: migration %q must start with a four-digit sequence", manifestPath, line, fields[1])
		}
		if prior, exists := sequences[sequence]; exists && !legacyDuplicateMigrationSequence(sequence, prior, fields[1]) {
			t.Fatalf("%s:%d: migration sequence %s is already used by %s; add the next unused sequence", manifestPath, line, sequence, prior)
		}
		sequences[sequence] = fields[1]
		expected[fields[1]] = strings.ToLower(fields[0])
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	actual := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(content))
		want, exists := expected[entry.Name()]
		if !exists {
			t.Fatalf("migration %s is missing from checksums.sha256; add a new manifest entry with the migration", entry.Name())
		}
		if checksum != want {
			t.Fatalf("migration %s changed: never edit, rename, or delete an existing migration; add a new uniquely numbered migration instead", entry.Name())
		}
		actual[entry.Name()] = struct{}{}
	}
	for name := range expected {
		if _, exists := actual[name]; !exists {
			t.Fatalf("checksums.sha256 references missing migration %s; never rename or delete an existing migration", name)
		}
	}
}

func legacyDuplicateMigrationSequence(sequence, first, second string) bool {
	if sequence != "0020" {
		return false
	}
	files := map[string]bool{first: true, second: true}
	return files["0020_contract_v3.sql"] && files["0020_package_download_contract.sql"]
}
