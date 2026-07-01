package verification

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/gomonobold"
)

// captchaAlphabet is the pool the codes are drawn from. It deliberately omits
// visually ambiguous glyphs (0/O, 1/I/L) so a human reading the rendered image
// is not penalised for a guess that "looks right".
const captchaAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// captchaLen is how many characters a generated code has.
const captchaLen = 6

// captchaFonts are several heavy faces with distinct letterforms (sans, italic,
// mono). Mixing them per glyph denies a solver a single trainable typeface,
// one of the cheap classical defenses that still trips naive OCR. We keep to
// bold weights because thin strokes vanish under the noise + warp.
var captchaFonts = []*truetype.Font{
	mustParseFont(gobold.TTF),
	mustParseFont(gobolditalic.TTF),
	mustParseFont(gomonobold.TTF),
}

func mustParseFont(ttf []byte) *truetype.Font {
	f, err := truetype.Parse(ttf)
	if err != nil {
		// The fonts are compiled-in constants; a parse failure is a programmer
		// error, not a runtime condition.
		panic("verification: parse captcha font: " + err.Error())
	}
	return f
}

// generateCode derives a deterministic-but-unguessable code from the user id and
// a per-attempt salt. Math/rand-style randomness is unavailable in templates and
// we want the same (uid, salt) to reproduce the same code, so we hash the seed
// and fold the digest bytes into alphabet indices.
func generateCode(seed string) string {
	sum := sha256.Sum256([]byte("verify-captcha:" + seed))
	b := make([]byte, captchaLen)
	for i := 0; i < captchaLen; i++ {
		b[i] = captchaAlphabet[int(sum[i])%len(captchaAlphabet)]
	}
	return string(b)
}

// captchaRNG is a deterministic byte stream seeded from the code (SHA-256 in
// counter mode). It gives unlimited pseudo-random bytes for the layout / warp /
// noise while staying reproducible, so re-rendering the same code yields the
// same image and a retry (new salt -> new code) yields a fresh one. The seed is
// the secret code, so the stream is unpredictable to anyone who can't read it.
type captchaRNG struct {
	seed    []byte
	block   [32]byte
	pos     int
	counter uint32
}

func newCaptchaRNG(seed string) *captchaRNG {
	return &captchaRNG{seed: []byte(seed), pos: 32}
}

func (r *captchaRNG) b() byte {
	if r.pos >= len(r.block) {
		h := sha256.New()
		h.Write(r.seed)
		h.Write([]byte{byte(r.counter), byte(r.counter >> 8), byte(r.counter >> 16), byte(r.counter >> 24)})
		copy(r.block[:], h.Sum(nil))
		r.counter++
		r.pos = 0
	}
	v := r.block[r.pos]
	r.pos++
	return v
}

// f returns a pseudo-random float in [0,1).
func (r *captchaRNG) f() float64 { return float64(r.b()) / 256.0 }

// span returns a pseudo-random float in [lo,hi).
func (r *captchaRNG) span(lo, hi float64) float64 { return lo + r.f()*(hi-lo) }

// intn returns a pseudo-random int in [0,n).
func (r *captchaRNG) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.b()) % n
}

// captchaPalette is the set of bright, high-contrast glyph colours. They read
// clearly on the dark background; the distractor arcs reuse them at lower alpha
// so a solver can't simply colour-key the text out.
var captchaPalette = []color.NRGBA{
	{R: 255, G: 99, B: 99, A: 255},   // rose
	{R: 124, G: 178, B: 255, A: 255}, // sky
	{R: 120, G: 224, B: 160, A: 255}, // mint
	{R: 246, G: 196, B: 92, A: 255},  // amber
	{R: 236, G: 236, B: 242, A: 255}, // paper
}

// renderCaptcha draws code onto a PNG hardened toward a real-world text CAPTCHA:
// a gradient background, per-glyph font / size / colour / rotation / baseline
// jitter with mild overlap (anti-segmentation), distractor sine arcs in the
// glyph-colour family, speckle noise, and finally a sine displacement warp of
// the whole image (the single most effective classical defence). It stays
// human-readable on purpose: this stops trivial OCR / scripted bots, not a
// determined AI solver (modern vision models read text CAPTCHAs at >95%), so it
// is a speed bump, layered with the behavioural gating, not the whole barrier.
func renderCaptcha(code string) ([]byte, error) {
	const w, h = 360, 130
	rng := newCaptchaRNG("verify-captcha-img:" + code)
	dc := gg.NewContext(w, h)

	// Background: a diagonal gradient of dark tones (no flat fill for a solver to
	// assume), with a faint speckle texture.
	grad := gg.NewLinearGradient(0, 0, w, h)
	grad.AddColorStop(0, color.NRGBA{R: 22, G: 22, B: 28, A: 255})
	grad.AddColorStop(1, color.NRGBA{R: 12, G: 12, B: 16, A: 255})
	dc.SetFillStyle(grad)
	dc.DrawRectangle(0, 0, w, h)
	dc.Fill()

	// Faint background interference: low-contrast wavy strokes behind the text.
	for i := 0; i < 5; i++ {
		drawArc(dc, rng, w, h, color.NRGBA{R: 70, G: 70, B: 82, A: 70}, 1.5)
	}

	// Glyphs: each character gets its own font, size, colour, rotation and
	// baseline offset, packed with mild overlap so segmentation can't cleanly cut
	// between them.
	pad := 28.0
	step := (float64(w) - 2*pad) / float64(len(code))
	for i, ch := range code {
		font := captchaFonts[rng.intn(len(captchaFonts))]
		dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: rng.span(46, 60)}))
		angle := rng.span(-22, 22) * math.Pi / 180
		cx := pad + step*float64(i) + step*0.5 + rng.span(-step*0.14, step*0.14)
		cy := float64(h)/2 + rng.span(-13, 13)

		dc.Push()
		dc.RotateAbout(angle, cx, cy)
		// Soft shadow for contrast against the noise, then the bright glyph.
		dc.SetColor(color.NRGBA{R: 0, G: 0, B: 0, A: 150})
		dc.DrawStringAnchored(string(ch), cx+1.5, cy+1.5, 0.5, 0.5)
		dc.SetColor(captchaPalette[rng.intn(len(captchaPalette))])
		dc.DrawStringAnchored(string(ch), cx, cy, 0.5, 0.5)
		dc.Pop()
	}

	// Foreground distractor arcs in the glyph-colour family (so they can't be
	// colour-keyed away) crossing the text.
	for i := 0; i < 3; i++ {
		c := captchaPalette[rng.intn(len(captchaPalette))]
		c.A = 120
		drawArc(dc, rng, w, h, c, rng.span(1.5, 3))
	}

	// Speckle noise over everything.
	for i := 0; i < 900; i++ {
		shade := uint8(50 + rng.intn(140))
		dc.SetColor(color.NRGBA{R: shade, G: shade, B: shade, A: 80})
		dc.DrawPoint(rng.span(0, w), rng.span(0, h), 1)
		dc.Fill()
	}

	// Warp: displace every pixel by an independent sine in x and y. Warping is the
	// strongest classical defence; the amplitude is kept modest so it stays
	// legible to a human.
	return encodeWarped(dc.Image(), rng)
}

// drawArc strokes a sine curve spanning the width at a random baseline / phase /
// amplitude / frequency, used for both the faint background and the coloured
// foreground distractors.
func drawArc(dc *gg.Context, rng *captchaRNG, w, h int, col color.NRGBA, width float64) {
	base := rng.span(float64(h)*0.2, float64(h)*0.8)
	amp := rng.span(float64(h)*0.08, float64(h)*0.22)
	freq := rng.span(0.015, 0.045)
	phase := rng.span(0, 2*math.Pi)
	dc.SetColor(col)
	dc.SetLineWidth(width)
	dc.MoveTo(0, base+amp*math.Sin(phase))
	for x := 0.0; x <= float64(w); x += 3 {
		dc.LineTo(x, base+amp*math.Sin(x*freq+phase))
	}
	dc.Stroke()
}

// encodeWarped applies a per-pixel sine displacement to src and returns the PNG
// bytes. dx depends on y and dy on x, so straight features bend in both axes.
func encodeWarped(src image.Image, rng *captchaRNG) ([]byte, error) {
	b := src.Bounds()
	w, hh := b.Dx(), b.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, hh))

	ax, fx, px := rng.span(2.5, 4.5), rng.span(0.04, 0.07), rng.span(0, 2*math.Pi)
	ay, fy, py := rng.span(2.5, 4.5), rng.span(0.04, 0.07), rng.span(0, 2*math.Pi)
	clamp := func(v, hi int) int {
		if v < 0 {
			return 0
		}
		if v > hi {
			return hi
		}
		return v
	}
	for y := 0; y < hh; y++ {
		for x := 0; x < w; x++ {
			sx := clamp(x+int(ax*math.Sin(float64(y)*fx+px)), w-1)
			sy := clamp(y+int(ay*math.Sin(float64(x)*fy+py)), hh-1)
			out.Set(x, y, src.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// obfuscate spaces out a code with thin separators as the text fallback when an
// image cannot be produced (keeps the challenge typed, not copy-pasted in one go).
func obfuscate(code string) string {
	var b bytes.Buffer
	for i, r := range code {
		if i > 0 {
			b.WriteString(" ​ ")
		}
		b.WriteRune(r)
	}
	return b.String()
}
