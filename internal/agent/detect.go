package agent

import "strings"

// registry is the ordered list of known agents, checked in priority order.
var registry = []Adapter{
	ClaudeCode{},
	Codex{},
	OpenCode{},
}

// Detect returns the first adapter whose Detect() returns true, or nil.
func Detect() Adapter {
	for _, a := range registry {
		if a.Detect() {
			return a
		}
	}
	return nil
}

// ByName returns the adapter matching the given slug (case-insensitive), or nil.
func ByName(name string) Adapter {
	lower := strings.ToLower(name)
	for _, a := range registry {
		if strings.ToLower(a.Slug()) == lower {
			return a
		}
	}
	return nil
}

// All returns all registered adapters.
func All() []Adapter {
	return registry
}

// Names returns a comma-separated list of all registered agent slugs.
func Names() string {
	names := make([]string, len(registry))
	for i, a := range registry {
		names[i] = a.Slug()
	}
	return strings.Join(names, ", ")
}
