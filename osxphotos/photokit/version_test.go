package photokit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge/fake"
)

const (
	versionPhotoID = "77777777-7777-7777-7777-777777777777"
	versionVideoID = "88888888-8888-8888-8888-888888888888"
	versionLiveID  = "DDDDDDDD-DDDD-DDDD-DDDD-DDDDDDDDDDDD"
)

var imageVersions = []Version{VersionCurrent, VersionUnadjusted, VersionOriginal}

// The data of each version differs, so that a test sees which one was
// written.
func versionFake(size int, metadata []byte) (*fake.Bridge, []byte) {
	record := bridge.ImageRecord{UTI: "public.jpeg", Orientation: 6, Degraded: true, Path: "/x/IMG_0001.JPG", Metadata: metadata}
	b := &fake.Bridge{
		Assets: []bridge.AssetRecord{
			{LocalIdentifier: versionPhotoID + "/L0/001", MediaType: 1},
			{LocalIdentifier: versionVideoID + "/L0/001", MediaType: 2},
			{LocalIdentifier: versionLiveID + "/L0/001", MediaType: 1, MediaSubtypes: subtypeLive},
		},
		Images: map[string]fake.ImageData{versionLiveID + "/0": {Data: []byte("still"), Record: record}},
	}
	for _, v := range imageVersions {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte((i + int(v)) % 251)
		}
		b.Images[imageKey(versionPhotoID, v)] = fake.ImageData{Data: data, Record: record}
	}
	return b, b.Images[imageKey(versionPhotoID, VersionOriginal)].Data
}

func imageKey(id string, v Version) string { return fmt.Sprintf("%s/%d", id, v) }

func binaryPlist(t *testing.T, v any) []byte {
	t.Helper()
	b, err := plist.Marshal(v, plist.BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// An XML list of exactly size bytes: the binary format cannot be sized to the byte.
func xmlPlist(size int) []byte {
	const head, tail = `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>x</key><string>`, `</string></dict></plist>`
	return []byte(head + strings.Repeat("a", size-len(head)-len(tail)) + tail)
}

func TestExportImage(t *testing.T) {
	fetchPhoto := []string{"FetchAssets " + versionPhotoID, "AssetResources " + versionPhotoID + "/L0/001"}
	requestOriginal := "RequestImage " + versionPhotoID + " 2"

	t.Run("photo", func(t *testing.T) {
		nested := map[string]any{"Make": "Camera"}
		meta := binaryPlist(t, map[string]any{"x": nested, "s": "text", "n": 6, "b": []byte{0, 1, 2}})
		for _, v := range imageVersions {
			b, _ := versionFake(1<<20, meta)
			data := b.Images[imageKey(versionPhotoID, v)].Data
			dir := t.TempDir()
			var progress []float64
			o := ExportOptions{Progress: func(f float64) { progress = append(progress, f) }}
			info, err := (&Library{b}).ExportImage(t.Context(), versionPhotoID, v, dir, "IMG_0001.JPG", o)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "IMG_0001.JPG")
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
				t.Errorf("version %d: file = %d bytes, %v; want the %d bytes of the version", v, len(got), err, len(data))
			}
			if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
				t.Errorf("version %d: Stat = %v, %v; want mode 0600", v, fi, err)
			}
			if len(progress) == 0 || !slices.IsSorted(progress) || progress[len(progress)-1] != 1 {
				t.Errorf("version %d: %d progress values, want values that grow to 1", v, len(progress))
			}
			if info.UTI != "public.jpeg" || info.Orientation != 6 || !info.Degraded || info.Path != "/x/IMG_0001.JPG" {
				t.Errorf("version %d: ImageInfo = %+v, want the fields of the record", v, info)
			}
			m := info.Metadata
			if len(m) != 4 || !reflect.DeepEqual(m["x"], nested) || m["s"] != "text" || fmt.Sprint(m["n"]) != "6" {
				t.Errorf("version %d: Metadata = %v, want the decoded dictionary", v, m)
			}
			if got, ok := m["b"].([]byte); !ok || !bytes.Equal(got, []byte{0, 1, 2}) {
				t.Errorf("version %d: Metadata[b] = %#v, want the bytes", v, m["b"])
			}
			wantCalls(t, b, append(fetchPhoto, fmt.Sprintf("RequestImage %s %d", versionPhotoID, v))...)
		}
	})

	t.Run("Live Photo", func(t *testing.T) {
		b, _ := versionFake(1, nil)
		dir := t.TempDir()
		if _, err := (&Library{b}).ExportImage(t.Context(), versionLiveID, VersionCurrent, dir, "IMG_0001.JPG", ExportOptions{}); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(filepath.Join(dir, "IMG_0001.JPG")); err != nil || string(got) != "still" {
			t.Errorf("file = %q, %v; want the still", got, err)
		}
		wantCalls(t, b, "FetchAssets "+versionLiveID, "AssetResources "+versionLiveID+"/L0/001", "RequestImage "+versionLiveID+" 0")
	})

	t.Run("metadata", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			meta []byte
			want bool
		}{
			{"none", nil, false},
			{"not a dictionary", binaryPlist(t, []any{"x"}), false},
			{"not a property list", []byte("("), false},
			{"1 MiB", xmlPlist(1 << 20), true},
			{"over 1 MiB", xmlPlist(1<<20 + 1), false},
		} {
			b, _ := versionFake(1, tt.meta)
			info, err := (&Library{b}).ExportImage(t.Context(), versionPhotoID, VersionOriginal, t.TempDir(), "IMG_0001.JPG", ExportOptions{})
			if err != nil || (info.Metadata != nil) != tt.want {
				t.Errorf("%s: Metadata of %d keys, %v; want decoded %t", tt.name, len(info.Metadata), err, tt.want)
			}
		}
	})

	t.Run("existing file", func(t *testing.T) {
		b, data := versionFake(1<<10, nil)
		dir := t.TempDir()
		path := filepath.Join(dir, "IMG_0001.JPG")
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		lib := &Library{b}
		exportImage := func(o ExportOptions) error {
			_, err := lib.ExportImage(t.Context(), versionPhotoID, VersionOriginal, dir, "IMG_0001.JPG", o)
			return err
		}
		if err := exportImage(ExportOptions{}); !errors.Is(err, fs.ErrExist) {
			t.Errorf("without Overwrite: err = %v, want fs.ErrExist", err)
		}
		wantCalls(t, b, fetchPhoto...)
		key := imageKey(versionPhotoID, VersionOriginal)
		img := b.Images[key]
		for _, tt := range []struct {
			name string
			set  func()
			want error
		}{
			{"PhotoKit error", func() { b.DoneErr = &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3164} }, ErrExport},
			{"no image data", func() { b.DoneErr, b.Images[key] = nil, fake.ImageData{} }, ErrNoResource},
		} {
			tt.set()
			if err := exportImage(ExportOptions{Overwrite: true}); !errors.Is(err, tt.want) {
				t.Errorf("Overwrite, %s: err = %v, want %v", tt.name, err, tt.want)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != "old" {
				t.Errorf("Overwrite, %s: file = %q, %v; want it kept", tt.name, got, err)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Errorf("Overwrite, %s: %d entries in dir, %v; want the old file only", tt.name, len(entries), err)
			}
		}
		b.Images[key] = img
		if err := exportImage(ExportOptions{Overwrite: true}); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
			t.Errorf("file = %d bytes, %v; want the %d bytes of the image", len(got), err, len(data))
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		for _, name := range []string{"", ".", "..", "a/b", "a\x00", "../a"} {
			b, _ := versionFake(1, nil)
			dir := t.TempDir()
			if _, err := (&Library{b}).ExportImage(t.Context(), versionPhotoID, VersionOriginal, dir, name, ExportOptions{}); !errors.Is(err, ErrInvalidName) {
				t.Errorf("ExportImage(%q): err = %v, want ErrInvalidName", name, err)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
				t.Errorf("ExportImage(%q): %d entries in dir, %v; want none", name, len(entries), err)
			}
			wantCalls(t, b)
		}
	})

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	bridgeErr := errors.New("bridge failed")
	for _, tt := range []struct {
		name  string
		ctx   context.Context
		set   func(*fake.Bridge)
		uuid  string
		v     Version
		want  error
		text  string
		calls []string
	}{
		{"no such asset", t.Context(), nil, unknownID, VersionOriginal, ErrNotFound, "", []string{"FetchAssets " + unknownID}},
		{"not a photo", t.Context(), nil, versionVideoID, VersionOriginal, ErrMediaType, ": 2", []string{"FetchAssets " + versionVideoID, "AssetResources " + versionVideoID + "/L0/001"}},
		{"version 3", t.Context(), nil, versionPhotoID, Version(3), ErrInvalidVersion, ": 3", fetchPhoto},
		{"version -1", t.Context(), nil, versionPhotoID, Version(-1), ErrInvalidVersion, ": -1", fetchPhoto},
		{"asset gone at the request", t.Context(), func(b *fake.Bridge) { clear(b.Images) }, versionPhotoID, VersionOriginal, ErrNoResource, "", append(fetchPhoto, requestOriginal)},
		{"no image data", t.Context(), func(b *fake.Bridge) { b.Images[imageKey(versionPhotoID, VersionOriginal)] = fake.ImageData{} }, versionPhotoID, VersionOriginal, ErrNoResource, "", append(fetchPhoto, requestOriginal)},
		{"PhotoKit error", t.Context(), func(b *fake.Bridge) { b.DoneErr = &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3164} }, versionPhotoID, VersionOriginal, ErrExport, "PHPhotosErrorDomain code 3164", append(fetchPhoto, requestOriginal)},
		{"bridge error", t.Context(), func(b *fake.Bridge) { b.FetchErr = bridgeErr }, versionPhotoID, VersionOriginal, bridgeErr, "", []string{"FetchAssets " + versionPhotoID}},
		{"canceled", canceled, nil, versionPhotoID, VersionOriginal, context.Canceled, "", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := versionFake(1<<10, nil)
			if tt.set != nil {
				tt.set(b)
			}
			dir := t.TempDir()
			info, err := (&Library{b}).ExportImage(tt.ctx, tt.uuid, tt.v, dir, "IMG_0001.JPG", ExportOptions{})
			if !errors.Is(err, tt.want) || !strings.HasSuffix(err.Error(), tt.text) {
				t.Errorf("err = %v, want %v ending in %q", err, tt.want, tt.text)
			}
			if !reflect.DeepEqual(info, ImageInfo{}) {
				t.Errorf("ImageInfo = %+v, want none", info)
			}
			noFile(t, filepath.Join(dir, "IMG_0001.JPG"))
			wantCalls(t, b, tt.calls...)
		})
	}

	// A cancel from Progress puts the cancel inside the request whatever the
	// load of the runner.
	t.Run("canceled, completion never comes", func(t *testing.T) {
		old := cancelGrace
		cancelGrace = 100 * time.Millisecond
		t.Cleanup(func() { cancelGrace = old })
		b, _ := versionFake(1, nil)
		b.HoldRequest = make(chan struct{})
		t.Cleanup(func() { close(b.HoldRequest) })
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		start := time.Now()
		o := ExportOptions{Progress: func(float64) { cancel() }}
		info, err := (&Library{b}).ExportImage(ctx, versionPhotoID, VersionOriginal, dir, "IMG_0001.JPG", o)
		if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(info, ImageInfo{}) {
			t.Errorf("ExportImage = %+v, %v; want none, context.Canceled", info, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("ExportImage returned after %v, want it within the grace after the cancel", d)
		}
		noFile(t, filepath.Join(dir, "IMG_0001.JPG"))
		wantCalls(t, b, append(fetchPhoto, requestOriginal, "Cancel "+versionPhotoID)...)
	})
}

// -race checks what the exports share: the bridge and the record of each sink.
func TestExportImageRace(t *testing.T) {
	b, data := versionFake(1<<14, nil)
	lib, dir := &Library{b}, t.TempDir()
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 10 {
				name := fmt.Sprintf("IMG_%d_%d.JPG", g, i)
				if info, err := lib.ExportImage(t.Context(), versionPhotoID, VersionOriginal, dir, name, ExportOptions{}); err != nil || info.UTI != "public.jpeg" {
					t.Errorf("ExportImage(%s) = %+v, %v", name, info, err)
				} else if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || !bytes.Equal(got, data) {
					t.Errorf("%s = %d bytes, %v; want %d", name, len(got), err, len(data))
				}
			}
		})
	}
	wg.Wait()
}
