package app

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

func TestDocumentedExamplesRemainRunnable(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		target          string
		wantRules       []string
		wantSkipped     int
		wantDiagnostics int
	}{
		{name: "monolith", path: "examples/monolith"},
		{name: "monorepo orders", path: "examples/microservices-monorepo", target: "orders"},
		{name: "monorepo notifications", path: "examples/microservices-monorepo", target: "notifications"},
		{name: "polyrepo joined target", path: "examples/polyrepo", target: "customer-api-development", wantRules: []string{"CFG003", "CFG004"}, wantDiagnostics: 1},
		{name: "polyrepo second environment", path: "examples/polyrepo", target: "customer-api-production", wantRules: []string{"CFG004"}, wantDiagnostics: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, err := filepath.Abs(filepath.Join("..", "..", test.path))
			if err != nil {
				t.Fatal(err)
			}
			result, err := New().Analyze(context.Background(), target, Options{Target: test.target})
			if err != nil {
				t.Fatal(err)
			}
			gotRules := make([]string, len(result.Findings))
			for index, finding := range result.Findings {
				gotRules[index] = finding.RuleID
			}
			if !slices.Equal(gotRules, test.wantRules) {
				t.Fatalf("rules=%v, want %v", gotRules, test.wantRules)
			}
			if len(result.Skipped) != test.wantSkipped {
				t.Fatalf("skipped=%d, want %d", len(result.Skipped), test.wantSkipped)
			}
			if len(result.Facts.Diagnostics) != test.wantDiagnostics {
				t.Fatalf("diagnostics=%d, want %d", len(result.Facts.Diagnostics), test.wantDiagnostics)
			}
		})
	}
}

func TestMonorepoWorkspaceAnalyzesAllTargets(t *testing.T) {
	target, err := filepath.Abs(filepath.Join("..", "..", "examples", "microservices-monorepo"))
	if err != nil {
		t.Fatal(err)
	}
	results, err := New().AnalyzeAll(context.Background(), target, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Name != "orders" || results[1].Name != "notifications" {
		t.Fatalf("unexpected workspace results: %#v", results)
	}
}

func TestPolyrepoWorkspaceJoinsRepositoriesForBothEnvironments(t *testing.T) {
	target, err := filepath.Abs(filepath.Join("..", "..", "examples", "polyrepo"))
	if err != nil {
		t.Fatal(err)
	}
	results, err := New().AnalyzeAll(context.Background(), target, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Name != "customer-api-development" || results[1].Name != "customer-api-production" {
		t.Fatalf("unexpected workspace results: %#v", results)
	}
}
