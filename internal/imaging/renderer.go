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
	"net"
	"net/http"
	"os"
	"syscall"
	"time"

	"sync"

	xdraw "github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"golang.org/x/image/font"

	"github.com/dia-bot/dia/internal/templating"
)

// safeHTTPClient returns an HTTP client whose dialer refuses to connect to
// private, loopback, link-local or unspecified addresses — an SSRF guard for the
// user-controlled image/avatar/font URLs the renderer fetches. The check runs on
// the resolved IP (post-DNS), so it also defeats DNS-rebinding. Set
// IMAGING_ALLOW_PRIVATE_FETCH=true to relax it for local dev (e.g. MinIO on
// localhost).
func safeHTTPClient(timeout time.Duration) *http.Client {
	allowPrivate := os.Getenv("IMAGING_ALLOW_PRIVATE_FETCH") == "true"
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			if blockedDialAddr(address) {
				return fmt.Errorf("blocked non-public address: %s", address)
			}
			return nil
		}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DialContext: dialer.DialContext, MaxIdleConns: 16, IdleConnTimeout: 30 * time.Second},
	}
}

// blockedDialAddr reports whether a resolved "host:port" is a private, loopback,
// link-local, unspecified or otherwise non-globally-routable address that the
// renderer must refuse to fetch (SSRF guard).
func blockedDialAddr(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || !ip.IsGlobalUnicast()
}

// Renderer holds parsed fonts (by family), an avatar HTTP client, a card-text
// template engine, and a concurrency limiter. Font loading + face caching live
// in fonts.go.
type Renderer struct {
	fonts         map[string]*familyFonts // family name → regular/bold
	defaultFamily string                  // used when a layer names no family

	mu     sync.Mutex
	faces  map[faceKey]font.Face
	custom map[string]*familyFonts // guild custom fonts by URL (fetched on demand)

	tmpl *templating.Engine // renders card-layer text ({tokens} + Go templates)

	http *http.Client
	sem  chan struct{}
	log  *slog.Logger
}

// New builds a Renderer, loading every available card font from fontsDir.
func New(fontsDir string, log *slog.Logger) *Renderer {
	r := &Renderer{
		faces:  map[faceKey]font.Face{},
		custom: map[string]*familyFonts{},
		tmpl:   templating.New(),
		http:   safeHTTPClient(8 * time.Second),
		sem:    make(chan struct{}, 4), // bound concurrent renders
		log:    log,
	}
	r.loadFamilies(fontsDir)
	return r
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
		if img := r.fetchDecoded(ctx, url); img != nil {
			return xdraw.Fill(img, size, size, xdraw.Center, xdraw.Lanczos)
		}
		r.log.Debug("avatar fetch failed; using placeholder", "url", url)
	}
	ph := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.Paste(ph, image.NewUniform(accent), image.Pt(0, 0))
	return ph
}

// Image fetch caps: bound both the download size and the DECODED pixel count, so
// a multi-GB file can't stream into memory and a small "decompression bomb" (a
// tiny file whose header claims enormous dimensions) is rejected before decode.
const (
	maxImageBytes  = 16 << 20   // 16 MiB on the wire
	maxImagePixels = 24_000_000 // ~4900×4900 decoded ceiling
)

// fetchDecoded downloads url (size-capped), checks the declared dimensions from
// the header, and only then decodes — returning nil on any failure or breach.
func (r *Renderer) fetchDecoded(ctx context.Context, url string) image.Image {
	data := r.fetchBytes(ctx, url, maxImageBytes+1)
	if data == nil {
		return nil
	}
	if int64(len(data)) > maxImageBytes {
		r.log.Debug("image exceeds size cap; skipped", "url", url, "bytes", len(data))
		return nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		r.log.Debug("image exceeds pixel cap; skipped", "url", url, "w", cfg.Width, "h", cfg.Height)
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}

// encodePNG renders the context to PNG bytes.
func encodePNG(dc *gg.Context) ([]byte, error) {
	var buf bytes.Buffer
	if err := dc.EncodePNG(&buf); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}
