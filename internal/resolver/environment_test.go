package resolver

import (
	"testing"

	"github.com/nzhussup/fleck/internal/model"
)

func TestResolveEnvironmentPrecedenceAndState(t *testing.T) {
	docker := func(value string) model.EnvSupply {
		return model.EnvSupply{ID: "docker", Name: "APP_PORT", Value: value, ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyDockerfile}
	}
	kube := func(id, value string) model.EnvSupply {
		return model.EnvSupply{ID: id, Name: "APP_PORT", Value: value, ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyKubernetes}
	}
	tests := []struct {
		name     string
		supplies []model.EnvSupply
		status   EnvironmentStatus
		value    string
		known    bool
		sourceID string
	}{
		{name: "Docker value", supplies: []model.EnvSupply{docker("8080")}, status: EnvironmentResolved, value: "8080", known: true, sourceID: "docker"},
		{name: "Kubernetes literal overrides Docker", supplies: []model.EnvSupply{docker("8080"), kube("literal", "9090")}, status: EnvironmentResolved, value: "9090", known: true, sourceID: "literal"},
		{name: "ConfigMap overrides Docker", supplies: []model.EnvSupply{docker("8080"), {ID: "configmap", Name: "APP_PORT", Value: "7070", ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyConfigMap}}, status: EnvironmentResolved, value: "7070", known: true, sourceID: "configmap"},
		{name: "unverified exact shadows Docker", supplies: []model.EnvSupply{docker("8080"), {ID: "external", Name: "APP_PORT", State: model.SupplyUnverified, SourceKind: model.SupplyConfigMap}}, status: EnvironmentUnverified, sourceID: "external"},
		{name: "unverified envFrom shadows Docker", supplies: []model.EnvSupply{docker("8080"), {ID: "external", State: model.SupplyUnverified, SourceKind: model.SupplyConfigMap}}, status: EnvironmentUnverified, sourceID: "external"},
		{name: "mandatory missing is invalid", supplies: []model.EnvSupply{docker("8080"), {ID: "missing", Name: "APP_PORT", State: model.SupplyMissingMandatory, SourceKind: model.SupplyConfigMap}}, status: EnvironmentInvalid, sourceID: "missing"},
		{name: "Secret establishes presence without value", supplies: []model.EnvSupply{docker("8080"), {ID: "secret", Name: "APP_PORT", State: model.SupplyDefinite, SourceKind: model.SupplySecret, Sensitive: true}}, status: EnvironmentResolved, sourceID: "secret"},
		{name: "optional missing preserves Docker", supplies: []model.EnvSupply{docker("8080"), {ID: "optional", Name: "APP_PORT", State: model.SupplyMissingOptional, SourceKind: model.SupplyConfigMap, Optional: true}}, status: EnvironmentResolved, value: "8080", known: true, sourceID: "docker"},
		{name: "later declaration wins", supplies: []model.EnvSupply{kube("first", "8080"), kube("second", "9090")}, status: EnvironmentResolved, value: "9090", known: true, sourceID: "second"},
		{name: "explicit declaration resolves earlier wildcard", supplies: []model.EnvSupply{{ID: "external", State: model.SupplyUnverified, SourceKind: model.SupplyConfigMap}, kube("explicit", "6060")}, status: EnvironmentResolved, value: "6060", known: true, sourceID: "explicit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Resolve(test.supplies, "APP_PORT")
			if got.Status != test.status || got.Value != test.value || got.ValueKnown != test.known || got.Source.ID != test.sourceID {
				t.Fatalf("resolution=%#v", got)
			}
		})
	}
}

func TestResolveAllIsDeterministic(t *testing.T) {
	supplies := []model.EnvSupply{
		{ID: "b", Name: "B", State: model.SupplyDefinite},
		{ID: "a", Name: "A", State: model.SupplyDefinite},
	}
	got := ResolveAll(supplies)
	if len(got) != 2 || got[0].Name != "A" || got[1].Name != "B" {
		t.Fatalf("resolutions=%#v", got)
	}
}
