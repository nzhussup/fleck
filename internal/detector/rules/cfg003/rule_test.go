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

func TestDoesNotGuessAmongMultipleOrUnresolvedListeners(t *testing.T) {
	tests := []struct {
		name      string
		listeners []model.ListenPort
	}{
		{name: "multiple listeners", listeners: []model.ListenPort{{Port: 8080, Resolved: true}, {Port: 9090, Resolved: true}}},
		{name: "unresolved listener", listeners: []model.ListenPort{{EnvName: "PORT"}}},
		{name: "known plus unresolved listener", listeners: []model.ListenPort{{Port: 8080, Resolved: true}, {EnvName: "METRICS_PORT"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := &model.Facts{ListenPorts: test.listeners, Container: model.Container{ID: "app"}, Services: []model.Service{{Name: "app", Ports: []model.ServicePort{{TargetPort: 3000, TargetResolved: true, Protocol: "TCP"}}}}}
			findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: graph.Queries{Facts: facts}})
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != 0 {
				t.Fatalf("findings=%#v", findings)
			}
		})
	}
}

func TestIgnoresNamedPortOwnedByAnotherContainer(t *testing.T) {
	facts := &model.Facts{
		ListenPorts: []model.ListenPort{{Port: 8080, Resolved: true}},
		Container:   model.Container{ID: "container:app"},
		Services:    []model.Service{{Name: "metrics", Ports: []model.ServicePort{{TargetPort: 9090, TargetResolved: true, TargetPortName: "metrics", TargetContainer: "container:sidecar", Protocol: "TCP"}}}},
	}
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: graph.Queries{Facts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestNumericServiceTargetDoesNotRequireContainerPortMetadata(t *testing.T) {
	facts := &model.Facts{
		ListenPorts: []model.ListenPort{{Port: 8080, Resolved: true}},
		Container:   model.Container{ID: "container:app"},
		Services:    []model.Service{{Name: "app", Ports: []model.ServicePort{{Port: 80, TargetPort: 8080, TargetResolved: true, Protocol: "TCP"}}}},
	}
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: graph.Queries{Facts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestNumericTargetInMultiContainerPodIsNotAttributedToSelectedContainer(t *testing.T) {
	selected := model.Container{ID: "container:app", Name: "app"}
	facts := &model.Facts{
		ListenPorts: []model.ListenPort{{Port: 8080, Resolved: true}},
		Workload:    model.Workload{Containers: []model.Container{selected, {ID: "container:sidecar", Name: "sidecar"}}},
		Container:   selected,
		Services:    []model.Service{{Name: "pod-service", Ports: []model.ServicePort{{Port: 80, TargetPort: 9090, TargetResolved: true, Protocol: "TCP"}}}},
	}
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: graph.Queries{Facts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestFindingCarriesDependencyEvidencePath(t *testing.T) {
	container := model.Container{ID: "deployment:default/app/container/app", Name: "app"}
	facts := &model.Facts{
		ApplicationID: "app",
		Layers:        model.LayerAvailability{Application: true, Deployment: true},
		ListenPorts:   []model.ListenPort{{ID: "listen:app", Port: 8080, Resolved: true}},
		Workload:      model.Workload{ID: "deployment:default/app", Kind: "Deployment", Name: "app", Containers: []model.Container{container}},
		Container:     container,
		Services:      []model.Service{{ID: "service:default/app", Name: "app", Ports: []model.ServicePort{{Port: 80, TargetPort: 3000, TargetResolved: true, Protocol: "TCP"}}}},
	}
	built := graph.Build(facts)
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Graph: built, Queries: graph.Queries{Facts: facts, Graph: built}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || len(findings[0].Evidence) != 1 || len(findings[0].Evidence[0].Path) < 3 {
		t.Fatalf("findings=%#v", findings)
	}
}
