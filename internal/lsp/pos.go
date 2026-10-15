package lsp

import (
	"unicode/utf16"
	"unicode/utf8"

	"github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar"
	"go.lsp.dev/protocol"
)

// lineStartByte returns the byte offset of the start of 1-based line1.
func lineStartByte(text string, line1 int) int {
	off := 0
	for cur := 1; off < len(text) && cur < line1; off++ {
		if text[off] == '\n' {
			cur++
		}
	}
	return off
}

// byteOffset returns the UTF-8 byte offset for an LSP position (UTF-16 columns).
func byteOffset(text string, pos protocol.Position) int {
	line := max(0, int(pos.Line))
	char := max(0, int(pos.Character))
	off := lineStartByte(text, line+1)
	// Walk UTF-16 code units on this line.
	u16 := 0
	for off < len(text) && u16 < char {
		r, size := utf8.DecodeRuneInString(text[off:])
		if r == '\n' {
			break
		}
		off += size
		if r == utf8.RuneError && size == 1 {
			u16++
			continue
		}
		u16 += utf16.RuneLen(r)
	}
	return off
}

// rangeFromBytes maps exclusive-end byte span to an LSP Range (UTF-16).
func rangeFromBytes(text string, start, end int, lines *grammar.LineIndex) protocol.Range {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if end > len(text) {
		end = len(text)
	}
	if lines == nil {
		lines = grammar.NewLineIndex(text)
	}
	sl, sc := lines.LineColumnAt(start)
	el, ec := lines.LineColumnAt(end)
	// LineIndex columns are 0-based *byte* offsets on the line; convert to UTF-16.
	return protocol.Range{
		Start: protocol.Position{Line: uint32(max(0, sl-1)), Character: uint32(byteColToUTF16(text, sl, sc))},
		End:   protocol.Position{Line: uint32(max(0, el-1)), Character: uint32(byteColToUTF16(text, el, ec))},
	}
}

// lineRange is a whole-line range (1-based line).
func lineRange(text string, line1 int) protocol.Range {
	lines := grammar.NewLineIndex(text)
	start := lineStartByte(text, max(1, line1))
	end := start
	for end < len(text) && text[end] != '\n' {
		end++
	}
	return rangeFromBytes(text, start, end, lines)
}

func byteColToUTF16(text string, line1, byteCol int) int {
	off := lineStartByte(text, max(1, line1))
	target := min(len(text), off+byteCol)
	u16 := 0
	for off < target {
		r, size := utf8.DecodeRuneInString(text[off:])
		if r == '\n' {
			break
		}
		off += size
		if r == utf8.RuneError && size == 1 {
			u16++
			continue
		}
		u16 += utf16.RuneLen(r)
	}
	return u16
}
