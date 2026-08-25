package platform

import (
	"strings"
	"testing"
)

func TestFinalizeWidgetAgentAnswerAllowsOnlyGroundedLinksAndAddsSources(t *testing.T) {
	t.Parallel()
	sources := []WidgetAgentSource{
		{Kind: "documentation", Title: "ComplicatedAuth setup", URI: "https://docs.example.com/setup"},
		{Kind: "recipe", Title: "Customer onboarding", URI: "dokosoko://recipes/customer-onboarding", Revision: 4},
	}
	answer := "Read [the setup guide](https://docs.example.com/setup), ignore [this invented guide](https://evil.example/setup), and never use https://evil.example/raw."

	got := finalizeWidgetAgentAnswer(answer, sources)

	if !strings.Contains(got, "[the setup guide](https://docs.example.com/setup)") {
		t.Fatalf("grounded link was removed: %q", got)
	}
	if strings.Contains(got, "evil.example") || strings.Contains(got, "](dokosoko://") {
		t.Fatalf("ungrounded link survived: %q", got)
	}
	if !strings.Contains(got, "### Sources") || !strings.Contains(got, "[ComplicatedAuth setup](https://docs.example.com/setup)") || !strings.Contains(got, "Customer onboarding — recipe revision 4") {
		t.Fatalf("source footer missing or incomplete: %q", got)
	}
}

func TestSafeWidgetAgentSourceLabelDoesNotInjectMarkdown(t *testing.T) {
	t.Parallel()
	got := safeWidgetAgentSourceLabel("[*Malicious*](https://evil.example) `source`")
	if strings.ContainsAny(got, "[]()*_`") || len([]rune(got)) > 160 {
		t.Fatalf("unsafe source label: %q", got)
	}
}

func TestFinalizeWidgetAgentAnswerKeepsOneSourcesSectionAndDropsEmptyFields(t *testing.T) {
	t.Parallel()
	answer := "## Setup\n\n- Issuer: ``\n- **Audience:** ``\n- **Path:** `/mcp`\n\n## Sources\n\n- Setup recipe"
	got := finalizeWidgetAgentAnswer(answer, []WidgetAgentSource{{Kind: "recipe", Title: "Setup recipe", Revision: 2}})
	if strings.Contains(got, "Issuer") || strings.Contains(got, "Audience") || strings.Count(strings.ToLower(got), "sources") != 1 || !strings.Contains(got, "`/mcp`") {
		t.Fatalf("answer cleanup failed: %q", got)
	}
}

func TestWidgetAgentModelSourcesDoesNotExposeInternalRecipeURI(t *testing.T) {
	t.Parallel()
	got := widgetAgentModelSources([]WidgetAgentSource{
		{Kind: "recipe", Title: "Setup recipe", URI: "dokosoko://products/internal/recipes/setup", Revision: 2},
		{Kind: "documentation", Title: "Setup guide", URI: "https://docs.example.com/setup"},
	})
	if got[0].URI != "" || got[1].URI != "https://docs.example.com/setup" {
		t.Fatalf("model source boundary failed: %#v", got)
	}
}

func TestWidgetAgentContextURLsAllowsOnlyExactReviewedURLs(t *testing.T) {
	t.Parallel()
	got := widgetAgentContextURLs(
		[]map[string]any{{"markdown": "Use `http://api.example.localhost:38080` and never https://user:secret@evil.example/path."}},
		[]map[string]any{{"text": "See https://docs.example.com/setup.", "url": "https://docs.example.com/setup"}},
	)
	if len(got) != 2 || got[0] != "http://api.example.localhost:38080" || got[1] != "https://docs.example.com/setup" {
		t.Fatalf("grounded URL allow-list = %#v", got)
	}
	answer := finalizeWidgetAgentAnswer("Use http://api.example.localhost:38080, not https://invented.example/setup.", nil, got...)
	if !strings.Contains(answer, "http://api.example.localhost:38080") || strings.Contains(answer, "invented.example") {
		t.Fatalf("grounded URL cleanup failed: %q", answer)
	}
}
