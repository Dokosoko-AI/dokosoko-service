package acceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

const TaskPlanVersion = "mcp-task-check-v1"
const maxTaskPlanBytes = 64 << 10

type TaskSelection struct {
	Title       string `json:"title"`
	Outcome     string `json:"outcome"`
	ResourceURI string `json:"resource_uri"`
	RevisionID  string `json:"revision_id"`
}

type ResourceExpectation struct {
	URI        string         `json:"uri"`
	Discover   bool           `json:"discover"`
	TextSHA256 string         `json:"text_sha256"`
	MIMEType   string         `json:"mime_type,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// A plan contains reviewed expectations, never credentials or executable commands.
// TextSHA256 hashes the exact UTF-8 contents[].text, not a source-content hash
// declared inside the text or its metadata.
type TaskPlan struct {
	SchemaVersion string                `json:"schema_version"`
	Endpoint      string                `json:"endpoint"`
	Task          TaskSelection         `json:"task"`
	Resources     []ResourceExpectation `json:"resources"`
}

type TaskReport struct {
	Selection            TaskSelection `json:"selection"`
	PlanSHA256           string        `json:"plan_sha256"`
	RetrievalStatus      Status        `json:"retrieval_status"`
	ImplementationStatus string        `json:"implementation_status"`
}

func LoadTaskPlan(path string) (*TaskPlan, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTaskPlanBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxTaskPlanBytes {
		return nil, errors.New("task plan exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var plan TaskPlan
	if err := decoder.Decode(&plan); err != nil {
		return nil, errors.New("task plan is not a supported JSON object")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("task plan must contain one JSON object")
	}
	if err := plan.validate(plan.Endpoint); err != nil {
		return nil, err
	}
	return &plan, nil
}

func validPlanLabel(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= limit && !strings.ContainsAny(value, "\x00\r\n\t")
}
func validResourceURI(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.IsAbs() && parsed.User == nil && parsed.Fragment == "" && validPlanLabel(value, 2048) && !strings.Contains(value, " ")
}
func validTextHash(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	decoded, err := hex.DecodeString(value[7:])
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}
func textHash(value []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(value)) }

func (p TaskPlan) validate(endpoint string) error {
	if p.SchemaVersion != TaskPlanVersion {
		return errors.New("task plan schema version is unsupported")
	}
	if p.Endpoint == "" || strings.TrimRight(p.Endpoint, "/") != strings.TrimRight(endpoint, "/") {
		return errors.New("task plan endpoint does not match the explicitly selected endpoint")
	}
	if !validPlanLabel(p.Task.Title, 200) || !validPlanLabel(p.Task.Outcome, 2000) || !validResourceURI(p.Task.ResourceURI) || !validPlanLabel(p.Task.RevisionID, 200) {
		return errors.New("task plan requires a title, outcome, exact task resource URI and revision")
	}
	if len(p.Resources) == 0 || len(p.Resources) > 32 {
		return errors.New("task plan requires between 1 and 32 exact resources")
	}
	seen, taskFound := make(map[string]bool), false
	for _, resource := range p.Resources {
		if !validResourceURI(resource.URI) || seen[resource.URI] || !validTextHash(resource.TextSHA256) || len(resource.Metadata) > 16 || len(resource.MIMEType) > 200 {
			return errors.New("task plan resource expectations are invalid or duplicated")
		}
		seen[resource.URI] = true
		if resource.URI == p.Task.ResourceURI {
			revision, ok := resource.Metadata["revision_id"].(string)
			if !resource.Discover || !ok || revision != p.Task.RevisionID {
				return errors.New("task resource must require discovery and pin the selected revision_id metadata")
			}
			taskFound = true
		}
	}
	if !taskFound {
		return errors.New("task plan is missing its selected task resource expectation")
	}
	data, err := json.Marshal(p)
	if err != nil || len(data) > maxTaskPlanBytes {
		return errors.New("task plan exceeds the supported JSON bounds")
	}
	return nil
}

func (p TaskPlan) fingerprint() string {
	data, _ := json.Marshal(p) // validate has already established a bounded JSON value.
	return textHash(data)
}

func metadataMatches(actual, expected map[string]any) bool {
	for key, value := range expected {
		got, exists := actual[key]
		left, leftErr := json.Marshal(got)
		right, rightErr := json.Marshal(value)
		if !exists || leftErr != nil || rightErr != nil || !bytes.Equal(left, right) {
			return false
		}
	}
	return true
}

type resourceDescription struct {
	URI      string         `json:"uri"`
	MIMEType string         `json:"mimeType"`
	Metadata map[string]any `json:"_meta"`
}

func resourceReadCheck(uri string, outcome callOutcome, expected *ResourceExpectation) Check {
	check := outcomeCheck("resources/read: "+uri, outcome, nil)
	check.ResourceURI = uri
	if check.Status != Pass {
		return check
	}
	var result struct {
		Contents []struct {
			resourceDescription
			Text *string `json:"text"`
			Blob *string `json:"blob"`
		} `json:"contents"`
	}
	if json.Unmarshal(outcome.Response.Result, &result) != nil || len(result.Contents) != 1 || result.Contents[0].URI != uri {
		check.Status, check.Detail = Fail, "read must return exactly one content item with the requested URI"
		return check
	}
	content := result.Contents[0]
	var data []byte
	switch {
	case content.Text != nil && content.Blob == nil:
		data = []byte(*content.Text)
	case content.Blob != nil && content.Text == nil && expected == nil:
		var err error
		data, err = base64.StdEncoding.DecodeString(*content.Blob)
		if err != nil {
			check.Status, check.Detail = Fail, "resource blob is not valid base64"
			return check
		}
	default:
		check.Status, check.Detail = Fail, "read must return one supported text or binary value; task checks require text"
		return check
	}
	if len(data) == 0 {
		check.Status, check.Detail = Fail, "resource content is empty"
		return check
	}
	check.ContentSHA256, check.ContentBytes = textHash(data), len(data)
	if expected != nil {
		if check.ContentSHA256 != expected.TextSHA256 {
			check.Status, check.Detail = Fail, "exact resource text does not match the reviewed SHA-256"
			return check
		}
		if expected.MIMEType != "" && content.MIMEType != expected.MIMEType {
			check.Status, check.Detail = Fail, "resource media type does not match the reviewed expectation"
			return check
		}
		if !metadataMatches(content.Metadata, expected.Metadata) {
			check.Status, check.Detail = Fail, "resource revision or publication metadata does not match the reviewed expectation"
			return check
		}
		check.Detail = "exact text, URI and configured revision/publication metadata matched"
	} else {
		check.Detail = "matching non-empty resource returned; content version was not pinned"
	}
	return check
}
