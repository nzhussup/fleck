package cfg001

import (
	"context"
	"testing"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name        string
		requirement model.Requirement
		supplied    bool
		want        int
	}{
		{name: "required and absent", requirement: model.RequirementRequired, want: 1},
		{name: "required and supplied", requirement: model.RequirementRequired, supplied: true, want: 0},
		{name: "optional and absent", requirement: model.RequirementOptional, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := &model.Facts{EnvReads: []model.EnvRead{{ID: "read", Name: "DATABASE_URL", Resolved: true, Requirement: test.requirement}}, Container: model.Container{ID: "container", Name: "app"}}
			if test.supplied {
				facts.Container.Supplies = []model.EnvSupply{{ID: "supply", Name: "DATABASE_URL", State: model.SupplyDefinite}}
			}
			findings, err := New().Detect(context.Background(), analysisContext(facts))
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != test.want {
				t.Fatalf("got %d findings, want %d", len(findings), test.want)
			}
		})
	}
}

func analysisContext(facts *model.Facts) *detector.AnalysisContext {
	built := graph.Build(facts)
	return &detector.AnalysisContext{Facts: facts, Graph: built, Queries: graph.Queries{Facts: facts, Graph: built}}
}
