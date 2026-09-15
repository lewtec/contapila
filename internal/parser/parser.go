package parser

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/diag"
	"github.com/lucasew/contapila-go/internal/period"
	"github.com/lucasew/contapila-go/internal/source"
	"github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar"
	bc "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/beancount"
)

// Parse converts Beancount source into directives using modernc tree-sitter.
// Prefer ParseFile when the caller already has a source.File.
func Parse(filename string, src []byte) ([]ast.Directive, diag.List, error) {
	return ParseFile(source.NewString(filename, string(src)))
}

// ParseFile is Parse with a pre-built source.File (path + text + line index).
func ParseFile(f *source.File) ([]ast.Directive, diag.List, error) {
	if f == nil {
		return nil, nil, fmt.Errorf("nil source file")
	}
	p := grammar.NewParser()
	defer p.Delete()
	if !p.SetLanguage(bc.Language()) {
		return nil, nil, fmt.Errorf("failed to set beancount language")
	}
	tree := p.ParseString(f.Text)
	defer tree.Delete()
	root := tree.RootNode()
	if root.IsNull() {
		return nil, nil, fmt.Errorf("null parse tree for %s", f.Path)
	}

	var diags diag.List
	var out []ast.Directive
	for i := uint32(0); i < root.NamedChildCount(); i++ {
		collectDirectives(f, root.NamedChild(i), &diags, &out)
	}
	return out, diags, nil
}

// collectDirectives walks the tree, ignoring org-mode section structure and comments.
// Section nodes are containers only (headlines like "* Assets"); nested directives are collected.
func collectDirectives(f *source.File, n *grammar.Node, diags *diag.List, out *[]ast.Directive) {
	if n == nil || n.IsNull() {
		return
	}
	if n.IsError() {
		diags.Error(f.Path, f.Lines.LineAtU32(n.StartByte()), fmt.Sprintf("syntax error near %q", clip(nodeText(f, n), 40)))
		return
	}
	switch n.Type() {
	case "section":
		// Org/markdown headings: structure only — do not warn, walk children.
		for i := uint32(0); i < n.NamedChildCount(); i++ {
			collectDirectives(f, n.NamedChild(i), diags, out)
		}
		return
	case "comment", "headline", "item":
		return
	}
	if d, ok := convert(f, n, diags); ok {
		*out = append(*out, d)
	}
}

func convert(f *source.File, n *grammar.Node, diags *diag.List) (ast.Directive, bool) {
	switch n.Type() {
	case "option":
		return ast.Option{Meta: meta(f, n), Key: unquote(textField(f, n, "key")), Value: unquote(textField(f, n, "value"))}, true
	case "include":
		// include has a string child, not always a field
		path := ""
		for i := uint32(0); i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if c.Type() == "string" {
				path = unquote(nodeText(f, c))
				break
			}
		}
		return ast.Include{Meta: meta(f, n), Path: path}, true
	case "commodity":
		o := ast.Commodity{
			Meta:     meta(f, n),
			Currency: textField(f, n, "currency"),
			Metadata: parseKeyValues(f, n),
		}
		return o, true
	case "open":
		o := ast.Open{
			Meta:     meta(f, n),
			Account:  textField(f, n, "account"),
			Metadata: parseKeyValues(f, n),
		}
		for i := uint32(0); i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if c.Type() == "currency" {
				o.Currencies = append(o.Currencies, strings.TrimSpace(nodeText(f, c)))
			}
		}
		return o, true
	case "close":
		return ast.Close{
			Meta:     meta(f, n),
			Account:  textField(f, n, "account"),
			Metadata: parseKeyValues(f, n),
		}, true
	case "transaction":
		return convertTxn(f, n, diags), true
	case "price":
		amt, ok := parseAmountNode(f, field(n, "amount"))
		if !ok {
			diags.Error(f.Path, f.Lines.LineAtU32(n.StartByte()), "price missing or invalid amount; skipped")
			return nil, false
		}
		return ast.Price{
			Meta:     meta(f, n),
			Currency: textField(f, n, "currency"),
			Amount:   amt,
			Metadata: parseKeyValues(f, n),
		}, true
	case "balance":
		// amount may be field "amount" or amount_tolerance child
		an := field(n, "amount")
		if an == nil {
			an = field(n, "amount_tolerance")
		}
		if an == nil {
			for i := uint32(0); i < n.NamedChildCount(); i++ {
				c := n.NamedChild(i)
				if c.Type() == "amount" || c.Type() == "amount_tolerance" {
					an = c
					break
				}
			}
		}
		amt, ok := parseAmountNode(f, an)
		if !ok {
			diags.Error(f.Path, f.Lines.LineAtU32(n.StartByte()), "balance missing or invalid amount; skipped")
			return nil, false
		}
		return ast.Balance{
			Meta:     meta(f, n),
			Account:  textField(f, n, "account"),
			Amount:   amt,
			Metadata: parseKeyValues(f, n),
		}, true
	case "pad":
		return ast.Pad{
			Meta:        meta(f, n),
			Account:     textField(f, n, "account"),
			FromAccount: textField(f, n, "from_account"),
			Metadata:    parseKeyValues(f, n),
		}, true
	case "note":
		return ast.Note{
			Meta:     meta(f, n),
			Account:  textField(f, n, "account"),
			Comment:  unquote(textField(f, n, "note")),
			Metadata: parseKeyValues(f, n),
		}, true
	case "event":
		return ast.Event{
			Meta:     meta(f, n),
			Type:     unquote(textField(f, n, "type")),
			Desc:     unquote(textField(f, n, "desc")),
			Metadata: parseKeyValues(f, n),
		}, true
	case "document":
		// 2020-01-01 document Assets:Cash "docs/..."
		path := ""
		if fn := field(n, "filename"); fn != nil {
			path = unquote(strings.TrimSpace(nodeText(f, fn)))
			if path == "" {
				// filename node may wrap a string child
				for i := uint32(0); i < fn.NamedChildCount(); i++ {
					c := fn.NamedChild(i)
					if c.Type() == "string" {
						path = unquote(nodeText(f, c))
						break
					}
				}
			}
		}
		if path == "" {
			for i := uint32(0); i < n.NamedChildCount(); i++ {
				c := n.NamedChild(i)
				if c.Type() == "string" {
					path = unquote(nodeText(f, c))
					break
				}
			}
		}
		return ast.Document{
			Meta:     meta(f, n),
			Account:  textField(f, n, "account"),
			Path:     path,
			Metadata: parseKeyValues(f, n),
		}, true
	case "query":
		diags.Warn(f.Path, f.Lines.LineAtU32(n.StartByte()), fmt.Sprintf("%s not supported; skipped", n.Type()))
		return nil, false
	case "custom":
		return convertCustom(f, n, diags)
	case "comment":
		return nil, false
	case "pushtag", "poptag", "pushmeta", "popmeta":
		diags.Warn(f.Path, f.Lines.LineAtU32(n.StartByte()), fmt.Sprintf("%s not supported; skipped", n.Type()))
		return nil, false
	default:
		diags.Warn(f.Path, f.Lines.LineAtU32(n.StartByte()), fmt.Sprintf("unsupported directive %q skipped", n.Type()))
		return nil, false
	}
}

// convertCustom parses: DATE custom "type" value…
// Values may be strings, numbers, or bare names (grammar may tag names as account).
func convertCustom(f *source.File, n *grammar.Node, diags *diag.List) (ast.Directive, bool) {
	var typ string
	var vals []ast.CustomValue
	// Named children: date, string (type), custom_value…, key_value
	for i := uint32(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		switch c.Type() {
		case "date", "key_value":
			continue
		case "string":
			s := unquote(nodeText(f, c))
			if typ == "" {
				typ = s
				continue
			}
			vals = append(vals, ast.CustomValue{Text: s})
		case "custom_value":
			vals = append(vals, parseCustomValue(f, c))
		case "number":
			if num := evalNumberExpr(f, c); num != nil {
				vals = append(vals, ast.CustomValue{Number: num})
			}
		case "account", "currency":
			vals = append(vals, ast.CustomValue{Text: strings.TrimSpace(nodeText(f, c))})
		}
	}
	if typ == "" {
		diags.Warn(f.Path, f.Lines.LineAtU32(n.StartByte()), `custom directive missing type string; skipped`)
		return nil, false
	}
	return ast.Custom{
		Meta:     meta(f, n),
		Type:     typ,
		Values:   vals,
		Metadata: parseKeyValues(f, n),
	}, true
}

func parseCustomValue(f *source.File, n *grammar.Node) ast.CustomValue {
	// custom_value wraps string | number | account | …
	for i := uint32(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		switch c.Type() {
		case "string":
			return ast.CustomValue{Text: unquote(nodeText(f, c))}
		case "number", "binary_number_expr", "unary_number_expr":
			if num := evalNumberExpr(f, c); num != nil {
				return ast.CustomValue{Number: num}
			}
		case "account", "currency":
			return ast.CustomValue{Text: strings.TrimSpace(nodeText(f, c))}
		case "amount":
			if amt, ok := parseAmountNode(f, c); ok {
				// Prefer number; commodity as text if present without separate slots.
				if amt.Commodity != "" {
					return ast.CustomValue{Text: amt.Number.FloatString(12) + " " + amt.Commodity}
				}
				return ast.CustomValue{Number: amt.Number}
			}
		}
	}
	// Fallback: whole node text
	t := strings.TrimSpace(nodeText(f, n))
	if num, ok := new(big.Rat).SetString(t); ok {
		return ast.CustomValue{Number: num}
	}
	return ast.CustomValue{Text: unquote(t)}
}

// parseKeyValues collects key_value children into a metadata map (string values).
func parseKeyValues(f *source.File, n *grammar.Node) ast.Metadata {
	if n == nil {
		return nil
	}
	var md ast.Metadata
	for i := uint32(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c.Type() != "key_value" {
			continue
		}
		key, val := keyValuePair(f, c)
		if key == "" {
			continue
		}
		if md == nil {
			md = ast.Metadata{}
		}
		md[key] = val
	}
	return md
}

func keyValuePair(f *source.File, n *grammar.Node) (key, val string) {
	key = textField(f, n, "key")
	if key == "" {
		for j := uint32(0); j < n.NamedChildCount(); j++ {
			ch := n.NamedChild(j)
			if ch.Type() == "key" {
				key = strings.TrimSpace(nodeText(f, ch))
				break
			}
		}
	}
	// value is often a field, or a string / number child under "value"
	if vf := field(n, "value"); vf != nil {
		val = metadataValue(f, vf)
	}
	if val == "" {
		for j := uint32(0); j < n.NamedChildCount(); j++ {
			ch := n.NamedChild(j)
			if ch.Type() == "value" || ch.Type() == "string" || ch.Type() == "number" {
				val = metadataValue(f, ch)
				if val != "" {
					break
				}
			}
		}
	}
	return key, val
}

func metadataValue(f *source.File, n *grammar.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "string":
		return unquote(nodeText(f, n))
	case "number", "bool", "currency", "account", "tag", "link", "date":
		return strings.TrimSpace(nodeText(f, n))
	case "value":
		// unwrap single named child if present
		for i := uint32(0); i < n.NamedChildCount(); i++ {
			if s := metadataValue(f, n.NamedChild(i)); s != "" {
				return s
			}
		}
		return unquote(strings.TrimSpace(nodeText(f, n)))
	default:
		// try nested string
		for i := uint32(0); i < n.NamedChildCount(); i++ {
			if s := metadataValue(f, n.NamedChild(i)); s != "" {
				return s
			}
		}
		t := strings.TrimSpace(nodeText(f, n))
		return unquote(t)
	}
}

func convertTxn(f *source.File, n *grammar.Node, diags *diag.List) ast.Transaction {
	txn := ast.Transaction{
		Meta:      meta(f, n),
		Flag:      strings.TrimSpace(textField(f, n, "txn")),
		Narration: unquote(textField(f, n, "narration")),
		Payee:     unquote(textField(f, n, "payee")),
	}
	// Walk named children in source order. Beancount grammar places posting key_value
	// as siblings of posting under the transaction (not nested under posting).
	// key_value before any posting → txn metadata; after a posting → that posting.
	var txnMD ast.Metadata
	for i := uint32(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		switch c.Type() {
		case "tags_links":
			for j := uint32(0); j < c.NamedChildCount(); j++ {
				ch := c.NamedChild(j)
				t := nodeText(f, ch)
				switch ch.Type() {
				case "tag":
					txn.Tags = append(txn.Tags, strings.TrimPrefix(t, "#"))
				case "link":
					txn.Links = append(txn.Links, strings.TrimPrefix(t, "^"))
				}
			}
		case "key_value":
			key, val := keyValuePair(f, c)
			if key == "" {
				continue
			}
			if len(txn.Postings) == 0 {
				if txnMD == nil {
					txnMD = ast.Metadata{}
				}
				txnMD[key] = val
			} else {
				last := &txn.Postings[len(txn.Postings)-1]
				if last.Metadata == nil {
					last.Metadata = ast.Metadata{}
				}
				last.Metadata[key] = val
			}
		case "posting":
			txn.Postings = append(txn.Postings, convertPosting(f, c, diags))
		}
	}
	txn.Metadata = txnMD
	return txn
}

func convertPosting(f *source.File, n *grammar.Node, diags *diag.List) ast.Posting {
	// Nested key_value under posting (if grammar ever nests them) plus sibling handling in convertTxn.
	p := ast.Posting{
		Account:  textField(f, n, "account"),
		Metadata: parseKeyValues(f, n),
	}
	// amount field or incomplete_amount child
	an := field(n, "amount")
	if an == nil {
		for i := uint32(0); i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if c.Type() == "amount" || c.Type() == "incomplete_amount" {
				an = c
				break
			}
		}
	}
	// Bare number without commodity often lands as ERROR under posting (not residual).
	if an == nil {
		for i := uint32(0); i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if c.Type() == "ERROR" || c.IsError() {
				if num := evalNumberExpr(f, c); num != nil {
					p.Units = &ast.Amount{Number: num, Commodity: ""}
					break
				}
			}
		}
	}
	if an != nil {
		if amt, ok := parseAmountNode(f, an); ok {
			p.Units = &amt
		}
	}
	// Number present but no commodity → error (not a residual empty leg).
	if p.Units != nil && p.Units.Number != nil && strings.TrimSpace(p.Units.Commodity) == "" {
		diags.Error(f.Path, f.Lines.LineAtU32(n.StartByte()), fmt.Sprintf("amount missing commodity on %s", p.Account))
	}
	if cs := field(n, "cost_spec"); cs != nil {
		p.Cost = parseCost(f, cs)
	}
	// price: child "atat" or "at" + field price_annotation
	hasAtAt := false
	hasAt := false
	for i := uint32(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Type() {
		case "atat":
			hasAtAt = true
		case "at":
			hasAt = true
		}
	}
	if pa := field(n, "price_annotation"); pa != nil {
		if amt, ok := parseAmountNode(f, firstAmountish(pa)); ok {
			p.Price = &ast.PriceSpec{Number: amt.Number, Commodity: amt.Commodity, Total: hasAtAt || !hasAt && hasAtAt}
			if hasAtAt {
				p.Price.Total = true
			} else if hasAt {
				p.Price.Total = false
			} else {
				// default total if @@ node missing but only one form
				p.Price.Total = hasAtAt
			}
		}
	}
	// Fix Total detection: if we saw atat token
	if p.Price != nil {
		p.Price.Total = hasAtAt
	}
	return p
}

func parseCost(f *source.File, n *grammar.Node) *ast.CostSpec {
	// cost_spec → cost_comp_list → cost_comp → compound_amount | date | …
	empty := true
	var num *big.Rat
	var cur string
	var costDate time.Time
	walkNamed(n, func(c *grammar.Node) {
		switch c.Type() {
		case "compound_amount":
			empty = false
			if per := field(c, "per"); per != nil {
				num = parseNumber(f, per)
			}
			if cu := field(c, "currency"); cu != nil {
				cur = strings.TrimSpace(nodeText(f, cu))
			}
			// also total form
			if num == nil {
				if t := field(c, "total"); t != nil {
					num = parseNumber(f, t)
				}
			}
			// compound_amount may expose number/currency as direct children
			if num == nil {
				for i := uint32(0); i < c.NamedChildCount(); i++ {
					ch := c.NamedChild(i)
					if ch.Type() == "number" || ch.Type() == "unary_number_expr" || ch.Type() == "binary_number_expr" {
						if num == nil {
							num = parseNumber(f, ch)
						}
					}
					if ch.Type() == "currency" && cur == "" {
						cur = strings.TrimSpace(nodeText(f, ch))
					}
				}
			}
		case "date":
			if d, err := period.ParseDate(strings.TrimSpace(nodeText(f, c))); err == nil {
				costDate = d
				empty = false
			}
		}
	})
	// empty braces {}
	text := strings.TrimSpace(nodeText(f, n))
	if text == "{}" || text == "{ }" {
		return &ast.CostSpec{Empty: true}
	}
	if empty && (strings.Contains(text, "{}") || text == "{}") {
		return &ast.CostSpec{Empty: true}
	}
	if num == nil && cur == "" && costDate.IsZero() {
		// treat as empty cost spec if no numbers found
		if strings.Contains(text, "{") {
			return &ast.CostSpec{Empty: true}
		}
		return nil
	}
	return &ast.CostSpec{Number: num, Commodity: cur, Empty: num == nil, Date: costDate}
}

func firstAmountish(n *grammar.Node) *grammar.Node {
	if n == nil {
		return nil
	}
	switch n.Type() {
	case "amount", "incomplete_amount", "amount_tolerance":
		return n
	}
	for i := uint32(0); i < n.NamedChildCount(); i++ {
		if c := firstAmountish(n.NamedChild(i)); c != nil {
			return c
		}
	}
	return n
}

func parseAmountNode(f *source.File, n *grammar.Node) (ast.Amount, bool) {
	if n == nil {
		return ast.Amount{}, false
	}
	n = firstAmountish(n)
	var num *big.Rat
	var cur string
	// Prefer top-level expr child (binary/unary), not the first nested number leaf.
	for i := uint32(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		switch c.Type() {
		case "binary_number_expr", "unary_number_expr":
			if num == nil {
				num = evalNumberExpr(f, c)
			}
		case "number":
			if num == nil {
				num = evalNumberExpr(f, c)
			}
		case "currency":
			cur = strings.TrimSpace(nodeText(f, c))
		}
	}
	if num == nil {
		num = evalNumberExpr(f, n)
	}
	if num == nil {
		return ast.Amount{}, false
	}
	return ast.Amount{Number: num, Commodity: cur}, true
}

func parseNumber(f *source.File, n *grammar.Node) *big.Rat {
	return evalNumberExpr(f, n)
}

func meta(f *source.File, n *grammar.Node) ast.Meta {
	d := time.Time{}
	if dn := field(n, "date"); dn != nil {
		d, _ = period.ParseDate(strings.TrimSpace(nodeText(f, dn)))
	}
	path, line := "", 0
	start, end := 0, 0
	if n != nil && !n.IsNull() {
		start = int(n.StartByte())
		end = int(n.EndByte())
	}
	if f != nil {
		path = f.Path
		if n != nil && !n.IsNull() {
			line = f.Lines.LineAtU32(n.StartByte())
		}
	}
	return ast.Meta{Date: d, File: path, Line: line, StartByte: start, EndByte: end}
}

func field(n *grammar.Node, name string) *grammar.Node {
	for i := uint32(0); i < n.ChildCount(); i++ {
		if n.FieldNameForChild(i) == name {
			c := n.Child(i)
			if !c.IsNull() {
				return c
			}
		}
	}
	return nil
}

func textField(f *source.File, n *grammar.Node, name string) string {
	c := field(n, name)
	if c == nil {
		return ""
	}
	return strings.TrimSpace(nodeText(f, c))
}

// nodeText returns the source text covered by a tree-sitter node.
func nodeText(f *source.File, n *grammar.Node) string {
	if f == nil || n == nil || n.IsNull() {
		return ""
	}
	return f.Slice(int(n.StartByte()), int(n.EndByte()))
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func rat(s string) *big.Rat {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	r := new(big.Rat)
	if _, ok := r.SetString(s); !ok {
		return nil
	}
	return r
}

func walkNamed(n *grammar.Node, fn func(*grammar.Node)) {
	if n == nil || n.IsNull() {
		return
	}
	fn(n)
	for i := uint32(0); i < n.NamedChildCount(); i++ {
		walkNamed(n.NamedChild(i), fn)
	}
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
