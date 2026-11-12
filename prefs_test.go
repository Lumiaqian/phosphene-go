package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPrefsTheme(t *testing.T) {
	light, dark, desktop := ui.LightTheme(), ui.DarkTheme(), ui.Preferences{}
	if th := (prefs{}).theme(dark, desktop); !th.Dark || th.Accent == dark.Accent {
		t.Errorf("default: dark=%v, accent %v not the app's", th.Dark, th.Accent)
	}
	if th := (prefs{Appearance: "light"}).theme(dark, desktop); th.Dark {
		t.Error("light appearance kept the dark theme")
	}
	if th := (prefs{Appearance: "dark"}).theme(light, desktop); !th.Dark {
		t.Error("dark appearance kept the light theme")
	}
	if th := (prefs{Accent: "system"}).theme(light, desktop); th.Accent != light.Accent {
		t.Errorf("system accent changed the theme's: %v", th.Accent)
	}
	if th := (prefs{Accent: "green"}).theme(light, desktop); th.Accent == light.Accent {
		t.Error("green accent not applied")
	}
}

func TestLookFor(t *testing.T) {
	for key, want := range map[string]string{"": "标准", "none": "无", "max": "极致", "unknown": "标准"} {
		if got := lookFor(key).label; got != want {
			t.Errorf("lookFor(%q) = %s, want %s", key, got, want)
		}
	}
}
