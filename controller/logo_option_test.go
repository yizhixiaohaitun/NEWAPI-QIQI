package controller

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func logoDataURL(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func encodedTestImage(t *testing.T, format string, width int, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.White)
	var output bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&output, img)
	case "jpeg":
		err = jpeg.Encode(&output, img, nil)
	case "gif":
		err = gif.Encode(&output, img, nil)
	}
	require.NoError(t, err)
	return output.Bytes()
}

func TestValidateLogoOption(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		require.NoError(t, validateLogoOption(logoDataURL("image/"+format, encodedTestImage(t, format, 1, 1))), format)
	}
	webp, err := base64.StdEncoding.DecodeString("UklGRh4AAABXRUJQVlA4TBEAAAAvAAAAAAfQ//73v/+BiOh/AAA=")
	require.NoError(t, err)
	require.NoError(t, validateLogoOption(logoDataURL("image/webp", webp)))

	for _, legacy := range []string{"", "https://example.com/logo.png", "http://example.com/legacy.svg"} {
		require.NoError(t, validateLogoOption(legacy), legacy)
	}
	require.Error(t, validateLogoOption("javascript:alert(1)"))
	require.Error(t, validateLogoOption("not a URL"))
}

func TestValidateLogoOptionRejectsUnsafeTruncatedOrInvalidData(t *testing.T) {
	validPNG := encodedTestImage(t, "png", 2, 2)
	invalid := []string{
		logoDataURL("image/svg+xml", []byte("<svg></svg>")),
		logoDataURL("text/html", []byte("<html></html>")),
		logoDataURL("image/png", []byte("\x89PNG\r\n\x1a\n")),
		logoDataURL("image/png", validPNG[:len(validPNG)-12]),
		logoDataURL("image/jpeg", []byte{0xff, 0xd8, 0xff}),
		logoDataURL("image/gif", []byte("GIF89a")),
		"data:image/png;base64,%%%",
		"data:image/png;base64,",
	}
	for _, value := range invalid {
		require.Error(t, validateLogoOption(value), value)
	}
}

func TestValidateLogoOptionSizeAndDimensionBoundaries(t *testing.T) {
	validPrefix := encodedTestImage(t, "png", 1, 1)
	exact := append(append([]byte{}, validPrefix...), make([]byte, maxLogoImageBytes-len(validPrefix))...)
	// A padded, decodable image proves base64 padding does not falsely reject 256 KiB.
	require.NoError(t, validateLogoOption(logoDataURL("image/png", exact)))

	over := append(exact, 0)
	err := validateLogoOption(logoDataURL("image/png", over))
	require.Error(t, err)
	require.Contains(t, err.Error(), "256 KiB")

	tooWide := encodedTestImage(t, "png", maxLogoDimension+1, 1)
	err = validateLogoOption(logoDataURL("image/png", tooWide))
	require.Error(t, err)
	require.Contains(t, err.Error(), "4096 x 4096")
}

func TestValidateLogoOptionRejectsMismatchedDeclaredType(t *testing.T) {
	value := logoDataURL("image/jpeg", encodedTestImage(t, "png", 1, 1))
	err := validateLogoOption(value)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "declared format"))
}
