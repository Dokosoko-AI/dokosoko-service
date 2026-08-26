package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var legacySDKEmptySelector = json.RawMessage(`{}`)

func legacySDKSelectorHash() string {
	digest := sha256.Sum256(legacySDKEmptySelector)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func legacySDKIdentityAssurance(checksum string) string {
	if checksum != "" {
		return "verified_digest"
	}
	return "metadata_only"
}

// legacySDKReleaseMatches compares every immutable release field represented
// by the deprecated SDK-reference contract. Hidden typed enrichment is never
// used to make two visibly different legacy releases appear identical.
func legacySDKReleaseMatches(release model.SDKRelease, reference model.SDKReference) bool {
	return release.ExactVersion == reference.ExactVersion &&
		release.InstallCommand == reference.InstallCommand &&
		release.DocumentationURL == reference.DocumentationURL &&
		release.SourceURL == reference.SourceURL &&
		release.UpstreamDigest == reference.Checksum &&
		release.Visibility == reference.Visibility
}

func legacySDKReleaseHash(packageID string, reference model.SDKReference) (string, error) {
	canonical, err := json.Marshal(struct {
		SDKPackageID     string           `json:"sdk_package_id"`
		ExactVersion     string           `json:"exact_version"`
		InstallCommand   string           `json:"install_command"`
		DocumentationURL string           `json:"documentation_url"`
		SourceURL        string           `json:"source_url"`
		UpstreamDigest   string           `json:"upstream_digest"`
		Visibility       model.Visibility `json:"visibility"`
	}{
		SDKPackageID: packageID, ExactVersion: reference.ExactVersion,
		InstallCommand: reference.InstallCommand, DocumentationURL: reference.DocumentationURL,
		SourceURL: reference.SourceURL, UpstreamDigest: reference.Checksum, Visibility: reference.Visibility,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func newStoreUUID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	buffer[6] = buffer[6]&0x0f | 0x40
	buffer[8] = buffer[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buffer[:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:]), nil
}
