package kubernetes

import (
	"bytes"
	"fmt"
	"io"
	"sort"
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
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   metadata          `yaml:"metadata"`
	Data       map[string]string `yaml:"data"`
	StringData map[string]string `yaml:"stringData"`
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
	Workload    model.Workload
	Container   model.Container
	Services    []model.Service
	Diagnostics []model.Diagnostic
}

func Extract(manifest []byte, workloadKind, workloadName, containerName string) (Result, error) {
	dec := yaml.NewDecoder(bytes.NewReader(manifest))
	var docs []document
	for {
		var d document
		err := dec.Decode(&d)
		if err == io.EOF {
			break
		}
		if err != nil {
			return Result{}, fmt.Errorf("parse rendered Kubernetes YAML: %w", err)
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
		if docs[i].Kind == workloadKind && docs[i].Metadata.Name == workloadName {
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
			resolveFrom(add, &out, configMaps, chosen.Metadata.Namespace, ef.Prefix, ef.ConfigMap.Name, ef.ConfigMap.Optional, false)
		} else if ef.Secret != nil {
			resolveFrom(add, &out, secrets, chosen.Metadata.Namespace, ef.Prefix, ef.Secret.Name, ef.Secret.Optional, true)
		}
	}
	for _, e := range selected.Env {
		if e.ValueFrom.ConfigMap == nil && e.ValueFrom.Secret == nil {
			add(model.EnvSupply{Name: e.Name, Value: e.Value, ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyKubernetes, Location: renderedLoc()})
			continue
		}
		if e.ValueFrom.ConfigMap != nil {
			ref := e.ValueFrom.ConfigMap
			resolveKey(add, &out, configMaps, chosen.Metadata.Namespace, e.Name, ref.Name, ref.Key, ref.Optional, false)
		} else {
			ref := e.ValueFrom.Secret
			resolveKey(add, &out, secrets, chosen.Metadata.Namespace, e.Name, ref.Name, ref.Key, ref.Optional, true)
		}
	}
	for _, d := range docs {
		if d.Kind != "Service" || d.Metadata.Namespace != chosen.Metadata.Namespace || !selectorMatches(selectorStrings(d.Spec.Selector), w.Labels) {
			continue
		}
		s := model.Service{ID: resourceID("Service", d.Metadata.Namespace, d.Metadata.Name), Name: d.Metadata.Name, Namespace: d.Metadata.Namespace, Selector: selectorStrings(d.Spec.Selector)}
		for _, p := range d.Spec.Ports {
			sp := model.ServicePort{Name: p.Name, Port: p.Port, Protocol: p.Protocol, Location: renderedLoc()}
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
					for _, cp := range out.Container.Ports {
						if cp.Name == x && strings.EqualFold(cp.Protocol, sp.Protocol) {
							sp.TargetPort = cp.Port
							sp.TargetResolved = true
							break
						}
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
	sort.Slice(out.Container.Supplies, func(i, j int) bool { return out.Container.Supplies[i].ID < out.Container.Supplies[j].ID })
	sort.Slice(out.Services, func(i, j int) bool { return out.Services[i].ID < out.Services[j].ID })
	return out, nil
}

func resolveKey(add func(model.EnvSupply), out *Result, resources map[string]document, ns, envName, resName, key string, optional, sensitive bool) {
	src := model.SupplyConfigMap
	if sensitive {
		src = model.SupplySecret
	}
	ref := ns + "/" + resName + ":" + key
	d, ok := resources[ns+"/"+resName]
	if !ok {
		add(model.EnvSupply{Name: envName, State: model.SupplyUnverified, SourceKind: src, Reference: ref, Optional: optional, Sensitive: sensitive, Location: renderedLoc()})
		out.Diagnostics = append(out.Diagnostics, model.Diagnostic{Level: "warning", Message: "referenced resource cannot be verified: " + ref, Location: renderedLoc()})
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
		add(model.EnvSupply{Name: envName, State: state, SourceKind: src, Reference: ref, Optional: optional, Sensitive: sensitive, Location: renderedLoc()})
		if !optional {
			out.Diagnostics = append(out.Diagnostics, model.Diagnostic{Level: "error", Message: "mandatory referenced key is missing: " + ref, Location: renderedLoc()})
		}
		return
	}
	s := model.EnvSupply{Name: envName, State: model.SupplyDefinite, SourceKind: src, Reference: ref, Optional: optional, Sensitive: sensitive, Location: renderedLoc()}
	if !sensitive {
		s.Value = v
		s.ValueKnown = true
	}
	add(s)
}
func resolveFrom(add func(model.EnvSupply), out *Result, resources map[string]document, ns, prefix, name string, optional, sensitive bool) {
	src := model.SupplyConfigMap
	if sensitive {
		src = model.SupplySecret
	}
	d, ok := resources[ns+"/"+name]
	if !ok {
		add(model.EnvSupply{Name: "", State: model.SupplyUnverified, SourceKind: src, Reference: ns + "/" + name, Optional: optional, Sensitive: sensitive, Location: renderedLoc()})
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
		s := model.EnvSupply{Name: prefix + k, State: model.SupplyDefinite, SourceKind: src, Reference: ns + "/" + name + ":" + k, Sensitive: sensitive, Location: renderedLoc()}
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
func renderedLoc() model.SourceLocation { return model.SourceLocation{File: "<helm-rendered>"} }
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
