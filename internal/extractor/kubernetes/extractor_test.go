package kubernetes

import (
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nzhussup/fleck/internal/model"
)

const manifest = `apiVersion: v1
kind: ConfigMap
metadata: {name: app, namespace: demo}
data: {db-url: postgres, PORT: "8080"}
---
apiVersion: v1
kind: Secret
metadata: {name: secrets, namespace: demo}
data: {token: Zm9v}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: app, namespace: demo}
spec:
  selector: {matchLabels: {app: pay}}
  template:
    metadata: {labels: {app: pay}}
    spec:
      containers:
      - name: sidecar
        env: [{name: DATABASE_URL, value: wrong-container}]
      - name: app
        envFrom: [{prefix: CFG_, configMapRef: {name: app}}]
        env:
        - {name: DATABASE_URL, valueFrom: {configMapKeyRef: {name: app, key: db-url}}}
        - {name: API_TOKEN, valueFrom: {secretKeyRef: {name: secrets, key: token}}}
        ports: [{name: http, containerPort: 8080}]
---
apiVersion: v1
kind: Service
metadata: {name: app, namespace: demo}
spec:
  selector: {app: pay}
  ports: [{port: 80, targetPort: http}]
`

func TestExtractResolvesEnvironmentAndNamedPort(t *testing.T) {
	r, err := Extract([]byte(manifest), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]model.EnvSupply{}
	for _, s := range r.Container.Supplies {
		found[s.Name] = s
	}
	if found["DATABASE_URL"].Value != "postgres" || found["CFG_PORT"].Value != "8080" {
		t.Fatalf("unexpected supplies: %#v", found)
	}
	if found["API_TOKEN"].ValueKnown || !found["API_TOKEN"].Sensitive {
		t.Fatalf("secret value was retained: %#v", found["API_TOKEN"])
	}
	if len(r.Services) != 1 || !r.Services[0].Ports[0].TargetResolved || r.Services[0].Ports[0].TargetPort != 8080 {
		t.Fatalf("named targetPort not resolved: %#v", r.Services)
	}
	for _, s := range r.Container.Supplies {
		if s.Value == "wrong-container" {
			t.Fatal("sidecar supply leaked into selected container")
		}
	}
}

func TestMissingExternalReferenceIsUnverified(t *testing.T) {
	m := `apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec: {template: {metadata: {labels: {app: x}}, spec: {containers: [{name: app, envFrom: [{configMapRef: {name: external}}]}]}}}
`
	r, err := Extract([]byte(m), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Container.Supplies) != 1 || r.Container.Supplies[0].State != model.SupplyUnverified || r.Container.Supplies[0].Name != "" {
		t.Fatalf("unexpected unresolved supply: %#v", r.Container.Supplies)
	}
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Level != "warning" {
		t.Fatalf("missing incomplete-manifest diagnostic: %#v", r.Diagnostics)
	}
}

func TestKnownMissingMandatoryKeyIsInvalidReference(t *testing.T) {
	m := `apiVersion: v1
kind: ConfigMap
metadata: {name: settings}
data: {present: value}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec:
  template:
    spec:
      containers:
        - name: app
          env:
            - name: REQUIRED
              valueFrom: {configMapKeyRef: {name: settings, key: absent}}
`
	r, err := Extract([]byte(m), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Diagnostics) != 0 || len(r.InvalidReferences) != 1 || r.InvalidReferences[0].Subject != "REQUIRED" {
		t.Fatalf("diagnostics=%#v invalidReferences=%#v", r.Diagnostics, r.InvalidReferences)
	}
	if len(r.Container.Supplies) != 1 || r.Container.Supplies[0].State != model.SupplyMissingMandatory {
		t.Fatalf("supplies=%#v", r.Container.Supplies)
	}
}

func TestReportsUnresolvedNamedTargetPort(t *testing.T) {
	m := `apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec: {template: {metadata: {labels: {app: x}}, spec: {containers: [{name: app, ports: [{name: http, containerPort: 8080}]}]}}}
---
apiVersion: v1
kind: Service
metadata: {name: app}
spec: {selector: {app: x}, ports: [{port: 80, targetPort: missing}]}
`
	r, err := Extract([]byte(m), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Diagnostics) != 0 || len(r.InvalidReferences) != 1 || r.InvalidReferences[0].Kind != "service-target-port" {
		t.Fatalf("diagnostics=%#v invalidReferences=%#v", r.Diagnostics, r.InvalidReferences)
	}
}

func TestReportsSelectorWithNoRepresentedWorkloadAsUncertain(t *testing.T) {
	m := `apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec: {template: {metadata: {labels: {app: x}}, spec: {containers: [{name: app}]}}}
---
apiVersion: v1
kind: Service
metadata: {name: orphan}
spec: {selector: {app: absent}, ports: [{port: 80}]}
`
	r, err := Extract([]byte(m), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Level != "warning" {
		t.Fatalf("diagnostics=%#v, want incomplete-set warning", r.Diagnostics)
	}
}

func TestWorkloadNamespaceDisambiguatesResources(t *testing.T) {
	m := `apiVersion: apps/v1
kind: Deployment
metadata: {name: app, namespace: development}
spec: {template: {spec: {containers: [{name: app, env: [{name: ENVIRONMENT, value: development}]}]}}}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: app, namespace: production}
spec: {template: {spec: {containers: [{name: app, env: [{name: ENVIRONMENT, value: production}]}]}}}
`
	if _, err := Extract([]byte(m), "Deployment", "app", "app"); err == nil {
		t.Fatal("expected omitted namespace to report an ambiguous workload")
	}
	result, err := ExtractInNamespace([]byte(m), "Deployment", "app", "production", "app")
	if err != nil {
		t.Fatal(err)
	}
	if result.Workload.Namespace != "production" || len(result.Container.Supplies) != 1 || result.Container.Supplies[0].Value != "production" {
		t.Fatalf("result=%#v", result)
	}
}

func TestGeneratedLocationsUseDocumentLinesWhenNoSourceIsAvailable(t *testing.T) {
	result, err := Extract([]byte(manifest), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Container.Supplies) == 0 || result.Container.Supplies[0].Location.File != "<generated Kubernetes manifest>" || result.Container.Supplies[0].Location.Line == 0 {
		t.Fatalf("location=%#v", result.Container.Supplies)
	}
}

func TestHelmSourceMarkerResolvesToChartTemplate(t *testing.T) {
	m := `---
# Source: payment-service/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec:
  template:
    spec:
      containers:
        - name: app
          env: [{name: UNUSED, value: value}]
`
	result, err := ExtractInNamespaceWithOrigins([]byte(m), "Deployment", "app", "", "app", OriginOptions{
		ChartPath: "/workspace/deploy/helm/payment-service",
		RootPath:  "/workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := model.SourceLocation{File: "deploy/helm/payment-service/templates/deployment.yaml"}
	if len(result.Container.Supplies) != 1 || result.Container.Supplies[0].Location != want {
		t.Fatalf("location=%#v, want %#v", result.Container.Supplies, want)
	}
}

func TestNamedPortPreservesOwningContainerAndAmbiguity(t *testing.T) {
	unique := `apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec:
  template:
    metadata: {labels: {app: demo}}
    spec:
      containers:
        - name: app
          ports: [{name: http, containerPort: 8080, protocol: TCP}]
        - name: sidecar
          ports: [{name: metrics, containerPort: 9090, protocol: TCP}]
---
apiVersion: v1
kind: Service
metadata: {name: metrics}
spec: {selector: {app: demo}, ports: [{port: 80, targetPort: metrics, protocol: TCP}]}
`
	result, err := Extract([]byte(unique), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	port := result.Services[0].Ports[0]
	if !port.TargetResolved || port.TargetPort != 9090 || port.TargetContainer != "deployment:default/app/container/sidecar" {
		t.Fatalf("port=%#v", port)
	}

	ambiguous := strings.Replace(unique, "{name: http, containerPort: 8080, protocol: TCP}", "{name: metrics, containerPort: 8080, protocol: TCP}", 1)
	result, err = Extract([]byte(ambiguous), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	port = result.Services[0].Ports[0]
	if port.TargetResolved || !port.TargetAmbiguous || len(result.Diagnostics) != 1 {
		t.Fatalf("port=%#v diagnostics=%#v", port, result.Diagnostics)
	}
}

func TestNamedPortRequiresMatchingProtocol(t *testing.T) {
	m := `apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec:
  template:
    metadata: {labels: {app: demo}}
    spec:
      containers:
        - name: app
          ports: [{name: dns, containerPort: 5353, protocol: TCP}]
---
apiVersion: v1
kind: Service
metadata: {name: dns}
spec: {selector: {app: demo}, ports: [{port: 53, targetPort: dns, protocol: UDP}]}
`
	result, err := Extract([]byte(m), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.InvalidReferences) != 1 || result.Services[0].Ports[0].TargetResolved {
		t.Fatalf("result=%#v", result)
	}
}

func TestReorderingIndependentResourcesPreservesResolvedFacts(t *testing.T) {
	parts := strings.Split(manifest, "---\n")
	reordered := strings.Join([]string{parts[2], parts[0], parts[3], parts[1]}, "---\n")
	first, err := Extract([]byte(manifest), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Extract([]byte(reordered), "Deployment", "app", "app")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func(result Result) []string {
		values := make([]string, 0, len(result.Container.Supplies)+len(result.Services))
		for _, supply := range result.Container.Supplies {
			values = append(values, supply.Name+"|"+string(supply.State)+"|"+supply.Value)
		}
		for _, service := range result.Services {
			for _, port := range service.Ports {
				values = append(values, service.Namespace+"/"+service.Name+"|"+port.TargetPortName+"|"+strconv.Itoa(port.TargetPort))
			}
		}
		sort.Strings(values)
		return values
	}
	left, right := snapshot(first), snapshot(second)
	if strings.Join(left, "\n") != strings.Join(right, "\n") {
		t.Fatalf("facts changed after resource reorder:\n%v\n%v", left, right)
	}
}
