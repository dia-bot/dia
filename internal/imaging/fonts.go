package imaging

import (
	"context"
	"embed"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// embeddedFonts guarantees the renderer always has a real proportional font even
// when FONTS_DIR is empty — so server-rendered text never falls back to gg's tiny
// built-in bitmap face (which made the PNG look nothing like the editor preview).
//
//go:embed embedded/Lato-Regular.ttf embedded/Lato-Bold.ttf
var embeddedFonts embed.FS

func parseEmbedded(name string) *sfnt.Font {
	b, err := embeddedFonts.ReadFile("embedded/" + name)
	if err != nil {
		return nil
	}
	f, err := opentype.Parse(b)
	if err != nil {
		return nil
	}
	return f
}

// fontEntry maps a public family name (the value stored in a layout and shown in
// the dashboard font picker) to the file base used on disk: <File>-Regular.ttf
// and <File>-Bold.ttf. Keep this list in sync with web/src/lib/layout/fonts.ts
// and scripts/fetch-fonts.sh.
type fontEntry struct{ Family, File string }

var fontRegistry = []fontEntry{
	{"Inter", "Inter"}, // honoured if dropped in; not part of the default roster
	{"Lato", "Lato"},
	{"Poppins", "Poppins"},
	{"Montserrat", "Montserrat"}, // honoured if dropped in
	{"Kanit", "Kanit"},
	{"Barlow", "Barlow"},
	{"Rajdhani", "Rajdhani"},
	{"Arvo", "Arvo"},
	{"Titillium Web", "TitilliumWeb"},
	{"Anton", "Anton"},
	{"Bebas Neue", "BebasNeue"},
	{"Lobster", "Lobster"},
	{"Pacifico", "Pacifico"},
}

// defaultFamilyPref is the order we pick the "no family specified" default from,
// among the families that actually loaded. Lato first so the editor preview
// (which defaults to Lato) matches the render.
var defaultFamilyPref = []string{"Lato", "Inter", "Poppins"}

type familyFonts struct{ regular, bold *sfnt.Font }

type faceKey struct {
	family string
	bold   bool
	size   float64
}

// loadFamilies scans fontsDir for every registry family and records those whose
// files are present, then chooses a default family for layers that name none.
func (r *Renderer) loadFamilies(dir string) {
	r.fonts = map[string]*familyFonts{}
	var first string
	for _, e := range fontRegistry {
		reg := loadFont(dir, []string{e.File + "-Regular.ttf", e.File + "-Regular.otf"}, r.log)
		if reg == nil {
			continue
		}
		bold := loadFont(dir, []string{e.File + "-Bold.ttf", e.File + "-Bold.otf"}, r.log)
		r.fonts[e.Family] = &familyFonts{regular: reg, bold: bold}
		if first == "" {
			first = e.Family
		}
	}
	// Guarantee Lato (the default family) is always available via the embedded
	// copy, so the renderer never falls back to gg's bitmap face.
	if r.fonts["Lato"] == nil {
		if reg := parseEmbedded("Lato-Regular.ttf"); reg != nil {
			r.fonts["Lato"] = &familyFonts{regular: reg, bold: parseEmbedded("Lato-Bold.ttf")}
			if first == "" {
				first = "Lato"
			}
		}
	}
	for _, pref := range defaultFamilyPref {
		if r.fonts[pref] != nil {
			r.defaultFamily = pref
			break
		}
	}
	if r.defaultFamily == "" {
		r.defaultFamily = first
	}
	if len(r.fonts) == 0 {
		r.log.Warn("no card fonts loaded; using gg's built-in face (run `make fonts`)", "dir", dir)
	}
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
		return f
	}
	return nil
}

// fontSource resolves a family+weight to a parsed font, falling back to the
// default family (then nil → gg's built-in face).
func (r *Renderer) fontSource(family string, bold bool) *sfnt.Font {
	ff := r.fonts[family]
	if ff == nil {
		ff = r.fonts[r.defaultFamily]
	}
	if ff == nil {
		return nil
	}
	if bold && ff.bold != nil {
		return ff.bold
	}
	return ff.regular
}

// cachedFace builds (and caches) a face from a parsed font + cache key.
func (r *Renderer) cachedFace(src *sfnt.Font, key faceKey) font.Face {
	if src == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.faces[key]; ok {
		return f
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: key.size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		r.log.Warn("font face create failed", "key", key.family, "size", key.size, "err", err)
		return nil
	}
	r.faces[key] = f
	return f
}

// face returns a cached face for a registered family+weight+size; nil means
// "use gg's default face".
func (r *Renderer) face(family string, bold bool, size float64) font.Face {
	return r.cachedFace(r.fontSource(family, bold), faceKey{family: family, bold: bold, size: size})
}

// faceFor resolves a family to a face, preferring a guild's uploaded (custom)
// font when its family matches an entry in custom (family → URL). Custom fonts
// are single-weight, so bold falls back to the same file.
func (r *Renderer) faceFor(ctx context.Context, family string, bold bool, size float64, custom map[string]string) font.Face {
	if url := custom[family]; url != "" {
		if src := r.customSource(ctx, url); src != nil {
			return r.cachedFace(src, faceKey{family: "u:" + url, size: size})
		}
	}
	return r.face(family, bold, size)
}

// customSource fetches + parses a custom font by URL, caching the result
// (including failures, as nil) so a render never refetches per text layer.
func (r *Renderer) customSource(ctx context.Context, url string) *sfnt.Font {
	r.mu.Lock()
	if ff, ok := r.custom[url]; ok {
		r.mu.Unlock()
		if ff == nil {
			return nil
		}
		return ff.regular
	}
	r.mu.Unlock()

	var parsed *sfnt.Font
	if data := r.fetchBytes(ctx, url, 4<<20); data != nil {
		if f, err := opentype.Parse(data); err == nil {
			parsed = f
		} else {
			r.log.Warn("custom font parse failed", "url", url, "err", err)
		}
	}
	r.mu.Lock()
	if len(r.custom) > 512 { // crude bound across many guilds/fonts
		r.custom = map[string]*familyFonts{}
	}
	if parsed == nil {
		r.custom[url] = nil
	} else {
		r.custom[url] = &familyFonts{regular: parsed}
	}
	r.mu.Unlock()
	return parsed
}

// fetchBytes GETs up to max bytes from url (used for custom fonts).
func (r *Renderer) fetchBytes(ctx context.Context, url string, max int64) []byte {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max))
	if err != nil {
		return nil
	}
	return data
}

// setFont applies a family/weight/size to the context (no-op keeps gg's default).
func (r *Renderer) setFont(dc *gg.Context, family string, bold bool, size float64) {
	if f := r.face(family, bold, size); f != nil {
		dc.SetFontFace(f)
	}
}
