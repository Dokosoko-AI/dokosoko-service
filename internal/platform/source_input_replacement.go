package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var ErrInvalidSourceReplacementInput = errors.New("invalid source replacement input")

func sourceReplacementInputError(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidSourceReplacementInput, message)
}

type SourceInputReplacementInput struct {
	ProductID, SourceID, Location, Filename, ContentDigest, RequestKey string
	Revision                                                           int64
}

func sourceReplacementRequestDigest(key string, actor Actor) (string, error) {
	if len(key) < 16 || len(key) > 200 {
		return "", sourceReplacementInputError("Idempotency-Key must contain 16 to 200 visible ASCII characters")
	}
	for _, c := range key {
		if c < 33 || c > 126 {
			return "", sourceReplacementInputError("Idempotency-Key must contain 16 to 200 visible ASCII characters")
		}
	}
	digest := sha256.Sum256([]byte(actor.ID + "\x00" + key))
	return hex.EncodeToString(digest[:]), nil
}

func (s *Service) SourceInputReplacement(ctx context.Context, productID, sourceID, key string, actor Actor) (store.SourceInputReplacementResult, error) {
	digest, err := sourceReplacementRequestDigest(key, actor)
	if err != nil {
		return store.SourceInputReplacementResult{}, err
	}
	if _, err = s.store.Source(ctx, productID, sourceID); err != nil {
		return store.SourceInputReplacementResult{}, err
	}
	return s.store.SourceInputReplacement(ctx, productID, sourceID, digest)
}

func (s *Service) ReplaceSourceInput(ctx context.Context, input SourceInputReplacementInput, actor Actor) (store.SourceInputReplacementResult, error) {
	digest, err := sourceReplacementRequestDigest(input.RequestKey, actor)
	if err != nil {
		return store.SourceInputReplacementResult{}, err
	}
	content, err := hex.DecodeString(input.ContentDigest)
	if err != nil || len(content) != sha256.Size || input.ContentDigest != strings.ToLower(input.ContentDigest) {
		return store.SourceInputReplacementResult{}, sourceReplacementInputError("replacement requires a validated upload content digest")
	}
	if input.Revision < 1 || input.Location == "" || len(input.Location) > 2048 || input.Location != filepath.Base(input.Location) || strings.ContainsAny(input.Location, "/\\") || input.Location == "." || input.Location == ".." || input.Filename == "" {
		return store.SourceInputReplacementResult{}, sourceReplacementInputError("replacement requires a source revision and a validated uploaded file")
	}
	source, err := s.store.Source(ctx, input.ProductID, input.SourceID)
	if err != nil {
		return store.SourceInputReplacementResult{}, err
	}
	if source.Kind != "upload" {
		return store.SourceInputReplacementResult{}, sourceReplacementInputError("only uploaded sources accept replacement files")
	}
	id, err := randomUUID()
	if err != nil {
		return store.SourceInputReplacementResult{}, err
	}
	payload, _ := json.Marshal([]any{source.OrganisationID, source.ProductID, source.ID, input.Revision, input.Filename, filepath.Ext(input.Location), input.ContentDigest})
	inputHash := sha256.Sum256(payload)
	return s.store.ReplaceSourceInput(ctx, store.SourceInputReplacement{ProductID: source.ProductID, SourceID: source.ID, Location: input.Location, ExpectedRevision: input.Revision, RequestDigest: digest, InputDigest: hex.EncodeToString(inputHash[:]), CrawlJobID: id, Audit: model.AuditEvent{ID: randomID("audit"), OrganisationID: source.OrganisationID, ProductID: source.ProductID, ActorID: actor.ID, Action: "source.input.replaced", TargetType: "source", TargetID: source.ID, RequestID: actor.RequestID, CreatedAt: s.now()}})
}
