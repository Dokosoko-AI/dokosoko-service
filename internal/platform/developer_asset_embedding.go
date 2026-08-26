package platform

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// The local embedding is deliberately deterministic and dependency-free. It
// supplies a stable dense-retrieval fallback when no external embedding model
// is configured; FTS and exact identifiers remain independent signals. The
// version is part of every index generation so a future model change rebuilds
// rather than silently mixing vector spaces.
const (
	developerAssetEmbeddingModel      = "local-feature-hash-v1"
	developerAssetEmbeddingDimensions = 384
)

var developerAssetRetrievalAliases = map[string][]string{
	"auth":           {"authentication", "credential", "token"},
	"authentication": {"auth", "credential", "token"},
	"install":        {"setup", "dependency", "package"},
	"setup":          {"install", "configure", "initialize"},
	"error":          {"failure", "exception", "problem"},
	"pagination":     {"paging", "cursor", "page"},
	"retry":          {"backoff", "resilience", "recovery"},
	"webhook":        {"callback", "event", "notification"},
}

func developerAssetTokens(value string) []string {
	value = strings.ToLower(value)
	fields := strings.FieldsFunc(value, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	result := make([]string, 0, len(fields)*3)
	for index, field := range fields {
		if field == "" {
			continue
		}
		result = append(result, "w:"+field)
		for _, alias := range developerAssetRetrievalAliases[field] {
			result = append(result, "a:"+alias)
		}
		runes := []rune(field)
		if len(runes) >= 3 {
			for start := 0; start+3 <= len(runes); start++ {
				result = append(result, "c:"+string(runes[start:start+3]))
			}
		}
		if index > 0 {
			result = append(result, "b:"+fields[index-1]+"_"+field)
		}
	}
	return result
}

func localDeveloperAssetEmbedding(value string) []float32 {
	vector := make([]float32, developerAssetEmbeddingDimensions)
	for _, token := range developerAssetTokens(value) {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(token))
		value := hash.Sum64()
		index := int(value % developerAssetEmbeddingDimensions)
		weight := float32(1)
		if strings.HasPrefix(token, "w:") {
			weight = 2
		} else if strings.HasPrefix(token, "b:") || strings.HasPrefix(token, "a:") {
			weight = 1.5
		}
		if value&(1<<63) != 0 {
			weight = -weight
		}
		vector[index] += weight
	}
	norm := float64(0)
	for _, value := range vector {
		norm += float64(value * value)
	}
	if norm == 0 {
		return vector
	}
	norm = math.Sqrt(norm)
	for index := range vector {
		vector[index] = float32(float64(vector[index]) / norm)
	}
	return vector
}
