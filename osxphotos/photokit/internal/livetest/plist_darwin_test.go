//go:build darwin && cgo

package livetest

import (
	"debug/macho"
	"os"
	"testing"

	"howett.net/plist"
)

func TestInfoPlistEmbedded(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f, err := macho.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	s := f.Section("__info_plist")
	if s == nil || s.Seg != "__TEXT" {
		t.Fatal("the test binary has no __TEXT,__info_plist section")
	}
	data, err := s.Data()
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		BundleID string `plist:"CFBundleIdentifier"`
		Usage    string `plist:"NSPhotoLibraryUsageDescription"`
	}
	if _, err := plist.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.BundleID != "io.github.windemiatrix.osxphotos.livetest" {
		t.Errorf("CFBundleIdentifier = %q", info.BundleID)
	}
	if info.Usage == "" {
		t.Error("NSPhotoLibraryUsageDescription is empty")
	}
}
