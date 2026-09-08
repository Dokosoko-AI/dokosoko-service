package store

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var ErrSourceCreationConflict = errors.New("source creation key was already used with different input")

type SourceCreation struct {
	Source        model.Source
	RequestDigest string
	InputDigest   string
	Audit         model.AuditEvent
}

type sourceCreationResult struct {
	SourceID    string
	InputDigest string
}

func validateSourceCreation(input SourceCreation) error {
	for _, value := range []string{input.RequestDigest, input.InputDigest} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || value != strings.ToLower(value) {
			return errors.New("source creation requires SHA-256 request and input digests")
		}
	}
	if strings.TrimSpace(input.Audit.ID) == "" || input.Audit.ProductID != input.Source.ProductID || input.Audit.OrganisationID != input.Source.OrganisationID || input.Audit.TargetID != input.Source.ID || input.Audit.Action != "source.created" || input.Audit.TargetType != "source" {
		return errors.New("source creation audit does not match the created source")
	}
	_, err := json.Marshal(input.Audit.Current)
	return err
}
