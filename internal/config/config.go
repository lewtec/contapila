package config

import (
	"embed"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

//go:embed prelude.cue
var fs embed.FS

const PreludeFilename = "prelude.cue"

// Ledger is one filesystem-discovered ledger injected into CUE as ledgers.<name>.
type Ledger struct {
	Name string // directory name
	Main string // absolute path to main.beancount
}

// PricePair is one base/quote pair discovered in prices.beancount (inventory only).
type PricePair struct {
	Base  string
	Quote string
}

type Config struct {
	Value cue.Value
}

// ProjectJournal is one auto-loaded root beancount file from prelude project_journals.
type ProjectJournal struct {
	Path    string // relative to project root
	Role    string // "prices" | "stream"
	Missing string // "warn" | "ignore"
}

var ledgerIDRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// Load unifies embedded prelude ⊔ generated ledgers ⊔ generated price pairs ⊔ user contapila.cue.
// discovered comes from scanning <root>/*/main.beancount; pricePairs from prices.beancount.
func Load(userCue []byte, userFilename string, discovered []Ledger, pricePairs []PricePair) (*Config, error) {
	ctx := cuecontext.New()

	preludeBytes, err := fs.ReadFile(PreludeFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded prelude: %w", err)
	}

	prelude := ctx.CompileBytes(preludeBytes, cue.Filename(PreludeFilename))
	if err := prelude.Err(); err != nil {
		return nil, fmt.Errorf("failed to compile prelude: %w", err)
	}

	gen := ctx.CompileString(encodeLedgersCUE(discovered), cue.Filename("ledgers.gen.cue"))
	if err := gen.Err(); err != nil {
		return nil, fmt.Errorf("failed to compile discovered ledgers: %w", err)
	}

	pairs := ctx.CompileString(encodePricePairsCUE(pricePairs), cue.Filename("price_pairs.gen.cue"))
	if err := pairs.Err(); err != nil {
		return nil, fmt.Errorf("failed to compile price pairs: %w", err)
	}

	user := ctx.CompileBytes(userCue, cue.Filename(userFilename))
	if err := user.Err(); err != nil {
		return nil, fmt.Errorf("failed to compile user config: %w", err)
	}

	unified := prelude.Unify(gen).Unify(pairs).Unify(user)
	if err := unified.Validate(); err != nil {
		return nil, fmt.Errorf("config unification failed: %w", err)
	}

	return &Config{Value: unified}, nil
}

// ProjectJournals reads project_journals from a unified config value (prelude defaults apply).
// Missing or empty project_journals yields a nil slice. A non-list value is an error.
//
// Per entry: path is required (empty/absent → skip entry). role and missing are optional
// (absent role → skip entry; absent/invalid missing enum → "ignore"). Present fields with
// the wrong CUE type return a decode error rather than silent empty defaults.
func ProjectJournals(v cue.Value) ([]ProjectJournal, error) {
	if !v.Exists() {
		return nil, nil
	}
	list := v.LookupPath(cue.ParsePath("project_journals"))
	if !list.Exists() {
		return nil, nil
	}
	iter, err := list.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list project_journals: %w", err)
	}
	var out []ProjectJournal
	for iter.Next() {
		item := iter.Value()
		path, err := journalStringField(item, "path")
		if err != nil {
			return nil, err
		}
		role, err := journalStringField(item, "role")
		if err != nil {
			return nil, err
		}
		missing, err := journalStringField(item, "missing")
		if err != nil {
			return nil, err
		}
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if role != "prices" && role != "stream" {
			continue
		}
		if missing != "warn" && missing != "ignore" {
			missing = "ignore"
		}
		out = append(out, ProjectJournal{Path: path, Role: role, Missing: missing})
	}
	return out, nil
}

// journalStringField returns the string value of a journal entry field.
// Absent fields yield ("", nil). Present non-string values are errors.
func journalStringField(item cue.Value, field string) (string, error) {
	fv := item.LookupPath(cue.ParsePath(field))
	s, err := fv.String()
	if err != nil {
		if !fv.Exists() {
			return "", nil
		}
		return "", fmt.Errorf("failed to decode project_journals.%s: %w", field, err)
	}
	return s, nil
}

// encodeLedgersCUE builds a closed ledgers struct from filesystem discovery.
// Example:
//
//	ledgers: close({
//		personal: {name: "personal", main: "/abs/personal/main.beancount"}
//	})
func encodeLedgersCUE(discovered []Ledger) string {
	// stable key order
	names := make([]string, 0, len(discovered))
	byName := make(map[string]Ledger, len(discovered))
	for _, l := range discovered {
		if l.Name == "" {
			continue
		}
		if _, ok := byName[l.Name]; ok {
			continue
		}
		byName[l.Name] = l
		names = append(names, l.Name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("// Generated from filesystem lookup of */main.beancount — do not edit.\n")
	b.WriteString("ledgers: close({\n")
	for _, name := range names {
		l := byName[name]
		b.WriteString("\t")
		b.WriteString(cueLabel(name))
		b.WriteString(": {\n")
		b.WriteString("\t\tname: ")
		b.WriteString(strconv.Quote(l.Name))
		b.WriteString("\n\t\tmain: ")
		b.WriteString(strconv.Quote(l.Main))
		b.WriteString("\n\t}\n")
	}
	b.WriteString("})\n")
	return b.String()
}

func cueLabel(name string) string {
	if ledgerIDRe.MatchString(name) {
		return name
	}
	return strconv.Quote(name)
}

// encodePricePairsCUE builds a closed price_pairs map from PriceDB pair inventory.
// Keys are "base|quote". Full series are not injected (volume).
//
//	price_pairs: close({
//		"USD|BRL": {base: "USD", quote: "BRL"}
//	})
func encodePricePairsCUE(pairs []PricePair) string {
	type pair struct{ Base, Quote string }
	seen := map[string]pair{}
	var keys []string
	for _, p := range pairs {
		if p.Base == "" || p.Quote == "" {
			continue
		}
		k := p.Base + "|" + p.Quote
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = pair{p.Base, p.Quote}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("// Generated from prices.beancount pair inventory — do not edit.\n")
	b.WriteString("price_pairs: close({\n")
	for _, k := range keys {
		p := seen[k]
		b.WriteString("\t")
		b.WriteString(strconv.Quote(k))
		b.WriteString(": {base: ")
		b.WriteString(strconv.Quote(p.Base))
		b.WriteString(", quote: ")
		b.WriteString(strconv.Quote(p.Quote))
		b.WriteString("}\n")
	}
	b.WriteString("})\n")
	return b.String()
}
