package dumbmethod

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"unicode"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/plugin"
	"github.com/lucasew/contapila-go/internal/plugins/dumbmethod/alloc"
)

// PluginKey is the CUE plugins map key and the page enable flag.
const PluginKey = "dumb_method"

// PageID is the URL slug under /l/{ledger}/{PageID}.
const PageID = "dumb_method"

// Settings is plugins.dumb_method.settings. #Classify is user-authored CUE and
// is not decoded here; the page reads it directly.
type Settings struct {
	Classes map[string]Class `json:"classes"`
}

// Class is one target allocation entry.
type Class struct {
	Target float64 `json:"target"`
	Name   string  `json:"name"`
}

// targetClass is one configured class with its resolved display name.
type targetClass struct {
	alloc.Class
	Name string
}

// settingsValue returns plugins.dumb_method.settings, or an empty value.
func settingsValue(cfg cue.Value) cue.Value {
	return plugin.PluginValue(cfg, PluginKey).LookupPath(cue.ParsePath("settings"))
}

// classTargets reads exact target percentages from CUE. The prelude enforces
// target type and sum <= 100, so failures here surface as warnings instead of
// silently changing the plan. Name is the optional display label; it defaults
// to title case of the class id.
func classTargets(cfg cue.Value) ([]targetClass, []string) {
	sv := settingsValue(cfg)
	if !sv.Exists() {
		return nil, []string{"plugins.dumb_method.settings is not defined"}
	}
	classes := sv.LookupPath(cue.ParsePath("classes"))
	if !classes.Exists() {
		return nil, []string{"settings.classes is not defined"}
	}

	iter, err := classes.Fields()
	if err != nil {
		return nil, []string{fmt.Sprintf("read settings.classes: %v", err)}
	}
	var out []targetClass
	var warns []string
	for iter.Next() {
		id := iter.Selector().Unquoted()
		tv := iter.Value().LookupPath(cue.ParsePath("target"))
		if !tv.Exists() {
			warns = append(warns, fmt.Sprintf("class %q has no target; skipped", id))
			continue
		}
		target, ok := cueNumber(tv)
		if !ok {
			warns = append(warns, fmt.Sprintf("class %q target is not a number; skipped", id))
			continue
		}
		name := ""
		if nv := iter.Value().LookupPath(cue.ParsePath("name")); nv.Exists() {
			if s, err := nv.String(); err == nil {
				name = strings.TrimSpace(s)
			}
		}
		out = append(out, targetClass{
			Class: alloc.Class{ID: id, Target: target},
			Name:  displayName(id, name),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if c := out[i].Target.Cmp(out[j].Target); c != 0 {
			return c > 0
		}
		return out[i].ID < out[j].ID
	})
	if len(out) == 0 {
		warns = append(warns, "settings.classes has no usable target")
	}
	return out, warns
}

// displayName uses the explicit name when set, else title-cases the class id:
// renda_fixa -> Renda Fixa.
func displayName(id, name string) string {
	if name != "" {
		return name
	}
	parts := strings.FieldsFunc(id, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, p := range parts {
		r := []rune(p)
		if len(r) == 0 {
			continue
		}
		r[0] = unicode.ToUpper(r[0])
		parts[i] = string(r)
	}
	if len(parts) == 0 {
		return id
	}
	return strings.Join(parts, " ")
}

// cueNumber reads an exact rational from a CUE number.
func cueNumber(v cue.Value) (*big.Rat, bool) {
	b, err := v.MarshalJSON()
	if err != nil {
		return nil, false
	}
	s := strings.TrimSpace(strings.Trim(string(b), `"`))
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return nil, false
	}
	return r, true
}
