package ast

import (
	"math/big"
	"time"
)

type Amount struct {
	Number    *big.Rat
	Commodity string
}

func (a Amount) Clone() Amount {
	if a.Number == nil {
		return Amount{Commodity: a.Commodity}
	}
	return Amount{Number: new(big.Rat).Set(a.Number), Commodity: a.Commodity}
}

type CostSpec struct {
	Number    *big.Rat // nil with Empty=true means {}
	Commodity string
	Empty     bool
	Date      time.Time // optional cost date → synthetic price (zero if omitted)
}

type PriceSpec struct {
	Number    *big.Rat
	Commodity string
	Total     bool // @@ when true, @ when false
}

type Posting struct {
	Account  string
	Units    *Amount // nil => residual
	Cost     *CostSpec
	Price    *PriceSpec
	Metadata Metadata // key_value under the posting (not CUE — journal stream only)
}

type Directive interface {
	GetDate() time.Time
	GetFile() string
}

type Meta struct {
	Date time.Time
	File string
	Line int // 1-based source line; 0 if unknown
	// StartByte/EndByte are source spans in the originating file (exclusive end).
	// Zero means unknown (e.g. synthesized or JSON-decoded).
	StartByte int
	EndByte   int
}

func (m Meta) GetDate() time.Time { return m.Date }
func (m Meta) GetFile() string    { return m.File }
func (m Meta) GetLine() int       { return m.Line }

type Option struct {
	Meta
	Key, Value string
}

type Include struct {
	Meta
	Path string
}

// Metadata is Beancount key: value attributes on a directive (strings normalized).
// Keys are stored as written (e.g. "asset-class", "institution").
type Metadata map[string]string

// Clone returns a shallow copy of m. Nil and empty maps both yield nil so
// callers cannot tell empty from missing after a clone (same as prior engine/prices helpers).
func (m Metadata) Clone() Metadata {
	if len(m) == 0 {
		return nil
	}
	out := make(Metadata, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

type Commodity struct {
	Meta
	Currency string
	Metadata Metadata // from key_value under the commodity directive
}

type Open struct {
	Meta
	Account    string
	Currencies []string // optional commodities declared on open (e.g. open Assets:Cash BRL)
	Metadata   Metadata // from key_value under the open directive
}

type Close struct {
	Meta
	Account  string
	Metadata Metadata
}

type Transaction struct {
	Meta
	Flag      string
	Narration string
	Payee     string
	Postings  []Posting
	Tags      []string
	Links     []string
	Metadata  Metadata // key_value under the txn header (not CUE — journal stream only)
}

type Price struct {
	Meta
	Currency string   // base commodity being priced
	Amount   Amount   // quote amount (Number + quote Commodity)
	Metadata Metadata // key_value under the price directive
}

type Balance struct {
	Meta
	Account  string
	Amount   Amount
	Metadata Metadata // key_value under the balance directive (journal stream only)
}

type Pad struct {
	Meta
	Account     string
	FromAccount string
	Metadata    Metadata
}

type Note struct {
	Meta
	Account  string
	Comment  string
	Metadata Metadata
}

type Event struct {
	Meta
	Type     string
	Desc     string
	Metadata Metadata // key_value under the event directive (journal stream only)
}

// Custom is a Beancount custom directive (typed bag of values).
// Index series use Type "index": values [indicator string, daily return number].
type Custom struct {
	Meta
	Type     string        // first string after `custom` (e.g. "index")
	Values   []CustomValue // remaining values in source order
	Metadata Metadata
}

// CustomValue is one custom directive argument (string name or number).
type CustomValue struct {
	// Exactly one of Text or Number is set.
	Text   string
	Number *big.Rat
}

// Document is a Beancount document directive, or one synthesized from <ledger>/docs/by-account.
type Document struct {
	Meta
	Account   string
	Path      string // project-relative (e.g. personal/docs/by-account/Assets/Cash/20240101_x.txt)
	Synthetic bool   // true when expanded from the docs/ tree at runtime
	Metadata  Metadata
}

// IngestIDMetaKey is the journal metadata key used by `contapila ingest` for upserts.
const IngestIDMetaKey = "ingest_id"

// DirectiveMetadata returns metadata map for directives that carry one (may be nil).
func DirectiveMetadata(d Directive) Metadata {
	switch v := d.(type) {
	case Commodity:
		return v.Metadata
	case Open:
		return v.Metadata
	case Close:
		return v.Metadata
	case Transaction:
		return v.Metadata
	case Price:
		return v.Metadata
	case Balance:
		return v.Metadata
	case Pad:
		return v.Metadata
	case Note:
		return v.Metadata
	case Event:
		return v.Metadata
	case Custom:
		return v.Metadata
	case Document:
		return v.Metadata
	default:
		return nil
	}
}

// SetDirectiveIngestID sets ingest_id metadata on d (must be a pointer-capable update via return).
func WithIngestID(d Directive, id string) Directive {
	if id == "" {
		return d
	}
	switch v := d.(type) {
	case Commodity:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Open:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Close:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Transaction:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Price:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Balance:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Pad:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Note:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Event:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Custom:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	case Document:
		v.Metadata = metaWith(v.Metadata, IngestIDMetaKey, id)
		return v
	default:
		return d
	}
}

func metaWith(md Metadata, k, v string) Metadata {
	out := Metadata{}
	for key, val := range md {
		out[key] = val
	}
	out[k] = v
	return out
}

type Unknown struct {
	Meta
	Kind string
	Text string
}
