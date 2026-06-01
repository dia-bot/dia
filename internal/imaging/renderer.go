// Package imaging renders welcome cards and rank cards entirely in pure Go
// (fogleman/gg for the canvas, disintegration/imaging for avatar fitting and
// golang.org/x/image/font/opentype for text). It is used by the worker to
// generate images on the fly and by the API to render dashboard previews.
//
// Fonts are optional: drop TTF/OTF files named Inter-Bold / Inter-Regular (or
// set them via Config) into the fonts directory for a polished look; otherwise
// the renderer falls back to gg's built-in face so it always works.
package imaging

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // register JPEG decoder for avatars
	_ "image/png"  // register PNG decoder for avatars
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	xdraw "github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// Renderer holds parsed fonts, an avatar HTTP client and a concurrency limiter.
type Renderer struct {
	regular *sfnt.Font
	bold    *sfnt.Font

	mu    sync.Mutex
	faces map[faceKey]font.Face

	http *http.Client
	sem  chan struct{}
	log  *slog.Logger
}

type faceKey struct {
	bold bool
	size float64
}

// New builds a Renderer, loading fonts from fontsDir if present.
func New(fontsDir string, log *slog.Logger) *Renderer {
	r := &Renderer{
		faces: map[faceKey]font.Face{},
		http:  &http.Client{Timeout: 8 * time.Second},
		sem:   make(chan struct{}, 4), // bound concurrent renders
		log:   log,
	}
	r.regular = loadFont(fontsDir, []string{"Inter-Regular.ttf", "Inter-Regular.otf", "Regular.ttf"}, log)
	r.bold = loadFont(fontsDir, []string{"Inter-Bold.ttf", "Inter-Bold.otf", "Bold.ttf"}, log)
	return r
}

func loadFont(dir string, names []string, log *slog.Logger) *sfnt.Font {
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		f, err := opentype.Parse(b)
		if err != nil {
			log.Warn("failed to parse font", "file", n, "err", err)
			continue
		}
		log.Info("loaded font", "file", n)
		return f
	}
	return nil
}

// face returns a cached font face at the given size; nil means "use gg default".
func (r *Renderer) face(bold bool, size float64) font.Face {
	src := r.regular
	if bold {
		src = r.bold
	}
	if src == nil {
		return nil
	}
	key := faceKey{bold: bold, size: size}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.faces[key]; ok {
		return f
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		r.log.Warn("font face create failed", "size", size, "err", err)
		return nil
	}
	r.faces[key] = f
	return f
}

// setFont applies a face to the context (no-op fallback keeps gg's default).
func (r *Renderer) setFont(dc *gg.Context, bold bool, size float64) {
	if f := r.face(bold, size); f != nil {
		dc.SetFontFace(f)
	}
}

// acquire/release bound concurrent renders to protect memory/CPU.
func (r *Renderer) acquire(ctx context.Context) error {
	select {
	case r.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (r *Renderer) release() { <-r.sem }

// fetchAvatar downloads and square-fits an avatar to size×size pixels. On any
// failure it returns a flat accent-coloured placeholder so rendering never fails.
func (r *Renderer) fetchAvatar(ctx context.Context, url string, size int, accent color.Color) image.Image {
	if url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			resp, err := r.http.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					if img, _, err := image.Decode(resp.Body); err == nil {
						return xdraw.Fill(img, size, size, xdraw.Center, xdraw.Lanczos)
					}
				}
			}
		}
		r.log.Debug("avatar fetch failed; using placeholder", "url", url)
	}
	ph := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.Paste(ph, image.NewUniform(accent), image.Pt(0, 0))
	return ph
}

// encodePNG renders the context to PNG bytes.
func encodePNG(dc *gg.Context) ([]byte, error) {
	var buf bytes.Buffer
	if err := dc.EncodePNG(&buf); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}
