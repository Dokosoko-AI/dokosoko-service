package acceptance

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func WriteHuman(w io.Writer, report Report) error {
	if _, err := fmt.Fprintf(w, "MCP acceptance report\nEndpoint: %s\nProtocol: %s\n\n", report.Endpoint, report.ProtocolVersion); err != nil {
		return err
	}
	for _, check := range report.Checks {
		marker := map[Status]string{Pass: "PASS", Fail: "FAIL", Skip: "SKIP"}[check.Status]
		if check.Status == Skip && check.Required {
			marker = "SKIP-REQUIRED"
		}
		if _, err := fmt.Fprintf(w, "[%s] %s", marker, check.Name); err != nil {
			return err
		}
		details := make([]string, 0, 4)
		if check.Detail != "" {
			details = append(details, check.Detail)
		}
		if check.HTTPStatus != 0 {
			details = append(details, fmt.Sprintf("HTTP %d", check.HTTPStatus))
		}
		if check.RequestID != "" {
			details = append(details, "request "+check.RequestID)
		}
		if len(details) > 0 {
			if _, err := fmt.Fprintf(w, " — %s", strings.Join(details, "; ")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "\nSummary: %d passed, %d failed, %d skipped (%d required; %d ms)\n", report.Summary.Passed, report.Summary.Failed, report.Summary.Skipped, report.Summary.RequiredSkipped, report.DurationMS)
	return err
}

func WriteJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
