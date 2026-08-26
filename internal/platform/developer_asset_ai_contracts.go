package platform

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
)

var ErrDeveloperAssetAIAdvisoryInvalid = errors.New("developer-asset AI advisory is invalid")

const developerAssetMapAdvisorySchemaTemplate = `{
  "type":"object","additionalProperties":false,
  "properties":{
    "status":{"type":"string","enum":["ready","uncertain"]},
    "entries":{"type":"array","maxItems":50,"items":{"type":"object","additionalProperties":false,"properties":{
      "kind":{"type":"string","enum":[%s]},
      "title":{"type":"string","minLength":1,"maxLength":120},
      "summary":{"type":"string","minLength":1,"maxLength":320},
      "evidence_ids":{"type":"array","minItems":1,"maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":200}}
    },"required":["kind","title","summary","evidence_ids"]}},
    "gaps":{"type":"array","maxItems":20,"items":{"type":"object","additionalProperties":false,"properties":{
      "code":{"type":"string","enum":["missing_evidence","ambiguous_evidence","conflicting_evidence","version_boundary","truncated_evidence"]},
      "evidence_ids":{"type":"array","maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":200}}
    },"required":["code","evidence_ids"]}}
  },"required":["status","entries","gaps"]
}`

var (
	documentationMapAdvisorySchema = json.RawMessage(strings.Replace(developerAssetMapAdvisorySchemaTemplate, "%s", `"topic","workflow","authentication","error","example","version","language"`, 1))
	sdkMapAdvisorySchema           = json.RawMessage(strings.Replace(developerAssetMapAdvisorySchemaTemplate, "%s", `"installation","initialization","authentication","module","symbol","workflow","sample","error","pagination","retry","webhook","deprecation","migration"`, 1))
	sdkApplicabilityAdvisorySchema = json.RawMessage(`{
  "type":"object","additionalProperties":false,
  "properties":{
    "status":{"type":"string","enum":["suggested","uncertain","unsupported"]},
    "coverage":{"type":"string","enum":["partial","unknown"]},
    "selectors":{"type":"array","maxItems":50,"items":{"type":"object","additionalProperties":false,"properties":{
      "kind":{"type":"string","enum":["module","operation","sample"]},
      "value":{"type":"string","minLength":1,"maxLength":240},
      "evidence_ids":{"type":"array","minItems":1,"maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":200}}
    },"required":["kind","value","evidence_ids"]}},
    "gaps":{"type":"array","maxItems":20,"items":{"type":"object","additionalProperties":false,"properties":{
      "code":{"type":"string","enum":["missing_evidence","ambiguous_evidence","conflicting_evidence","version_boundary","truncated_evidence"]},
      "evidence_ids":{"type":"array","maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":200}}
    },"required":["code","evidence_ids"]}}
  },"required":["status","coverage","selectors","gaps"]
}`)
	sdkSampleReviewAdvisorySchema = json.RawMessage(`{
  "type":"object","additionalProperties":false,
  "properties":{
    "recommendation":{"type":"string","enum":["pass","revise","uncertain"]},
    "findings":{"type":"array","maxItems":30,"items":{"type":"object","additionalProperties":false,"properties":{
      "code":{"type":"string","enum":["version_mismatch","unsupported_import","unsupported_symbol","unsupported_operation","unsupported_field","authentication_assumption","missing_prerequisite","unobservable_result","unsafe_placeholder","secret_like_content","destructive_behavior","hidden_network_assumption","cross_api_evidence","mixed_release","uncited_claim","insufficient_evidence"]},
      "evidence_ids":{"type":"array","minItems":1,"maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":200}}
    },"required":["code","evidence_ids"]}}
  },"required":["recommendation","findings"]
}`)
)

type developerAssetAIMapEntry struct {
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type developerAssetAIGap struct {
	Code        string   `json:"code"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type developerAssetAIMapResult struct {
	Status  string                     `json:"status"`
	Entries []developerAssetAIMapEntry `json:"entries"`
	Gaps    []developerAssetAIGap      `json:"gaps"`
}

type developerAssetAISelector struct {
	Kind        string   `json:"kind"`
	Value       string   `json:"value"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type developerAssetAIApplicabilityResult struct {
	Status    string                     `json:"status"`
	Coverage  string                     `json:"coverage"`
	Selectors []developerAssetAISelector `json:"selectors"`
	Gaps      []developerAssetAIGap      `json:"gaps"`
}

type developerAssetAIFinding struct {
	Code        string   `json:"code"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type developerAssetAISampleReviewResult struct {
	Recommendation string                    `json:"recommendation"`
	Findings       []developerAssetAIFinding `json:"findings"`
}

var developerAssetAIGapCodes = map[string]bool{
	"missing_evidence": true, "ambiguous_evidence": true, "conflicting_evidence": true,
	"version_boundary": true, "truncated_evidence": true,
}

func safeDeveloperAssetAIText(value string, maximum int) bool {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maximum || containsAISecretText(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validateDeveloperAssetAIEvidenceIDs(values []string, allowed map[string]bool, allowEmpty bool) ([]string, bool) {
	if len(values) == 0 {
		return []string{}, allowEmpty
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	for index, value := range result {
		value = strings.TrimSpace(value)
		if value == "" || !allowed[value] || index > 0 && result[index-1] == value {
			return nil, false
		}
		result[index] = value
	}
	return result, true
}

func validateDeveloperAssetAIGaps(values []developerAssetAIGap, allowed map[string]bool) ([]developerAssetAIGap, bool) {
	seen := make(map[string]bool)
	result := append([]developerAssetAIGap(nil), values...)
	for index := range result {
		if !developerAssetAIGapCodes[result[index].Code] || seen[result[index].Code] {
			return nil, false
		}
		seen[result[index].Code] = true
		ids, valid := validateDeveloperAssetAIEvidenceIDs(result[index].EvidenceIDs, allowed, true)
		if !valid {
			return nil, false
		}
		result[index].EvidenceIDs = ids
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result, true
}

func validateDeveloperAssetAIMapResult(raw json.RawMessage, allowed map[string]bool, allowedKinds map[string]bool) (json.RawMessage, error) {
	var result developerAssetAIMapResult
	if err := decodeStrictAIResult(raw, &result); err != nil {
		return nil, ErrDeveloperAssetAIAdvisoryInvalid
	}
	if result.Status != "ready" && result.Status != "uncertain" || result.Status == "ready" && len(result.Entries) == 0 || result.Status == "uncertain" && len(result.Gaps) == 0 {
		return nil, ErrDeveloperAssetAIAdvisoryInvalid
	}
	seen := make(map[string]bool)
	for index := range result.Entries {
		entry := &result.Entries[index]
		entry.Title, entry.Summary = strings.TrimSpace(entry.Title), strings.TrimSpace(entry.Summary)
		if !allowedKinds[entry.Kind] || !safeDeveloperAssetAIText(entry.Title, 120) || !safeDeveloperAssetAIText(entry.Summary, 320) {
			return nil, ErrDeveloperAssetAIAdvisoryInvalid
		}
		ids, valid := validateDeveloperAssetAIEvidenceIDs(entry.EvidenceIDs, allowed, false)
		if !valid {
			return nil, ErrDeveloperAssetAIAdvisoryInvalid
		}
		entry.EvidenceIDs = ids
		key := entry.Kind + "\x00" + strings.ToLower(entry.Title) + "\x00" + strings.Join(ids, "\x00")
		if seen[key] {
			return nil, ErrDeveloperAssetAIAdvisoryInvalid
		}
		seen[key] = true
	}
	gaps, valid := validateDeveloperAssetAIGaps(result.Gaps, allowed)
	if !valid {
		return nil, ErrDeveloperAssetAIAdvisoryInvalid
	}
	result.Gaps = gaps
	sort.Slice(result.Entries, func(i, j int) bool {
		if result.Entries[i].Kind == result.Entries[j].Kind {
			return result.Entries[i].Title < result.Entries[j].Title
		}
		return result.Entries[i].Kind < result.Entries[j].Kind
	})
	return json.Marshal(result)
}

func validateDeveloperAssetAIApplicabilityResult(raw json.RawMessage, allowedEvidence map[string]bool, allowedSelectors map[string]map[string]bool) (json.RawMessage, error) {
	var result developerAssetAIApplicabilityResult
	if err := decodeStrictAIResult(raw, &result); err != nil {
		return nil, ErrDeveloperAssetAIAdvisoryInvalid
	}
	if result.Status != "suggested" && result.Status != "uncertain" && result.Status != "unsupported" ||
		result.Coverage != "partial" && result.Coverage != "unknown" ||
		result.Status == "suggested" && (result.Coverage != "partial" || len(result.Selectors) == 0) ||
		result.Status != "suggested" && (result.Coverage != "unknown" || len(result.Selectors) != 0) ||
		result.Status != "suggested" && len(result.Gaps) == 0 {
		return nil, ErrDeveloperAssetAIAdvisoryInvalid
	}
	seen := make(map[string]bool)
	for index := range result.Selectors {
		selector := &result.Selectors[index]
		selector.Value = strings.TrimSpace(selector.Value)
		if !safeDeveloperAssetAIText(selector.Value, 240) || !allowedSelectors[selector.Kind][selector.Value] {
			return nil, ErrDeveloperAssetAIAdvisoryInvalid
		}
		ids, valid := validateDeveloperAssetAIEvidenceIDs(selector.EvidenceIDs, allowedEvidence, false)
		if !valid || seen[selector.Kind+"\x00"+selector.Value] {
			return nil, ErrDeveloperAssetAIAdvisoryInvalid
		}
		selector.EvidenceIDs = ids
		seen[selector.Kind+"\x00"+selector.Value] = true
	}
	gaps, valid := validateDeveloperAssetAIGaps(result.Gaps, allowedEvidence)
	if !valid {
		return nil, ErrDeveloperAssetAIAdvisoryInvalid
	}
	result.Gaps = gaps
	sort.Slice(result.Selectors, func(i, j int) bool {
		if result.Selectors[i].Kind == result.Selectors[j].Kind {
			return result.Selectors[i].Value < result.Selectors[j].Value
		}
		return result.Selectors[i].Kind < result.Selectors[j].Kind
	})
	return json.Marshal(result)
}

func validateDeveloperAssetAISampleReviewResult(raw json.RawMessage, allowedEvidence map[string]bool) (json.RawMessage, error) {
	var result developerAssetAISampleReviewResult
	if err := decodeStrictAIResult(raw, &result); err != nil {
		return nil, ErrDeveloperAssetAIAdvisoryInvalid
	}
	allowedCodes := map[string]bool{
		"version_mismatch": true, "unsupported_import": true, "unsupported_symbol": true,
		"unsupported_operation": true, "unsupported_field": true, "authentication_assumption": true,
		"missing_prerequisite": true, "unobservable_result": true, "unsafe_placeholder": true,
		"secret_like_content": true, "destructive_behavior": true, "hidden_network_assumption": true,
		"cross_api_evidence": true, "mixed_release": true, "uncited_claim": true, "insufficient_evidence": true,
	}
	if result.Recommendation != "pass" && result.Recommendation != "revise" && result.Recommendation != "uncertain" ||
		result.Recommendation == "pass" && len(result.Findings) != 0 ||
		result.Recommendation != "pass" && len(result.Findings) == 0 {
		return nil, ErrDeveloperAssetAIAdvisoryInvalid
	}
	seen := make(map[string]bool)
	for index := range result.Findings {
		finding := &result.Findings[index]
		ids, valid := validateDeveloperAssetAIEvidenceIDs(finding.EvidenceIDs, allowedEvidence, false)
		if !allowedCodes[finding.Code] || !valid || seen[finding.Code] {
			return nil, ErrDeveloperAssetAIAdvisoryInvalid
		}
		finding.EvidenceIDs = ids
		seen[finding.Code] = true
	}
	if result.Recommendation == "uncertain" && !seen["insufficient_evidence"] {
		return nil, ErrDeveloperAssetAIAdvisoryInvalid
	}
	sort.Slice(result.Findings, func(i, j int) bool { return result.Findings[i].Code < result.Findings[j].Code })
	return json.Marshal(result)
}
