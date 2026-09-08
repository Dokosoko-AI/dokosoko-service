package platform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Service) developerAssetCreation(key string, input any, deployment model.Deployment, actor Actor, kind, id, action string, current map[string]any) ([]store.DeveloperAssetCreation, error) {
	if key == "" {
		return nil, nil
	}
	if len(key) < 16 || len(key) > 200 {
		return nil, errors.New("Idempotency-Key must contain 16 to 200 visible ASCII characters")
	}
	for _, char := range key {
		if char < 33 || char > 126 {
			return nil, errors.New("Idempotency-Key must contain 16 to 200 visible ASCII characters")
		}
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	// Canonicalize nested selectors rather than depending on JSON object order.
	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	payload, err = json.Marshal(canonical)
	if err != nil {
		return nil, err
	}
	inputDigest := sha256.Sum256(payload)
	requestDigest := sha256.Sum256([]byte(actor.ID + "\x00" + key))
	return []store.DeveloperAssetCreation{{RequestDigest: hex.EncodeToString(requestDigest[:]), InputDigest: hex.EncodeToString(inputDigest[:]), Audit: model.AuditEvent{
		ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID,
		ActorID: actor.ID, Action: action, TargetType: kind, TargetID: id,
		Current: current, RequestID: actor.RequestID, CreatedAt: s.now(),
	}}}, nil
}
