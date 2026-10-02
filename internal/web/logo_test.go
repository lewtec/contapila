package web

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogo(t *testing.T) {
	img, err := Logo()
	require.NoError(t, err)
	bounds := img.Bounds()
	require.Positive(t, bounds.Dx())
	require.Positive(t, bounds.Dy())
	_, _, _, alpha := img.At(bounds.Min.X, bounds.Min.Y).RGBA()
	require.Zero(t, alpha)
}
