package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
)

var ErrWidgetAssistantUnavailable = errors.New("widget assistant is unavailable")

func requestedWidgetToolExecution(message string, toolNames []string) (string, bool) {
	requestVerb := false
	for _, word := range strings.FieldsFunc(strings.ToLower(message), func(value rune) bool { return !unicode.IsLetter(value) }) {
		switch word {
		case "call", "execute", "invoke", "perform", "run", "trigger":
			requestVerb = true
		}
	}
	if !requestVerb {
		return "", false
	}
	lowerMessage := strings.ToLower(message)
	for _, name := range toolNames {
		if strings.Contains(lowerMessage, strings.ToLower(name)) {
			return name, true
		}
	}
	return "", false
}

func widgetGuidanceOnlyToolReply(toolName string) string {
	return fmt.Sprintf("I can explain the published `%s` operation, but this widget is currently configured for guidance only, so I can't run it. I can show you the exact prerequisites and verification steps, or an administrator can enable a confirmed action path for this widget.", toolName)
}

func (s *Service) requireWidgetAssistant(ctx context.Context, deploymentID, _ string) error {
	product, err := s.store.Product(ctx, deploymentID)
	if err != nil {
		return errors.New("configure and enable the Assistant workload before activating the widget")
	}
	_, _, credential, err := s.aiWorkloadConfiguration(ctx, product, airuntime.WorkloadAssistant)
	if err != nil {
		return errors.New("configure and enable the Assistant workload before activating the widget")
	}
	zeroBytes(credential)
	return nil
}

func (s *Service) AnswerWidgetMessage(ctx context.Context, principal WidgetPrincipal, message string) (string, error) {
	response, err := s.AnswerWidgetMessageDetailed(ctx, principal, message)
	return response.Answer, err
}
