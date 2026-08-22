package platform

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var (
	ErrWidgetDisabled       = errors.New("widget is not active")
	ErrWidgetOriginDenied   = errors.New("widget origin is not allowed")
	ErrWidgetAuthentication = errors.New("widget credential is invalid or expired")
)

const (
	widgetBootstrapTTL = time.Minute
	widgetSessionTTL   = 15 * time.Minute
)

var widgetAccentPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type WidgetAppearance struct {
	Theme            string `json:"theme"`
	AccentColour     string `json:"accentColour,omitempty"`
	LauncherPosition string `json:"launcherPosition"`
	Greeting         string `json:"greeting,omitempty"`
}

type WidgetInput struct {
	Name           string
	AllowedOrigins []string
	IntegrationIDs []string
	Appearance     WidgetAppearance
	Revision       int64
}

type WidgetProvisioning struct {
	Widget model.Widget `json:"widget"`
	Secret string       `json:"secret"`
}

type WidgetBootstrapResult struct {
	BootstrapToken string    `json:"bootstrapToken"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type WidgetSessionResult struct {
	SessionToken string              `json:"sessionToken"`
	ExpiresAt    time.Time           `json:"expiresAt"`
	Session      model.WidgetSession `json:"session"`
}

type WidgetPrincipal struct {
	Widget  model.Widget
	Session model.WidgetSession
}

func normalizeWidgetOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("allowed origins must be origins without a path, query, fragment, or credentials")
	}
	local := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && local) {
		return "", errors.New("allowed origins must use HTTPS; HTTP is accepted only for localhost")
	}
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeWidgetOrigins(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		origin, err := normalizeWidgetOrigin(raw)
		if err != nil {
			return nil, err
		}
		if !seen[origin] {
			seen[origin] = true
			result = append(result, origin)
		}
	}
	slices.Sort(result)
	if len(result) > 20 {
		return nil, errors.New("a widget can allow at most 20 origins")
	}
	return result, nil
}

func normalizeWidgetAppearance(value WidgetAppearance) (json.RawMessage, error) {
	value.Theme = strings.ToLower(strings.TrimSpace(value.Theme))
	if value.Theme == "" {
		value.Theme = "auto"
	}
	if value.Theme != "auto" && value.Theme != "light" && value.Theme != "dark" {
		return nil, errors.New("widget theme must be auto, light, or dark")
	}
	value.LauncherPosition = strings.TrimSpace(value.LauncherPosition)
	if value.LauncherPosition == "" {
		value.LauncherPosition = "right"
	}
	if value.LauncherPosition != "right" && value.LauncherPosition != "left" {
		return nil, errors.New("widget launcher position must be left or right")
	}
	value.AccentColour = strings.TrimSpace(value.AccentColour)
	if value.AccentColour != "" && !widgetAccentPattern.MatchString(value.AccentColour) {
		return nil, errors.New("widget accent colour must be a six-digit hex colour")
	}
	if len(value.Greeting) > 160 {
		return nil, errors.New("widget appearance values are too long")
	}
	encoded, err := json.Marshal(value)
	return encoded, err
}

func (s *Service) validateWidgetIntegrations(ctx context.Context, deploymentID string, values []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, id := range values {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		if _, err := s.store.Integration(ctx, deploymentID, id); err != nil {
			return nil, errors.New("every widget integration must belong to this deployment")
		}
		seen[id] = true
		result = append(result, id)
	}
	slices.Sort(result)
	return result, nil
}

func newWidgetToken(prefix string) (string, []byte, string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", nil, "", err
	}
	raw := prefix + base64.RawURLEncoding.EncodeToString(randomBytes)
	digest := sha256.Sum256([]byte(raw))
	fingerprint := raw
	if len(fingerprint) > 12 {
		fingerprint = fingerprint[len(fingerprint)-12:]
	}
	return raw, digest[:], fingerprint, nil
}

func idempotentWidgetToken(rawSecret, widgetID, userID, customerOrganisationID, origin, key string) (string, []byte) {
	mac := hmac.New(sha256.New, []byte(rawSecret))
	_, _ = mac.Write([]byte("dokosoko-widget-bootstrap-v1\x00" + widgetID + "\x00" + userID + "\x00" + customerOrganisationID + "\x00" + origin + "\x00" + key))
	raw := "doko_wbt_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	digest := sha256.Sum256([]byte(raw))
	return raw, digest[:]
}

func widgetTokenDigest(raw, prefix string) ([]byte, error) {
	if !strings.HasPrefix(raw, prefix) || len(raw) < len(prefix)+40 {
		return nil, ErrWidgetAuthentication
	}
	digest := sha256.Sum256([]byte(raw))
	return digest[:], nil
}

func (s *Service) CreateWidget(ctx context.Context, input WidgetInput, actor Actor) (WidgetProvisioning, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return WidgetProvisioning{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 120 {
		return WidgetProvisioning{}, errors.New("widget name must be between 1 and 120 characters")
	}
	origins, err := normalizeWidgetOrigins(input.AllowedOrigins)
	if err != nil {
		return WidgetProvisioning{}, err
	}
	integrations, err := s.validateWidgetIntegrations(ctx, deployment.ID, input.IntegrationIDs)
	if err != nil {
		return WidgetProvisioning{}, err
	}
	appearance, err := normalizeWidgetAppearance(input.Appearance)
	if err != nil {
		return WidgetProvisioning{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return WidgetProvisioning{}, err
	}
	created, err := s.store.CreateWidget(ctx, model.Widget{ID: id, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, Name: input.Name, State: "draft", AllowedOrigins: origins, IntegrationIDs: integrations, Appearance: appearance})
	if err != nil {
		return WidgetProvisioning{}, err
	}
	secret, digest, fingerprint, err := newWidgetToken("doko_wsk_")
	if err != nil {
		return WidgetProvisioning{}, err
	}
	secretID, err := randomUUID()
	if err != nil {
		return WidgetProvisioning{}, err
	}
	if _, err := s.store.CreateWidgetSecret(ctx, model.WidgetSecret{ID: secretID, WidgetID: created.ID, Digest: digest, Fingerprint: fingerprint}); err != nil {
		return WidgetProvisioning{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: created.OrganisationID, ProductID: created.DeploymentID, ActorID: actor.ID, Action: "widget.created", TargetType: "widget", TargetID: created.ID, Current: map[string]any{"name": created.Name, "state": created.State, "allowed_origins": created.AllowedOrigins, "integration_ids": created.IntegrationIDs}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return WidgetProvisioning{Widget: created, Secret: secret}, nil
}

func (s *Service) UpdateWidget(ctx context.Context, widgetID string, input WidgetInput, actor Actor) (model.Widget, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Widget{}, err
	}
	current, err := s.store.Widget(ctx, deployment.ID, widgetID)
	if err != nil {
		return model.Widget{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 120 {
		return model.Widget{}, errors.New("widget name must be between 1 and 120 characters")
	}
	origins, err := normalizeWidgetOrigins(input.AllowedOrigins)
	if err != nil {
		return model.Widget{}, err
	}
	integrations, err := s.validateWidgetIntegrations(ctx, deployment.ID, input.IntegrationIDs)
	if err != nil {
		return model.Widget{}, err
	}
	if current.State == "active" && (len(origins) == 0 || len(integrations) == 0) {
		return model.Widget{}, errors.New("an active widget must keep an allowed origin and an integration")
	}
	appearance, err := normalizeWidgetAppearance(input.Appearance)
	if err != nil {
		return model.Widget{}, err
	}
	current.Name, current.AllowedOrigins, current.IntegrationIDs, current.Appearance = input.Name, origins, integrations, appearance
	updated, err := s.store.UpdateWidget(ctx, current, input.Revision)
	if err != nil {
		return model.Widget{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.DeploymentID, ActorID: actor.ID, Action: "widget.updated", TargetType: "widget", TargetID: updated.ID, Current: map[string]any{"name": updated.Name, "state": updated.State, "allowed_origins": updated.AllowedOrigins, "integration_ids": updated.IntegrationIDs, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) SetWidgetState(ctx context.Context, widgetID, state string, revision int64, actor Actor) (model.Widget, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Widget{}, err
	}
	current, err := s.store.Widget(ctx, deployment.ID, widgetID)
	if err != nil {
		return model.Widget{}, err
	}
	if state != "active" && state != "disabled" {
		return model.Widget{}, errors.New("widget state must be active or disabled")
	}
	if state == "active" && (len(current.AllowedOrigins) == 0 || len(current.IntegrationIDs) == 0) {
		return model.Widget{}, errors.New("add an allowed origin and an integration before activating the widget")
	}
	if state == "active" {
		if err := s.requireWidgetAssistant(ctx, current.DeploymentID, current.OrganisationID); err != nil {
			return model.Widget{}, err
		}
	}
	current.State = state
	if state == "active" {
		now := s.now()
		current.ActivatedAt = &now
	}
	updated, err := s.store.UpdateWidget(ctx, current, revision)
	if err != nil {
		return model.Widget{}, err
	}
	if state == "disabled" {
		if err := s.store.RevokeWidgetSessions(ctx, updated.ID, s.now()); err != nil {
			return model.Widget{}, err
		}
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.DeploymentID, ActorID: actor.ID, Action: "widget." + state, TargetType: "widget", TargetID: updated.ID, Current: map[string]any{"state": state, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) RotateWidgetSecret(ctx context.Context, widgetID string, actor Actor) (string, model.WidgetSecret, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return "", model.WidgetSecret{}, err
	}
	widget, err := s.store.Widget(ctx, deployment.ID, widgetID)
	if err != nil {
		return "", model.WidgetSecret{}, err
	}
	raw, digest, fingerprint, err := newWidgetToken("doko_wsk_")
	if err != nil {
		return "", model.WidgetSecret{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return "", model.WidgetSecret{}, err
	}
	created, err := s.store.CreateWidgetSecret(ctx, model.WidgetSecret{ID: id, WidgetID: widgetID, Digest: digest, Fingerprint: fingerprint})
	if err != nil {
		return "", model.WidgetSecret{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: widget.OrganisationID, ProductID: widget.DeploymentID, ActorID: actor.ID, Action: "widget.secret.created", TargetType: "widget_secret", TargetID: created.ID, Current: map[string]any{"widget_id": widgetID, "fingerprint": created.Fingerprint}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return raw, created, nil
}

func (s *Service) CreateWidgetBootstrap(ctx context.Context, widgetID, rawSecret, userID, customerOrganisationID, origin, idempotencyKey string) (WidgetBootstrapResult, error) {
	userID, customerOrganisationID = strings.TrimSpace(userID), strings.TrimSpace(customerOrganisationID)
	if userID == "" || len(userID) > 255 || len(customerOrganisationID) > 255 {
		return WidgetBootstrapResult{}, errors.New("userId is required and identity values must not exceed 255 characters")
	}
	digest, err := widgetTokenDigest(strings.TrimSpace(rawSecret), "doko_wsk_")
	if err != nil {
		return WidgetBootstrapResult{}, err
	}
	if _, err := s.store.WidgetSecretByDigest(ctx, widgetID, digest); err != nil {
		return WidgetBootstrapResult{}, ErrWidgetAuthentication
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return WidgetBootstrapResult{}, err
	}
	widget, err := s.store.Widget(ctx, deployment.ID, widgetID)
	if err != nil {
		return WidgetBootstrapResult{}, ErrWidgetAuthentication
	}
	if widget.State != "active" {
		return WidgetBootstrapResult{}, ErrWidgetDisabled
	}
	normalizedOrigin, err := normalizeWidgetOrigin(origin)
	if err != nil || !slices.Contains(widget.AllowedOrigins, normalizedOrigin) {
		return WidgetBootstrapResult{}, ErrWidgetOriginDenied
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 255 || strings.IndexFunc(idempotencyKey, func(value rune) bool { return value < 0x21 || value > 0x7e }) >= 0 {
		return WidgetBootstrapResult{}, errors.New("Idempotency-Key must contain 1 to 255 visible ASCII characters")
	}
	var raw string
	var bootstrapDigest []byte
	if idempotencyKey != "" {
		raw, bootstrapDigest = idempotentWidgetToken(rawSecret, widget.ID, userID, customerOrganisationID, normalizedOrigin, idempotencyKey)
	} else {
		raw, bootstrapDigest, _, err = newWidgetToken("doko_wbt_")
		if err != nil {
			return WidgetBootstrapResult{}, err
		}
	}
	expiresAt := s.now().Add(widgetBootstrapTTL)
	if err := s.store.CreateWidgetBootstrap(ctx, model.WidgetBootstrap{Digest: bootstrapDigest, WidgetID: widget.ID, UserID: userID, CustomerOrganisationID: customerOrganisationID, Origin: normalizedOrigin, ExpiresAt: expiresAt}); err != nil {
		if idempotencyKey != "" && errors.Is(err, store.ErrConflict) {
			existing, lookupErr := s.store.WidgetBootstrap(ctx, bootstrapDigest)
			if lookupErr == nil {
				return WidgetBootstrapResult{BootstrapToken: raw, ExpiresAt: existing.ExpiresAt}, nil
			}
		}
		return WidgetBootstrapResult{}, err
	}
	return WidgetBootstrapResult{BootstrapToken: raw, ExpiresAt: expiresAt}, nil
}

func (s *Service) ExchangeWidgetBootstrap(ctx context.Context, rawToken, origin string) (WidgetSessionResult, error) {
	digest, err := widgetTokenDigest(strings.TrimSpace(rawToken), "doko_wbt_")
	if err != nil {
		return WidgetSessionResult{}, err
	}
	now := s.now()
	bootstrap, err := s.store.ConsumeWidgetBootstrap(ctx, digest, now)
	if err != nil {
		return WidgetSessionResult{}, ErrWidgetAuthentication
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return WidgetSessionResult{}, err
	}
	widget, err := s.store.Widget(ctx, deployment.ID, bootstrap.WidgetID)
	if err != nil || widget.State != "active" {
		return WidgetSessionResult{}, ErrWidgetDisabled
	}
	normalizedOrigin, err := normalizeWidgetOrigin(origin)
	if err != nil || bootstrap.Origin != normalizedOrigin || !slices.Contains(widget.AllowedOrigins, normalizedOrigin) {
		return WidgetSessionResult{}, ErrWidgetOriginDenied
	}
	raw, sessionDigest, _, err := newWidgetToken("doko_wss_")
	if err != nil {
		return WidgetSessionResult{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return WidgetSessionResult{}, err
	}
	session, err := s.store.CreateWidgetSession(ctx, model.WidgetSession{ID: id, WidgetID: widget.ID, Digest: sessionDigest, UserID: bootstrap.UserID, CustomerOrganisationID: bootstrap.CustomerOrganisationID, Origin: normalizedOrigin, ExpiresAt: now.Add(widgetSessionTTL)})
	if err != nil {
		return WidgetSessionResult{}, err
	}
	return WidgetSessionResult{SessionToken: raw, ExpiresAt: session.ExpiresAt, Session: session}, nil
}

func (s *Service) AuthenticateWidgetSession(ctx context.Context, rawToken string) (WidgetPrincipal, error) {
	digest, err := widgetTokenDigest(strings.TrimSpace(rawToken), "doko_wss_")
	if err != nil {
		return WidgetPrincipal{}, err
	}
	session, err := s.store.WidgetSessionByDigest(ctx, digest, s.now())
	if err != nil {
		return WidgetPrincipal{}, ErrWidgetAuthentication
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return WidgetPrincipal{}, err
	}
	widget, err := s.store.Widget(ctx, deployment.ID, session.WidgetID)
	if err != nil || widget.State != "active" {
		return WidgetPrincipal{}, ErrWidgetDisabled
	}
	if !slices.Contains(widget.AllowedOrigins, session.Origin) {
		return WidgetPrincipal{}, ErrWidgetOriginDenied
	}
	return WidgetPrincipal{Widget: widget, Session: session}, nil
}
