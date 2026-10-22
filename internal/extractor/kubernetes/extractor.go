package kubernetes

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nzhussup/fleck/internal/model"
	"gopkg.in/yaml.v3"
)

type metadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}
type keyRef struct {
	Name     string `yaml:"name"`
	Key      string `yaml:"key"`
	Optional bool   `yaml:"optional"`
}
type envVar struct {
	Name      string `yaml:"name"`
	Value     string `yaml:"value"`
	ValueFrom struct {
		ConfigMap *keyRef `yaml:"configMapKeyRef"`
		Secret    *keyRef `yaml:"secretKeyRef"`
	} `yaml:"valueFrom"`
}
type envFrom struct {
	Prefix    string `yaml:"prefix"`
	ConfigMap *struct {
		Name     string `yaml:"name"`
		Optional bool   `yaml:"optional"`
	} `yaml:"configMapRef"`
	Secret *struct {
		Name     string `yaml:"name"`
		Optional bool   `yaml:"optional"`
	} `yaml:"secretRef"`
}
type container struct {
	Name    string    `yaml:"name"`
	Env     []envVar  `yaml:"env"`
	EnvFrom []envFrom `yaml:"envFrom"`
	Ports   []struct {
		Name          string `yaml:"name"`
		ContainerPort int    `yaml:"containerPort"`
		Protocol      string `yaml:"protocol"`
	} `yaml:"ports"`
}
type document struct {
	Location   model.SourceLocation `yaml:"-"`
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	Metadata   metadata             `yaml:"metadata"`
	Data       map[string]string    `yaml:"data"`
	StringData map[string]string    `yaml:"stringData"`
	Spec       struct {
		Selector map[string]any `yaml:"selector"`
		Template struct {
			Metadata struct {
				Labels map[string]string `yaml:"labels"`
			} `yaml:"metadata"`
			Spec struct {
				Containers []container `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
		Ports []struct {
			Name       string `yaml:"name"`
			Port       int    `yaml:"port"`
			TargetPort any    `yaml:"targetPort"`
			Protocol   string `yaml:"protocol"`
		} `yaml:"ports"`
	} `yaml:"spec"`
}

type Result struct {
	Workload          model.Workload
	Container         model.Container
	Services          []model.Service
	Diagnostics       []model.Diagnostic
	InvalidReferences []model.InvalidReference
}

// OriginOptions tells the extractor how to turn provenance comments in a
// manifest stream into repository paths. Helm emits "# Source:" comments for
// rendered resources. LoadManifests emits "# Fleck-Source:" comments for raw
// YAML files.
type OriginOptions struct {
	ChartPath string
	RootPath  string
}

func Extract(manifest []byte, workloadKind, workloadName, containerName string) (Result, error) {
	return ExtractInNamespace(manifest, workloadKind, workloadName, "", containerName)
}

func ExtractInNamespace(manifest []byte, workloadKind, workloadName, workloadNamespace, containerName string) (Result, error) {
	return ExtractInNamespaceWithOrigins(manifest, workloadKind, workloadName, workloadNamespace, containerName, OriginOptions{})
}

func ExtractInNamespaceWithOrigins(manifest []byte, workloadKind, workloadName, workloadNamespace, containerName string, origins OriginOptions) (Result, error) {
	dec := yaml.NewDecoder(bytes.NewReader(manifest))
	lines := strings.Split(string(manifest), "\n")
	var docs []document
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			return Result{}, fmt.Errorf("parse rendered Kubernetes YAML: %w", err)
		}
		var d document
		if err := node.Decode(&d); err != nil {
			return Result{}, fmt.Errorf("decode rendered Kubernetes YAML: %w", err)
		}
		if len(node.Content) > 0 {
			d.Location = manifestLocation(lines, node.Content[0].Line, origins)
		}
		if d.Kind != "" {
			if d.Metadata.Namespace == "" {
				d.Metadata.Namespace = "default"
			}
			docs = append(docs, d)
		}
	}
	var chosen *document
	for i := range docs {
		if docs[i].Kind == workloadKind && docs[i].Metadata.Name == workloadName && (workloadNamespace == "" || docs[i].Metadata.Namespace == workloadNamespace) {
			if chosen != nil {
				return Result{}, fmt.Errorf("workload %s/%s is ambiguous", workloadKind, workloadName)
			}
			chosen = &docs[i]
		}
	}
	if chosen == nil {
		return Result{}, fmt.Errorf("workload %s/%s not found in rendered manifests", workloadKind, workloadName)
	}
	wid := resourceID(chosen.Kind, chosen.Metadata.Namespace, chosen.Metadata.Name)
	w := model.Workload{ID: wid, Kind: chosen.Kind, Name: chosen.Metadata.Name, Namespace: chosen.Metadata.Namespace, Labels: chosen.Spec.Template.Metadata.Labels}
	var selected *container
	for i := range chosen.Spec.Template.Spec.Containers {
		c := &chosen.Spec.Template.Spec.Containers[i]
		mc := model.Container{ID: wid + "/container/" + c.Name, Name: c.Name, Workload: wid}
		for _, p := range c.Ports {
			proto := p.Protocol
			if proto == "" {
				proto = "TCP"
			}
			mc.Ports = append(mc.Ports, model.ContainerPort{Name: p.Name, Port: p.ContainerPort, Protocol: proto})
		}
		w.Containers = append(w.Containers, mc)
		if c.Name == containerName {
			selected = c
		}
	}
	if selected == nil {
		return Result{}, fmt.Errorf("container %q not found in workload %s/%s", containerName, workloadKind, workloadName)
	}
	cid := wid + "/container/" + containerName
	out := Result{Workload: w, Container: model.Container{ID: cid, Name: containerName, Workload: wid}}
	for _, c := range w.Containers {
		if c.Name == containerName {
			out.Container.Ports = c.Ports
		}
	}
	configMaps := map[string]document{}
	secrets := map[string]document{}
	for _, d := range docs {
		key := d.Metadata.Namespace + "/" + d.Metadata.Name
		switch d.Kind {
		case "ConfigMap":
			configMaps[key] = d
		case "Secret":
			secrets[key] = d
		}
	}
	add := func(s model.EnvSupply) {
		s.ID = fmt.Sprintf("envsupply:%s:%s:%d", cid, s.Name, len(out.Container.Supplies))
		s.Container = cid
		out.Container.Supplies = append(out.Container.Supplies, s)
	}
	// Kubernetes applies envFrom sources before explicit env entries; later values win.
	for _, ef := range selected.EnvFrom {
		if ef.ConfigMap != nil {
			resolveFrom(add, &out, configMaps, chosen.Metadata.Namespace, ef.Prefix, ef.ConfigMap.Name, ef.ConfigMap.Optional, false, chosen.Location)
		} else if ef.Secret != nil {
			resolveFrom(add, &out, secrets, chosen.Metadata.Namespace, ef.Prefix, ef.Secret.Name, ef.Secret.Optional, true, chosen.Location)
		}
	}
	for _, e := range selected.Env {
		if e.ValueFrom.ConfigMap == nil && e.ValueFrom.Secret == nil {
			add(model.EnvSupply{Name: e.Name, Value: e.Value, ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyKubernetes, Location: chosen.Location})
			continue
		}
		if e.ValueFrom.ConfigMap != nil {
			ref := e.ValueFrom.ConfigMap
			resolveKey(add, &out, configMaps, chosen.Metadata.Namespace, e.Name, ref.Name, ref.Key, ref.Optional, false, chosen.Location)
		} else {
			ref := e.ValueFrom.Secret
			resolveKey(add, &out, secrets, chosen.Metadata.Namespace, e.Name, ref.Name, ref.Key, ref.Optional, true, chosen.Location)
		}
	}
	for _, d := range docs {
		if d.Kind != "Service" || d.Metadata.Namespace != chosen.Metadata.Namespace {
			continue
		}
		selector := selectorStrings(d.Spec.Selector)
		if !selectorMatchesAnyWorkload(docs, d.Metadata.Namespace, selector) {
			out.Diagnostics = append(out.Diagnostics, model.Diagnostic{Level: "warning", Message: fmt.Sprintf("Service %s selector matches no workload represented in the analyzed manifests; the manifest set may be incomplete", d.Metadata.Name), Location: d.Location})
		}
		if !selectorMatches(selector, w.Labels) {
			continue
		}
		s := model.Service{ID: resourceID("Service", d.Metadata.Namespace, d.Metadata.Name), Name: d.Metadata.Name, Namespace: d.Metadata.Namespace, Selector: selector}
		for _, p := range d.Spec.Ports {
			sp := model.ServicePort{Name: p.Name, Port: p.Port, Protocol: p.Protocol, Location: d.Location}
			if sp.Protocol == "" {
				sp.Protocol = "TCP"
			}
			switch x := p.TargetPort.(type) {
			case int:
				sp.TargetPort = x
				sp.TargetResolved = x > 0
			case string:
				if n := parseYAMLInt(x); n > 0 {
					sp.TargetPort = n
					sp.TargetResolved = true
				} else {
					sp.TargetPortName = x
					var matches []struct {
						container string
						port      int
					}
					for _, workloadContainer := range w.Containers {
						for _, cp := range workloadContainer.Ports {
							if cp.Name == x && strings.EqualFold(cp.Protocol, sp.Protocol) {
								matches = append(matches, struct {
									container string
									port      int
								}{container: workloadContainer.ID, port: cp.Port})
							}
						}
					}
					switch len(matches) {
					case 0:
						out.InvalidReferences = append(out.InvalidReferences, model.InvalidReference{Kind: "service-target-port", Subject: d.Metadata.Name, Reference: x, Message: fmt.Sprintf("Service %s targetPort %q does not match a port declared by the selected workload", d.Metadata.Name, x), Location: d.Location})
					case 1:
						sp.TargetPort = matches[0].port
						sp.TargetContainer = matches[0].container
						sp.TargetResolved = true
					default:
						sp.TargetAmbiguous = true
						out.Diagnostics = append(out.Diagnostics, model.Diagnostic{Level: "warning", Message: fmt.Sprintf("Service %s targetPort %q matches multiple workload containers", d.Metadata.Name, x), Location: d.Location})
					}
				}
			case nil:
				sp.TargetPort = p.Port
				sp.TargetResolved = true
			}
			s.Ports = append(s.Ports, sp)
		}
		out.Services = append(out.Services, s)
	}
	sort.Slice(out.Services, func(i, j int) bool { return out.Services[i].ID < out.Services[j].ID })
	return out, nil
}

func resolveKey(add func(model.EnvSupply), out *Result, resources map[string]document, ns, envName, resName, key string, optional, sensitive bool, location model.SourceLocation) {
	src := model.SupplyConfigMap
	if sensitive {
		src = model.SupplySecret
	}
	ref := ns + "/" + resName + ":" + key
	d, ok := resources[ns+"/"+resName]
	if !ok {
		add(model.EnvSupply{Name: envName, State: model.SupplyUnverified, SourceKind: src, Reference: ref, Optional: optional, Sensitive: sensitive, Location: location})
		out.Diagnostics = append(out.Diagnostics, model.Diagnostic{Level: "warning", Message: "referenced resource cannot be verified: " + ref, Location: location})
		return
	}
	v, ok := d.Data[key]
	if !ok {
		v, ok = d.StringData[key]
	}
	if !ok {
		state := model.SupplyMissingMandatory
		if optional {
			state = model.SupplyMissingOptional
		}
		add(model.EnvSupply{Name: envName, State: state, SourceKind: src, Reference: ref, Optional: optional, Sensitive: sensitive, Location: location})
		if !optional {
			out.InvalidReferences = append(out.InvalidReferences, model.InvalidReference{Kind: "environment-key", Subject: envName, Reference: ref, Message: "mandatory referenced key is missing: " + ref, Location: location})
		}
		return
	}
	s := model.EnvSupply{Name: envName, State: model.SupplyDefinite, SourceKind: src, Reference: ref, Optional: optional, Sensitive: sensitive, Location: location}
	if !sensitive {
		s.Value = v
		s.ValueKnown = true
	}
	add(s)
}
func resolveFrom(add func(model.EnvSupply), out *Result, resources map[string]document, ns, prefix, name string, optional, sensitive bool, location model.SourceLocation) {
	src := model.SupplyConfigMap
	if sensitive {
		src = model.SupplySecret
	}
	d, ok := resources[ns+"/"+name]
	if !ok {
		add(model.EnvSupply{Name: "", State: model.SupplyUnverified, SourceKind: src, Reference: ns + "/" + name, Optional: optional, Sensitive: sensitive, Location: location})
		out.Diagnostics = append(out.Diagnostics, model.Diagnostic{Level: "warning", Message: "referenced resource cannot be verified: " + ns + "/" + name, Location: location})
		return
	}
	keys := make([]string, 0, len(d.Data)+len(d.StringData))
	for k := range d.Data {
		keys = append(keys, k)
	}
	for k := range d.StringData {
		if _, ok := d.Data[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := model.EnvSupply{Name: prefix + k, State: model.SupplyDefinite, SourceKind: src, Reference: ns + "/" + name + ":" + k, Sensitive: sensitive, Location: location}
		if !sensitive {
			s.Value = d.Data[k]
			if v, ok := d.StringData[k]; ok {
				s.Value = v
			}
			s.ValueKnown = true
		}
		add(s)
	}
}
func selectorMatches(sel, labels map[string]string) bool {
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func selectorMatchesAnyWorkload(documents []document, namespace string, selector map[string]string) bool {
	for _, candidate := range documents {
		if candidate.Kind == "Deployment" && candidate.Metadata.Namespace == namespace && selectorMatches(selector, candidate.Spec.Template.Metadata.Labels) {
			return true
		}
	}
	return false
}

func selectorStrings(raw map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range raw {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}
func resourceID(k, ns, n string) string { return strings.ToLower(k) + ":" + ns + "/" + n }

func manifestLocation(lines []string, rootLine int, origins OriginOptions) model.SourceLocation {
	for index := rootLine - 2; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if strings.HasPrefix(line, "# Fleck-Source:") {
			encoded := strings.TrimSpace(strings.TrimPrefix(line, "# Fleck-Source:"))
			source, err := strconv.Unquote(encoded)
			if err != nil {
				break
			}
			return model.SourceLocation{File: reportPath(source, origins.RootPath), Line: rootLine - (index + 1)}
		}
		if strings.HasPrefix(line, "# Source:") {
			source := strings.TrimSpace(strings.TrimPrefix(line, "# Source:"))
			return model.SourceLocation{File: helmSourcePath(source, origins)}
		}
	}
	return model.SourceLocation{File: "<generated Kubernetes manifest>", Line: rootLine}
}

func helmSourcePath(source string, origins OriginOptions) string {
	if origins.ChartPath == "" {
		return source
	}
	source = filepath.FromSlash(filepath.Clean(source))
	parts := strings.Split(source, string(filepath.Separator))
	var candidate string
	if len(parts) > 1 && parts[0] == filepath.Base(filepath.Clean(origins.ChartPath)) {
		candidate = filepath.Join(origins.ChartPath, filepath.Join(parts[1:]...))
	} else {
		candidate = filepath.Join(origins.ChartPath, "charts", source)
	}
	return reportPath(candidate, origins.RootPath)
}

func reportPath(path, root string) string {
	path = filepath.Clean(path)
	if root != "" && filepath.IsAbs(path) {
		if relative, err := filepath.Rel(root, path); err == nil {
			path = relative
		}
	}
	return filepath.ToSlash(path)
}
func parseYAMLInt(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
