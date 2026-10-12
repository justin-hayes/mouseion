package bookcover

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"mime"
	"strings"

	"golang.org/x/image/draw"
)

const (
	maxWidth  = 600
	maxHeight = 900
	maxPixels = 40_000_000
)

var (
	ErrEmpty              = errors.New("book cover is empty")
	ErrUnsupported        = errors.New("unsupported book cover image")
	ErrMalformed          = errors.New("malformed book cover image")
	ErrAnimated           = errors.New("animated book cover image")
	ErrMediaTypeMismatch  = errors.New("advertised and decoded media types differ")
	ErrDimensionsTooLarge = errors.New("book cover dimensions exceed 40 megapixels")
)

var pngSignature = [...]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// Normalize validates and converts a catalog-supplied cover into one bounded,
// aspect-preserving display raster. Every non-empty advertised media type must
// agree with the decoded raster. The caller is responsible for bounding the
// input before calling this function.
func Normalize(raw []byte, advertisedTypes ...string) ([]byte, string, int, int, string, error) {
	if len(raw) == 0 {
		return nil, "", 0, 0, "", ErrEmpty
	}

	decoded, err := sniff(raw)
	if err != nil {
		return nil, "", 0, 0, "", err
	}
	if err := checkMediaTypeClaims(decoded, advertisedTypes); err != nil {
		return nil, "", 0, 0, "", err
	}
	if err := rejectAnimated(raw, decoded); err != nil {
		return nil, "", 0, 0, "", err
	}

	config, err := checkCoverLimits(raw, decoded)
	if err != nil {
		return nil, "", 0, 0, "", err
	}
	source, err := decodeCover(raw, decoded)
	if err != nil {
		return nil, "", 0, 0, "", err
	}

	width, height := targetSize(config.Width, config.Height)
	if width != config.Width || height != config.Height {
		source = resize(source, width, height)
	}

	normalizedBytes, err := encodeCover(source, decoded)
	if err != nil {
		return nil, "", 0, 0, "", err
	}
	digest := sha256.Sum256(normalizedBytes)
	return normalizedBytes, decoded, width, height, "sha256:" + hex.EncodeToString(digest[:]), nil
}

// checkMediaTypeClaims rejects any advertised media type that disagrees with
// the decoded raster. Empty claims are ignored.
func checkMediaTypeClaims(decoded string, advertisedTypes []string) error {
	for _, advertisedType := range advertisedTypes {
		advertised, err := normalizedAdvertisedType(advertisedType)
		if err != nil {
			return err
		}
		if advertised != "" && advertised != decoded {
			return fmt.Errorf("%w: advertised %q, decoded %q", ErrMediaTypeMismatch, advertised, decoded)
		}
	}
	return nil
}

// rejectAnimated refuses an animated PNG before the far larger full decode.
func rejectAnimated(raw []byte, decoded string) error {
	if decoded != "image/png" {
		return nil
	}
	animated, err := pngIsAnimated(raw)
	if err != nil {
		return err
	}
	if animated {
		return ErrAnimated
	}
	return nil
}

// checkCoverLimits validates the header-declared media type and dimensions
// without decoding the full raster.
func checkCoverLimits(raw []byte, decoded string) (image.Config, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return image.Config{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if formatMediaType(format) != decoded {
		return image.Config{}, ErrMediaTypeMismatch
	}
	if config.Width <= 0 || config.Height <= 0 {
		return image.Config{}, ErrMalformed
	}
	if int64(config.Width) > maxPixels/int64(config.Height) {
		return image.Config{}, ErrDimensionsTooLarge
	}
	return config, nil
}

// decodeCover decodes the full raster and rechecks its media type.
func decodeCover(raw []byte, decoded string) (image.Image, error) {
	source, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if formatMediaType(format) != decoded {
		return nil, ErrMediaTypeMismatch
	}
	return source, nil
}

// encodeCover re-encodes the display raster in the decoded format.
func encodeCover(source image.Image, decoded string) ([]byte, error) {
	var normalized bytes.Buffer
	var err error
	switch decoded {
	case "image/jpeg":
		err = jpeg.Encode(&normalized, source, &jpeg.Options{Quality: 90})
	case "image/png":
		err = png.Encode(&normalized, source)
	default:
		return nil, ErrUnsupported
	}
	if err != nil {
		return nil, fmt.Errorf("encode normalized cover: %w", err)
	}
	return normalized.Bytes(), nil
}

func normalizedAdvertisedType(advertised string) (string, error) {
	if strings.TrimSpace(advertised) == "" {
		return "", nil
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(advertised))
	if err != nil {
		return "", fmt.Errorf("%w: invalid advertised media type", ErrUnsupported)
	}
	mediaType = strings.ToLower(mediaType)
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		return "", fmt.Errorf("%w: %s", ErrUnsupported, mediaType)
	}
	return mediaType, nil
}

func sniff(raw []byte) (string, error) {
	if len(raw) >= len(pngSignature) && bytes.Equal(raw[:len(pngSignature)], pngSignature[:]) {
		return "image/png", nil
	}
	if len(raw) >= 3 && raw[0] == 0xff && raw[1] == 0xd8 && raw[2] == 0xff {
		return "image/jpeg", nil
	}
	return "", ErrUnsupported
}

func formatMediaType(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	default:
		return ""
	}
}

func pngIsAnimated(raw []byte) (bool, error) {
	offset := len(pngSignature)
	for offset < len(raw) {
		if len(raw)-offset < 12 {
			return false, ErrMalformed
		}
		chunkLength := binary.BigEndian.Uint32(raw[offset : offset+4])
		if int64(chunkLength) > int64(len(raw)-offset-12) {
			return false, ErrMalformed
		}
		chunkType := raw[offset+4 : offset+8]
		if bytes.Equal(chunkType, []byte("acTL")) || bytes.Equal(chunkType, []byte("fcTL")) {
			return true, nil
		}

		offset += 12 + int(chunkLength)
		if bytes.Equal(chunkType, []byte("IEND")) {
			if offset != len(raw) {
				return false, ErrMalformed
			}
			return false, nil
		}
	}
	return false, ErrMalformed
}

func targetSize(width, height int) (int, int) {
	if width <= maxWidth && height <= maxHeight {
		return width, height
	}

	if int64(width)*maxHeight >= int64(height)*maxWidth {
		return maxWidth, roundedRatio(height, maxWidth, width)
	}
	return roundedRatio(width, maxHeight, height), maxHeight
}

func roundedRatio(value, numerator, denominator int) int {
	result := int((int64(value)*int64(numerator) + int64(denominator)/2) / int64(denominator))
	if result < 1 {
		return 1
	}
	return result
}

func resize(source image.Image, width, height int) image.Image {
	destination := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(destination, destination.Bounds(), source, source.Bounds(), draw.Src, nil)
	return destination
}
