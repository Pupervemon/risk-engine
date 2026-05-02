package imageadapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestExternalImageFetcherFetchSingleImageFromRelativeURL(t *testing.T) {
	sourceImage := mustEncodeJPEG(t, newTestImage(640, 360))

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "200",
			"data": map[string]any{
				"imgurl": "/images/source.jpg",
			},
		})
	})
	mux.HandleFunc("/images/source.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(sourceImage)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	fetcher := NewExternalImageFetcher(ExternalImageAPIConfig{
		URL:                server.URL + "/api",
		Timeout:            5 * time.Second,
		RateLimitPerMinute: 60,
	}, zap.NewNop(), 320, 180)

	meta, err := fetcher.fetchSingleImage(context.Background())
	if err != nil {
		t.Fatalf("fetchSingleImage returned error: %v", err)
	}

	if meta.URL != server.URL+"/images/source.jpg" {
		t.Fatalf("unexpected image URL: %q", meta.URL)
	}

	format, width, height := mustDecodeImageConfig(t, meta.Data)
	if format != "png" {
		t.Fatalf("expected png output, got %q", format)
	}
	if width != 320 || height != 180 {
		t.Fatalf("unexpected output size: %dx%d", width, height)
	}
}

func TestExternalImageFetcherFetchSingleImageFromRawBase64JSON(t *testing.T) {
	sourceImage := mustEncodePNG(t, newTestImage(400, 220))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": base64.StdEncoding.EncodeToString(sourceImage),
		})
	}))
	defer server.Close()

	fetcher := NewExternalImageFetcher(ExternalImageAPIConfig{
		URL:                server.URL,
		Timeout:            5 * time.Second,
		RateLimitPerMinute: 60,
	}, zap.NewNop(), 320, 180)

	meta, err := fetcher.fetchSingleImage(context.Background())
	if err != nil {
		t.Fatalf("fetchSingleImage returned error: %v", err)
	}

	if !strings.HasSuffix(meta.URL, "#inline") {
		t.Fatalf("expected inline source marker, got %q", meta.URL)
	}

	format, width, height := mustDecodeImageConfig(t, meta.Data)
	if format != "png" {
		t.Fatalf("expected png output, got %q", format)
	}
	if width != 320 || height != 180 {
		t.Fatalf("unexpected output size: %dx%d", width, height)
	}
}

func TestExternalImageFetcherFetchSingleImageFromDataURIJSON(t *testing.T) {
	sourceImage := mustEncodePNG(t, newTestImage(360, 360))
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(sourceImage)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"image": dataURI,
			},
		})
	}))
	defer server.Close()

	fetcher := NewExternalImageFetcher(ExternalImageAPIConfig{
		URL:                server.URL,
		Timeout:            5 * time.Second,
		RateLimitPerMinute: 60,
	}, zap.NewNop(), 320, 180)

	meta, err := fetcher.fetchSingleImage(context.Background())
	if err != nil {
		t.Fatalf("fetchSingleImage returned error: %v", err)
	}

	format, width, height := mustDecodeImageConfig(t, meta.Data)
	if format != "png" {
		t.Fatalf("expected png output, got %q", format)
	}
	if width != 320 || height != 180 {
		t.Fatalf("unexpected output size: %dx%d", width, height)
	}
}

func newTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / maxInt(width-1, 1)),
				G: uint8((y * 255) / maxInt(height-1, 1)),
				B: 180,
				A: 255,
			})
		}
	}
	return img
}

func mustEncodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func mustEncodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func mustDecodeImageConfig(t *testing.T, data []byte) (string, int, int) {
	t.Helper()

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode image config: %v", err)
	}

	return format, cfg.Width, cfg.Height
}
