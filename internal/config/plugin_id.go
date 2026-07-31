package config

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
)

// ValidatePluginID rejects empty ids and ids that contain '/'.
// Plugin map keys, journal plugin "…" names, and RegisterTyped ids share this rule.
func ValidatePluginID(id string) error {
	if id == "" {
		return fmt.Errorf("plugin id is empty")
	}
	if strings.Contains(id, "/") {
		return fmt.Errorf("plugin id %q: '/' not allowed", id)
	}
	return nil
}

// ValidatePluginMapKeys checks every concrete key under plugins in a unified config.
func ValidatePluginMapKeys(v cue.Value) error {
	if !v.Exists() {
		return nil
	}
	root := v.LookupPath(cue.ParsePath("plugins"))
	if !root.Exists() {
		return nil
	}
	iter, err := root.Fields()
	if err != nil {
		return fmt.Errorf("plugins: %w", err)
	}
	for iter.Next() {
		key := iter.Selector().Unquoted()
		if err := ValidatePluginID(key); err != nil {
			return err
		}
	}
	return nil
}
