package httpapi

import (
	"strings"
	"testing"
)

func TestWidgetResponseChunksPreserveMarkdownAndUnicode(t *testing.T) {
	value := "## Setup ✨\n\n1. First step\n2. Second step\n\n```go\nready := true\n```"
	chunks := widgetResponseChunks(value, 7)
	if joined := strings.Join(chunks, ""); joined != value {
		t.Fatalf("widget stream changed Markdown:\nwant %q\n got %q", value, joined)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %#v", chunks)
	}
}
