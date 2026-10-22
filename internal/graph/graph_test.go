package graph

import (
	"strings"
	"testing"

	"github.com/nzhussup/fleck/internal/model"
)

func TestBuildCreatesTypedRelationshipsAndStableDOT(t *testing.T) {
	f := &model.Facts{
		ApplicationID: "app",
		Layers:        model.LayerAvailability{Application: true, Deployment: true},
		EnvReads:      []model.EnvRead{{ID: "read", Name: "DB", Resolved: true}},
		Workload:      model.Workload{ID: "workload", Kind: "Deployment", Name: "app"},
		Container: model.Container{ID: "container", Name: "app", Supplies: []model.EnvSupply{
			{ID: "supply", Name: "DB", State: model.SupplyDefinite, SourceKind: model.SupplySecret, Reference: "default/database:password", Sensitive: true, Value: "must-not-leak"},
		}},
	}
	g := Build(f)
	kinds := map[EdgeKind]bool{}
	for _, edges := range g.Outgoing {
		for _, edge := range edges {
			kinds[edge.Kind] = true
		}
	}
	for _, kind := range []EdgeKind{EdgeRuns, EdgeReads, EdgeSupplies, EdgeMatches, EdgeReferences, EdgeContains} {
		if !kinds[kind] {
			t.Errorf("missing edge %s", kind)
		}
	}
	first, second := g.DOT(), g.DOT()
	if first != second {
		t.Fatal("DOT output is not deterministic")
	}
	if strings.Contains(first, "must-not-leak") {
		t.Fatal("DOT leaked secret value")
	}
}

func TestEvidencePathUsesVerifiedRelationships(t *testing.T) {
	g := New()
	for _, id := range []string{"service", "workload", "container", "application", "listener", "unresolved"} {
		g.addNode(id, NodeApplication, id)
	}
	g.addEdge("service", "workload", EdgeSelects)
	g.addEdge("workload", "container", EdgeContains)
	g.addEdge("application", "container", EdgeRuns)
	g.addEdge("application", "listener", EdgeListens)
	g.addEdgeState("service", "unresolved", EdgeReferences, RelationshipUnresolved)
	path := g.EvidencePath("service", "listener")
	want := []NodeID{"service", "workload", "container", "application", "listener"}
	if len(path) != len(want) {
		t.Fatalf("path=%v", path)
	}
	for index := range want {
		if path[index] != want[index] {
			t.Fatalf("path=%v, want %v", path, want)
		}
	}
}

func TestBuildPreservesContainerAndServicePortOwnership(t *testing.T) {
	app := model.Container{ID: "deployment:default/app/container/app", Name: "app", Ports: []model.ContainerPort{{Name: "http", Port: 8080, Protocol: "TCP"}}}
	sidecar := model.Container{ID: "deployment:default/app/container/metrics", Name: "metrics", Ports: []model.ContainerPort{{Name: "metrics", Port: 9090, Protocol: "TCP"}}}
	facts := &model.Facts{
		Layers:    model.LayerAvailability{Deployment: true},
		Workload:  model.Workload{ID: "deployment:default/app", Kind: "Deployment", Name: "app", Containers: []model.Container{app, sidecar}},
		Container: app,
		Services:  []model.Service{{ID: "service:default/metrics", Name: "metrics", Ports: []model.ServicePort{{Port: 80, TargetPortName: "metrics", TargetPort: 9090, TargetResolved: true, TargetContainer: sidecar.ID, Protocol: "TCP"}}}},
	}
	g := Build(facts)
	servicePort := NodeID("serviceport:service:default/metrics:0")
	sidecarPort := NodeID("containerport:" + sidecar.ID + ":0")
	if g.Nodes[sidecarPort].Kind != NodeContainerPort {
		t.Fatalf("sidecar port node missing: %#v", g.Nodes)
	}
	wantEdges := map[EdgeKind]bool{EdgeContains: false, EdgeRoutes: false}
	for _, edge := range g.Outgoing[NodeID("service:default/metrics")] {
		if edge.To == servicePort && edge.Kind == EdgeContains {
			wantEdges[EdgeContains] = true
		}
	}
	for _, edge := range g.Outgoing[servicePort] {
		if edge.To == sidecarPort && edge.Kind == EdgeRoutes {
			wantEdges[EdgeRoutes] = true
		}
	}
	for kind, found := range wantEdges {
		if !found {
			t.Errorf("missing %s relationship", kind)
		}
	}
	for _, edges := range g.Outgoing {
		for _, edge := range edges {
			if _, ok := g.Nodes[edge.From]; !ok {
				t.Errorf("edge source %s does not exist", edge.From)
			}
			if _, ok := g.Nodes[edge.To]; !ok {
				t.Errorf("edge target %s does not exist", edge.To)
			}
		}
	}
}
