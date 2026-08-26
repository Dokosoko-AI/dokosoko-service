package platform

import "regexp"

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
	return containsToolBuilderSecretText(value)
}
