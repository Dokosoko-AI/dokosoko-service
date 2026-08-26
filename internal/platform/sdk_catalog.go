package platform

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	sdkChecksumPattern        = regexp.MustCompile(`^(sha256|sha384|sha512):[a-f0-9]+$`)
	sdkNPMCoordinatePattern   = regexp.MustCompile(`^(?:@[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?/)?[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$`)
	sdkPyPICoordinatePattern  = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`)
	sdkGoCoordinatePattern    = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?(?:/[A-Za-z0-9](?:[A-Za-z0-9._~-]*[A-Za-z0-9])?)+$`)
	sdkCargoCoordinatePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_-]*[A-Za-z0-9])?$`)
	sdkSemverPattern          = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	sdkCargoVersionPattern    = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	sdkGoVersionPattern       = regexp.MustCompile(`^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+incompatible)?$`)
	sdkPyPIVersionPattern     = regexp.MustCompile(`(?i)^[0-9]+(?:\.[0-9]+)*(?:(?:a|b|rc)[0-9]+)?(?:\.post[0-9]+)?(?:\.dev[0-9]+)?(?:\+[a-z0-9]+(?:[._-][a-z0-9]+)*)?$`)
)

func validSDKURL(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func canonicalSDKInstallCommand(ecosystem, coordinate, exactVersion string) (string, error) {
	switch ecosystem {
	case "npm":
		if !sdkNPMCoordinatePattern.MatchString(coordinate) {
			return "", errors.New("npm coordinate must be one registry package name, optionally with a scope")
		}
		if !sdkSemverPattern.MatchString(exactVersion) {
			return "", errors.New("npm exact_version must be one complete semantic version")
		}
		return fmt.Sprintf("npm install %s@%s", coordinate, exactVersion), nil
	case "pypi":
		if !sdkPyPICoordinatePattern.MatchString(coordinate) {
			return "", errors.New("pypi coordinate must be one registry project name without extras or a URL")
		}
		if !sdkPyPIVersionPattern.MatchString(exactVersion) {
			return "", errors.New("pypi exact_version must be one fixed PEP 440 version without shell-significant epoch syntax")
		}
		return fmt.Sprintf("python -m pip install %s==%s", coordinate, exactVersion), nil
	case "go":
		firstElement, _, _ := strings.Cut(coordinate, "/")
		if !strings.Contains(firstElement, ".") || !sdkGoCoordinatePattern.MatchString(coordinate) {
			return "", errors.New("go coordinate must be one module path, not a URL or package pattern")
		}
		if !sdkGoVersionPattern.MatchString(exactVersion) {
			return "", errors.New("go exact_version must be one canonical v-prefixed semantic or pseudo-version")
		}
		return fmt.Sprintf("go get %s@%s", coordinate, exactVersion), nil
	case "cargo":
		if !sdkCargoCoordinatePattern.MatchString(coordinate) {
			return "", errors.New("cargo coordinate must be one registry crate name")
		}
		if !sdkCargoVersionPattern.MatchString(exactVersion) {
			return "", errors.New("cargo exact_version must be one complete semantic version without ignored build metadata")
		}
		return fmt.Sprintf("cargo add %s@=%s", coordinate, exactVersion), nil
	default:
		return "", errors.New("unsupported SDK ecosystem; supported ecosystems are npm, pypi, go, and cargo")
	}
}
