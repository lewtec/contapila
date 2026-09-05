package ingest

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/lucasew/contapila-go/internal/ast"
)

// FormatDirective renders one directive as Beancount source ending with a newline.
func FormatDirective(d ast.Directive) (string, error) {
	var b strings.Builder
	switch v := d.(type) {
	case ast.Option:
		fmt.Fprintf(&b, "%s option %q %q\n", fmtDate(v.Date), v.Key, v.Value)
	case ast.Include:
		fmt.Fprintf(&b, "%s include %q\n", fmtDate(v.Date), v.Path)
	case ast.Commodity:
		fmt.Fprintf(&b, "%s commodity %s\n", fmtDate(v.Date), v.Currency)
		writeMeta(&b, v.Metadata)
	case ast.Open:
		fmt.Fprintf(&b, "%s open %s", fmtDate(v.Date), v.Account)
		for _, c := range v.Currencies {
			fmt.Fprintf(&b, " %s", c)
		}
		b.WriteByte('\n')
		writeMeta(&b, v.Metadata)
	case ast.Close:
		fmt.Fprintf(&b, "%s close %s\n", fmtDate(v.Date), v.Account)
		writeMeta(&b, v.Metadata)
	case ast.Transaction:
		flag := v.Flag
		if flag == "" {
			flag = "*"
		}
		fmt.Fprintf(&b, "%s %s", fmtDate(v.Date), flag)
		if v.Payee != "" {
			fmt.Fprintf(&b, " %s %s", quoteStr(v.Payee), quoteStr(v.Narration))
		} else if v.Narration != "" {
			fmt.Fprintf(&b, " %s", quoteStr(v.Narration))
		}
		for _, t := range v.Tags {
			fmt.Fprintf(&b, " #%s", t)
		}
		for _, l := range v.Links {
			fmt.Fprintf(&b, " ^%s", l)
		}
		b.WriteByte('\n')
		writeMeta(&b, v.Metadata)
		for _, p := range v.Postings {
			if err := writePosting(&b, p); err != nil {
				return "", err
			}
		}
	case ast.Price:
		num, err := formatRat(v.Amount.Number)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s price %s %s %s\n", fmtDate(v.Date), v.Currency, num, v.Amount.Commodity)
		writeMeta(&b, v.Metadata)
	case ast.Balance:
		num, err := formatRat(v.Amount.Number)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s balance %s %s %s\n", fmtDate(v.Date), v.Account, num, v.Amount.Commodity)
		writeMeta(&b, v.Metadata)
	case ast.Pad:
		fmt.Fprintf(&b, "%s pad %s %s\n", fmtDate(v.Date), v.Account, v.FromAccount)
		writeMeta(&b, v.Metadata)
	case ast.Note:
		fmt.Fprintf(&b, "%s note %s %s\n", fmtDate(v.Date), v.Account, quoteStr(v.Comment))
		writeMeta(&b, v.Metadata)
	case ast.Event:
		fmt.Fprintf(&b, "%s event %s %s\n", fmtDate(v.Date), quoteStr(v.Type), quoteStr(v.Desc))
		writeMeta(&b, v.Metadata)
	case ast.Custom:
		fmt.Fprintf(&b, "%s custom %s", fmtDate(v.Date), quoteStr(v.Type))
		for _, cv := range v.Values {
			if cv.Number != nil {
				num, err := formatRat(cv.Number)
				if err != nil {
					return "", err
				}
				fmt.Fprintf(&b, " %s", num)
			} else {
				fmt.Fprintf(&b, " %s", quoteStr(cv.Text))
			}
		}
		b.WriteByte('\n')
		writeMeta(&b, v.Metadata)
	case ast.Document:
		fmt.Fprintf(&b, "%s document %s %s\n", fmtDate(v.Date), v.Account, quoteStr(v.Path))
		writeMeta(&b, v.Metadata)
	case ast.Unknown:
		return "", fmt.Errorf("cannot format unknown directive %q", v.Kind)
	default:
		return "", fmt.Errorf("cannot format directive %T", d)
	}
	return b.String(), nil
}

func writePosting(b *strings.Builder, p ast.Posting) error {
	fmt.Fprintf(b, "  %s", p.Account)
	if p.Units != nil && p.Units.Number != nil {
		num, err := formatRat(p.Units.Number)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, " %s %s", num, p.Units.Commodity)
	}
	if p.Cost != nil {
		if p.Cost.Empty {
			b.WriteString(" {}")
		} else if p.Cost.Number != nil {
			num, err := formatRat(p.Cost.Number)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, " {%s %s", num, p.Cost.Commodity)
			if !p.Cost.Date.IsZero() {
				fmt.Fprintf(b, ", %s", fmtDate(p.Cost.Date))
			}
			b.WriteByte('}')
		}
	}
	if p.Price != nil && p.Price.Number != nil {
		num, err := formatRat(p.Price.Number)
		if err != nil {
			return err
		}
		op := "@"
		if p.Price.Total {
			op = "@@"
		}
		fmt.Fprintf(b, " %s %s %s", op, num, p.Price.Commodity)
	}
	b.WriteByte('\n')
	writeMeta(b, p.Metadata)
	return nil
}

func writeMeta(b *strings.Builder, md ast.Metadata) {
	if len(md) == 0 {
		return
	}
	keys := make([]string, 0, len(md))
	for k := range md {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "  %s: %s\n", k, quoteStr(md[k]))
	}
}

func fmtDate(t time.Time) string {
	if t.IsZero() {
		return "1970-01-01"
	}
	return t.UTC().Format("2006-01-02")
}

func quoteStr(s string) string {
	return fmt.Sprintf("%q", s)
}

// ratFormatPrec is decimal digits for non-integer big.Rat rendering: enough for
// beancount-ish amounts while avoiding float-noise tails after trim.
const ratFormatPrec = 18

func formatRat(r *big.Rat) (string, error) {
	if r == nil {
		return "", fmt.Errorf("nil number")
	}
	if r.IsInt() {
		return r.Num().String(), nil
	}
	s := r.FloatString(ratFormatPrec)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0", nil
	}
	return s, nil
}
