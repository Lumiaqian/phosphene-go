package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Lumiaqian/phosphene-go/internal/catalog"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// prefs are the manager's own settings, kept apart from the library the
// extension reads: how its window looks.
type prefs struct {
	Appearance string `json:"appearance,omitempty"` // "light" or "dark"; empty follows the system
	Accent     string `json:"accent,omitempty"`     // a key of accents; empty is the first
	Glass      string `json:"glass,omitempty"`      // a key of looks; empty is the standard
}

// appearances are the choices of prefs.Appearance, in the order the
// settings show them.
var appearances = []string{"", "light", "dark"}

// accents are the choices of prefs.Accent. The last, with no color, keeps
// the desktop's accent.
var accents = []struct {
	key, label string
	color      ui.Color
}{
	{"violet", "紫色", ui.Oklch(0.6, 0.2, 290)},
	{"blue", "蓝色", ui.Oklch(0.62, 0.17, 255)},
	{"pink", "粉色", ui.Oklch(0.66, 0.2, 355)},
	{"orange", "橙色", ui.Oklch(0.72, 0.17, 55)},
	{"green", "绿色", ui.Oklch(0.68, 0.16, 155)},
	{"system", "跟随系统", ui.Color{}},
}

func prefsPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// loadPrefs reads the saved settings; missing or unreadable ones are the
// defaults.
func loadPrefs() prefs {
	var p prefs
	if path, err := prefsPath(); err == nil {
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &p)
		}
	}
	return p
}

// savePrefs writes the settings and applies the appearance to the app's
// windows, menus and dialogs.
func (a *application) savePrefs() {
	mygo.Theme.SetSource(a.prefs.themeSource())
	path, err := prefsPath()
	if err == nil {
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			err = catalog.AtomicJSON(path, a.prefs)
		}
	}
	if err != nil {
		a.status = "外观设置保存失败：" + err.Error()
	}
}

func (p prefs) themeSource() mygo.ThemeSource {
	switch p.Appearance {
	case "light":
		return mygo.ThemeLight
	case "dark":
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}

// theme is the window's theme under these settings: def, the window's own,
// follows the system and the desktop's accent.
func (p prefs) theme(def *ui.Theme, desktop ui.Preferences) ui.Theme {
	base := def
	switch {
	case p.Appearance == "light" && def.Dark:
		base = ui.LightTheme().WithPreferences(desktop)
	case p.Appearance == "dark" && !def.Dark:
		base = ui.DarkTheme().WithPreferences(desktop)
	}
	t := *base
	for i, ac := range accents {
		if ac.key == p.Accent || (p.Accent == "" && i == 0) {
			if ac.color.A > 0 {
				t = *t.WithAccent(ac.color)
			}
			break
		}
	}
	return t
}
