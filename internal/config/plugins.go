package config

import (
	"fmt"

	"cuelang.org/go/cue"
)

// PluginFlags reads plugins from a unified config value.
// Keys present in contapila.cue (e.g. "web/accounts") map to their enabled flag.
// Absent plugins map or missing keys are omitted (callers treat omit as enabled).
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
			out[key] = true
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

// PluginEnabled returns whether pluginKey is on. Missing key or missing plugins → true.
// Decode errors → true plus non-nil err (caller may log and keep defaults).
func PluginEnabled(v cue.Value, pluginKey string) (bool, error) {
	flags, err := PluginFlags(v)
	if err != nil {
		return true, err
	}
	if flags == nil {
		return true, nil
	}
	if b, ok := flags[pluginKey]; ok {
		return b, nil
	}
	return true, nil
}

// PluginsEnabledFunc returns a predicate for FilterPagesByPlugins.
// On decode error, returns (all-true func, err) so the UI stays usable.
func PluginsEnabledFunc(v cue.Value) (func(pluginKey string) bool, error) {
	flags, err := PluginFlags(v)
	fn := func(pluginKey string) bool {
		if flags == nil {
			return true
		}
		if b, ok := flags[pluginKey]; ok {
			return b
		}
		return true
	}
	return fn, err
}
