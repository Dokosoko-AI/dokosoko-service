package backend

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var contextKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)

func validateSubmission(value SupportSubmissionRequest) error {
	if err := boundedRequired("submission_id", value.SubmissionID, 200); err != nil {
		return err
	}
	if err := dateTime("created_at", value.CreatedAt); err != nil {
		return err
	}
	submission := value.Submission
	if submission.SchemaVersion != "2026-08-25" {
		return fmt.Errorf("submission.schema_version must be 2026-08-25")
	}
	if submission.Kind != "bug" && submission.Kind != "feedback" {
		return fmt.Errorf("submission.kind must be bug or feedback")
	}
	if submission.Kind == "bug" && (submission.Bug == nil || submission.Feedback != nil) {
		return fmt.Errorf("a bug submission must contain bug and must not contain feedback")
	}
	if submission.Kind == "feedback" && (submission.Feedback == nil || submission.Bug != nil) {
		return fmt.Errorf("a feedback submission must contain feedback and must not contain bug")
	}
	if submission.Channel != "private_mcp" {
		return fmt.Errorf("submission.channel must be private_mcp")
	}
	if err := dateTime("submission.confirmed_at", submission.ConfirmedAt); err != nil {
		return err
	}
	if err := boundedRequired("submission.request_id", submission.RequestID, 200); err != nil {
		return err
	}
	if err := validateReporter(submission.Reporter); err != nil {
		return err
	}
	if submission.Provider.Key != "dokosoko" {
		return fmt.Errorf("submission.provider.key must be dokosoko")
	}
	if utf8.RuneCountInString(submission.Provider.Name) > 200 || utf8.RuneCountInString(submission.Provider.Version) > 100 {
		return fmt.Errorf("submission.provider field exceeds its documented maximum length")
	}
	if err := validateResource("submission.resource", submission.Resource); err != nil {
		return err
	}
	if len(submission.RelatedResources) > 20 {
		return fmt.Errorf("submission.related_resources must contain no more than 20 items")
	}
	seen := map[string]bool{submission.Resource.Type + "\x00" + submission.Resource.ID: true}
	for index, resource := range submission.RelatedResources {
		if err := validateResource(fmt.Sprintf("submission.related_resources[%d]", index), resource); err != nil {
			return err
		}
		key := resource.Type + "\x00" + resource.ID
		if seen[key] {
			return fmt.Errorf("submission resource identities must be unique")
		}
		seen[key] = true
	}
	if submission.Extensions == nil {
		return fmt.Errorf("submission.extensions is required")
	}
	if submission.Extensions.DokoSoko.Integration != nil {
		if err := validateIntegration(*submission.Extensions.DokoSoko.Integration); err != nil {
			return err
		}
	}
	if submission.Bug != nil {
		return validateBug(*submission.Bug)
	}
	return validateFeedback(*submission.Feedback)
}

func validateReporter(value ReporterContext) error {
	if err := absoluteURI("submission.reporter.principal.issuer", value.Principal.Issuer); err != nil {
		return err
	}
	if err := boundedRequired("submission.reporter.principal.subject", value.Principal.Subject, 0); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value string
		max   int
	}{
		{"display_name", value.DisplayName, 200},
		{"external_customer_id", value.ExternalCustomerID, 200},
		{"installation_id", value.InstallationID, 200},
	} {
		if utf8.RuneCountInString(field.value) > field.max {
			return fmt.Errorf("submission.reporter.%s must not exceed %d characters", field.name, field.max)
		}
	}
	if utf8.RuneCountInString(value.Email) > 320 {
		return fmt.Errorf("submission.reporter.email must not exceed 320 characters")
	}
	if value.Email != "" {
		address, err := mail.ParseAddress(value.Email)
		if err != nil || address.Address != value.Email {
			return fmt.Errorf("submission.reporter.email must be a valid email address")
		}
	}
	if value.AllowContact == nil {
		return fmt.Errorf("submission.reporter.allow_contact is required")
	}
	return nil
}

func validateResource(name string, value ResourceContext) error {
	if !contextKeyPattern.MatchString(value.Type) || utf8.RuneCountInString(value.Type) > 64 {
		return fmt.Errorf("%s.type is invalid", name)
	}
	if err := boundedRequired(name+".id", value.ID, 200); err != nil {
		return err
	}
	if err := boundedRequired(name+".name", value.Name, 200); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value string
		max   int
	}{
		{"version_id", value.VersionID, 200},
		{"version", value.Version, 100},
		{"environment_id", value.EnvironmentID, 200},
		{"installation_id", value.InstallationID, 200},
		{"state", value.State, 64},
	} {
		if utf8.RuneCountInString(field.value) > field.max {
			return fmt.Errorf("%s.%s must not exceed %d characters", name, field.name, field.max)
		}
	}
	if value.Revision < 0 {
		return fmt.Errorf("%s.revision must be at least 1 when supplied", name)
	}
	return nil
}

func validateIntegration(value IntegrationContext) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{"integration_id", value.IntegrationID},
		{"family_key", value.FamilyKey},
		{"version_key", value.VersionKey},
		{"display_name", value.DisplayName},
		{"lifecycle", value.Lifecycle},
	} {
		if err := boundedRequired("submission.extensions.dokosoko.integration."+field.name, field.value, 0); err != nil {
			return err
		}
	}
	if value.Revision < 1 {
		return fmt.Errorf("submission.extensions.dokosoko.integration.revision must be at least 1")
	}
	return nil
}

func validateBug(value BugReport) error {
	if err := boundedRequired("submission.bug.summary", value.Summary, 160); err != nil {
		return err
	}
	if err := boundedRequired("submission.bug.description", value.Description, 10000); err != nil {
		return err
	}
	if len(value.ReproductionSteps) > 20 {
		return fmt.Errorf("submission.bug.reproduction_steps must contain no more than 20 items")
	}
	for index, step := range value.ReproductionSteps {
		if step == "" || utf8.RuneCountInString(step) > 1000 {
			return fmt.Errorf("submission.bug.reproduction_steps[%d] must contain 1 to 1000 characters", index)
		}
	}
	limits := []struct {
		name  string
		value string
		max   int
	}{
		{"expected_behavior", value.ExpectedBehavior, 4000},
		{"actual_behavior", value.ActualBehavior, 4000},
		{"error_code", value.ErrorCode, 120},
		{"error_message", value.ErrorMessage, 8000},
		{"stack_trace", value.StackTrace, 16000},
		{"diagnostic_context", value.DiagnosticContext, 20000},
		{"related_tool", value.RelatedTool, 160},
		{"integration_run_id", value.IntegrationRunID, 160},
	}
	for _, field := range limits {
		if utf8.RuneCountInString(field.value) > field.max {
			return fmt.Errorf("submission.bug.%s must not exceed %d characters", field.name, field.max)
		}
	}
	if value.Severity != nil && *value.Severity != "unknown" && *value.Severity != "low" && *value.Severity != "medium" && *value.Severity != "high" && *value.Severity != "critical" {
		return fmt.Errorf("submission.bug.severity is invalid")
	}
	return nil
}

func validateFeedback(value FeedbackReport) error {
	if err := boundedRequired("submission.feedback.message", value.Message, 10000); err != nil {
		return err
	}
	if value.Category != nil && *value.Category != "general" && *value.Category != "usability" && *value.Category != "documentation" && *value.Category != "performance" && *value.Category != "feature_request" && *value.Category != "other" {
		return fmt.Errorf("submission.feedback.category is invalid")
	}
	if value.Rating != nil && (*value.Rating < 1 || *value.Rating > 5) {
		return fmt.Errorf("submission.feedback.rating must be between 1 and 5")
	}
	if utf8.RuneCountInString(value.RelatedTool) > 160 || utf8.RuneCountInString(value.IntegrationRunID) > 160 {
		return fmt.Errorf("submission.feedback related identifiers must not exceed 160 characters")
	}
	return nil
}

func boundedRequired(name, value string, max int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if max > 0 && utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%s must not exceed %d characters", name, max)
	}
	return nil
}

func dateTime(name, value string) error {
	if _, err := time.Parse(time.RFC3339, strings.ToUpper(value)); err != nil {
		return fmt.Errorf("%s must be an RFC 3339 date-time", name)
	}
	return nil
}

func absoluteURI(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return fmt.Errorf("%s must be an absolute URI", name)
	}
	return nil
}
