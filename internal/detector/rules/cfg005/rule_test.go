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
		Docker:       model.DockerFacts{ExposedPorts: []int{9000}, Location: model.SourceLocation{File: "services/api/Dockerfile", Line: 7}},
		Services:     []model.Service{{Name: "app", Ports: []model.ServicePort{{TargetPort: 3000, TargetResolved: true, Protocol: "TCP"}}}},
	}
	queries := graph.Queries{Facts: facts}
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: queries})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings=%#v, want one metadata-only Docker difference", findings)
	}
	if findings[0].Location != facts.Docker.Location {
		t.Fatalf("location=%#v, want %#v", findings[0].Location, facts.Docker.Location)
	}
	for _, finding := range findings {
		if finding.Severity != model.SeverityWarning {
			t.Fatalf("finding severity=%s, want warning", finding.Severity)
		}
	}
}

func TestDoesNotRequirePortMetadataSetsToMatchExactly(t *testing.T) {
	facts := &model.Facts{
		Capabilities: model.CapabilitySet{model.CapabilityContainerPorts: true, model.CapabilityDockerPorts: true},
		Container:    model.Container{Name: "app", Ports: []model.ContainerPort{{Port: 8080, Protocol: "TCP"}, {Port: 9090, Protocol: "TCP"}}},
		Docker:       model.DockerFacts{ExposedPorts: []int{8080}},
	}
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: graph.Queries{Facts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestSuppressesMetadataDifferenceWhenRuntimeMismatchIsVerified(t *testing.T) {
	container := model.Container{ID: "container:app", Name: "app", Ports: []model.ContainerPort{{Name: "http", Port: 3000, Protocol: "TCP"}}}
	facts := &model.Facts{
		Capabilities: model.CapabilitySet{model.CapabilityContainerPorts: true, model.CapabilityDockerPorts: true},
		ListenPorts:  []model.ListenPort{{Port: 8080, Resolved: true}},
		Workload:     model.Workload{Containers: []model.Container{container}},
		Container:    container,
		Docker:       model.DockerFacts{ExposedPorts: []int{8080}},
		Services:     []model.Service{{Name: "app", Ports: []model.ServicePort{{TargetPortName: "http", TargetPort: 3000, TargetResolved: true, TargetContainer: container.ID, Protocol: "TCP"}}}},
	}
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: graph.Queries{Facts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings=%#v", findings)
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
