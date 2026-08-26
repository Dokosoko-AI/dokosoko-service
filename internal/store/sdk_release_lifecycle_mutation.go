package store

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// SDKReleaseLifecycleMutation is persisted as one atomic unit so an effective
// lifecycle transition can never exist without its corresponding audit fact.
type SDKReleaseLifecycleMutation struct {
	Event model.SDKReleaseLifecycleEvent
	Audit model.AuditEvent
}

func prepareSDKReleaseLifecycleMutation(deploymentID string, mutation SDKReleaseLifecycleMutation) ([]byte, []byte, string, error) {
	event, audit := mutation.Event, mutation.Audit
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.SDKReleaseID) == "" ||
		strings.TrimSpace(event.RecordedBy) == "" || event.ObservedAt.IsZero() {
		return nil, nil, "", errors.New("SDK release lifecycle event identity, actor, and observed_at are required")
	}
	switch event.Lifecycle {
	case "active", "deprecated", "yanked", "archived":
	default:
		return nil, nil, "", errors.New("SDK release lifecycle event is invalid")
	}
	if strings.TrimSpace(audit.ID) == "" {
		return nil, nil, "", errors.New("audit event ID is required")
	}
	if audit.ProductID != deploymentID || audit.ActorID != event.RecordedBy || audit.Action != "sdk_release.lifecycle_event.appended" ||
		audit.TargetType != "sdk_release" || audit.TargetID != event.SDKReleaseID {
		return nil, nil, "", errors.New("SDK release lifecycle audit does not match its event")
	}
	if eventID, _ := audit.Current["sdk_release_lifecycle_event_id"].(string); eventID != event.ID {
		return nil, nil, "", errors.New("SDK release lifecycle audit omits the exact event ID")
	}
	prior, err := json.Marshal(audit.Prior)
	if err != nil {
		return nil, nil, "", errors.New("marshal lifecycle audit prior state: " + err.Error())
	}
	current, err := json.Marshal(audit.Current)
	if err != nil {
		return nil, nil, "", errors.New("marshal lifecycle audit current state: " + err.Error())
	}
	outcome := audit.Outcome
	if outcome == "" {
		outcome = "success"
	}
	return prior, current, outcome, nil
}
