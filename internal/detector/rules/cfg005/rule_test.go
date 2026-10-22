package cfg005

import (
	"context"
	"testing"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
)

func TestDetectsCautiousDockerAndServicePortDifferences(t *testing.T) {
	facts := &model.Facts{
		Capabilities: model.CapabilitySet{model.CapabilityContainerPorts: true, model.CapabilityDockerPorts: true},
		Container:    model.Container{Name: "app", Ports: []model.ContainerPort{{Port: 8080, Protocol: "TCP"}}},
		Docker:       model.DockerFacts{ExposedPorts: []int{9000}},
		Services:     []model.Service{{Name: "app", Ports: []model.ServicePort{{TargetPort: 3000, TargetResolved: true, Protocol: "TCP"}}}},
	}
	queries := graph.Queries{Facts: facts}
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: queries})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings=%#v, want Docker and Service differences", findings)
	}
	for _, finding := range findings {
		if finding.Severity != model.SeverityWarning {
			t.Fatalf("finding severity=%s, want warning", finding.Severity)
		}
	}
}

func TestNoFindingWithoutConcreteContainerPortDeclarations(t *testing.T) {
	facts := &model.Facts{Capabilities: model.CapabilitySet{model.CapabilityContainerPorts: true, model.CapabilityDockerPorts: true}, Docker: model.DockerFacts{ExposedPorts: []int{8080}}}
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: graph.Queries{Facts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings=%#v, want none", findings)
	}
}
