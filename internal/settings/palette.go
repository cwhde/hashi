package settings

// Palette is a theme's colours for pages and mails Hashi renders outside the web app (alert mails, the CAPTCHA
// challenge page). The built-in values are web/src/routes/layout.css's --h-* tokens (TestPalettesMatchThemes).
type Palette struct {
	Canvas, Shell, Surface, Surface2, Border, Text, Muted, Accent, AccentText string
	// Dark is true for a dark theme, so the page asks the browser for dark form controls and scrollbars.
	Dark bool
}

// Palettes are the built-in themes.
var Palettes = map[string]Palette{
	"graphite": {"#08090b", "#0d0f12", "#13161a", "#191d23", "#2b323b", "#f2f5f7", "#9da6b2", "#2563eb", "#ffffff", true},
	"linen":    {"#f3eee5", "#eae1d4", "#faf7f1", "#f0e7da", "#d4c7b6", "#2b251f", "#6b6156", "#b6533a", "#ffffff", false},
	"verdant":  {"#030a07", "#06120d", "#0a1a12", "#10251a", "#27513b", "#edf7f0", "#94aa9b", "#e8ce55", "#08110c", true},
	"violet":   {"#05030a", "#0a0712", "#110c1d", "#191128", "#382653", "#f4f0fa", "#aa9fbc", "#e6cf62", "#120c1e", true},
}

// PaletteOf is the palette of an install's theme: a custom theme in its own colours, an unknown one as Warm Linen,
// the default (10-ui §2).
func PaletteOf(theme string, custom *CustomTheme) Palette {
	if theme == ThemeCustom && custom != nil {
		c := *custom
		// A custom theme has no shell or second surface: the surface stands in for both.
		return Palette{c.Canvas, c.Surface, c.Surface, c.Surface, c.Border, c.Text, c.Muted, c.Accent, c.AccentText(),
			!c.Light()} // the app's own light/dark rule
	}
	if p, ok := Palettes[theme]; ok {
		return p
	}
	return Palettes["linen"]
}
