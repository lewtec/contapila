package cmdpalette

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// Style emits package CSS. Safe on every page.
func Style() templ.Component {
	return rawBlock("style", "data-cmdpalette-css", commandCSS)
}

// Script emits the keyboard/filter runtime (JS self-guards double bind).
func Script() templ.Component {
	return rawBlock("script", "data-cmdpalette-js", commandJS)
}

// Assets emits CSS + JS. Prefer once per document in <head>.
func Assets() templ.Component {
	return templ.Join(Style(), Script())
}

// (p Palette) Assets is the same as package Assets — kept for method-call style.
func (p Palette) Assets() templ.Component {
	return Assets()
}

// rawBlock writes <tag attr>\nbody\n</tag> without HTML-escaping body.
func rawBlock(tag, attr, body string) templ.Component {
	open := "<" + tag + " " + attr + ">\n"
	close := "\n</" + tag + ">"
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, open); err != nil {
			return err
		}
		if _, err := io.WriteString(w, body); err != nil {
			return err
		}
		_, err := io.WriteString(w, close)
		return err
	})
}
