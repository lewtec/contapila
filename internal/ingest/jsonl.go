package ingest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/lucasew/contapila-go/internal/ast"
	"github.com/lucasew/contapila-go/internal/period"
)

// DecodeJSONL reads JSONL (one directive object per line) until EOF.
// Empty lines are skipped. Any error aborts the whole decode.
// Duplicate ids: last wins; warnings go to warnw if non-nil.
func DecodeJSONL(r io.Reader, warnw io.Writer) ([]ast.Directive, error) {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 10*1024*1024)

	byID := map[string]int{}
	var out []ast.Directive
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		d, id, err := decodeDirectiveLine([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if id != "" {
			if prev, ok := byID[id]; ok {
				if warnw != nil {
					fmt.Fprintf(warnw, "ingest: duplicate id %q (line %d replaces earlier entry)\n", id, lineNo)
				}
				out[prev] = d
				continue
			}
			byID[id] = len(out)
		}
		out = append(out, d)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeDirectiveLine(raw []byte) (ast.Directive, string, error) {
	var base struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Date string `json:"date"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		return nil, "", err
	}
	if base.Type == "" {
		return nil, "", fmt.Errorf("missing type")
	}
	date, err := period.ParseDate(base.Date)
	if err != nil {
		return nil, "", fmt.Errorf("date: %w", err)
	}
	id := strings.TrimSpace(base.ID)

	var d ast.Directive
	switch base.Type {
	case "option":
		var o struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		d = ast.Option{Meta: ast.Meta{Date: date}, Key: o.Key, Value: o.Value}
	case "include":
		var o struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		d = ast.Include{Meta: ast.Meta{Date: date}, Path: o.Path}
	case "commodity":
		var o struct {
			Currency string            `json:"currency"`
			Metadata map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		d = ast.Commodity{Meta: ast.Meta{Date: date}, Currency: o.Currency, Metadata: o.Metadata}
	case "open":
		var o struct {
			Account    string            `json:"account"`
			Currencies []string          `json:"currencies"`
			Metadata   map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		d = ast.Open{Meta: ast.Meta{Date: date}, Account: o.Account, Currencies: o.Currencies, Metadata: o.Metadata}
	case "close":
		var o struct {
			Account  string            `json:"account"`
			Metadata map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		d = ast.Close{Meta: ast.Meta{Date: date}, Account: o.Account, Metadata: o.Metadata}
	case "transaction":
		var o struct {
			Flag      string            `json:"flag"`
			Narration string            `json:"narration"`
			Payee     string            `json:"payee"`
			Tags      []string          `json:"tags"`
			Links     []string          `json:"links"`
			Metadata  map[string]string `json:"metadata"`
			Postings  []jsonPosting     `json:"postings"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		posts := make([]ast.Posting, 0, len(o.Postings))
		for i, jp := range o.Postings {
			p, err := jp.toPosting()
			if err != nil {
				return nil, "", fmt.Errorf("posting %d: %w", i, err)
			}
			posts = append(posts, p)
		}
		d = ast.Transaction{
			Meta: ast.Meta{Date: date}, Flag: o.Flag, Narration: o.Narration, Payee: o.Payee,
			Tags: o.Tags, Links: o.Links, Metadata: o.Metadata, Postings: posts,
		}
	case "price":
		var o struct {
			Currency string            `json:"currency"`
			Amount   jsonAmount        `json:"amount"`
			Metadata map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		amt, err := o.Amount.toAmount()
		if err != nil {
			return nil, "", err
		}
		d = ast.Price{Meta: ast.Meta{Date: date}, Currency: o.Currency, Amount: amt, Metadata: o.Metadata}
	case "balance":
		var o struct {
			Account  string            `json:"account"`
			Amount   jsonAmount        `json:"amount"`
			Metadata map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		amt, err := o.Amount.toAmount()
		if err != nil {
			return nil, "", err
		}
		d = ast.Balance{Meta: ast.Meta{Date: date}, Account: o.Account, Amount: amt, Metadata: o.Metadata}
	case "pad":
		var o struct {
			Account     string            `json:"account"`
			FromAccount string            `json:"from_account"`
			Metadata    map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		d = ast.Pad{Meta: ast.Meta{Date: date}, Account: o.Account, FromAccount: o.FromAccount, Metadata: o.Metadata}
	case "note":
		var o struct {
			Account  string            `json:"account"`
			Comment  string            `json:"comment"`
			Metadata map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		d = ast.Note{Meta: ast.Meta{Date: date}, Account: o.Account, Comment: o.Comment, Metadata: o.Metadata}
	case "event":
		// type is discriminant; event_type is AST Event.Type
		var o struct {
			EventType string            `json:"event_type"`
			Desc      string            `json:"desc"`
			Metadata  map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		if o.EventType == "" {
			return nil, "", fmt.Errorf("event requires event_type")
		}
		d = ast.Event{Meta: ast.Meta{Date: date}, Type: o.EventType, Desc: o.Desc, Metadata: o.Metadata}
	case "custom":
		var o struct {
			CustomType string            `json:"custom_type"`
			Values     []jsonCustomValue `json:"values"`
			Metadata   map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		if o.CustomType == "" {
			return nil, "", fmt.Errorf("custom requires custom_type")
		}
		vals := make([]ast.CustomValue, 0, len(o.Values))
		for i, jv := range o.Values {
			cv, err := jv.toCustomValue()
			if err != nil {
				return nil, "", fmt.Errorf("values[%d]: %w", i, err)
			}
			vals = append(vals, cv)
		}
		d = ast.Custom{Meta: ast.Meta{Date: date}, Type: o.CustomType, Values: vals, Metadata: o.Metadata}
	case "document":
		var o struct {
			Account  string            `json:"account"`
			Path     string            `json:"path"`
			Metadata map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, "", err
		}
		d = ast.Document{Meta: ast.Meta{Date: date}, Account: o.Account, Path: o.Path, Metadata: o.Metadata}
	default:
		return nil, "", fmt.Errorf("unknown type %q", base.Type)
	}

	if id != "" {
		d = ast.WithIngestID(d, id)
	}
	return d, id, nil
}

type jsonAmount struct {
	Number    string `json:"number"`
	Commodity string `json:"commodity"`
}

func (a jsonAmount) toAmount() (ast.Amount, error) {
	n, err := parseRat(a.Number)
	if err != nil {
		return ast.Amount{}, err
	}
	return ast.Amount{Number: n, Commodity: a.Commodity}, nil
}

type jsonCustomValue struct {
	Text   string `json:"text"`
	Number string `json:"number"`
}

func (v jsonCustomValue) toCustomValue() (ast.CustomValue, error) {
	if v.Number != "" {
		n, err := parseRat(v.Number)
		if err != nil {
			return ast.CustomValue{}, err
		}
		return ast.CustomValue{Number: n}, nil
	}
	return ast.CustomValue{Text: v.Text}, nil
}

type jsonPosting struct {
	Account  string            `json:"account"`
	Units    *jsonAmount       `json:"units"`
	Cost     *jsonCost         `json:"cost"`
	Price    *jsonPrice        `json:"price"`
	Metadata map[string]string `json:"metadata"`
}

type jsonCost struct {
	Number    string `json:"number"`
	Commodity string `json:"commodity"`
	Empty     bool   `json:"empty"`
	Date      string `json:"date"`
}

type jsonPrice struct {
	Number    string `json:"number"`
	Commodity string `json:"commodity"`
	Total     bool   `json:"total"`
}

func (jp jsonPosting) toPosting() (ast.Posting, error) {
	p := ast.Posting{Account: jp.Account, Metadata: jp.Metadata}
	if jp.Units != nil {
		if jp.Units.Number != "" || jp.Units.Commodity != "" {
			amt, err := jp.Units.toAmount()
			if err != nil {
				return p, err
			}
			p.Units = &amt
		}
	}
	if jp.Cost != nil {
		cs := &ast.CostSpec{Empty: jp.Cost.Empty, Commodity: jp.Cost.Commodity}
		if jp.Cost.Number != "" {
			n, err := parseRat(jp.Cost.Number)
			if err != nil {
				return p, err
			}
			cs.Number = n
		}
		if jp.Cost.Date != "" {
			dt, err := period.ParseDate(jp.Cost.Date)
			if err != nil {
				return p, err
			}
			cs.Date = dt
		}
		p.Cost = cs
	}
	if jp.Price != nil {
		n, err := parseRat(jp.Price.Number)
		if err != nil {
			return p, err
		}
		p.Price = &ast.PriceSpec{Number: n, Commodity: jp.Price.Commodity, Total: jp.Price.Total}
	}
	return p, nil
}

func parseRat(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty number")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid number %q", s)
	}
	return r, nil
}
