package kubernetes

import (
	"github.com/nzhussup/fleck/internal/model"
	"testing"
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
}
