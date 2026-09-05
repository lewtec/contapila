package lsp

import (
	"testing"
	"unicode/utf16"

	"go.lsp.dev/protocol"
)

func TestByteOffsetASCII(t *testing.T) {
	text := "ab\ncde\nf"
	// lines (0-based): "ab\n" | "cde\n" | "f"
	tests := []struct {
		name string
		pos  protocol.Position
		want int
	}{
		{name: "start", pos: protocol.Position{Line: 0, Character: 0}, want: 0},
		{name: "first line mid", pos: protocol.Position{Line: 0, Character: 1}, want: 1},
		{name: "first line end before nl", pos: protocol.Position{Line: 0, Character: 2}, want: 2},
		// past line content stops at newline
		{name: "first line past eol", pos: protocol.Position{Line: 0, Character: 99}, want: 2},
		{name: "second line start", pos: protocol.Position{Line: 1, Character: 0}, want: 3},
		{name: "second line mid", pos: protocol.Position{Line: 1, Character: 2}, want: 5},
		{name: "third line start", pos: protocol.Position{Line: 2, Character: 0}, want: 7},
		{name: "third line past end", pos: protocol.Position{Line: 2, Character: 5}, want: 8},
		{name: "line past eof", pos: protocol.Position{Line: 9, Character: 0}, want: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := byteOffset(text, tt.pos); got != tt.want {
				t.Errorf("byteOffset(%v) = %d; want %d", tt.pos, got, tt.want)
			}
		})
	}
}

func TestByteOffsetEmpty(t *testing.T) {
	if got := byteOffset("", protocol.Position{Line: 0, Character: 0}); got != 0 {
		t.Errorf("empty = %d; want 0", got)
	}
	if got := byteOffset("", protocol.Position{Line: 3, Character: 2}); got != 0 {
		t.Errorf("empty past = %d; want 0", got)
	}
}

func TestByteOffsetUTF16Surrogate(t *testing.T) {
	// 😀 is U+1F600: 4 UTF-8 bytes, 2 UTF-16 code units.
	emoji := "😀"
	if utf16.RuneLen([]rune(emoji)[0]) != 2 {
		t.Fatalf("precondition: emoji should be 2 UTF-16 units")
	}
	text := "a" + emoji + "b"
	// UTF-16 layout: a(1) + emoji(2) + b(1). Byte layout: a@0, emoji@1..4, b@5, end@6.
	// byteOffset consumes whole runes; a char in the middle of a surrogate pair
	// lands at the byte after that rune (cannot split a code point).
	tests := []struct {
		name string
		char uint32
		want int
	}{
		{name: "before emoji", char: 0, want: 0},
		{name: "at emoji high surrogate", char: 1, want: 1},
		{name: "low surrogate lands after emoji", char: 2, want: 5},
		{name: "after emoji", char: 3, want: 5},
		{name: "at b", char: 4, want: 6},
		{name: "past end", char: 99, want: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := byteOffset(text, protocol.Position{Line: 0, Character: tt.char})
			if got != tt.want {
				t.Errorf("byteOffset(char=%d) = %d; want %d (text=%q)", tt.char, got, tt.want, text)
			}
		})
	}
}

func TestRangeFromBytesClamps(t *testing.T) {
	text := "hi\n世界\n" // 世界 = 3+3 UTF-8 bytes, each 1 UTF-16 unit
	// bytes: h i \n 世(3) 界(3) \n  → len 10

	r := rangeFromBytes(text, 0, 2, nil)
	if r.Start.Line != 0 || r.Start.Character != 0 || r.End.Line != 0 || r.End.Character != 2 {
		t.Errorf("ASCII span = %+v; want L0C0–L0C2", r)
	}

	r = rangeFromBytes(text, -5, 1, nil)
	if r.Start.Line != 0 || r.Start.Character != 0 || r.End.Character != 1 {
		t.Errorf("clamp neg start = %+v; want L0C0–L0C1", r)
	}

	r = rangeFromBytes(text, 3, 1, nil) // end < start → end = start
	if r.Start != r.End {
		t.Errorf("end<start should collapse, got start=%+v end=%+v", r.Start, r.End)
	}

	// multi-byte: span covering 世 (bytes 3–6)
	r = rangeFromBytes(text, 3, 6, nil)
	if r.Start.Line != 1 || r.Start.Character != 0 {
		t.Errorf("世 start = L%d C%d; want L1 C0", r.Start.Line, r.Start.Character)
	}
	if r.End.Line != 1 || r.End.Character != 1 {
		t.Errorf("世 end = L%d C%d; want L1 C1", r.End.Line, r.End.Character)
	}

	// full 世界 (bytes 3–9)
	r = rangeFromBytes(text, 3, 9, nil)
	if r.Start.Line != 1 || r.Start.Character != 0 || r.End.Line != 1 || r.End.Character != 2 {
		t.Errorf("世界 span = %+v; want L1C0–L1C2", r)
	}

	// end past len clamps to len(text). With a trailing newline and no empty
	// line after it, LineIndex maps EOF onto the last line and byteColToUTF16
	// stops at '\n', so the exclusive end round-trips to the final newline byte.
	r = rangeFromBytes(text, 0, 999, nil)
	gotEnd := byteOffset(text, r.End)
	if gotEnd != len(text)-1 { // index of final '\n'
		t.Errorf("end past len → byteOffset(end)=%d; want %d (range=%+v)", gotEnd, len(text)-1, r)
	}
	if r.End.Line != 1 || r.End.Character != 2 {
		t.Errorf("end past len range end = L%d C%d; want L1 C2", r.End.Line, r.End.Character)
	}
}

func TestLineRange(t *testing.T) {
	text := "hi\n世界\n"

	lr := lineRange(text, 1)
	if lr.Start.Line != 0 || lr.Start.Character != 0 || lr.End.Line != 0 || lr.End.Character != 2 {
		t.Errorf("lineRange(1) = %+v; want L0C0–L0C2 (\"hi\")", lr)
	}

	lr = lineRange(text, 2)
	if lr.Start.Line != 1 || lr.Start.Character != 0 || lr.End.Line != 1 || lr.End.Character != 2 {
		t.Errorf("lineRange(2) = %+v; want L1C0–L1C2 (\"世界\")", lr)
	}

	// line < 1 clamps to line 1
	lr = lineRange(text, 0)
	if lr.Start.Line != 0 || lr.Start.Character != 0 || lr.End.Character != 2 {
		t.Errorf("lineRange(0) = %+v; want same as line 1", lr)
	}

	// line past last content: scan reaches EOF (start==len), then rangeFromBytes
	// maps that offset onto the trailing-newline column on the last line.
	lr = lineRange(text, 99)
	if lr.Start != lr.End {
		t.Errorf("lineRange(99) should be empty, got %+v", lr)
	}
	if got := byteOffset(text, lr.Start); got != len(text)-1 {
		t.Errorf("lineRange(99) start byte=%d; want %d (final '\\n', range=%+v)", got, len(text)-1, lr)
	}
}

func TestByteColToUTF16(t *testing.T) {
	// line 2 (1-based): €😀y — € is 3 UTF-8 / 1 UTF-16; 😀 is 4 UTF-8 / 2 UTF-16
	text := "x\n€😀y\n"
	tests := []struct {
		name       string
		line1, col int
		want       int
	}{
		{name: "col0", line1: 2, col: 0, want: 0},
		{name: "at emoji start", line1: 2, col: 3, want: 1},
		{name: "at y", line1: 2, col: 7, want: 3}, // €(1)+😀(2)
		{name: "at nl", line1: 2, col: 8, want: 4},
		{name: "line clamps to 1", line1: 0, col: 0, want: 0}, // line "x"
		{name: "overshoot col", line1: 2, col: 999, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := byteColToUTF16(text, tt.line1, tt.col); got != tt.want {
				t.Errorf("byteColToUTF16(line=%d,col=%d) = %d; want %d", tt.line1, tt.col, got, tt.want)
			}
		})
	}
}

func TestByteOffsetRoundTripWithRange(t *testing.T) {
	// For pure ASCII (no exclusive end past a trailing newline), rangeFromBytes
	// endpoints convert back via byteOffset.
	text := "Assets:Cash  10 BRL\n  Equity:Open  -10 BRL\n"
	for _, span := range [][2]int{
		{0, 11},  // "Assets:Cash"
		{13, 15}, // "10"
		{21, 32}, // "Equity:Open"
		{0, 19},  // first line without '\n'
		{20, 42}, // second line content through final char before '\n'
	} {
		r := rangeFromBytes(text, span[0], span[1], nil)
		gotStart := byteOffset(text, r.Start)
		gotEnd := byteOffset(text, r.End)
		if gotStart != span[0] || gotEnd != span[1] {
			t.Errorf("round-trip span %v → range %+v → [%d,%d)", span, r, gotStart, gotEnd)
		}
	}
}

func TestMax(t *testing.T) {
	if max(1, 2) != 2 || max(3, 1) != 3 || max(0, 0) != 0 {
		t.Fatal("max broken")
	}
}
