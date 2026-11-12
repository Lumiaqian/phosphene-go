// Package catalog owns the versioned configuration exchanged with the extension.
package catalog

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const SchemaVersion = 1
const ExtensionID = "local.lumiaqian.phosphenego.extension"

type MediaEntry struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Filename        string  `json:"filename"`
	Width           int64   `json:"width"`
	Height          int64   `json:"height"`
	DurationSeconds float64 `json:"durationSeconds"`
	FrameRate       float64 `json:"frameRate"`
	AddedAtUnix     float64 `json:"addedAtUnix"`
}
type PlaylistSpec struct {
	VideoIDs                []string `json:"videoIDs"`
	AdvanceMode             string   `json:"advanceMode"`
	IntervalSeconds         int      `json:"intervalSeconds"`
	KeepAwakeWhenLockedOnAC bool     `json:"keepAwakeWhenLockedOnAC"`
}
type Config struct {
	// RemovedMediaIDs records explicit deletions. Unpublished imports are never cleanup candidates.
	RemovedMediaIDs []string     `json:"removedMediaIDs,omitempty"`
	SchemaVersion   int          `json:"schemaVersion"`
	Revision        int64        `json:"revision"`
	Media           []MediaEntry `json:"media"`
	Playlist        PlaylistSpec `json:"playlist"`
}
type DisplayProgress struct {
	DisplayKey         string  `json:"displayKey"`
	VideoID            string  `json:"videoID"`
	PositionSeconds    float64 `json:"positionSeconds"`
	ItemElapsedSeconds float64 `json:"itemElapsedSeconds"`
}
type Runtime struct {
	SchemaVersion     int               `json:"schemaVersion"`
	AppliedRevision   int64             `json:"appliedRevision"`
	UpdatedAtUnix     float64           `json:"updatedAtUnix"`
	Displays          []DisplayProgress `json:"displays"`
	ActiveDisplayKeys []string          `json:"activeDisplayKeys"`
	OpenVideoIDs      []string          `json:"openVideoIDs"`
	KeepAwakeActive   bool              `json:"keepAwakeActive"`
	Errors            []string          `json:"errors"`
}

func Default() Config {
	return Config{SchemaVersion: 1, Media: []MediaEntry{}, Playlist: PlaylistSpec{VideoIDs: []string{}, AdvanceMode: "end", IntervalSeconds: 60, KeepAwakeWhenLockedOnAC: true}}
}
func Root() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Containers", ExtensionID, "Data", "Documents")
}
func safeID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
func (c Config) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema version: %d", c.SchemaVersion)
	}
	if c.Playlist.AdvanceMode != "end" && c.Playlist.AdvanceMode != "duration" {
		return errors.New("invalid advance mode")
	}
	if c.Playlist.IntervalSeconds <= 0 {
		return errors.New("interval must be a positive integer")
	}
	ids := map[string]bool{}
	for _, m := range c.Media {
		if !safeID(m.ID) || ids[m.ID] || filepath.Base(m.Filename) != m.Filename || m.Filename == "." || strings.ContainsAny(m.Filename, "/\\") {
			return errors.New("invalid media identity or filename")
		}
		if m.Width <= 0 || m.Height <= 0 || m.DurationSeconds <= 0 || math.IsNaN(m.DurationSeconds) || math.IsInf(m.DurationSeconds, 0) || m.FrameRate <= 0 {
			return errors.New("invalid media dimensions or duration")
		}
		ids[m.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range c.Playlist.VideoIDs {
		if !ids[id] || seen[id] {
			return errors.New("playlist contains missing or duplicate media")
		}
		seen[id] = true
	}
	return nil
}
func Load(root string) (Config, error) {
	c := Default()
	b, e := os.ReadFile(filepath.Join(root, "catalog.json"))
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	return c, c.Validate()
}
func AtomicJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".write-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func Save(root string, c *Config) error {
	if e := c.Validate(); e != nil {
		return e
	}
	next := *c
	next.Revision++
	if e := AtomicJSON(filepath.Join(root, "catalog.json"), next); e != nil {
		return e
	}
	*c = next
	return nil
}
func ReadRuntime(root string) (Runtime, error) {
	var r Runtime
	b, e := os.ReadFile(filepath.Join(root, "runtime.json"))
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return r, e
	}
	if r.SchemaVersion != 1 {
		return r, errors.New("unsupported runtime schema")
	}
	return r, nil
}
func VideoPath(root string, m MediaEntry) string {
	return filepath.Join(root, "videos", m.ID, m.Filename)
}
func ThumbnailPath(root string, m MediaEntry) string {
	return filepath.Join(root, "videos", m.ID, "thumbnail.jpg")
}

type Probe func(context.Context, string, string) (MediaEntry, error)

// Import copies first, then probes the copy. Only a fully validated file becomes visible.
func Import(ctx context.Context, root, source string, probe Probe) (entry MediaEntry, err error) {
	ext := strings.ToLower(filepath.Ext(source))
	if !slices.Contains([]string{".mp4", ".mov", ".m4v"}, ext) {
		return entry, errors.New("仅支持 MP4、MOV 和 M4V")
	}
	base := filepath.Join(root, "videos")
	if err = os.MkdirAll(base, 0700); err != nil {
		return
	}
	stage, err := os.MkdirTemp(base, ".import-*")
	if err != nil {
		return entry, err
	}
	defer os.RemoveAll(stage)
	in, err := os.Open(source)
	if err != nil {
		return entry, err
	}
	defer in.Close()
	out, err := os.Create(filepath.Join(stage, "video"+ext))
	if err != nil {
		return entry, err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return entry, err
	}
	if closeErr != nil {
		return entry, closeErr
	}
	entry, err = probe(ctx, filepath.Join(stage, "video"+ext), filepath.Join(stage, "thumbnail.jpg"))
	if err != nil {
		return
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	entry.ID = fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	entry.Name = strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	entry.Filename = "video" + ext
	entry.AddedAtUnix = float64(time.Now().Unix())
	check := Default()
	check.Media = []MediaEntry{entry}
	if err = check.Validate(); err != nil {
		return
	}
	err = os.Rename(stage, filepath.Join(base, entry.ID))
	return
}

// Reap removes only explicitly deleted media after the extension acknowledges the new
// configuration and closes its readers. A stopped/stale extension cannot authorize deletion.
func Reap(root string) error {
	// A delayed cleanup may race a later import. Always use the latest committed catalog.
	c, loadErr := Load(root)
	if loadErr != nil {
		return loadErr
	}

	r, e := ReadRuntime(root)
	if e != nil || r.AppliedRevision < c.Revision || time.Since(time.Unix(int64(r.UpdatedAtUnix), 0)) > 15*time.Second {
		return nil
	}
	for _, id := range c.RemovedMediaIDs {
		if !safeID(id) || slices.ContainsFunc(c.Media, func(m MediaEntry) bool { return m.ID == id }) || slices.Contains(r.OpenVideoIDs, id) {
			continue
		}
		if e = os.RemoveAll(filepath.Join(root, "videos", id)); e != nil {
			return e
		}
	}
	return nil
}
