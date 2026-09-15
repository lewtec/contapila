package ast

import (
	"math/big"
	"reflect"
	"testing"
)

func TestDirectiveMetadata(t *testing.T) {
	md := Metadata{"asset-class": "cash", "institution": "Bank"}

	tests := []struct {
		name string
		d    Directive
		want Metadata
	}{
		{
			name: "open with metadata",
			d:    Open{Account: "Assets:Cash", Metadata: md},
			want: md,
		},
		{
			name: "open nil metadata",
			d:    Open{Account: "Assets:Cash"},
			want: nil,
		},
		{
			name: "transaction with metadata",
			d: Transaction{
				Flag:      "*",
				Narration: "coffee",
				Metadata:  md,
			},
			want: md,
		},
		{
			name: "transaction nil metadata",
			d:    Transaction{Flag: "*", Narration: "x"},
			want: nil,
		},
		{
			name: "commodity",
			d:    Commodity{Currency: "BRL", Metadata: md},
			want: md,
		},
		{
			name: "close",
			d:    Close{Account: "Assets:Old", Metadata: md},
			want: md,
		},
		{
			name: "price",
			d:    Price{Currency: "USD", Metadata: md},
			want: md,
		},
		{
			name: "balance",
			d:    Balance{Account: "Assets:Cash", Metadata: md},
			want: md,
		},
		{
			name: "pad",
			d:    Pad{Account: "A", FromAccount: "B", Metadata: md},
			want: md,
		},
		{
			name: "note",
			d:    Note{Account: "Assets:Cash", Comment: "hi", Metadata: md},
			want: md,
		},
		{
			name: "event",
			d:    Event{Type: "location", Desc: "home", Metadata: md},
			want: md,
		},
		{
			name: "custom",
			d:    Custom{Type: "index", Metadata: md},
			want: md,
		},
		{
			name: "document",
			d:    Document{Account: "Assets:Cash", Path: "x.txt", Metadata: md},
			want: md,
		},
		{
			name: "option has no metadata field",
			d:    Option{Key: "title", Value: "Books"},
			want: nil,
		},
		{
			name: "include has no metadata field",
			d:    Include{Path: "other.beancount"},
			want: nil,
		},
		{
			name: "unknown has no metadata field",
			d:    Unknown{Kind: "plugin", Text: "plugin \"x\""},
			want: nil,
		},
		{
			name: "nil directive",
			d:    nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DirectiveMetadata(tt.d)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DirectiveMetadata() = %#v; want %#v", got, tt.want)
			}
		})
	}
}

func TestWithIngestID(t *testing.T) {
	existing := Metadata{"payee": "Cafe", "note": "lunch"}

	tests := []struct {
		name      string
		d         Directive
		id        string
		wantID    string
		wantOther Metadata // keys other than ingest_id that must remain
	}{
		{
			name:      "transaction sets ingest_id and preserves other meta",
			d:         Transaction{Flag: "*", Narration: "buy", Metadata: existing},
			id:        "evt-42",
			wantID:    "evt-42",
			wantOther: existing,
		},
		{
			name:      "open on nil metadata creates map with ingest_id",
			d:         Open{Account: "Assets:Cash"},
			id:        "open-1",
			wantID:    "open-1",
			wantOther: nil,
		},
		{
			name:      "empty id leaves directive unchanged",
			d:         Transaction{Flag: "*", Narration: "x", Metadata: existing},
			id:        "",
			wantID:    "",
			wantOther: existing,
		},
		{
			name:      "overwrites existing ingest_id",
			d:         Balance{Account: "Assets:Cash", Metadata: Metadata{IngestIDMetaKey: "old", "k": "v"}},
			id:        "new",
			wantID:    "new",
			wantOther: Metadata{"k": "v"},
		},
		{
			name:   "unknown directive is a no-op",
			d:      Unknown{Kind: "plugin", Text: "plugin \"x\""},
			id:     "nope",
			wantID: "",
		},
		{
			name:      "commodity",
			d:         Commodity{Currency: "USD", Metadata: existing},
			id:        "c-1",
			wantID:    "c-1",
			wantOther: existing,
		},
		{
			name:      "close",
			d:         Close{Account: "Assets:Old", Metadata: existing},
			id:        "cl-1",
			wantID:    "cl-1",
			wantOther: existing,
		},
		{
			name:      "price",
			d:         Price{Currency: "EUR", Metadata: existing},
			id:        "p-1",
			wantID:    "p-1",
			wantOther: existing,
		},
		{
			name:      "pad",
			d:         Pad{Account: "A", FromAccount: "B", Metadata: existing},
			id:        "pad-1",
			wantID:    "pad-1",
			wantOther: existing,
		},
		{
			name:      "note",
			d:         Note{Account: "A", Comment: "c", Metadata: existing},
			id:        "n-1",
			wantID:    "n-1",
			wantOther: existing,
		},
		{
			name:      "event",
			d:         Event{Type: "location", Desc: "office", Metadata: existing},
			id:        "e-1",
			wantID:    "e-1",
			wantOther: existing,
		},
		{
			name:      "custom",
			d:         Custom{Type: "index", Metadata: existing},
			id:        "cu-1",
			wantID:    "cu-1",
			wantOther: existing,
		},
		{
			name:      "document",
			d:         Document{Account: "A", Path: "x.txt", Metadata: existing},
			id:        "d-1",
			wantID:    "d-1",
			wantOther: existing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Snapshot original metadata so we can assert non-mutation of the input map.
			var origCopy Metadata
			if md := DirectiveMetadata(tt.d); md != nil {
				origCopy = Metadata{}
				for k, v := range md {
					origCopy[k] = v
				}
			}

			got := WithIngestID(tt.d, tt.id)
			gotMD := DirectiveMetadata(got)

			if tt.id == "" {
				if !reflect.DeepEqual(got, tt.d) {
					t.Errorf("WithIngestID empty id: got %#v; want original %#v", got, tt.d)
				}
				return
			}

			if tt.wantID == "" {
				// Types without metadata (Unknown, etc.): result should equal input.
				if !reflect.DeepEqual(got, tt.d) {
					t.Errorf("WithIngestID on non-meta directive: got %#v; want %#v", got, tt.d)
				}
				return
			}

			if gotMD == nil {
				t.Fatal("WithIngestID() metadata is nil; want map with ingest_id")
			}
			if gotMD[IngestIDMetaKey] != tt.wantID {
				t.Errorf("ingest_id = %q; want %q", gotMD[IngestIDMetaKey], tt.wantID)
			}
			for k, v := range tt.wantOther {
				if k == IngestIDMetaKey {
					continue
				}
				if gotMD[k] != v {
					t.Errorf("preserved meta[%q] = %q; want %q", k, gotMD[k], v)
				}
			}

			// metaWith must copy: mutating the result must not change the original map.
			if orig := DirectiveMetadata(tt.d); orig != nil {
				gotMD[IngestIDMetaKey] = "mutated"
				if !reflect.DeepEqual(orig, origCopy) {
					t.Errorf("WithIngestID mutated original metadata: got %#v; want %#v", orig, origCopy)
				}
			}
		})
	}
}

func TestWithIngestID_keyConstant(t *testing.T) {
	t.Parallel()
	if IngestIDMetaKey != "ingest_id" {
		t.Errorf("IngestIDMetaKey = %q; want %q", IngestIDMetaKey, "ingest_id")
	}
}

func TestMetadataClone(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		var m Metadata
		if got := m.Clone(); got != nil {
			t.Errorf("nil.Clone() = %#v; want nil", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		m := Metadata{}
		if got := m.Clone(); got != nil {
			t.Errorf("empty.Clone() = %#v; want nil", got)
		}
	})

	t.Run("independent copy", func(t *testing.T) {
		t.Parallel()
		src := Metadata{"asset-class": "cash", "institution": "Bank"}
		cl := src.Clone()
		if cl == nil {
			t.Fatal("Clone() is nil; want non-empty map")
		}
		if !reflect.DeepEqual(cl, src) {
			t.Errorf("Clone() = %#v; want %#v", cl, src)
		}
		// Same contents, different map: mutating clone must not affect source.
		cl["asset-class"] = "equity"
		cl["new"] = "x"
		if src["asset-class"] != "cash" {
			t.Errorf("mutating clone changed source asset-class: got %q", src["asset-class"])
		}
		if _, ok := src["new"]; ok {
			t.Error("mutating clone added key to source")
		}
	})
}

func TestAmountClone(t *testing.T) {
	tests := []struct {
		name string
		src  Amount
	}{
		{
			name: "nil number keeps commodity",
			src:  Amount{Number: nil, Commodity: "BRL"},
		},
		{
			name: "non-nil number is deep-copied",
			src:  Amount{Number: big.NewRat(10, 3), Commodity: "USD"},
		},
		{
			name: "zero rat",
			src:  Amount{Number: big.NewRat(0, 1), Commodity: "EUR"},
		},
		{
			name: "empty commodity",
			src:  Amount{Number: big.NewRat(1, 1), Commodity: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cl := tt.src.Clone()

			if cl.Commodity != tt.src.Commodity {
				t.Errorf("Commodity = %q; want %q", cl.Commodity, tt.src.Commodity)
			}

			if tt.src.Number == nil {
				if cl.Number != nil {
					t.Errorf("Number = %v; want nil", cl.Number)
				}
				return
			}

			if cl.Number == nil {
				t.Fatal("Number is nil; want cloned *big.Rat")
			}
			if cl.Number == tt.src.Number {
				t.Fatal("Clone returned same *big.Rat pointer; want independent copy")
			}
			if cl.Number.Cmp(tt.src.Number) != 0 {
				t.Errorf("Number value = %s; want %s", cl.Number.String(), tt.src.Number.String())
			}

			// Mutating the clone must not affect the source.
			before := new(big.Rat).Set(tt.src.Number)
			cl.Number.Add(cl.Number, big.NewRat(1, 1))
			if tt.src.Number.Cmp(before) != 0 {
				t.Errorf("mutating clone changed source: got %s; want %s", tt.src.Number.String(), before.String())
			}
		})
	}
}
