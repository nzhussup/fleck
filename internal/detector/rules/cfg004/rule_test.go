package cfg004

import (
	"context"
	"testing"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		reads []model.EnvRead
		want  int
	}{
		{name: "unmatched supply", want: 1},
		{name: "matching read", reads: []model.EnvRead{{ID: "read", Name: "LEGACY_KEY", Resolved: true}}, want: 0},
		{name: "dynamic read suppresses heuristic", reads: []model.EnvRead{{ID: "read", Resolved: false}}, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := &model.Facts{EnvReads: test.reads, Container: model.Container{ID: "container", Name: "app", Supplies: []model.EnvSupply{{ID: "supply", Name: "LEGACY_KEY", State: model.SupplyDefinite}}}}
			built := graph.Build(facts)
			analysis := &detector.AnalysisContext{Facts: facts, Graph: built, Queries: graph.Queries{Facts: facts, Graph: built}}
			findings, err := New().Detect(context.Background(), analysis)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != test.want {
				t.Fatalf("got %d findings, want %d", len(findings), test.want)
			}
		})
	}
}
