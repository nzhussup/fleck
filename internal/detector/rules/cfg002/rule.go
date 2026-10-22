package cfg002

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/model"
	"github.com/nzhussup/fleck/internal/resolver"
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
	for _, resolution := range resolver.ResolveAll(analysis.Facts.Container.Supplies) {
		if resolution.Status != resolver.EnvironmentAbsent {
			declared[resolution.Name] = true
		}
		if resolution.Status == resolver.EnvironmentResolved {
			supplied[resolution.Name] = resolution.Source
		}
	}

	reads := map[string]model.EnvRead{}
	for _, read := range analysis.Queries.EnvironmentReads() {
		if !read.Resolved || declared[read.Name] {
			continue
		}
		if previous, exists := reads[read.Name]; !exists || read.ID < previous.ID {
			reads[read.Name] = read
		}
	}

	type proposal struct {
		read      model.EnvRead
		candidate string
		score     float64
	}
	names := make([]string, 0, len(reads))
	for name := range reads {
		names = append(names, name)
	}
	sort.Strings(names)
	byCandidate := map[string][]proposal{}
	for _, name := range names {
		bestName, bestScore, ambiguous := bestCandidate(name, supplied)
		if ambiguous || bestScore < current.threshold {
			continue
		}
		byCandidate[bestName] = append(byCandidate[bestName], proposal{read: reads[name], candidate: bestName, score: bestScore})
	}

	var selected []proposal
	for _, proposals := range byCandidate {
		sort.Slice(proposals, func(i, j int) bool {
			if proposals[i].score != proposals[j].score {
				return proposals[i].score > proposals[j].score
			}
			return proposals[i].read.Name < proposals[j].read.Name
		})
		if len(proposals) > 1 && proposals[0].score == proposals[1].score {
			// One supplied name is equally plausible for competing reads. Do
			// not manufacture a preferred association.
			continue
		}
		selected = append(selected, proposals[0])
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].read.Name < selected[j].read.Name })

	var findings []model.Finding
	for _, proposal := range selected {
		metadata := current.Metadata()
		candidate := supplied[proposal.candidate]
		score := strconv.FormatFloat(proposal.score, 'f', 4, 64)
		findings = append(findings, model.Finding{
			RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity, Confidence: model.ConfidenceMedium,
			CorrelationKey: "environment:" + analysis.Facts.ApplicationID + ":" + proposal.read.Name,
			Subject:        proposal.read.Name,
			Message:        fmt.Sprintf("Application expects %s, but configuration supplies %s (possible naming mismatch)", proposal.read.Name, proposal.candidate),
			Location:       proposal.read.Source, Related: []model.SourceLocation{candidate.Location},
			Evidence: []model.Evidence{{Kind: "string-similarity", Description: fmt.Sprintf("normalized similarity %.2f", proposal.score), Attributes: map[string]string{
				"expected": proposal.read.Name, "candidate": proposal.candidate, "score": score, "threshold": strconv.FormatFloat(current.threshold, 'f', 4, 64),
			}}},
			Suggestion: "Verify the environment-variable names in source and deployment configuration.",
		})
	}
	return findings, nil
}

func bestCandidate(expected string, supplied map[string]model.EnvSupply) (string, float64, bool) {
	bestName := ""
	bestScore := 0.0
	names := make([]string, 0, len(supplied))
	for name := range supplied {
		names = append(names, name)
	}
	sort.Strings(names)
	ambiguous := false
	for _, name := range names {
		if name == expected {
			continue
		}
		score := similarity(expected, name)
		if score > bestScore {
			bestName, bestScore = name, score
			ambiguous = false
		} else if score == bestScore && score > 0 {
			ambiguous = true
		}
	}
	return bestName, bestScore, ambiguous
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
