package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"net/url"
	"strings"
	"time"
)

func randomID(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buffer)
}

func randomUUID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	buffer[6] = buffer[6]&0x0f | 0x40
	buffer[8] = buffer[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buffer[:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:]), nil
}

func validateNameSlug(name, slug string) error {
	if strings.TrimSpace(name) == "" || len(strings.TrimSpace(name)) > 120 {
		return errors.New("name must be between 1 and 120 characters")
	}
	if !slugPattern.MatchString(slug) || len(slug) > 63 {
		return errors.New("slug must use lower-case letters, numbers, and single hyphens")
	}
	return nil
}

func (s *Service) CreateOrganisation(ctx context.Context, name, slug string, actor Actor) (model.Organisation, error) {
	name, slug = strings.TrimSpace(name), strings.TrimSpace(slug)
	if err := validateNameSlug(name, slug); err != nil {
		return model.Organisation{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.Organisation{}, err
	}
	value, err := s.store.CreateOrganisation(ctx, model.Organisation{ID: id, Name: name, Slug: slug})
	if err != nil {
		return model.Organisation{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: value.ID, ActorID: actor.ID, Action: "organisation.created", TargetType: "organisation", TargetID: value.ID, Current: map[string]any{"name": value.Name, "slug": value.Slug}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) CreateProduct(ctx context.Context, organisationID, name, slug string, actor Actor) (model.Product, error) {
	name, slug = strings.TrimSpace(name), strings.TrimSpace(slug)
	if err := validateNameSlug(name, slug); err != nil {
		return model.Product{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.Product{}, err
	}
	value, err := s.store.CreateProduct(ctx, model.Product{ID: id, OrganisationID: organisationID, Name: name, Slug: slug})
	if err != nil {
		return model.Product{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: organisationID, ProductID: value.ID, ActorID: actor.ID, Action: "product.created", TargetType: "product", TargetID: value.ID, Current: map[string]any{"name": value.Name, "slug": value.Slug, "public_mcp_enabled": false}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) CreateEnvironment(ctx context.Context, organisationID, productID, name, slug string, production bool, actor Actor) (model.Environment, error) {
	name, slug = strings.TrimSpace(name), strings.TrimSpace(slug)
	if err := validateNameSlug(name, slug); err != nil {
		return model.Environment{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.Environment{}, err
	}
	value, err := s.store.CreateEnvironment(ctx, model.Environment{ID: id, OrganisationID: organisationID, ProductID: productID, Name: name, Slug: slug, IsProduction: production})
	if err != nil {
		return model.Environment{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: organisationID, ProductID: productID, ActorID: actor.ID, Action: "environment.created", TargetType: "environment", TargetID: value.ID, Current: map[string]any{"name": value.Name, "slug": value.Slug, "is_production": value.IsProduction}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) CreateSource(ctx context.Context, organisationID, productID, name, kind, location string, actor Actor) (model.Source, error) {
	name, kind, location = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(kind)), strings.TrimSpace(location)
	if location == "" || len(location) > 2048 {
		return model.Source{}, errors.New("source location is required and must not exceed 2048 bytes")
	}
	allowedKinds := map[string]bool{"website": true, "openapi": true, "git": true, "upload": true}
	if !allowedKinds[kind] {
		return model.Source{}, errors.New("unsupported source kind")
	}
	if kind == "website" || kind == "openapi" {
		parsed, err := url.Parse(location)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return model.Source{}, errors.New("web source must use an absolute http(s) URL without embedded credentials")
		}
	}
	if name == "" {
		name = sourceNameFromLocation(kind, location)
	}
	if name == "" || len([]rune(name)) > 120 {
		return model.Source{}, errors.New("source name must not exceed 120 characters")
	}
	id, err := randomUUID()
	if err != nil {
		return model.Source{}, err
	}
	value, err := s.store.CreateSource(ctx, model.Source{ID: id, OrganisationID: organisationID, ProductID: productID, Name: name, Kind: kind, Location: location, Visibility: model.VisibilityPrivate})
	if err != nil {
		return model.Source{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: organisationID, ProductID: productID, ActorID: actor.ID, Action: "source.created", TargetType: "source", TargetID: value.ID, Current: map[string]any{"name": value.Name, "kind": value.Kind, "visibility": model.VisibilityPrivate}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func sourceNameFromLocation(kind, location string) string {
	name := strings.TrimSpace(location)
	parsed, err := url.Parse(location)
	if err == nil && parsed.Host != "" {
		path := strings.Trim(parsed.Path, "/")
		if decoded, decodeErr := url.PathUnescape(path); decodeErr == nil {
			path = decoded
		}
		if kind == "git" && path != "" {
			parts := strings.Split(path, "/")
			parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], ".git")
			if len(parts) > 1 {
				name = parts[len(parts)-2] + "/" + parts[len(parts)-1]
			} else {
				name = parts[0]
			}
		} else if path != "" {
			name = parsed.Host + "/" + path
		} else {
			name = parsed.Host
		}
	}
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > 120 {
		runes = runes[:120]
	}
	return string(runes)
}

func (s *Service) QueueCrawl(ctx context.Context, productID, sourceID string, actor Actor) (model.CrawlJob, error) {
	source, err := s.store.Source(ctx, productID, sourceID)
	if err != nil {
		return model.CrawlJob{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.CrawlJob{}, err
	}
	job, err := s.store.CreateCrawlJob(ctx, model.CrawlJob{ID: id, OrganisationID: source.OrganisationID, ProductID: productID, SourceID: sourceID, State: "queued"})
	if err != nil {
		return model.CrawlJob{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: source.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "source.crawl.queued", TargetType: "crawl_job", TargetID: job.ID, Current: map[string]any{"source_id": sourceID, "state": "queued"}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return job, err
}
