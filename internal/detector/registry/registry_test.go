package registry_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/detector/registry"
	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
)

func TestAllRulesAgainstSharedFacts(t *testing.T) {
	analysis := analysisContext(fixtureFacts())
	want := map[string]int{"CFG001": 1, "CFG002": 1, "CFG003": 1, "CFG004": 2}
	for _, rule := range registry.All() {
		findings, err := rule.Detect(context.Background(), analysis)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != want[rule.Metadata().ID] {
			t.Errorf("%s returned %d findings, want %d: %#v", rule.Metadata().ID, len(findings), want[rule.Metadata().ID], findings)
		}
	}
}

func TestMetadataUsesUniqueStableIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, metadata := range registry.Metadata() {
		if metadata.ID == "" || seen[metadata.ID] {
			t.Fatalf("invalid or duplicate rule ID %q", metadata.ID)
		}
		seen[metadata.ID] = true
	}
	if len(seen) != 4 {
		t.Fatalf("registered %d rules, want 4", len(seen))
	}
}

func TestCFG001RespectsOptionalUnknownAndUnverifiedReads(t *testing.T) {
	facts := fixtureFacts()
	facts.EnvReads = append(facts.EnvReads,
		model.EnvRead{ID: "optional", Name: "OPTIONAL", Resolved: true, Requirement: model.RequirementOptional},
		model.EnvRead{ID: "unknown", Name: "UNKNOWN", Resolved: true, Requirement: model.RequirementUnknown},
		model.EnvRead{ID: "external", Name: "EXTERNAL", Resolved: true, Requirement: model.RequirementRequired},
	)
	facts.Container.Supplies = append(facts.Container.Supplies, model.EnvSupply{ID: "wildcard", State: model.SupplyUnverified})
	findings, err := ruleByID(t, "CFG001").Detect(context.Background(), analysisContext(facts))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("unverified envFrom should suppress definite missing findings, got %#v", findings)
	}
}

func TestCFG004NeverExposesSecretValues(t *testing.T) {
	facts := fixtureFacts()
	facts.Container.Supplies = []model.EnvSupply{{ID: "secret", Name: "UNUSED_SECRET", State: model.SupplyDefinite, SourceKind: model.SupplySecret, Sensitive: true, Value: "super-secret", ValueKnown: false}}
	findings, err := ruleByID(t, "CFG004").Detect(context.Background(), analysisContext(facts))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	if strings.Contains(strings.ToLower(findings[0].Message+findings[0].Suggestion), "super-secret") {
		t.Fatal("secret value leaked")
	}
}

func ruleByID(t *testing.T, id string) detector.Detector {
	t.Helper()
	for _, rule := range registry.All() {
		if rule.Metadata().ID == id {
			return rule
		}
	}
	t.Fatalf("rule %s not registered", id)
	return nil
}

func analysisContext(facts *model.Facts) *detector.AnalysisContext {
	built := graph.Build(facts)
	return &detector.AnalysisContext{Facts: facts, Graph: built, Queries: graph.Queries{Facts: facts, Graph: built}}
}

func fixtureFacts() *model.Facts {
	return &model.Facts{
		ApplicationID: "payment",
		EnvReads:      []model.EnvRead{{ID: "read-db", Name: "DATABASE_URL", Resolved: true, Requirement: model.RequirementRequired, Source: model.SourceLocation{File: "main.go", Line: 10}}},
		ListenPorts:   []model.ListenPort{{ID: "listen", Port: 8080, Resolved: true, Source: model.SourceLocation{File: "main.go", Line: 20}}},
		Workload:      model.Workload{ID: "deployment:default/payment", Kind: "Deployment", Name: "payment", Namespace: "default"},
		Container: model.Container{ID: "container:payment", Name: "payment", Supplies: []model.EnvSupply{
			{ID: "s1", Name: "DATABASES_URL", State: model.SupplyDefinite, SourceKind: model.SupplyKubernetes},
			{ID: "s2", Name: "LEGACY_KEY", State: model.SupplyDefinite, SourceKind: model.SupplyKubernetes},
		}},
		Services: []model.Service{{ID: "service:payment", Name: "payment", Ports: []model.ServicePort{{Port: 80, TargetPort: 3000, TargetResolved: true, Protocol: "TCP"}}}},
	}
}
