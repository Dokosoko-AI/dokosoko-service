package docreview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// SafeAssessment deliberately fails closed for malformed, null, or non-array
// indicator payloads. A review can select only validated/published evidence
// whose classifier produced an explicitly empty indicator array.
func SafeAssessment(state string, indicators json.RawMessage) bool {
	if state != "validated" && state != "published" {
		return false
	}
	var decoded any
	if err := json.Unmarshal(indicators, &decoded); err != nil {
		return false
	}
	values, ok := decoded.([]any)
	return ok && len(values) == 0
}

// PublicationContentHash hashes the exact immutable document identity/content
// and frozen generation assessment returned by SourceReview. Sorting a copy
// keeps reconstruction deterministic without mutating the caller's selection.
func PublicationContentHash(documents []model.CrawlReviewDocument) (string, error) {
	ordered := append([]model.CrawlReviewDocument(nil), documents...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	encoded, err := json.Marshal(ordered)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
