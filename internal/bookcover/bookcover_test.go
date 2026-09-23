package bookcover

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestNormalizeJPEG(t *testing.T) {
	raw := encodeJPEG(t, solidImage(1200, 1800, color.RGBA{R: 220, G: 40, B: 30, A: 255}))

	normalized, mediaType, width, height, hash, err := Normalize(raw, "image/jpeg")
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if mediaType != "image/jpeg" || width != 600 || height != 900 {
		t.Fatalf("Normalize() metadata = (%q, %d, %d), want (image/jpeg, 600, 900)", mediaType, width, height)
	}
	if !bytes.HasPrefix(normalized, []byte{0xff, 0xd8, 0xff}) {
		t.Fatal("normalized JPEG does not have JPEG magic bytes")
	}
	if len(hash) != len("sha256:")+64 || hash[:len("sha256:")] != "sha256:" {
		t.Fatalf("content hash = %q, want sha256-prefixed digest", hash)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(normalized))
	if err != nil {
		t.Fatalf("decode normalized JPEG: %v", err)
	}
	if got := decoded.Bounds().Size(); got != image.Pt(600, 900) {
		t.Fatalf("normalized JPEG size = %v, want 600x900", got)
	}
}

func TestNormalizePNGPreservesAspectRatioAndDoesNotUpscale(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantWidth     int
		wantHeight    int
	}{
		{name: "resize", width: 1000, height: 1000, wantWidth: 600, wantHeight: 600},
		{name: "no upscale", width: 40, height: 60, wantWidth: 40, wantHeight: 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := encodePNG(t, solidImage(tt.width, tt.height, color.NRGBA{R: 30, G: 100, B: 220, A: 180}))
			normalized, mediaType, width, height, _, err := Normalize(raw, "image/png")
			if err != nil {
				t.Fatalf("Normalize() error = %v", err)
			}
			if mediaType != "image/png" || width != tt.wantWidth || height != tt.wantHeight {
				t.Fatalf("Normalize() metadata = (%q, %d, %d), want (image/png, %d, %d)", mediaType, width, height, tt.wantWidth, tt.wantHeight)
			}
			if !bytes.HasPrefix(normalized, pngSignature[:]) {
				t.Fatal("normalized PNG does not have PNG magic bytes")
			}
		})
	}
}

func TestNormalizeRejectsInvalidCover(t *testing.T) {
	validPNG := encodePNG(t, solidImage(2, 3, color.NRGBA{B: 255, A: 255}))
	animatedPNG := insertPNGChunk(validPNG, []byte("acTL"), make([]byte, 8))
	overlargePNG := pngWithDimensions(40_000_001, 1)

	tests := []struct {
		name       string
		raw        []byte
		advertised string
		wantErr    error
	}{
		{name: "empty", raw: nil, advertised: "image/png", wantErr: ErrEmpty},
		{name: "svg", raw: []byte("<svg></svg>"), advertised: "image/svg+xml", wantErr: ErrUnsupported},
		{name: "unsupported magic", raw: []byte("GIF89a"), advertised: "image/gif", wantErr: ErrUnsupported},
		{name: "malformed jpeg", raw: []byte{0xff, 0xd8, 0xff}, advertised: "image/jpeg", wantErr: ErrMalformed},
		{name: "advertised mismatch", raw: validPNG, advertised: "image/jpeg", wantErr: ErrMediaTypeMismatch},
		{name: "animated png", raw: animatedPNG, advertised: "image/png", wantErr: ErrAnimated},
		{name: "too many pixels", raw: overlargePNG, advertised: "image/png", wantErr: ErrDimensionsTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, _, _, err := Normalize(tt.raw, tt.advertised)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Normalize() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func solidImage(width, height int, pixel color.Color) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, pixel)
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode JPEG fixture: %v", err)
	}
	return out.Bytes()
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode PNG fixture: %v", err)
	}
	return out.Bytes()
}

func insertPNGChunk(raw, chunkType, data []byte) []byte {
	firstChunkLength := 8 + 4 + 4 + 13 + 4
	chunk := pngChunk(chunkType, data)
	return append(append(append([]byte(nil), raw[:firstChunkLength]...), chunk...), raw[firstChunkLength:]...)
}

func pngWithDimensions(width, height uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 6
	result := append(pngSignature[:], pngChunk([]byte("IHDR"), ihdr)...)
	return append(result, pngChunk([]byte("IEND"), nil)...)
}

func pngChunk(chunkType, data []byte) []byte {
	var out bytes.Buffer
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data))) //nolint:gosec // test chunks are deliberately small.
	out.Write(length[:])
	out.Write(chunkType)
	out.Write(data)
	checksum := crc32.ChecksumIEEE(append(append([]byte(nil), chunkType...), data...))
	binary.BigEndian.PutUint32(length[:], checksum)
	out.Write(length[:])
	return out.Bytes()
}
