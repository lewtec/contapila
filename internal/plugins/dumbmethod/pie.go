package dumbmethod

import (
	"fmt"
	"math"

	"github.com/lucasew/contapila-go/internal/web"
)

// DumbMethodDonutRadius is the r used by dmDonut in body.templ. Segment
// dash lengths are expressed against this circle's circumference.
const DumbMethodDonutRadius = 48

// pieColors is a fixed oklch palette. Lightness mid-range keeps every slice
// readable on both the light and the dark daisyUI theme.
var pieColors = []string{
	"oklch(0.62 0.17 25)",  // red
	"oklch(0.64 0.14 80)",  // gold
	"oklch(0.62 0.15 145)", // green
	"oklch(0.62 0.13 195)", // cyan
	"oklch(0.62 0.16 250)", // blue
	"oklch(0.62 0.16 300)", // violet
	"oklch(0.63 0.16 335)", // pink
	"oklch(0.66 0.14 55)",  // orange
}

func pieColor(i int) string {
	return pieColors[i%len(pieColors)]
}

// classSlices turns per-class shares into donut geometry. Zero and
// near-zero shares are skipped so they do not paint a seam.
func classSlices(rows []web.DumbMethodClassRow, share func(int) float64) []web.DumbMethodSlice {
	circumference := 2 * math.Pi * DumbMethodDonutRadius
	var out []web.DumbMethodSlice
	var cumulative float64
	for i, row := range rows {
		f := share(i)
		if f <= 0 {
			continue
		}
		dash := f * circumference
		gap := circumference - dash
		if f >= 0.99999 {
			gap = 0
		}
		out = append(out, web.DumbMethodSlice{
			Name:   row.Name,
			Pct:    fmt.Sprintf("%.2f%%", f*100),
			Color:  row.Color,
			Dash:   fmt.Sprintf("%.3f %.3f", dash, gap),
			Offset: fmt.Sprintf("%.3f", -cumulative*circumference),
		})
		cumulative += f
	}
	return out
}
