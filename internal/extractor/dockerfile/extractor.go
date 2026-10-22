package dockerfile

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/linter"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/nzhussup/fleck/internal/model"
)

func Extract(path, containerID string) (model.DockerFacts, error) {
	f, err := os.Open(path)
	if err != nil {
		return model.DockerFacts{}, fmt.Errorf("open Dockerfile: %w", err)
	}
	defer f.Close()
	parsed, err := parser.Parse(f)
	if err != nil {
		return model.DockerFacts{}, fmt.Errorf("parse Dockerfile: %w", err)
	}
	stages, metaArgs, err := instructions.Parse(parsed.AST, linter.New(&linter.Config{}))
	if err != nil {
		return model.DockerFacts{}, fmt.Errorf("parse Dockerfile instructions: %w", err)
	}
	if len(stages) == 0 {
		return model.DockerFacts{}, fmt.Errorf("Dockerfile contains no FROM stage")
	}
	byName := map[string]int{}
	for i, s := range stages {
		if s.Name != "" {
			byName[strings.ToLower(s.Name)] = i
		}
	}
	order := []int{}
	seen := map[int]bool{}
	var addStage func(int)
	addStage = func(i int) {
		if seen[i] {
			return
		}
		seen[i] = true
		if p, ok := byName[strings.ToLower(stages[i].BaseName)]; ok {
			addStage(p)
		}
		order = append(order, i)
	}
	addStage(len(stages) - 1)
	env := map[string]model.EnvSupply{}
	args := map[string]string{}
	for _, command := range metaArgs {
		for _, pair := range command.Args {
			if pair.Value != nil {
				args[pair.Key] = *pair.Value
			}
		}
	}
	var facts model.DockerFacts
	for _, i := range order {
		for _, raw := range stages[i].Commands {
			switch c := raw.(type) {
			case *instructions.ArgCommand:
				for _, pair := range c.Args {
					if pair.Value != nil {
						args[pair.Key] = *pair.Value
					}
				}
			case *instructions.EnvCommand:
				for _, pair := range c.Env {
					v, known := expand(pair.Value, env, args)
					env[pair.Key] = model.EnvSupply{Name: pair.Key, Value: v, ValueKnown: known, State: model.SupplyDefinite, SourceKind: model.SupplyDockerfile, Container: containerID, Location: model.SourceLocation{File: "Dockerfile"}}
				}
			case *instructions.ExposeCommand:
				for _, p := range c.Ports {
					n, _ := strconv.Atoi(strings.Split(p, "/")[0])
					if n > 0 {
						facts.ExposedPorts = append(facts.ExposedPorts, n)
					}
				}
			case *instructions.EntrypointCommand:
				facts.Entrypoint = append([]string(nil), c.CmdLine...)
			case *instructions.CmdCommand:
				facts.Command = append([]string(nil), c.CmdLine...)
			}
		}
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		s := env[k]
		s.ID = fmt.Sprintf("envsupply:%s:%s:docker:%d", containerID, k, i)
		facts.Environment = append(facts.Environment, s)
	}
	sort.Ints(facts.ExposedPorts)
	return facts, nil
}

func expand(v string, env map[string]model.EnvSupply, args map[string]string) (string, bool) {
	known := true
	for {
		start := strings.IndexByte(v, '$')
		if start < 0 {
			break
		}
		end := start + 1
		if end < len(v) && v[end] == '{' {
			close := strings.IndexByte(v[end:], '}')
			if close < 0 {
				return "", false
			}
			name := v[end+1 : end+close]
			replacement, ok := expansionValue(name, env, args)
			if !ok {
				return "", false
			}
			v = v[:start] + replacement + v[end+close+1:]
		} else {
			for end < len(v) && (v[end] == '_' || v[end] >= 'A' && v[end] <= 'Z' || v[end] >= 'a' && v[end] <= 'z' || v[end] >= '0' && v[end] <= '9') {
				end++
			}
			if end == start+1 {
				break
			}
			replacement, ok := expansionValue(v[start+1:end], env, args)
			if !ok {
				return "", false
			}
			v = v[:start] + replacement + v[end:]
		}
	}
	return v, known
}

func expansionValue(name string, env map[string]model.EnvSupply, args map[string]string) (string, bool) {
	if supply, ok := env[name]; ok && supply.ValueKnown {
		return supply.Value, true
	}
	value, ok := args[name]
	return value, ok
}
