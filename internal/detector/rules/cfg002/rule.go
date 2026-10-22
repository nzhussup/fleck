package cfg002

import (
	"context"
	"fmt"
	"strings"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/model"
)

const DefaultThreshold = 0.8

type rule struct{ threshold float64 }

func New(threshold float64) detector.Detector { return rule{threshold: threshold} }

func (rule) Metadata() detector.Metadata {
	return detector.Metadata{ID: "CFG002", Name: "Probable environment-variable naming mismatch", Description: "Application and supplied variable names are probably inconsistent", Severity: model.SeverityWarning}
}

func (rule) Requires() []model.Capability {
	return []model.Capability{model.CapabilityEnvReads, model.CapabilityEnvSupplies}
}

func (current rule) Detect(_ context.Context, analysis *detector.AnalysisContext) ([]model.Finding, error) {
	supplied := map[string]model.EnvSupply{}
	declared := map[string]bool{}
	for _, supply := range analysis.Queries.EffectiveSupplies() {
		if supply.Name != "" && supply.State != model.SupplyMissingMandatory && supply.State != model.SupplyMissingOptional {
			declared[supply.Name] = true
		}
		if supply.Name != "" && supply.State == model.SupplyDefinite {
			supplied[supply.Name] = supply
		}
	}

	matched := map[string]bool{}
	var findings []model.Finding
	for _, read := range analysis.Queries.EnvironmentReads() {
		if !read.Resolved {
			continue
		}
		if declared[read.Name] {
			continue
		}

		bestName, bestScore := bestCandidate(read.Name, supplied, matched)
		if bestScore < current.threshold {
			continue
		}
		matched[bestName] = true

		metadata := current.Metadata()
		candidate := supplied[bestName]
		findings = append(findings, model.Finding{
			RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity,
			Subject:  read.Name,
			Message:  fmt.Sprintf("Application expects %s, but configuration supplies %s (possible naming mismatch)", read.Name, bestName),
			Location: read.Source, Related: []model.SourceLocation{candidate.Location},
			Evidence:   []model.Evidence{{Kind: "string-similarity", Description: fmt.Sprintf("normalized similarity %.2f", bestScore)}},
			Suggestion: "Verify the environment-variable names in source and deployment configuration.",
		})
	}
	return findings, nil
}

func bestCandidate(expected string, supplied map[string]model.EnvSupply, matched map[string]bool) (string, float64) {
	bestName := ""
	bestScore := 0.0
	for name := range supplied {
		if matched[name] || name == expected {
			continue
		}
		score := similarity(expected, name)
		if score > bestScore || (score == bestScore && name < bestName) {
			bestName, bestScore = name, score
		}
	}
	return bestName, bestScore
}

func similarity(a, b string) float64 {
	a, b = normalizeEnvironmentName(a), normalizeEnvironmentName(b)
	if a == b {
		return 1
	}
	longest := max(len(a), len(b))
	if longest == 0 {
		return 1
	}
	return 1 - float64(levenshtein(a, b))/float64(longest)
}

func normalizeEnvironmentName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(name, "-", "_"), "__", "_"))
}

func levenshtein(a, b string) int {
	previous := make([]int, len(b)+1)
	for index := range previous {
		previous[index] = index
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}
