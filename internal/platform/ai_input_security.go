package platform

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	aiSecretAssignment = regexp.MustCompile(`(?i)["']?(?:authorization|api[-_ ]?key|access[-_ ]?token|refresh[-_ ]?token|client[-_ ]?secret|password|secret|aws[-_ ]?secret[-_ ]?access[-_ ]?key|aws[-_ ]?session[-_ ]?token|account[-_ ]?key|sas[-_ ]?token|private[-_ ]?key)["']?\s*[:=]\s*["']?[^\s,"'}]{8,}`)
	aiAWSAccessKey     = regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)
	aiPEMPrivateKey    = regexp.MustCompile(`(?i)-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`)
)

// containsAISecretText is a conservative last-line check immediately before
// data crosses an AI-provider boundary. It complements typed credential
// storage: no prompt body, operator request, or reviewed evidence excerpt is
// allowed to carry common live credential forms to a primary or backup model.
func containsAISecretText(value string) bool {
	// Provider inputs are often JSON envelopes containing source text. Inspect
	// decoded values so JSON escapes do not extend a source variable into a
	// secret-looking assignment, and escaped credential characters cannot hide
	// from the same checks. Literal credential fields retain the text policy.
	if json.Valid([]byte(value)) {
		decoder := json.NewDecoder(strings.NewReader(value))
		decoder.UseNumber()
		_, unsafe, err := scanAIJSONSecrets(decoder)
		if err == nil {
			return unsafe
		}
	}
	return containsToolBuilderSecretText(value)
}

// Visit every JSON member, including duplicate keys. Decoding straight into a
// map would discard an earlier secret even though its bytes reach the model.
func scanAIJSONSecrets(decoder *json.Decoder) (any, bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, false, err
	}
	switch token {
	case json.Delim('{'):
		unsafe := false
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, false, err
			}
			child, childUnsafe, err := scanAIJSONSecrets(decoder)
			if err != nil {
				return nil, false, err
			}
			name := key.(string) // json.Valid and the decoder enforce object keys.
			unsafe = unsafe || childUnsafe || containsToolBuilderSecretText(name) || aiJSONSecretFieldValue(name, child)
		}
		_, err = decoder.Token()
		return nil, unsafe, err
	case json.Delim('['):
		unsafe := false
		for decoder.More() {
			_, childUnsafe, err := scanAIJSONSecrets(decoder)
			if err != nil {
				return nil, false, err
			}
			unsafe = unsafe || childUnsafe
		}
		_, err = decoder.Token()
		return nil, unsafe, err
	default:
		value, ok := token.(string)
		return token, ok && containsToolBuilderSecretText(value), nil
	}
}

func aiJSONSecretFieldValue(name string, child any) bool {
	// Scan decoded scalar assignments with the existing key vocabulary and
	// thresholds. Objects (including field schemas), booleans such as
	// requiresAuthorization, and null are not credential literals. Their nested
	// members have already been checked individually without discarding duplicates.
	var value string
	switch scalar := child.(type) {
	case string:
		value = scalar
	case json.Number:
		value = scalar.String()
	default:
		return false
	}
	return aiSecretAssignment.MatchString(name + "=" + value)
}
