package cfg003

import (
	"context"
	"testing"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		target   int
		resolved bool
		want     int
	}{
		{name: "mismatch", target: 3000, resolved: true, want: 1},
		{name: "matching", target: 8080, resolved: true, want: 0},
		{name: "unresolved target", target: 3000, resolved: false, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := &model.Facts{
				ListenPorts: []model.ListenPort{{ID: "listener", Port: 8080, Resolved: true}},
				Container:   model.Container{ID: "container", Name: "app"},
				Services:    []model.Service{{ID: "service", Name: "app", Ports: []model.ServicePort{{TargetPort: test.target, TargetResolved: test.resolved, Protocol: "TCP"}}}},
			}
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
