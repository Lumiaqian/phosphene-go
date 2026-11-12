package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) Config {
	t.Helper()
	b, e := os.ReadFile("testdata/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	var c Config
	if e = json.Unmarshal(b, &c); e != nil {
		t.Fatal(e)
	}
	return c
}
func TestContracts(t *testing.T) {
	c := fixture(t)
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	if path := os.Getenv("GO_CONTRACT_OUTPUT"); path != "" {
		if e := AtomicJSON(path, c); e != nil {
			t.Fatal(e)
		}
		r := Runtime{SchemaVersion: 1, AppliedRevision: 7, ActiveDisplayKeys: []string{}, OpenVideoIDs: []string{}, Errors: []string{}, Displays: []DisplayProgress{{DisplayKey: "display-A", VideoID: c.Media[1].ID, PositionSeconds: 61.5, ItemElapsedSeconds: 61.5}}}
		if e := AtomicJSON(filepath.Join(filepath.Dir(path), "go-runtime.json"), r); e != nil {
			t.Fatal(e)
		}
	}
	if path := os.Getenv("SWIFT_CONTRACT_OUTPUT"); path != "" {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		var got Config
		if e = json.Unmarshal(b, &got); e != nil {
			t.Fatal(e)
		}
		if e = got.Validate(); e != nil {
			t.Fatal(e)
		}
		if got.Media[0].Name != c.Media[0].Name || got.Media[1].FrameRate != 29.97 || got.Playlist.IntervalSeconds != 60 {
			t.Fatal("Swift round trip changed data")
		}
	}
	if path := os.Getenv("SWIFT_CONTRACT_OUTPUT"); path != "" {
		r, e := ReadRuntime(filepath.Dir(path))
		if e != nil {
			t.Fatal(e)
		}
		if len(r.Displays) != 1 || r.Displays[0].DisplayKey != "display-B" || r.Displays[0].PositionSeconds != 61.5 {
			t.Fatal("Swift progress changed", r)
		}
	}

	for _, change := range []func(*Config){func(c *Config) { c.SchemaVersion = 2 }, func(c *Config) { c.Playlist.IntervalSeconds = 0 }, func(c *Config) { c.Playlist.AdvanceMode = "random" }, func(c *Config) { c.Media[0].Filename = "../escape.mp4" }, func(c *Config) { c.Media[0].ID = "not-a-uuid" }, func(c *Config) { c.Playlist.VideoIDs = append(c.Playlist.VideoIDs, "missing") }, func(c *Config) { c.Media[0].DurationSeconds = 0 }} {
		bad := fixture(t)
		change(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}
func TestAtomicSaveAndInvalidImport(t *testing.T) {
	root := t.TempDir()
	c := fixture(t)
	if e := Save(root, &c); e != nil {
		t.Fatal(e)
	}
	saved, e := Load(root)
	if e != nil || saved.Revision != 8 {
		t.Fatal(saved, e)
	}
	source := filepath.Join(t.TempDir(), "broken.mp4")
	os.WriteFile(source, []byte("broken"), 0600)
	_, e = Import(context.Background(), root, source, func(context.Context, string, string) (MediaEntry, error) {
		return MediaEntry{}, errors.New("decode failed")
	})
	if e == nil {
		t.Fatal("bad video accepted")
	}
	dirs, _ := os.ReadDir(filepath.Join(root, "videos"))
	if len(dirs) != 0 {
		t.Fatal("partially imported video exposed")
	}
	b, _ := os.ReadFile(source)
	if string(b) != "broken" {
		t.Fatal("source changed")
	}
}
func TestDeleteWaitsForNativeReadersAndUsesLatestCatalog(t *testing.T) {
	root := t.TempDir()
	c := fixture(t)
	Save(root, &c)
	removed := c.Media[0]
	dir := filepath.Dir(VideoPath(root, removed))
	os.MkdirAll(dir, 0700)
	c.RemovedMediaIDs = append(c.RemovedMediaIDs, removed.ID)
	c.Media = c.Media[1:]
	c.Playlist.VideoIDs = c.Playlist.VideoIDs[1:]
	Save(root, &c)
	r := Runtime{SchemaVersion: 1, AppliedRevision: c.Revision, UpdatedAtUnix: float64(time.Now().Unix()), OpenVideoIDs: []string{removed.ID}}
	AtomicJSON(filepath.Join(root, "runtime.json"), r)
	if e := Reap(root); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(dir); e != nil {
		t.Fatal("deleted file with active reader")
	}
	r.OpenVideoIDs = []string{}
	AtomicJSON(filepath.Join(root, "runtime.json"), r)
	// A stale delayed cleanup must not erase a subsequent import.
	latest := c
	latest.Media = append(latest.Media, removed)
	Save(root, &latest)
	r.AppliedRevision = latest.Revision
	AtomicJSON(filepath.Join(root, "runtime.json"), r)
	Reap(root)
	if _, e := os.Stat(dir); e != nil {
		t.Fatal("stale cleanup deleted newly imported media")
	}
	Save(root, &c)
	r.AppliedRevision = c.Revision
	AtomicJSON(filepath.Join(root, "runtime.json"), r)
	Reap(root)
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("unused copy not cleaned")
	}
}

func TestCleanupCannotDeleteUnpublishedImport(t *testing.T) {
	root := t.TempDir()
	c := Default()
	if e := Save(root, &c); e != nil {
		t.Fatal(e)
	}
	id := "44444444-4444-4444-8444-444444444444"
	dir := filepath.Join(root, "videos", id)
	os.MkdirAll(dir, 0700)
	// A batch importer finished copying this item but is probing its next file.
	r := Runtime{SchemaVersion: 1, AppliedRevision: c.Revision, UpdatedAtUnix: float64(time.Now().Unix())}
	AtomicJSON(filepath.Join(root, "runtime.json"), r)
	if e := Reap(root); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(dir); e != nil {
		t.Fatal("cleanup deleted an unpublished import")
	}
}
