package httpapi

import (
	"bytes"
	"errors"
	"testing"
)

type sourceUploadFailingWriter struct{}

func (sourceUploadFailingWriter) Write([]byte) (int, error) {
	return 0, errors.New("storage full")
}

func TestSourceUploadUTF8ValidationSpansChunks(t *testing.T) {
	var destination bytes.Buffer
	writer := &utf8StreamWriter{destination: &destination}
	if _, err := writer.Write([]byte{0xe2}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte{0x82, 0xac}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	if destination.String() != "€" {
		t.Fatalf("validated output = %q", destination.String())
	}
}

func TestSourceUploadWrapsDestinationWriteFailureAsStorageError(t *testing.T) {
	writer := &utf8StreamWriter{destination: sourceUploadFailingWriter{}}
	_, err := writer.Write([]byte("content"))
	var storageError *sourceUploadStorageError
	if !errors.As(err, &storageError) {
		t.Fatalf("destination error type = %T, want sourceUploadStorageError", err)
	}
}
