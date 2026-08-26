package store

import (
	"errors"
	"fmt"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// ValidateSDKContentCandidateGraph verifies the immutable ownership and
// ancestry links in one candidate before it crosses a persistence,
// publication, or retrieval boundary. A section is owned by exactly one file;
// an entity that names both a file and a section must name that same file.
func ValidateSDKContentCandidateGraph(value SDKContentCandidateRecord) error {
	if value.Candidate.ID == "" || value.Candidate.DeploymentID == "" {
		return errors.New("SDK candidate identity is incomplete")
	}

	files := make(map[string]model.SDKPublicationFile, len(value.Files))
	for _, file := range value.Files {
		if file.ID == "" || file.SDKContentCandidateID != value.Candidate.ID {
			return errors.New("SDK candidate contains an inconsistent file")
		}
		if _, exists := files[file.ID]; exists {
			return fmt.Errorf("SDK candidate contains duplicate file %s", file.ID)
		}
		files[file.ID] = file
	}

	sections := make(map[string]model.SDKSection, len(value.Sections))
	for _, section := range value.Sections {
		if section.ID == "" || section.SDKContentCandidateID != value.Candidate.ID {
			return errors.New("SDK candidate contains an inconsistent section")
		}
		if _, exists := files[section.SDKPublicationFileID]; !exists {
			return fmt.Errorf("SDK section %s references an unknown candidate file", section.ID)
		}
		if _, exists := sections[section.ID]; exists {
			return fmt.Errorf("SDK candidate contains duplicate section %s", section.ID)
		}
		sections[section.ID] = section
	}
	for _, section := range value.Sections {
		if section.ParentSectionID == "" {
			continue
		}
		parent, exists := sections[section.ParentSectionID]
		if !exists {
			return fmt.Errorf("SDK section %s references an unknown parent section", section.ID)
		}
		if parent.SDKPublicationFileID != section.SDKPublicationFileID {
			return fmt.Errorf("SDK section %s and its parent belong to different files", section.ID)
		}
	}

	symbols := make(map[string]bool, len(value.Symbols))
	for _, symbol := range value.Symbols {
		if symbol.ID == "" || symbol.SDKContentCandidateID != value.Candidate.ID || symbols[symbol.ID] {
			return errors.New("SDK candidate contains an inconsistent symbol")
		}
		symbols[symbol.ID] = true
		if symbol.SDKPublicationFileID != "" {
			if _, exists := files[symbol.SDKPublicationFileID]; !exists {
				return fmt.Errorf("SDK symbol %s references an unknown candidate file", symbol.ID)
			}
		}
		if symbol.SDKSectionID == "" {
			continue
		}
		section, exists := sections[symbol.SDKSectionID]
		if !exists {
			return fmt.Errorf("SDK symbol %s references an unknown candidate section", symbol.ID)
		}
		if symbol.SDKPublicationFileID != "" && symbol.SDKPublicationFileID != section.SDKPublicationFileID {
			return fmt.Errorf("SDK symbol %s names a file outside its section ancestry", symbol.ID)
		}
	}

	samples := make(map[string]bool, len(value.Samples))
	for _, sample := range value.Samples {
		if sample.ID == "" || sample.DeploymentID != value.Candidate.DeploymentID ||
			sample.SDKContentCandidateID != value.Candidate.ID || samples[sample.ID] {
			return errors.New("SDK candidate contains an inconsistent sample")
		}
		samples[sample.ID] = true
		if sample.SDKPublicationFileID != "" {
			if _, exists := files[sample.SDKPublicationFileID]; !exists {
				return fmt.Errorf("SDK sample %s references an unknown candidate file", sample.ID)
			}
		}
		if sample.SDKSectionID == "" {
			continue
		}
		section, exists := sections[sample.SDKSectionID]
		if !exists {
			return fmt.Errorf("SDK sample %s references an unknown candidate section", sample.ID)
		}
		if sample.SDKPublicationFileID != "" && sample.SDKPublicationFileID != section.SDKPublicationFileID {
			return fmt.Errorf("SDK sample %s names a file outside its section ancestry", sample.ID)
		}
	}

	if value.Map != nil && (value.Map.DeploymentID != value.Candidate.DeploymentID || value.Map.SDKContentCandidateID != value.Candidate.ID) {
		return errors.New("SDK candidate contains an inconsistent map")
	}
	for _, reference := range value.SampleRefs {
		if reference.DeploymentID != value.Candidate.DeploymentID || reference.SDKContentCandidateID != value.Candidate.ID || !samples[reference.SDKCodeSampleID] {
			return errors.New("SDK candidate contains an inconsistent sample API reference")
		}
	}
	return nil
}
