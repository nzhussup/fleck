package resolver

import (
	"sort"

	"github.com/nzhussup/fleck/internal/model"
)

// EnvironmentStatus describes the effective runtime state of one variable.
// Presence and value knowledge are intentionally separate: a Secret-backed
// variable can be resolved and present while its value remains hidden.
type EnvironmentStatus string

const (
	EnvironmentResolved   EnvironmentStatus = "resolved"
	EnvironmentAbsent     EnvironmentStatus = "absent"
	EnvironmentUnverified EnvironmentStatus = "unverified"
	EnvironmentInvalid    EnvironmentStatus = "invalid"
)

type Environment struct {
	Name       string
	Status     EnvironmentStatus
	Value      string
	ValueKnown bool
	Source     model.EnvSupply
}

// Resolve applies container-runtime precedence to the supplied declarations.
// Callers must provide Docker image declarations first and Kubernetes
// declarations in Kubernetes processing order: envFrom entries followed by
// explicit env entries. An unresolved envFrom wildcard may shadow a lower
// declaration; a later explicit env entry can establish the final value.
func Resolve(supplies []model.EnvSupply, name string) Environment {
	result := Environment{Name: name, Status: EnvironmentAbsent}
	invalid := false
	var invalidSource model.EnvSupply
	for _, supply := range supplies {
		if supply.State == model.SupplyMissingMandatory {
			if supply.Name == name || supply.Name == "" {
				invalid = true
				invalidSource = supply
			}
			continue
		}
		if supply.State == model.SupplyMissingOptional {
			// Kubernetes omits an optional missing source. It therefore does
			// not override a value inherited from the image or an earlier source.
			continue
		}
		if supply.Name == "" {
			if supply.State == model.SupplyUnverified {
				result = Environment{Name: name, Status: EnvironmentUnverified, Source: supply}
			}
			continue
		}
		if supply.Name != name {
			continue
		}
		switch supply.State {
		case model.SupplyDefinite:
			result = Environment{Name: name, Status: EnvironmentResolved, Value: supply.Value, ValueKnown: supply.ValueKnown, Source: supply}
		case model.SupplyUnverified:
			result = Environment{Name: name, Status: EnvironmentUnverified, Source: supply}
		}
	}
	if invalid {
		return Environment{Name: name, Status: EnvironmentInvalid, Source: invalidSource}
	}
	return result
}

// ResolveAll returns deterministic results for every explicitly named
// declaration. Wildcard envFrom declarations are evaluated when a particular
// application read is resolved, since their variable names are unknowable.
func ResolveAll(supplies []model.EnvSupply) []Environment {
	names := map[string]bool{}
	for _, supply := range supplies {
		if supply.Name != "" {
			names[supply.Name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	resolved := make([]Environment, 0, len(ordered))
	for _, name := range ordered {
		resolved = append(resolved, Resolve(supplies, name))
	}
	return resolved
}
