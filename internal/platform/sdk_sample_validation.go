package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"unicode"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const sdkSampleValidationSchemaVersion = "sdk-sample-validation-v1"

var (
	sdkIncompleteExpressionEnd = regexp.MustCompile(`(?:=>|->|\+=|-=|\*=|/=|%=|&&|\|\||\?\?|[=+*/%.,])\s*$`)
	sdkMissingRightOperand     = regexp.MustCompile(`(?:^|[\s({\[;,])(?:=>|->|\+=|-=|\*=|/=|%=|&&|\|\||\?\?|=|[+*/%])\s*(?:[;)}\]]|$)`)
	sdkPythonBlockHeader       = regexp.MustCompile(`^(?:(?:async\s+)?def|class|if|elif|else|for|while|try|except|finally|with|match|case)\b`)
	sdkRubyBlockHeader         = regexp.MustCompile(`^(?:class|module|def|if|unless|case|while|until|for|begin)\b`)
	sdkRubyDoBlock             = regexp.MustCompile(`\bdo(?:\s*\|[^|]*\|)?\s*(?:#.*)?$`)
	sdkRubyEnd                 = regexp.MustCompile(`\bend\b`)
)

type sdkLexicalProfile struct {
	slashComments   bool
	hashComments    bool
	blockComments   bool
	backtickStrings bool
	tripleStrings   bool
}

func sdkSampleValidationEvidence(language, validator, checkKind string, completeFile, validated bool, reason string) json.RawMessage {
	evidence := map[string]any{
		"schema_version":        sdkSampleValidationSchemaVersion,
		"language":              language,
		"validated":             validated,
		"result":                "not_checked",
		"check_kind":            checkKind,
		"complete_file":         completeFile,
		"no_execution":          true,
		"no_dependency_install": true,
		"no_dependency_loading": true,
	}
	if validator != "" {
		evidence["validator"] = validator
	}
	if validated {
		evidence["result"] = "passed"
	} else if strings.TrimSpace(reason) != "" {
		evidence["reason"] = strings.TrimSpace(reason)
	}
	value, _ := json.Marshal(evidence)
	return value
}

func staticSDKSampleValidation(language, code string, completeFile bool) (model.SDKSampleValidation, json.RawMessage) {
	language = normalizeSDKLanguage(language, "")
	code = strings.TrimSpace(code)
	if code == "" {
		return model.SDKSampleNotChecked, sdkSampleValidationEvidence(language, "", "none", completeFile, false, "sample is empty")
	}
	if language == "json" {
		if json.Valid([]byte(code)) {
			return model.SDKSampleSyntaxChecked, sdkSampleValidationEvidence(language, "encoding/json", "parse_only", completeFile, true, "")
		}
		return model.SDKSampleNotChecked, sdkSampleValidationEvidence(language, "encoding/json", "parse_only", completeFile, false, "JSON parser rejected the sample")
	}
	if language == "go" {
		contextName, err := parseGoSDKSample(code, completeFile)
		if err == nil {
			evidence := sdkSampleValidationEvidence(language, "go/parser", "parse_only", completeFile, true, "")
			var body map[string]any
			_ = json.Unmarshal(evidence, &body)
			body["parse_context"] = contextName
			evidence, _ = json.Marshal(body)
			return model.SDKSampleSyntaxChecked, evidence
		}
		return model.SDKSampleNotChecked, sdkSampleValidationEvidence(language, "go/parser", "parse_only", completeFile, false, compactSDKValidationReason(err))
	}

	validator := "dokosoko/" + language + "-conservative-structure-v1"
	var err error
	switch language {
	case "javascript", "typescript":
		err = validateCStyleSDKSample(language, code, false)
	case "java", "csharp", "php":
		err = validateCStyleSDKSample(language, code, true)
	case "python":
		err = validatePythonSDKSample(code)
	case "ruby":
		err = validateRubySDKSample(code)
	default:
		return model.SDKSampleNotChecked, sdkSampleValidationEvidence(language, "", "unsupported", completeFile, false, "no in-process conservative validator is available for this language")
	}
	if err != nil {
		return model.SDKSampleNotChecked, sdkAdvisoryStructureEvidence(language, validator, completeFile, false, compactSDKValidationReason(err))
	}
	return model.SDKSampleNotChecked, sdkAdvisoryStructureEvidence(language, validator, completeFile, true, "structure check passed, but no in-process grammar parser is available for this language")
}

func sdkAdvisoryStructureEvidence(language, validator string, completeFile, passed bool, reason string) json.RawMessage {
	evidence := sdkSampleValidationEvidence(language, validator, "conservative_structure", completeFile, false, reason)
	var body map[string]any
	_ = json.Unmarshal(evidence, &body)
	body["structure_passed"] = passed
	value, _ := json.Marshal(body)
	return value
}

func compactSDKValidationReason(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 400 {
		value = value[:400]
	}
	return value
}

func parseGoSDKSample(code string, completeFile bool) (string, error) {
	parse := func(name, source string) error {
		_, err := parser.ParseFile(token.NewFileSet(), name, source, parser.AllErrors)
		return err
	}
	if err := parse("sample.go", code); err == nil {
		return "file", nil
	} else if completeFile {
		return "", fmt.Errorf("Go parser rejected the complete file: %w", err)
	}
	attempts := []struct {
		name   string
		source string
	}{
		{name: "declarations", source: "package sample\n" + code + "\n"},
		{name: "statements", source: "package sample\nfunc sample() {\n" + code + "\n}\n"},
		{name: "expression", source: "package sample\nvar _ = (" + code + ")\n"},
	}
	var last error
	for _, attempt := range attempts {
		if err := parse("sample.go", attempt.source); err == nil {
			return attempt.name, nil
		} else {
			last = err
		}
	}
	return "", fmt.Errorf("Go parser rejected all conservative snippet contexts: %w", last)
}

func sdkLexicalProfileFor(language string) sdkLexicalProfile {
	switch language {
	case "javascript", "typescript":
		return sdkLexicalProfile{slashComments: true, blockComments: true, backtickStrings: true}
	case "java":
		return sdkLexicalProfile{slashComments: true, blockComments: true, tripleStrings: true}
	case "csharp":
		return sdkLexicalProfile{slashComments: true, blockComments: true}
	case "php":
		return sdkLexicalProfile{slashComments: true, hashComments: true, blockComments: true, backtickStrings: true}
	case "python":
		return sdkLexicalProfile{hashComments: true, tripleStrings: true}
	case "ruby":
		return sdkLexicalProfile{hashComments: true, backtickStrings: true}
	default:
		return sdkLexicalProfile{}
	}
}

func lexicalSDKSample(language, code string) (string, error) {
	profile := sdkLexicalProfileFor(language)
	if (language == "php" && strings.Contains(code, "<<<")) || (language == "csharp" && strings.Contains(code, `@"`)) {
		return "", errors.New("sample uses a string form outside the conservative validator subset")
	}
	masked := []byte(code)
	stack := make([]byte, 0)
	matching := map[byte]byte{')': '(', ']': '[', '}': '{'}
	mask := func(start, end int) {
		for index := start; index < end; index++ {
			if masked[index] != '\n' && masked[index] != '\r' {
				masked[index] = ' '
			}
		}
	}
	for index := 0; index < len(code); {
		current := code[index]
		if current < 0x20 && current != '\n' && current != '\r' && current != '\t' {
			return "", fmt.Errorf("sample contains control byte 0x%02x", current)
		}
		if profile.slashComments && current == '/' && index+1 < len(code) && code[index+1] == '/' {
			end := index + 2
			for end < len(code) && code[end] != '\n' {
				end++
			}
			mask(index, end)
			index = end
			continue
		}
		if profile.blockComments && current == '/' && index+1 < len(code) && code[index+1] == '*' {
			end := strings.Index(code[index+2:], "*/")
			if end < 0 {
				return "", errors.New("unterminated block comment")
			}
			end += index + 4
			mask(index, end)
			index = end
			continue
		}
		if profile.hashComments && current == '#' {
			end := index + 1
			for end < len(code) && code[end] != '\n' {
				end++
			}
			mask(index, end)
			index = end
			continue
		}
		if current == '\'' || current == '"' || (current == '`' && profile.backtickStrings) {
			start, delimiter, width := index, current, 1
			if profile.tripleStrings && current != '`' && index+2 < len(code) && code[index+1] == current && code[index+2] == current {
				width = 3
			}
			index += width
			closed := false
			for index < len(code) {
				if width == 1 && delimiter == '`' && index+1 < len(code) && code[index] == '$' && code[index+1] == '{' {
					return "", errors.New("template interpolation is outside the conservative validator subset")
				}
				if code[index] == '\\' {
					index += 2
					continue
				}
				if width == 3 && index+2 < len(code) && code[index] == delimiter && code[index+1] == delimiter && code[index+2] == delimiter {
					index += 3
					closed = true
					break
				}
				if width == 1 && code[index] == delimiter {
					index++
					closed = true
					break
				}
				if width == 1 && delimiter != '`' && (code[index] == '\n' || code[index] == '\r') {
					return "", errors.New("unterminated single-line string")
				}
				index++
			}
			if !closed {
				return "", errors.New("unterminated string")
			}
			mask(start, index)
			masked[start] = 'v'
			continue
		}
		switch current {
		case '(', '[', '{':
			stack = append(stack, current)
		case ')', ']', '}':
			if len(stack) == 0 || stack[len(stack)-1] != matching[current] {
				return "", fmt.Errorf("unmatched delimiter %q", current)
			}
			stack = stack[:len(stack)-1]
		}
		index++
	}
	if len(stack) != 0 {
		return "", fmt.Errorf("unclosed delimiter %q", stack[len(stack)-1])
	}
	return string(masked), nil
}

func validateExpressionBoundaries(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("sample contains no syntax after comments")
	}
	if strings.HasSuffix(value, "++") || strings.HasSuffix(value, "--") {
		return nil
	}
	if sdkIncompleteExpressionEnd.MatchString(value) {
		return errors.New("sample ends with an incomplete expression")
	}
	if sdkMissingRightOperand.MatchString(value) {
		return errors.New("sample contains an operator without a right operand")
	}
	return nil
}

func validateCStyleSDKSample(language, code string, requireTerminator bool) error {
	masked, err := lexicalSDKSample(language, code)
	if err != nil {
		return err
	}
	if err := validateExpressionBoundaries(masked); err != nil {
		return err
	}
	if requireTerminator {
		value := strings.TrimSpace(masked)
		if language == "php" {
			value = strings.TrimSpace(strings.TrimSuffix(value, "?>"))
			value = strings.TrimSpace(strings.TrimPrefix(value, "<?php"))
		}
		if value == "" || (!strings.HasSuffix(value, ";") && !strings.HasSuffix(value, "}")) {
			return errors.New("sample does not end at a conservative statement or block boundary")
		}
	}
	return nil
}

func validatePythonSDKSample(code string) error {
	masked, err := lexicalSDKSample("python", code)
	if err != nil {
		return err
	}
	if err := validateExpressionBoundaries(masked); err != nil {
		return err
	}
	indentStack := []int{0}
	expectIndentedAfter := -1
	for _, line := range strings.Split(masked, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		leading := line[:len(line)-len(strings.TrimLeftFunc(line, unicode.IsSpace))]
		if strings.Contains(leading, "\t") {
			return errors.New("tab-indented Python is outside the conservative validator subset")
		}
		indent := len(leading)
		if expectIndentedAfter >= 0 {
			if indent <= expectIndentedAfter {
				return errors.New("Python block header is not followed by an indented suite")
			}
			indentStack = append(indentStack, indent)
			expectIndentedAfter = -1
		} else if indent > indentStack[len(indentStack)-1] {
			return errors.New("Python sample contains an unexpected indent")
		} else if indent < indentStack[len(indentStack)-1] {
			for len(indentStack) > 1 && indent < indentStack[len(indentStack)-1] {
				indentStack = indentStack[:len(indentStack)-1]
			}
			if indent != indentStack[len(indentStack)-1] {
				return errors.New("Python sample dedents to an unknown indentation level")
			}
		}
		if sdkPythonBlockHeader.MatchString(trimmed) {
			colon := strings.Index(trimmed, ":")
			if colon < 0 {
				return errors.New("Python block header is missing a colon")
			}
			if strings.TrimSpace(trimmed[colon+1:]) == "" {
				expectIndentedAfter = indent
			}
		}
	}
	if expectIndentedAfter >= 0 {
		return errors.New("Python sample ends with an incomplete block header")
	}
	return nil
}

func validateRubySDKSample(code string) error {
	masked, err := lexicalSDKSample("ruby", code)
	if err != nil {
		return err
	}
	if err := validateExpressionBoundaries(masked); err != nil {
		return err
	}
	depth := 0
	for _, line := range strings.Split(masked, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		startsBlock := sdkRubyBlockHeader.MatchString(trimmed)
		if strings.HasPrefix(trimmed, "def ") && strings.Contains(trimmed, " = ") {
			startsBlock = false
		}
		if startsBlock || (!startsBlock && sdkRubyDoBlock.MatchString(trimmed)) {
			depth++
		}
		depth -= len(sdkRubyEnd.FindAllString(trimmed, -1))
		if depth < 0 {
			return errors.New("Ruby sample contains an unmatched end")
		}
	}
	if depth != 0 {
		return errors.New("Ruby sample contains an unclosed keyword block")
	}
	return nil
}
