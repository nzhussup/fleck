package graph

import (
	"strings"
	"testing"

	"github.com/nzhussup/fleck/internal/model"
)

func TestBuildCreatesTypedRelationshipsAndStableDOT(t *testing.T) {
	f := &model.Facts{
		ApplicationID: "app",
		EnvReads:      []model.EnvRead{{ID: "read", Name: "DB", Resolved: true}},
		Workload:      model.Workload{ID: "workload", Kind: "Deployment", Name: "app"},
		Container: model.Container{ID: "container", Name: "app", Supplies: []model.EnvSupply{
			{ID: "supply", Name: "DB", State: model.SupplyDefinite, Sensitive: true, Value: "must-not-leak"},
		}},
	}
	g := Build(f)
	kinds := map[EdgeKind]bool{}
	for _, edges := range g.Outgoing {
		for _, edge := range edges {
			kinds[edge.Kind] = true
		}
	}
	for _, kind := range []EdgeKind{EdgeRuns, EdgeReads, EdgeSupplies, EdgeReferences} {
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
