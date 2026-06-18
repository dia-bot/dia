package verification

import (
	"bytes"
	"crypto/sha256"
	"image/color"
	"math"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/gobold"
)

// captchaAlphabet is the pool the codes are drawn from. It deliberately omits
// visually ambiguous glyphs (0/O, 1/I/L) so a human reading the rendered image
// is not penalised for a guess that "looks right".
const captchaAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// captchaLen is how many characters a generated code has.
const captchaLen = 6

// captchaFont is the parsed Go Bold face used for the image. Parsed once; gg's
// own default face is too small/thin to be legible as a challenge.
var captchaFont = mustParseFont(gobold.TTF)

func mustParseFont(ttf []byte) *truetype.Font {
	f, err := truetype.Parse(ttf)
	if err != nil {
		// gobold.TTF is a compiled-in constant; a parse failure is a programmer
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

// renderCaptcha draws code onto a small noisy PNG: jittered, rotated glyphs over
// a speckled background with a few distractor lines. It is intentionally modest
// (defeats trivial OCR-less bots, not a determined attacker) and never fails.
func renderCaptcha(code string) ([]byte, error) {
	const w, h = 360, 130
	dc := gg.NewContext(w, h)

	// Paper-ish background.
	dc.SetColor(color.NRGBA{R: 24, G: 24, B: 27, A: 255})
	dc.Clear()

	// Speckle noise so flat-fill OCR has something to fight.
	sum := sha256.Sum256([]byte("noise:" + code))
	for i := 0; i < 700; i++ {
		x := float64(int(sum[i%len(sum)])*7+i*13) / 1.0
		y := float64(int(sum[(i*3)%len(sum)])*5+i*7) / 1.0
		px := math.Mod(x, float64(w))
		py := math.Mod(y, float64(h))
		shade := uint8(60 + int(sum[(i*5)%len(sum)])%120)
		dc.SetColor(color.NRGBA{R: shade, G: shade, B: shade, A: 90})
		dc.DrawPoint(px, py, 1)
		dc.Fill()
	}

	// A couple of distractor lines.
	for i := 0; i < 4; i++ {
		dc.SetColor(color.NRGBA{R: 120, G: 120, B: 130, A: 110})
		dc.SetLineWidth(2)
		x1 := math.Mod(float64(int(sum[i])*9), float64(w))
		x2 := math.Mod(float64(int(sum[i+8])*11), float64(w))
		y1 := math.Mod(float64(int(sum[i+4])*7), float64(h))
		y2 := math.Mod(float64(int(sum[i+12])*5), float64(h))
		dc.DrawLine(x1, y1, x2, y2)
		dc.Stroke()
	}

	face := truetype.NewFace(captchaFont, &truetype.Options{Size: 54})
	dc.SetFontFace(face)

	// Lay glyphs out with per-character jitter + rotation.
	startX := 40.0
	step := (float64(w) - 80) / float64(len(code))
	for i, r := range code {
		angle := (float64(int(sum[i%len(sum)])%30) - 15) * math.Pi / 180
		dx := startX + step*float64(i) + step/2
		dy := float64(h)/2 + float64(int(sum[(i+5)%len(sum)])%18-9)

		dc.Push()
		dc.RotateAbout(angle, dx, dy)
		// Soft shadow then bright glyph for contrast against the noise.
		dc.SetColor(color.NRGBA{R: 0, G: 0, B: 0, A: 160})
		dc.DrawStringAnchored(string(r), dx+2, dy+2, 0.5, 0.5)
		dc.SetColor(color.NRGBA{R: 255, G: 99, B: 99, A: 255}) // rose accent
		dc.DrawStringAnchored(string(r), dx, dy, 0.5, 0.5)
		dc.Pop()
	}

	var buf bytes.Buffer
	if err := dc.EncodePNG(&buf); err != nil {
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
