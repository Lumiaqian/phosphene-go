// Package platform is a native module: its interfaces are implemented in
// Swift, Rust or Zig, in the directory of the language (see AGENTS.md).
package platform

import "context"

type MediaInfo struct {
	Width     int64
	Height    int64
	FrameRate float64
	Duration  float64
}

type DisplayInfo struct {
	Identifier    string
	Name          string
	RuntimeNumber int64
	BuiltIn       bool
}

// Host exposes the macOS operations needed by the local video library.
//
//mygo:native
type Host interface {
	// Probe validates a decodable video and writes its thumbnail.
	Probe(ctx context.Context, path string, thumbnailPath string) (MediaInfo, error)
	// Displays returns stable identifiers instead of display names as identities.
	//mygo:mainthread
	Displays() ([]DisplayInfo, error)
	// NotifyLibraryChanged invalidates this provider's tiles and notifies its extension.
	NotifyLibraryChanged() error
}
