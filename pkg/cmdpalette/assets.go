package cmdpalette

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// Style emits package CSS. Safe on every page.
func Style() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<style data-cmdpalette-css>\n")
		if err != nil {
			return err
		}
		if _, err := io.WriteString(w, commandCSS); err != nil {
			return err
		}
		_, err = io.WriteString(w, "\n</style>")
		return err
	})
}

// Script emits the keyboard/filter runtime (JS self-guards double bind).
func Script() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<script data-cmdpalette-js>\n")
		if err != nil {
			return err
		}
		if _, err := io.WriteString(w, commandJS); err != nil {
			return err
		}
		_, err = io.WriteString(w, "\n</script>")
		return err
	})
}

// Assets emits CSS + JS. Prefer once per document in <head>.
func Assets() templ.Component {
	return templ.Join(Style(), Script())
}

// (p Palette) Assets is the same as package Assets — kept for method-call style.
func (p Palette) Assets() templ.Component {
	return Assets()
}
