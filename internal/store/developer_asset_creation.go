package store

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var ErrDeveloperAssetCreationConflict = errors.New("creation key was already used with different input")

type DeveloperAssetCreation struct {
	RequestDigest string
	InputDigest   string
	Audit         model.AuditEvent
}

type developerAssetCreationResult struct{ ResourceID, InputDigest string }

func validateDeveloperAssetCreation(requests []DeveloperAssetCreation, deploymentID, organisationID, kind, id string, expected int64) (*DeveloperAssetCreation, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	if len(requests) != 1 || expected != 0 {
		return nil, ErrConflict
	}
	request := requests[0]
	for _, value := range []string{request.RequestDigest, request.InputDigest} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || value != strings.ToLower(value) {
			return nil, errors.New("creation requires SHA-256 request and input digests")
		}
	}
	action := "api_contract.saved"
	if kind == "documentation_collection" {
		action = "documentation_collection.revision_saved"
	} else if kind != "api_contract" {
		return nil, ErrConflict
	}
	event := request.Audit
	if event.ID == "" || event.ProductID != deploymentID || event.OrganisationID != organisationID || event.TargetType != kind || event.TargetID != id || event.Action != action {
		return nil, errors.New("creation audit does not match the resource")
	}
	if _, err := json.Marshal(event.Current); err != nil {
		return nil, err
	}
	return &request, nil
}

func (m *Memory) developerAssetCreationLocked(request *DeveloperAssetCreation) (string, error) {
	if request == nil {
		return "", nil
	}
	key := request.Audit.ProductID + ":" + request.Audit.TargetType + ":" + request.RequestDigest
	if previous, exists := m.developerAssetCreationRequests[key]; exists {
		if previous.InputDigest != request.InputDigest {
			return "", ErrDeveloperAssetCreationConflict
		}
		return previous.ResourceID, nil
	}
	for _, event := range m.audit {
		if event.ID == request.Audit.ID {
			return "", ErrConflict
		}
	}
	return "", nil
}

func (m *Memory) saveDeveloperAssetCreationLocked(request *DeveloperAssetCreation) {
	if request == nil {
		return
	}
	if m.developerAssetCreationRequests == nil {
		m.developerAssetCreationRequests = make(map[string]developerAssetCreationResult)
	}
	key := request.Audit.ProductID + ":" + request.Audit.TargetType + ":" + request.RequestDigest
	m.developerAssetCreationRequests[key] = developerAssetCreationResult{ResourceID: request.Audit.TargetID, InputDigest: request.InputDigest}
	event := request.Audit
	event.Outcome = "success"
	m.audit = append(m.audit, memoryClone(event))
}
