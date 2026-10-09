package controller

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"strings"

	_ "golang.org/x/image/webp"
)

const maxLogoImageBytes = 256 * 1024
const maxLogoEncodedBytes = ((maxLogoImageBytes + 2) / 3) * 4
const maxLogoDimension = 4096
const maxLogoPixels = 4096 * 4096

var logoDataURLPrefixes = map[string]string{
	"data:image/png;base64,":  "png",
	"data:image/jpeg;base64,": "jpeg",
	"data:image/webp;base64,": "webp",
	"data:image/gif;base64,":  "gif",
}

func validateLogoOption(value string) error {
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(strings.ToLower(value), "data:") {
		// Existing remote HTTP(S) URLs remain compatible.
		parsed, err := url.ParseRequestURI(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("logo must be an HTTP(S) URL or an uploaded image")
		}
		return nil
	}

	var declaredFormat string
	var encoded string
	for prefix, format := range logoDataURLPrefixes {
		if strings.HasPrefix(value, prefix) {
			declaredFormat = format
			encoded = strings.TrimPrefix(value, prefix)
			break
		}
	}
	if declaredFormat == "" {
		return fmt.Errorf("logo must be a PNG, JPEG, WebP, or GIF image")
	}
	if len(encoded) == 0 || len(encoded) > maxLogoEncodedBytes {
		return fmt.Errorf("logo image must be 256 KiB or smaller")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("logo contains invalid base64 data")
	}
	if len(decoded) == 0 || len(decoded) > maxLogoImageBytes {
		return fmt.Errorf("logo image must be 256 KiB or smaller")
	}

	config, detectedFormat, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil || detectedFormat != declaredFormat {
		return fmt.Errorf("logo image content does not match its declared format")
	}
	if config.Width < 1 || config.Height < 1 || config.Width > maxLogoDimension || config.Height > maxLogoDimension || int64(config.Width)*int64(config.Height) > maxLogoPixels {
		return fmt.Errorf("logo dimensions must not exceed 4096 x 4096 pixels")
	}
	if _, decodedFormat, err := image.Decode(bytes.NewReader(decoded)); err != nil || decodedFormat != declaredFormat {
		return fmt.Errorf("logo image cannot be decoded")
	}
	return nil
}
