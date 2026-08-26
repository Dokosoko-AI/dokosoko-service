package platform

import (
	"net/http"
	"strings"
)

// ProductBuilderDoer is retained as a source-compatible name for the HTTP
// transport injected into the AI runtime. The product builder itself no
// longer exists; analysis, recipes, and tool authoring still use this client.
type ProductBuilderDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastHyphen := false
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			builder.WriteRune(character)
			lastHyphen = false
		case builder.Len() > 0 && !lastHyphen:
			builder.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(builder.String(), "-")
}
