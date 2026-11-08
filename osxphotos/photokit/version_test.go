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

// The file of PhotoKit lies outside dir, as in the library, and is read-only,
// so that an export that opens it for writing fails. Both versions of
// PHVideoRequestOptionsVersion give it.
func videoFake(t *testing.T, size int) (*fake.Bridge, string, []byte) {
	t.Helper()
	b, _ := versionFake(1, nil)
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	src := filepath.Join(t.TempDir(), "IMG_0001.MOV")
	if err := os.WriteFile(src, data, 0o400); err != nil {
		t.Fatal(err)
	}
	b.Videos = map[string]string{versionVideoID + "/0": src, versionVideoID + "/1": src}
	return b, src, data
}

func TestExportVideo(t *testing.T) {
	fetchVideo := []string{"FetchAssets " + versionVideoID, "AssetResources " + versionVideoID + "/L0/001"}
	fetchPhoto := []string{"FetchAssets " + versionPhotoID, "AssetResources " + versionPhotoID + "/L0/001"}
	requestOriginal := "RequestVideo " + versionVideoID + " 1"

	// The size is not a multiple of the chunk, so that a copy that drops the
	// last part of the file fails here.
	t.Run("video", func(t *testing.T) {
		for _, tt := range []struct {
			v       Version
			version int
		}{{VersionCurrent, 0}, {VersionUnadjusted, 1}, {VersionOriginal, 1}} {
			b, src, data := videoFake(t, 3<<20+17)
			before, err := os.Stat(src)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			progress := 0
			o := ExportOptions{Progress: func(float64) { progress++ }}
			if err := (&Library{b}).ExportVideo(t.Context(), versionVideoID, tt.v, dir, "IMG_0001.MOV", o); err != nil {
				t.Fatalf("version %d: %v", tt.v, err)
			}
			if progress != 0 {
				t.Errorf("version %d: Progress called %d times, want none, as upstream has no progress", tt.v, progress)
			}
			path := filepath.Join(dir, "IMG_0001.MOV")
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
				t.Errorf("version %d: file = %d bytes, %v; want the %d bytes of the file of PhotoKit", tt.v, len(got), err, len(data))
			}
			if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
				t.Errorf("version %d: Stat = %v, %v; want mode 0600", tt.v, fi, err)
			}
			if got, err := os.ReadFile(src); err != nil || !bytes.Equal(got, data) {
				t.Errorf("version %d: the file of PhotoKit = %d bytes, %v; want it as it was", tt.v, len(got), err)
			}
			if fi, err := os.Stat(src); err != nil || !fi.ModTime().Equal(before.ModTime()) || fi.Mode() != before.Mode() {
				t.Errorf("version %d: the file of PhotoKit changed its time or mode: %v", tt.v, err)
			}
			wantCalls(t, b, append(fetchVideo, fmt.Sprintf("RequestVideo %s %d", versionVideoID, tt.version))...)
		}
	})

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tt := range []struct {
		name  string
		ctx   context.Context
		set   func(b *fake.Bridge, src string)
		uuid  string
		v     Version
		want  error
		cause error
		text  string
		calls []string
	}{
		{"no such asset", t.Context(), nil, unknownID, VersionOriginal, ErrNotFound, nil, "", []string{"FetchAssets " + unknownID}},
		{"no such asset, version 3", t.Context(), nil, unknownID, Version(3), ErrNotFound, nil, "", []string{"FetchAssets " + unknownID}},
		{"photo", t.Context(), nil, versionPhotoID, VersionOriginal, ErrMediaType, nil, ": 1", fetchPhoto},
		{"photo, version 3", t.Context(), nil, versionPhotoID, Version(3), ErrMediaType, nil, ": 1", fetchPhoto},
		{"Live Photo", t.Context(), nil, versionLiveID, VersionOriginal, ErrMediaType, nil, ": 1", []string{"FetchAssets " + versionLiveID, "AssetResources " + versionLiveID + "/L0/001"}},
		{"version 3", t.Context(), nil, versionVideoID, Version(3), ErrInvalidVersion, nil, ": 3", fetchVideo},
		{"version -1", t.Context(), nil, versionVideoID, Version(-1), ErrInvalidVersion, nil, ": -1", fetchVideo},
		{"asset gone at the request", t.Context(), func(b *fake.Bridge, _ string) { clear(b.Videos) }, versionVideoID, VersionOriginal, ErrNoResource, nil, "", append(fetchVideo, requestOriginal)},
		{"no file", t.Context(), func(b *fake.Bridge, _ string) { b.Videos[versionVideoID+"/1"] = "" }, versionVideoID, VersionOriginal, ErrNoResource, fs.ErrNotExist, "", append(fetchVideo, requestOriginal)},
		{"file gone", t.Context(), func(b *fake.Bridge, src string) { b.Videos[versionVideoID+"/1"] = src + ".gone" }, versionVideoID, VersionOriginal, ErrNoResource, fs.ErrNotExist, "", append(fetchVideo, requestOriginal)},
		{"not a file", t.Context(), func(b *fake.Bridge, src string) { b.Videos[versionVideoID+"/1"] = filepath.Dir(src) }, versionVideoID, VersionOriginal, ErrNoResource, nil, "", append(fetchVideo, requestOriginal)},
		{"PhotoKit error", t.Context(), func(b *fake.Bridge, _ string) {
			b.DoneErr = &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3164}
		}, versionVideoID, VersionOriginal, ErrExport, nil, "PHPhotosErrorDomain code 3164", append(fetchVideo, requestOriginal)},
		{"canceled", canceled, nil, versionVideoID, VersionOriginal, context.Canceled, nil, "", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, src, _ := videoFake(t, 1<<10)
			if tt.set != nil {
				tt.set(b, src)
			}
			dir := t.TempDir()
			err := (&Library{b}).ExportVideo(tt.ctx, tt.uuid, tt.v, dir, "IMG_0001.MOV", ExportOptions{})
			if !errors.Is(err, tt.want) || !strings.HasSuffix(err.Error(), tt.text) {
				t.Errorf("err = %v, want %v ending in %q", err, tt.want, tt.text)
			}
			if tt.cause != nil && !errors.Is(err, tt.cause) {
				t.Errorf("err = %v, want it to wrap %v", err, tt.cause)
			}
			// The path is inside the library, which the error of an export must not show.
			if err != nil && strings.Contains(err.Error(), filepath.Dir(src)) {
				t.Errorf("err = %v, want it without the path of the file of PhotoKit", err)
			}
			noFile(t, filepath.Join(dir, "IMG_0001.MOV"))
			wantCalls(t, b, tt.calls...)
		})
	}

	// The cancel comes with the first chunk, so that the copy has 63 MiB left
	// whatever the load of the runner.
	t.Run("canceled during the copy", func(t *testing.T) {
		b, _, _ := videoFake(t, 1)
		src := filepath.Join(t.TempDir(), "IMG_0002.MOV")
		if err := os.WriteFile(src, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(src, 64<<20); err != nil {
			t.Fatal(err)
		}
		b.Videos[versionVideoID+"/1"] = src
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		old, writes, largest := fileWrite, 0, 0
		fileWrite = func(f *os.File, p []byte) (int, error) {
			writes++
			largest = max(largest, len(p))
			cancel()
			return old(f, p)
		}
		t.Cleanup(func() { fileWrite = old })
		dir := t.TempDir()
		if err := (&Library{b}).ExportVideo(ctx, versionVideoID, VersionOriginal, dir, "IMG_0001.MOV", ExportOptions{}); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		if writes != 1 || largest > 1<<20 {
			t.Errorf("%d chunks written, the largest of %d bytes; want the copy to stop after the first, of 1 MiB at most", writes, largest)
		}
		noFile(t, filepath.Join(dir, "IMG_0001.MOV"))
		wantCalls(t, b, append(fetchVideo, requestOriginal)...)
	})

	t.Run("write error", func(t *testing.T) {
		diskFull := errors.New("disk full")
		old := fileWrite
		fileWrite = func(*os.File, []byte) (int, error) { return 0, diskFull }
		t.Cleanup(func() { fileWrite = old })
		b, _, _ := videoFake(t, 1<<10)
		dir := t.TempDir()
		if err := (&Library{b}).ExportVideo(t.Context(), versionVideoID, VersionOriginal, dir, "IMG_0001.MOV", ExportOptions{}); !errors.Is(err, diskFull) {
			t.Errorf("err = %v, want the error of the write", err)
		}
		noFile(t, filepath.Join(dir, "IMG_0001.MOV"))
	})

	// FileUtil.copy raises on a file that is_file() accepts but open() refuses
	// (photokit.py:904-905, 912), as one is without Full Disk Access.
	t.Run("file that cannot be opened", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root opens a file of mode 0000")
		}
		b, src, _ := videoFake(t, 1<<10)
		if err := os.Chmod(src, 0); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		err := (&Library{b}).ExportVideo(t.Context(), versionVideoID, VersionOriginal, dir, "IMG_0001.MOV", ExportOptions{})
		if !errors.Is(err, fs.ErrPermission) || errors.Is(err, ErrNoResource) || strings.Contains(err.Error(), filepath.Dir(src)) {
			t.Errorf("err = %v, want fs.ErrPermission, not ErrNoResource, without the path of the file of PhotoKit", err)
		}
		noFile(t, filepath.Join(dir, "IMG_0001.MOV"))
		wantCalls(t, b, append(fetchVideo, requestOriginal)...)
	})

	t.Run("existing file", func(t *testing.T) {
		b, src, data := videoFake(t, 1<<10)
		dir := t.TempDir()
		path := filepath.Join(dir, "IMG_0001.MOV")
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		lib := &Library{b}
		if err := lib.ExportVideo(t.Context(), versionVideoID, VersionOriginal, dir, "IMG_0001.MOV", ExportOptions{}); !errors.Is(err, fs.ErrExist) {
			t.Errorf("without Overwrite: err = %v, want fs.ErrExist", err)
		}
		wantCalls(t, b, fetchVideo...)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		old := fileWrite
		t.Cleanup(func() { fileWrite = old })
		// In this order: the second case cancels ctx for good.
		for _, tt := range []struct {
			name string
			set  func()
			want error
		}{
			{"file gone", func() { b.Videos[versionVideoID+"/1"] = src + ".gone" }, ErrNoResource},
			{"canceled during the copy", func() {
				b.Videos[versionVideoID+"/1"] = src
				fileWrite = func(f *os.File, p []byte) (int, error) {
					cancel()
					return old(f, p)
				}
			}, context.Canceled},
		} {
			tt.set()
			if err := lib.ExportVideo(ctx, versionVideoID, VersionOriginal, dir, "IMG_0001.MOV", ExportOptions{Overwrite: true}); !errors.Is(err, tt.want) {
				t.Errorf("Overwrite, %s: err = %v, want %v", tt.name, err, tt.want)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != "old" {
				t.Errorf("Overwrite, %s: file = %q, %v; want it kept", tt.name, got, err)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Errorf("Overwrite, %s: %d entries in dir, %v; want the old file only", tt.name, len(entries), err)
			}
		}
		fileWrite = old
		if err := lib.ExportVideo(t.Context(), versionVideoID, VersionOriginal, dir, "IMG_0001.MOV", ExportOptions{Overwrite: true}); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
			t.Errorf("file = %d bytes, %v; want the %d bytes of the file of PhotoKit", len(got), err, len(data))
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		for _, name := range []string{"", ".", "..", "a/b", "a\x00"} {
			b, _, _ := videoFake(t, 1)
			dir := t.TempDir()
			if err := (&Library{b}).ExportVideo(t.Context(), versionVideoID, VersionOriginal, dir, name, ExportOptions{}); !errors.Is(err, ErrInvalidName) {
				t.Errorf("ExportVideo(%q): err = %v, want ErrInvalidName", name, err)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
				t.Errorf("ExportVideo(%q): %d entries in dir, %v; want none", name, len(entries), err)
			}
			wantCalls(t, b)
		}
	})

	t.Run("canceled during the request", func(t *testing.T) {
		old := cancelGrace
		cancelGrace = 100 * time.Millisecond
		t.Cleanup(func() { cancelGrace = old })
		b, _, _ := videoFake(t, 1)
		b.HoldRequest = make(chan struct{})
		t.Cleanup(func() { close(b.HoldRequest) })
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() {
			for ctx.Err() == nil && !slices.Contains(b.Calls(), requestOriginal) {
				time.Sleep(time.Millisecond)
			}
			cancel()
		}()
		dir := t.TempDir()
		if err := (&Library{b}).ExportVideo(ctx, versionVideoID, VersionOriginal, dir, "IMG_0001.MOV", ExportOptions{}); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		noFile(t, filepath.Join(dir, "IMG_0001.MOV"))
		wantCalls(t, b, append(fetchVideo, requestOriginal, "Cancel "+versionVideoID)...)
	})
}

const versionSlowMoID = "99999999-9999-9999-9999-999999999999"

// The render of the composition, returned before the file of PhotoKit, differs
// from that file, so that a test sees which one was written; its size is not a
// multiple of the chunk of the copy.
func slowMoFake(t *testing.T) (*fake.Bridge, []byte, []byte) {
	t.Helper()
	b, src, file := videoFake(t, 1<<10)
	b.Assets = append(b.Assets, bridge.AssetRecord{LocalIdentifier: versionSlowMoID + "/L0/001", MediaType: 2, MediaSubtypes: subtypeHighFrameRate})
	render := make([]byte, 3<<20+17)
	for i := range render {
		render[i] = byte(i % 241)
	}
	b.Compositions = map[string][]byte{versionSlowMoID + "/0": render}
	b.Videos[versionSlowMoID+"/1"] = src
	return b, render, file
}

func onlyEntries(t *testing.T, dir string, want int) {
	t.Helper()
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != want {
		t.Errorf("%d entries in dir, %v; want %d, without the directory of the render", len(entries), err, want)
	}
}

// Upstream renders the current version of a slow-motion video only
// (photokit.py:876-884).
func TestExportVideoSlowMo(t *testing.T) {
	fetchSlowMo := []string{"FetchAssets " + versionSlowMoID, "AssetResources " + versionSlowMoID + "/L0/001"}
	render := "ExportComposition " + versionSlowMoID + " 0"
	slowMoErr := &bridge.PhotoKitError{Domain: "AVFoundationErrorDomain", Code: -11800}

	t.Run("current", func(t *testing.T) {
		b, want, _ := slowMoFake(t)
		dir := t.TempDir()
		progress := 0
		o := ExportOptions{Progress: func(float64) { progress++ }}
		if err := (&Library{b}).ExportVideo(t.Context(), versionSlowMoID, VersionCurrent, dir, "IMG_0001.mov", o); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "IMG_0001.mov")
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
			t.Errorf("file = %d bytes, %v; want the %d bytes of the render", len(got), err, len(want))
		}
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("Stat = %v, %v; want mode 0600", fi, err)
		}
		if progress != 0 {
			t.Errorf("Progress called %d times, want none, as upstream has no progress", progress)
		}
		// The render of a personal video must stay inside dir, out of sight and out of reach of other users.
		if tmp, mode := b.LastRenderDir(); filepath.Dir(tmp) != dir || !strings.HasPrefix(filepath.Base(tmp), ".osxphotos-") || !mode.IsDir() || mode.Perm() != 0o700 {
			t.Errorf("render in %q of mode %v, want a directory .osxphotos-* of mode 0700 inside dir", tmp, mode)
		}
		onlyEntries(t, dir, 1)
		wantCalls(t, b, append(fetchSlowMo, render)...)
	})

	t.Run("other versions", func(t *testing.T) {
		for _, v := range []Version{VersionUnadjusted, VersionOriginal} {
			b, _, file := slowMoFake(t)
			dir := t.TempDir()
			if err := (&Library{b}).ExportVideo(t.Context(), versionSlowMoID, v, dir, "IMG_0001.MOV", ExportOptions{}); err != nil {
				t.Fatalf("version %d: %v", v, err)
			}
			if got, err := os.ReadFile(filepath.Join(dir, "IMG_0001.MOV")); err != nil || !bytes.Equal(got, file) {
				t.Errorf("version %d: file = %d bytes, %v; want the %d bytes of the file of PhotoKit", v, len(got), err, len(file))
			}
			wantCalls(t, b, append(fetchSlowMo, "RequestVideo "+versionSlowMoID+" 1")...)
		}
	})

	t.Run("current of a video that is not slow motion", func(t *testing.T) {
		b, _, file := slowMoFake(t)
		dir := t.TempDir()
		if err := (&Library{b}).ExportVideo(t.Context(), versionVideoID, VersionCurrent, dir, "IMG_0001.MOV", ExportOptions{}); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(filepath.Join(dir, "IMG_0001.MOV")); err != nil || !bytes.Equal(got, file) {
			t.Errorf("file = %d bytes, %v; want the %d bytes of the file of PhotoKit", len(got), err, len(file))
		}
		wantCalls(t, b, "FetchAssets "+versionVideoID, "AssetResources "+versionVideoID+"/L0/001", "RequestVideo "+versionVideoID+" 0")
	})

	// The class of upstream is PhotoKitMediaTypeError for an AVAsset that is not
	// a composition of two tracks (photokit.py:938-942) and PhotoKitExportError
	// for a failed session (826-828).
	for _, tt := range []struct {
		name string
		set  func(*fake.Bridge)
		want error
		text string
	}{
		{"not a composition", func(b *fake.Bridge) { clear(b.Compositions) }, ErrMediaType, ": not a slow-motion composition"},
		{"failed session", func(b *fake.Bridge) { b.DoneErr = slowMoErr }, ErrExport, "AVFoundationErrorDomain code -11800"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := slowMoFake(t)
			tt.set(b)
			dir := t.TempDir()
			err := (&Library{b}).ExportVideo(t.Context(), versionSlowMoID, VersionCurrent, dir, "IMG_0001.mov", ExportOptions{})
			if !errors.Is(err, tt.want) || !strings.HasSuffix(err.Error(), tt.text) {
				t.Errorf("err = %v, want %v ending in %q", err, tt.want, tt.text)
			}
			if want := errors.Is(tt.want, ErrExport); errors.Is(err, ErrExport) != want {
				t.Errorf("err = %v: ErrExport %t, want %t", err, !want, want)
			}
			onlyEntries(t, dir, 0)
			wantCalls(t, b, append(fetchSlowMo, render)...)
		})
	}

	t.Run("directory of the render", func(t *testing.T) {
		old := mkdirTemp
		mkdirTemp = func(string, string) (string, error) {
			return "", &fs.PathError{Op: "mkdirtemp", Path: "/x/.osxphotos-1", Err: fs.ErrPermission}
		}
		t.Cleanup(func() { mkdirTemp = old })
		b, _, _ := slowMoFake(t)
		dir := t.TempDir()
		err := (&Library{b}).ExportVideo(t.Context(), versionSlowMoID, VersionCurrent, dir, "IMG_0001.mov", ExportOptions{})
		if !errors.Is(err, fs.ErrPermission) || strings.Contains(err.Error(), "/x/") {
			t.Errorf("err = %v, want fs.ErrPermission without the path", err)
		}
		onlyEntries(t, dir, 0)
		wantCalls(t, b, fetchSlowMo...)
	})

	t.Run("canceled during the render", func(t *testing.T) {
		old := cancelGrace
		cancelGrace = 100 * time.Millisecond
		t.Cleanup(func() { cancelGrace = old })
		b, _, _ := slowMoFake(t)
		b.HoldRequest = make(chan struct{})
		t.Cleanup(func() { close(b.HoldRequest) })
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() {
			for ctx.Err() == nil && !slices.Contains(b.Calls(), render) {
				time.Sleep(time.Millisecond)
			}
			cancel()
		}()
		dir := t.TempDir()
		if err := (&Library{b}).ExportVideo(ctx, versionSlowMoID, VersionCurrent, dir, "IMG_0001.mov", ExportOptions{}); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		onlyEntries(t, dir, 0)
		wantCalls(t, b, append(fetchSlowMo, render, "Cancel "+versionSlowMoID)...)
	})

	// The render has three chunks of the copy left after the first.
	t.Run("canceled during the copy of the render", func(t *testing.T) {
		b, _, _ := slowMoFake(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		old, writes := fileWrite, 0
		fileWrite = func(f *os.File, p []byte) (int, error) {
			writes++
			cancel()
			return old(f, p)
		}
		t.Cleanup(func() { fileWrite = old })
		dir := t.TempDir()
		if err := (&Library{b}).ExportVideo(ctx, versionSlowMoID, VersionCurrent, dir, "IMG_0001.mov", ExportOptions{}); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		if writes != 1 {
			t.Errorf("%d chunks written, want the copy to stop after the first", writes)
		}
		onlyEntries(t, dir, 0)
		wantCalls(t, b, append(fetchSlowMo, render)...)
	})

	t.Run("existing file", func(t *testing.T) {
		b, want, _ := slowMoFake(t)
		dir := t.TempDir()
		path := filepath.Join(dir, "IMG_0001.mov")
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		lib := &Library{b}
		b.DoneErr = slowMoErr
		if err := lib.ExportVideo(t.Context(), versionSlowMoID, VersionCurrent, dir, "IMG_0001.mov", ExportOptions{Overwrite: true}); !errors.Is(err, ErrExport) {
			t.Errorf("Overwrite, failed session: err = %v, want ErrExport", err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != "old" {
			t.Errorf("Overwrite, failed session: file = %q, %v; want it kept", got, err)
		}
		onlyEntries(t, dir, 1)
		first, _ := b.LastRenderDir()
		b.DoneErr = nil
		if err := lib.ExportVideo(t.Context(), versionSlowMoID, VersionCurrent, dir, "IMG_0001.mov", ExportOptions{Overwrite: true}); err != nil {
			t.Fatal(err)
		}
		// A fixed name would fail on the directory that a killed export left.
		if second, _ := b.LastRenderDir(); second == first {
			t.Errorf("both renders in %q, want a new name for each", first)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
			t.Errorf("Overwrite: file = %d bytes, %v; want the %d bytes of the render", len(got), err, len(want))
		}
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("Overwrite: Stat = %v, %v; want mode 0600", fi, err)
		}
		onlyEntries(t, dir, 1)
	})

	t.Run("invalid name", func(t *testing.T) {
		b, _, _ := slowMoFake(t)
		dir := t.TempDir()
		if err := (&Library{b}).ExportVideo(t.Context(), versionSlowMoID, VersionCurrent, dir, "../x", ExportOptions{}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("err = %v, want ErrInvalidName", err)
		}
		onlyEntries(t, dir, 0)
		wantCalls(t, b)
	})
}

// The data of each resource names its type, so that a test sees which one was
// written.
func liveFake(types ...ResourceType) *fake.Bridge {
	b, _ := versionFake(1, nil)
	b.Resources, b.Data = map[string][]bridge.ResourceRecord{}, map[string][]byte{}
	for _, typ := range types {
		b.Resources[versionLiveID] = append(b.Resources[versionLiveID], bridge.ResourceRecord{Type: int(typ)})
		b.Data[fmt.Sprintf("%s/%d", versionLiveID, typ)] = fmt.Appendf(nil, "resource %d", typ)
	}
	return b
}

func TestExportLivePhoto(t *testing.T) {
	fetchLive := []string{"FetchAssets " + versionLiveID, "AssetResources " + versionLiveID + "/L0/001", "AssetResources " + versionLiveID}
	requestPhoto := "RequestResource " + versionLiveID + " 1"
	plain := []ResourceType{ResourcePhoto, ResourcePairedVideo}
	// AdjustmentBasePhoto has data, so that an Unadjusted version taken from it fails.
	edited := []ResourceType{ResourcePhoto, ResourcePairedVideo, ResourceFullSizePhoto, ResourceFullSizePairedVideo, ResourceAdjustmentData, ResourceAdjustmentBasePhoto}
	photoEdited := []ResourceType{ResourcePhoto, ResourcePairedVideo, ResourceFullSizePhoto, ResourceAdjustmentData}
	videoEdited := []ResourceType{ResourcePhoto, ResourcePairedVideo, ResourceFullSizePairedVideo, ResourceAdjustmentData}

	t.Run("resource of the version", func(t *testing.T) {
		for _, tt := range []struct {
			name      string
			resources []ResourceType
			kind      ResourceType
			byVersion [3]ResourceType
		}{
			{"photo without edits", plain, ResourcePhoto, [3]ResourceType{ResourcePhoto, ResourcePhoto, ResourcePhoto}},
			{"video without edits", plain, ResourcePairedVideo, [3]ResourceType{ResourcePairedVideo, ResourcePairedVideo, ResourcePairedVideo}},
			{"edited photo", edited, ResourcePhoto, [3]ResourceType{ResourceFullSizePhoto, ResourcePhoto, ResourcePhoto}},
			{"edited video", edited, ResourcePairedVideo, [3]ResourceType{ResourceFullSizePairedVideo, ResourcePairedVideo, ResourcePairedVideo}},
			{"photo of a photo-only edit", photoEdited, ResourcePhoto, [3]ResourceType{ResourceFullSizePhoto, ResourcePhoto, ResourcePhoto}},
			{"video of a photo-only edit", photoEdited, ResourcePairedVideo, [3]ResourceType{ResourcePairedVideo, ResourcePairedVideo, ResourcePairedVideo}},
			{"photo of a video-only edit", videoEdited, ResourcePhoto, [3]ResourceType{ResourcePhoto, ResourcePhoto, ResourcePhoto}},
			{"video of a video-only edit", videoEdited, ResourcePairedVideo, [3]ResourceType{ResourceFullSizePairedVideo, ResourcePairedVideo, ResourcePairedVideo}},
		} {
			for _, v := range imageVersions {
				t.Run(fmt.Sprintf("%s, version %d", tt.name, v), func(t *testing.T) {
					b := liveFake(tt.resources...)
					dir := t.TempDir()
					progress := 0
					o := ExportOptions{Progress: func(float64) { progress++ }}
					if err := (&Library{b}).ExportLivePhoto(t.Context(), versionLiveID, v, tt.kind, dir, "IMG_0001.HEIC", o); err != nil {
						t.Fatal(err)
					}
					want := tt.byVersion[v]
					path := filepath.Join(dir, "IMG_0001.HEIC")
					if got, err := os.ReadFile(path); err != nil || string(got) != fmt.Sprintf("resource %d", want) {
						t.Errorf("file = %q, %v; want the resource of type %d", got, err, want)
					}
					if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
						t.Errorf("Stat = %v, %v; want mode 0600", fi, err)
					}
					if progress == 0 {
						t.Error("Progress never called, want it called as by Export")
					}
					wantCalls(t, b, append(fetchLive, fmt.Sprintf("RequestResource %s %d", versionLiveID, want))...)
				})
			}
		}
	})

	t.Run("existing file", func(t *testing.T) {
		b := liveFake(plain...)
		dir := t.TempDir()
		path := filepath.Join(dir, "IMG_0001.HEIC")
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		lib := &Library{b}
		if err := lib.ExportLivePhoto(t.Context(), versionLiveID, VersionOriginal, ResourcePhoto, dir, "IMG_0001.HEIC", ExportOptions{}); !errors.Is(err, fs.ErrExist) {
			t.Errorf("without Overwrite: err = %v, want fs.ErrExist", err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != "old" {
			t.Errorf("without Overwrite: file = %q, %v; want it kept", got, err)
		}
		wantCalls(t, b, fetchLive...)
		if err := lib.ExportLivePhoto(t.Context(), versionLiveID, VersionOriginal, ResourcePhoto, dir, "IMG_0001.HEIC", ExportOptions{Overwrite: true}); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != fmt.Sprintf("resource %d", ResourcePhoto) {
			t.Errorf("Overwrite: file = %q, %v; want the Photo resource", got, err)
		}
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
			t.Errorf("Overwrite: %d entries in dir, %v; want the file only", len(entries), err)
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			kind ResourceType
			want error
		}{
			{"../x", ResourcePhoto, ErrInvalidName},
			{"", ResourcePairedVideo, ErrInvalidName},
			{"../x", ResourceVideo, ErrNoResource},
		} {
			b := liveFake(plain...)
			dir := t.TempDir()
			if err := (&Library{b}).ExportLivePhoto(t.Context(), versionLiveID, VersionOriginal, tt.kind, dir, tt.name, ExportOptions{}); !errors.Is(err, tt.want) {
				t.Errorf("ExportLivePhoto(%d, %q): err = %v, want %v", tt.kind, tt.name, err, tt.want)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
				t.Errorf("ExportLivePhoto(%d, %q): %d entries in dir, %v; want none", tt.kind, tt.name, len(entries), err)
			}
			wantCalls(t, b)
		}
	})

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tt := range []struct {
		name  string
		ctx   context.Context
		set   func(*fake.Bridge)
		uuid  string
		v     Version
		kind  ResourceType
		want  error
		text  string
		calls []string
	}{
		{"Video as kind", t.Context(), nil, versionLiveID, VersionCurrent, ResourceVideo, ErrNoResource, ": 2", nil},
		{"FullSizePhoto as kind", t.Context(), nil, versionLiveID, VersionCurrent, ResourceFullSizePhoto, ErrNoResource, ": 5", nil},
		{"no paired video", t.Context(), func(b *fake.Bridge) {
			b.Resources[versionLiveID] = []bridge.ResourceRecord{{Type: int(ResourcePhoto)}}
		}, versionLiveID, VersionOriginal, ResourcePairedVideo, ErrNoResource, ": 9", fetchLive},
		{"asset gone after the fetch", t.Context(), func(b *fake.Bridge) { delete(b.Resources, versionLiveID) }, versionLiveID, VersionOriginal, ResourcePhoto, ErrNotFound, "", fetchLive},
		{"not a Live Photo", t.Context(), nil, versionPhotoID, VersionOriginal, ResourcePhoto, ErrMediaType, ": 1", []string{"FetchAssets " + versionPhotoID, "AssetResources " + versionPhotoID + "/L0/001"}},
		{"not a Live Photo, version 3", t.Context(), nil, versionPhotoID, Version(3), ResourcePhoto, ErrMediaType, ": 1", []string{"FetchAssets " + versionPhotoID, "AssetResources " + versionPhotoID + "/L0/001"}},
		{"video", t.Context(), nil, versionVideoID, VersionOriginal, ResourcePairedVideo, ErrMediaType, ": 2", []string{"FetchAssets " + versionVideoID, "AssetResources " + versionVideoID + "/L0/001"}},
		{"video, version 3", t.Context(), nil, versionVideoID, Version(3), ResourcePairedVideo, ErrMediaType, ": 2", []string{"FetchAssets " + versionVideoID, "AssetResources " + versionVideoID + "/L0/001"}},
		{"version 3", t.Context(), nil, versionLiveID, Version(3), ResourcePhoto, ErrInvalidVersion, ": 3", []string{"FetchAssets " + versionLiveID, "AssetResources " + versionLiveID + "/L0/001"}},
		{"no such asset", t.Context(), nil, unknownID, VersionOriginal, ResourcePhoto, ErrNotFound, "", []string{"FetchAssets " + unknownID}},
		{"resource gone at the request", t.Context(), func(b *fake.Bridge) { clear(b.Data) }, versionLiveID, VersionOriginal, ResourcePhoto, ErrNoResource, "", append(fetchLive, requestPhoto)},
		{"PhotoKit error", t.Context(), func(b *fake.Bridge) {
			b.DoneErr = &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3164}
		}, versionLiveID, VersionOriginal, ResourcePhoto, ErrExport, "PHPhotosErrorDomain code 3164", append(fetchLive, requestPhoto)},
		{"canceled", canceled, nil, versionLiveID, VersionOriginal, ResourcePhoto, context.Canceled, "", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := liveFake(plain...)
			if tt.set != nil {
				tt.set(b)
			}
			dir := t.TempDir()
			err := (&Library{b}).ExportLivePhoto(tt.ctx, tt.uuid, tt.v, tt.kind, dir, "IMG_0001.HEIC", ExportOptions{})
			if !errors.Is(err, tt.want) || !strings.HasSuffix(err.Error(), tt.text) {
				t.Errorf("err = %v, want %v ending in %q", err, tt.want, tt.text)
			}
			noFile(t, filepath.Join(dir, "IMG_0001.HEIC"))
			wantCalls(t, b, tt.calls...)
		})
	}

	// A cancel from Progress puts the cancel inside the request whatever the
	// load of the runner.
	t.Run("canceled, completion never comes", func(t *testing.T) {
		old := cancelGrace
		cancelGrace = 100 * time.Millisecond
		t.Cleanup(func() { cancelGrace = old })
		b := liveFake(plain...)
		b.HoldRequest = make(chan struct{})
		t.Cleanup(func() { close(b.HoldRequest) })
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		o := ExportOptions{Progress: func(float64) { cancel() }}
		done := make(chan error, 1)
		go func() {
			done <- (&Library{b}).ExportLivePhoto(ctx, versionLiveID, VersionOriginal, ResourcePhoto, dir, "IMG_0001.HEIC", o)
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("err = %v, want context.Canceled", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("ExportLivePhoto still waits 5 s after the cancel, want it to return within the grace")
		}
		noFile(t, filepath.Join(dir, "IMG_0001.HEIC"))
		wantCalls(t, b, append(fetchLive, requestPhoto, "Cancel "+versionLiveID)...)
	})
}
