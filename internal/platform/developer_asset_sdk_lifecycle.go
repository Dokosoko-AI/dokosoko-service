package platform

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var ErrSDKReleaseUnavailable = errors.New("SDK release is unavailable for new bindings and publications")

type SDKReleaseLifecycleEventInput struct {
	Lifecycle         string
	Reason            string
	ObservedSourceURI string
	ObservedAt        time.Time
}

// SDKReleaseLifecycleState keeps the immutable release's initial lifecycle
// separate from later registry or administrative facts. Events are returned
// newest first and the effective event is selected deterministically.
type SDKReleaseLifecycleState struct {
	SDKReleaseID       string                           `json:"sdk_release_id"`
	InitialLifecycle   string                           `json:"initial_lifecycle"`
	EffectiveLifecycle string                           `json:"effective_lifecycle"`
	Selectable         bool                             `json:"selectable"`
	EffectiveEvent     *model.SDKReleaseLifecycleEvent  `json:"effective_event,omitempty"`
	Events             []model.SDKReleaseLifecycleEvent `json:"events"`
}

func validSDKReleaseLifecycle(value string) bool {
	switch value {
	case "active", "deprecated", "yanked", "archived":
		return true
	default:
		return false
	}
}

func sdkReleaseLifecycleSelectable(value string) bool {
	return value != "yanked" && value != "archived"
}

func sortSDKReleaseLifecycleEvents(events []model.SDKReleaseLifecycleEvent) {
	sort.Slice(events, func(i, j int) bool {
		if !events[i].ObservedAt.Equal(events[j].ObservedAt) {
			return events[i].ObservedAt.After(events[j].ObservedAt)
		}
		if !events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].CreatedAt.After(events[j].CreatedAt)
		}
		return events[i].ID > events[j].ID
	})
}

func (s *Service) sdkReleaseLifecycleState(ctx context.Context, deploymentID string, release model.SDKRelease) (SDKReleaseLifecycleState, error) {
	events, err := s.store.SDKReleaseLifecycleEvents(ctx, deploymentID, release.ID)
	if err != nil {
		return SDKReleaseLifecycleState{}, err
	}
	sortSDKReleaseLifecycleEvents(events)
	state := SDKReleaseLifecycleState{
		SDKReleaseID: release.ID, InitialLifecycle: release.Lifecycle,
		EffectiveLifecycle: release.Lifecycle, Events: events,
	}
	if len(events) > 0 {
		state.EffectiveLifecycle = events[0].Lifecycle
		effective := events[0]
		state.EffectiveEvent = &effective
	}
	state.Selectable = sdkReleaseLifecycleSelectable(state.EffectiveLifecycle)
	return state, nil
}

func (s *Service) SDKReleaseLifecycle(ctx context.Context, releaseID string) (SDKReleaseLifecycleState, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return SDKReleaseLifecycleState{}, err
	}
	release, err := s.store.SDKRelease(ctx, deployment.ID, strings.TrimSpace(releaseID))
	if err != nil {
		return SDKReleaseLifecycleState{}, err
	}
	return s.sdkReleaseLifecycleState(ctx, deployment.ID, release)
}

func (s *Service) ensureSDKReleaseSelectable(ctx context.Context, deploymentID string, release model.SDKRelease) error {
	state, err := s.sdkReleaseLifecycleState(ctx, deploymentID, release)
	if err != nil {
		return err
	}
	if !state.Selectable {
		return fmt.Errorf("%w: exact release %s is %s", ErrSDKReleaseUnavailable, release.ID, state.EffectiveLifecycle)
	}
	return nil
}

func (s *Service) AppendSDKReleaseLifecycleEvent(ctx context.Context, releaseID string, input SDKReleaseLifecycleEventInput, actor Actor) (SDKReleaseLifecycleState, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return SDKReleaseLifecycleState{}, err
	}
	release, err := s.store.SDKRelease(ctx, deployment.ID, strings.TrimSpace(releaseID))
	if err != nil {
		return SDKReleaseLifecycleState{}, err
	}
	previous, err := s.sdkReleaseLifecycleState(ctx, deployment.ID, release)
	if err != nil {
		return SDKReleaseLifecycleState{}, err
	}
	input.Lifecycle = strings.ToLower(strings.TrimSpace(input.Lifecycle))
	input.Reason = strings.TrimSpace(input.Reason)
	input.ObservedSourceURI = strings.TrimSpace(input.ObservedSourceURI)
	actor.ID = strings.TrimSpace(actor.ID)
	if !validSDKReleaseLifecycle(input.Lifecycle) {
		return SDKReleaseLifecycleState{}, errors.New("SDK release lifecycle must be active, deprecated, yanked, or archived")
	}
	if input.Reason == "" || len(input.Reason) > 2000 {
		return SDKReleaseLifecycleState{}, errors.New("a lifecycle event reason is required and must not exceed 2000 characters")
	}
	if len(input.ObservedSourceURI) > 2048 || !validSDKURL(input.ObservedSourceURI) {
		return SDKReleaseLifecycleState{}, errors.New("observed_source_uri must be a fixed public HTTPS URL")
	}
	if actor.ID == "" {
		return SDKReleaseLifecycleState{}, errors.New("lifecycle events require an authenticated actor")
	}
	now := s.now().UTC()
	if input.ObservedAt.IsZero() {
		input.ObservedAt = now
	} else {
		input.ObservedAt = input.ObservedAt.UTC()
	}
	if input.ObservedAt.After(now) {
		return SDKReleaseLifecycleState{}, errors.New("observed_at cannot be in the future")
	}
	id, err := randomUUID()
	if err != nil {
		return SDKReleaseLifecycleState{}, err
	}
	event := model.SDKReleaseLifecycleEvent{
		ID: id, SDKReleaseID: release.ID, Lifecycle: input.Lifecycle, Reason: input.Reason,
		ObservedSourceURI: input.ObservedSourceURI, ObservedAt: input.ObservedAt,
		RecordedBy: actor.ID, CreatedAt: now,
	}
	predictedEvents := append([]model.SDKReleaseLifecycleEvent(nil), previous.Events...)
	predictedEvents = append(predictedEvents, event)
	sortSDKReleaseLifecycleEvents(predictedEvents)
	predictedEffective := release.Lifecycle
	if len(predictedEvents) > 0 {
		predictedEffective = predictedEvents[0].Lifecycle
	}
	audit := model.AuditEvent{
		ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID,
		ActorID: actor.ID, Action: "sdk_release.lifecycle_event.appended", TargetType: "sdk_release", TargetID: release.ID,
		RequestID: actor.RequestID, CreatedAt: now,
		Current: map[string]any{
			"sdk_release_lifecycle_event_id": event.ID,
			"previous_effective_lifecycle":   previous.EffectiveLifecycle,
			"event_lifecycle":                event.Lifecycle,
			"effective_lifecycle":            predictedEffective,
			"reason":                         event.Reason,
			"observed_source_uri":            event.ObservedSourceURI,
			"observed_at":                    event.ObservedAt,
		},
	}
	if _, err := s.store.AppendSDKReleaseLifecycleEvent(ctx, deployment.ID, store.SDKReleaseLifecycleMutation{Event: event, Audit: audit}); err != nil {
		return SDKReleaseLifecycleState{}, err
	}
	return s.sdkReleaseLifecycleState(ctx, deployment.ID, release)
}
