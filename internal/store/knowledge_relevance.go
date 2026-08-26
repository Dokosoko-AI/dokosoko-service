package store

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const (
	maxRelevantKnowledgeResults    = 20
	maxRelevantKnowledgeQueryRunes = 2000
	maxRelevantKnowledgeTerms      = 32
)

var knowledgeSearchStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true,
	"at": true, "be": true, "by": true, "for": true, "from": true,
	"how": true, "in": true, "is": true, "it": true, "of": true,
	"on": true, "or": true, "that": true, "the": true, "this": true,
	"to": true, "with": true,
}

func boundedRelevantKnowledgeLimit(limit int) int {
	if limit < 1 {
		return 0
	}
	if limit > maxRelevantKnowledgeResults {
		return maxRelevantKnowledgeResults
	}
	return limit
}

func boundedRelevantKnowledgeQuery(query string) string {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) <= maxRelevantKnowledgeQueryRunes {
		return query
	}
	runes := []rune(query)
	return strings.TrimSpace(string(runes[:maxRelevantKnowledgeQueryRunes]))
}

func relevantKnowledgeTerms(query string) []string {
	query = boundedRelevantKnowledgeQuery(query)
	fields := strings.FieldsFunc(strings.ToLower(query), func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsDigit(value)
	})
	terms := make([]string, 0, min(len(fields), maxRelevantKnowledgeTerms))
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if field == "" || knowledgeSearchStopWords[field] || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
		if len(terms) == maxRelevantKnowledgeTerms {
			break
		}
	}
	return terms
}

type relevantKnowledgeCandidate struct {
	record model.KnowledgeRecord
	score  int
}

func scoreRelevantKnowledge(record model.KnowledgeRecord, query string, terms []string) int {
	title := strings.ToLower(record.Title)
	body := strings.ToLower(record.Text)
	score := 0
	for _, term := range terms {
		if strings.Contains(title, term) {
			score += 12
		}
		if strings.Contains(body, term) {
			score += 2
		}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query != "" {
		if strings.Contains(title, query) {
			score += 24
		}
		if strings.Contains(body, query) {
			score += 4
		}
	}
	return score
}

// rankRelevantKnowledge returns the best evidence in source-diverse rounds:
// the strongest document from every matching source precedes the second
// document from any source. Weak sources do not consume extra slots once their
// matching documents are exhausted.
func rankRelevantKnowledge(records []model.KnowledgeRecord, query string, limit int) []model.KnowledgeRecord {
	limit = boundedRelevantKnowledgeLimit(limit)
	terms := relevantKnowledgeTerms(query)
	if limit == 0 || len(terms) == 0 {
		return []model.KnowledgeRecord{}
	}

	bySource := make(map[string][]relevantKnowledgeCandidate)
	for _, record := range records {
		score := scoreRelevantKnowledge(record, query, terms)
		if score == 0 {
			continue
		}
		bySource[record.SourceID] = append(bySource[record.SourceID], relevantKnowledgeCandidate{record: record, score: score})
	}
	for sourceID := range bySource {
		sort.Slice(bySource[sourceID], func(i, j int) bool {
			left, right := bySource[sourceID][i], bySource[sourceID][j]
			if left.score != right.score {
				return left.score > right.score
			}
			return left.record.ID < right.record.ID
		})
	}

	result := make([]model.KnowledgeRecord, 0, min(limit, len(records)))
	for sourceRank := 0; len(result) < limit; sourceRank++ {
		round := make([]relevantKnowledgeCandidate, 0, len(bySource))
		for _, candidates := range bySource {
			if sourceRank < len(candidates) {
				round = append(round, candidates[sourceRank])
			}
		}
		if len(round) == 0 {
			break
		}
		sort.Slice(round, func(i, j int) bool {
			if round[i].score != round[j].score {
				return round[i].score > round[j].score
			}
			return round[i].record.ID < round[j].record.ID
		})
		for _, candidate := range round {
			result = append(result, candidate.record)
			if len(result) == limit {
				break
			}
		}
	}
	return result
}
