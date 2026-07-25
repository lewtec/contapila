package web

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// chartJSONScript emits a pre-marshaled JSON payload script element.
// templ does not evaluate @/expressions inside <script> bodies, so this is written from Go.
func chartJSONScript(id, rawJSON string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<script type="application/json" id="`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(id)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, rawJSON); err != nil {
			return err
		}
		_, err := io.WriteString(w, `</script>`)
		return err
	})
}
