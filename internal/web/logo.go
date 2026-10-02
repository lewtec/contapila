package web

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// Logo is the contapila mark. The folder window uses it so the
// default LEWTEC lockup stays off that screen.
func Logo() (image.Image, error) {
	raw, err := staticFS.ReadFile("static/logo.png")
	if err != nil {
		return nil, fmt.Errorf("logo: %w", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("logo: %w", err)
	}
	return img, nil
}
