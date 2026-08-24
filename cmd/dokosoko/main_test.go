package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceUploadConfigDefaultsToDisabled(t *testing.T) {
	t.Setenv("DOKOSOKO_UPLOAD_DIR", "")
	t.Setenv("DOKOSOKO_UPLOAD_MAX_BYTES", "")
	directory, maxBytes, err := sourceUploadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if directory != "" || maxBytes != 5_000_000 {
		t.Fatalf("upload config = (%q, %d)", directory, maxBytes)
	}
}

func TestSourceUploadConfigCreatesPrivateDirectoryAndReadsLimit(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "uploads")
	t.Setenv("DOKOSOKO_UPLOAD_DIR", directory)
	t.Setenv("DOKOSOKO_UPLOAD_MAX_BYTES", "1024")
	configured, maxBytes, err := sourceUploadConfig()
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	if configured != absolute || maxBytes != 1024 {
		t.Fatalf("upload config = (%q, %d), want (%q, 1024)", configured, maxBytes, absolute)
	}
	info, err := os.Stat(configured)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("upload directory mode = %o, want 700", info.Mode().Perm())
	}
}

func TestSourceUploadConfigRejectsInvalidLimit(t *testing.T) {
	t.Setenv("DOKOSOKO_UPLOAD_DIR", "")
	t.Setenv("DOKOSOKO_UPLOAD_MAX_BYTES", "0")
	if _, _, err := sourceUploadConfig(); err == nil {
		t.Fatal("expected invalid upload limit error")
	}
}
