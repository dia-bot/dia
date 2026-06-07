package imaging

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

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

// face returns a cached font face for family+weight+size; nil means "use gg's
// default face".
func (r *Renderer) face(family string, bold bool, size float64) font.Face {
	src := r.fontSource(family, bold)
	if src == nil {
		return nil
	}
	key := faceKey{family: family, bold: bold, size: size}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.faces[key]; ok {
		return f
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		r.log.Warn("font face create failed", "family", family, "size", size, "err", err)
		return nil
	}
	r.faces[key] = f
	return f
}

// setFont applies a family/weight/size to the context (no-op keeps gg's default).
func (r *Renderer) setFont(dc *gg.Context, family string, bold bool, size float64) {
	if f := r.face(family, bold, size); f != nil {
		dc.SetFontFace(f)
	}
}
