package store

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var ErrSourceInputChanged = errors.New("source input or replacement request changed")
var ErrSourceImportActive = errors.New("source has a queued or running import")

type SourceInputReplacement struct {
	ProductID, SourceID, Location, RequestDigest, InputDigest string
	ExpectedRevision                                          int64
	CrawlJobID                                                string
	Audit                                                     model.AuditEvent
}

type SourceInputReplacementResult struct {
	Source   model.Source   `json:"source"`
	CrawlJob model.CrawlJob `json:"crawl_job"`
}

type sourceInputReplacementRecord struct {
	InputDigest, CrawlJobID string
	ExpectedRevision        int64
}

func validateSourceInputReplacement(input SourceInputReplacement) error {
	for _, value := range []string{input.RequestDigest, input.InputDigest} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || value != strings.ToLower(value) {
			return errors.New("source replacement requires SHA-256 request and input digests")
		}
	}
	if input.ExpectedRevision < 1 || strings.TrimSpace(input.Location) == "" || input.CrawlJobID == "" {
		return errors.New("replacement requires an input, revision and import identity")
	}
	if input.Audit.ID == "" || input.Audit.ProductID != input.ProductID || input.Audit.TargetID != input.SourceID || input.Audit.Action != "source.input.replaced" || input.Audit.TargetType != "source" {
		return errors.New("replacement audit does not match source")
	}
	return nil
}
