package config

import (
	"fmt"

	"cuelang.org/go/cue"
)

// PluginFlags reads plugins from a unified config value.
// Keys present (e.g. "web/accounts") map to their enabled flag after CUE defaults
// (enabled defaults false unless the key sets true or the prelude product default).
// Empty/absent plugins map yields nil.
func PluginFlags(v cue.Value) (map[string]bool, error) {
	if !v.Exists() {
		return nil, nil
	}
	root := v.LookupPath(cue.ParsePath("plugins"))
	if !root.Exists() {
		return nil, nil
	}
	iter, err := root.Fields()
	if err != nil {
		return nil, fmt.Errorf("plugins: %w", err)
	}
	out := map[string]bool{}
	for iter.Next() {
		key := iter.Selector().Unquoted()
		item := iter.Value()
		en := item.LookupPath(cue.ParsePath("enabled"))
		if !en.Exists() {
			// Schema default is false (*false).
			out[key] = false
			continue
		}
		b, err := en.Bool()
		if err != nil {
			return nil, fmt.Errorf("plugins.%s.enabled: %w", key, err)
		}
		out[key] = b
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// PluginEnabled reports whether pluginKey is on (opt-in).
// Missing key or missing plugins → false. Decode errors → false + err.
func PluginEnabled(v cue.Value, pluginKey string) (bool, error) {
	flags, err := PluginFlags(v)
	if err != nil {
		return false, err
	}
	if flags == nil {
		return false, nil
	}
	if b, ok := flags[pluginKey]; ok {
		return b, nil
	}
	return false, nil
}

// PluginsEnabledFunc returns a predicate for optional modules (ContributePage).
// Missing keys are off. On decode error, returns (all-false func, err).
func PluginsEnabledFunc(v cue.Value) (func(pluginKey string) bool, error) {
	flags, err := PluginFlags(v)
	fn := func(pluginKey string) bool {
		if flags == nil {
			return false
		}
		if b, ok := flags[pluginKey]; ok {
			return b
		}
		return false
	}
	return fn, err
}
