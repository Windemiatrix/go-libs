package photokit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge/fake"
)

const exportID = "66666666-6666-6666-6666-666666666666"

// The Video resource has no data: it is gone by the time it is requested.
func exportFake(size int) (*fake.Bridge, []byte) {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	return &fake.Bridge{
		Resources: map[string][]bridge.ResourceRecord{exportID: {{Type: 1}, {Type: 4}, {Type: 7}, {Type: 2}}},
		Data:      map[string][]byte{exportID + "/1": data, exportID + "/4": []byte("raw")},
		Chunk:     4096,
	}, data
}

func noFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat after the export: %v, want no file", err)
	}
}

func wantCalls(t *testing.T, b *fake.Bridge, want ...string) {
	t.Helper()
	if !slices.Equal(b.Calls(), want) {
		t.Errorf("calls = %q, want %q", b.Calls(), want)
	}
}

func TestExport(t *testing.T) {
	resources := "AssetResources " + exportID
	requestPhoto := fmt.Sprintf("RequestResource %s 1", exportID)

	t.Run("photo", func(t *testing.T) {
		b, data := exportFake(1 << 20)
		dir := t.TempDir()
		var progress []float64
		o := ExportOptions{Progress: func(f float64) { progress = append(progress, f) }}
		if err := (&Library{b}).Export(t.Context(), exportID, ResourcePhoto, dir, "IMG_0001.JPG", o); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "IMG_0001.JPG")
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
			t.Errorf("file = %d bytes, %v; want the %d bytes of the resource", len(got), err, len(data))
		}
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("Stat = %v, %v; want mode 0600", fi, err)
		}
		if len(progress) == 0 || !slices.IsSorted(progress) || progress[len(progress)-1] != 1 {
			t.Errorf("%d progress values, want values that grow to 1", len(progress))
		}
		wantCalls(t, b, resources, requestPhoto)

		// PhotoKit may deliver after Export returned: a late Write and Progress
		// are dropped.
		n := len(progress)
		sink := b.LastSink()
		if err := sink.Write([]byte("late")); !errors.Is(err, bridge.ErrCanceled) {
			t.Errorf("a late Write: err = %v, want bridge.ErrCanceled", err)
		}
		sink.Progress(0.5)
		if len(progress) != n {
			t.Errorf("a late Progress reached the caller")
		}
	})

	t.Run("RAW", func(t *testing.T) {
		b, _ := exportFake(1)
		dir := t.TempDir()
		if err := (&Library{b}).Export(t.Context(), exportID, ResourceAlternatePhoto, dir, "IMG_0001.CR2", ExportOptions{}); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(filepath.Join(dir, "IMG_0001.CR2")); err != nil || string(got) != "raw" {
			t.Errorf("file = %q, %v; want the AlternatePhoto resource", got, err)
		}
		wantCalls(t, b, resources, fmt.Sprintf("RequestResource %s 4", exportID))
	})

	t.Run("existing file", func(t *testing.T) {
		b, data := exportFake(1 << 10)
		path := filepath.Join(t.TempDir(), "IMG_0001.JPG")
		// Longer than the resource, so that a write over it without a new file
		// leaves a tail.
		old := bytes.Repeat([]byte("old"), 1<<10)
		if err := os.WriteFile(path, old, 0o600); err != nil {
			t.Fatal(err)
		}
		lib := &Library{b}
		if err := lib.Export(t.Context(), exportID, ResourcePhoto, filepath.Dir(path), "IMG_0001.JPG", ExportOptions{}); !errors.Is(err, fs.ErrExist) {
			t.Errorf("Export without Overwrite: err = %v, want fs.ErrExist", err)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, old) {
			t.Errorf("file = %d bytes, %v; want it kept", len(got), err)
		}
		wantCalls(t, b, resources)
		if err := lib.Export(t.Context(), exportID, ResourcePhoto, filepath.Dir(path), "IMG_0001.JPG", ExportOptions{Overwrite: true}); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
			t.Errorf("file = %d bytes, %v; want the %d bytes of the resource", len(got), err, len(data))
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		for _, name := range []string{"", ".", "..", "a/b", "a\x00", "/a", "../a"} {
			b, _ := exportFake(1)
			dir := t.TempDir()
			if err := (&Library{b}).Export(t.Context(), exportID, ResourcePhoto, dir, name, ExportOptions{}); !errors.Is(err, ErrInvalidName) {
				t.Errorf("Export(%q): err = %v, want ErrInvalidName", name, err)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
				t.Errorf("Export(%q): %d entries in dir, %v; want none", name, len(entries), err)
			}
			wantCalls(t, b)
		}
	})

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	bridgeErr := errors.New("bridge failed")
	photoKitErr := &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3164}
	for _, tt := range []struct {
		name  string
		ctx   context.Context
		set   func(*fake.Bridge)
		uuid  string
		kind  ResourceType
		dir   string
		want  error
		calls []string
	}{
		{"no such asset", t.Context(), nil, unknownID, ResourcePhoto, "", ErrNotFound, []string{"AssetResources " + unknownID}},
		{"no resource of the kind", t.Context(), nil, exportID, ResourceFullSizePhoto, "", ErrNoResource, []string{resources}},
		{"resource gone at the request", t.Context(), nil, exportID, ResourceVideo, "", ErrNoResource, []string{resources, fmt.Sprintf("RequestResource %s 2", exportID)}},
		{"bridge error", t.Context(), func(b *fake.Bridge) { b.ResourcesErr = bridgeErr }, exportID, ResourcePhoto, "", bridgeErr, []string{resources}},
		{"PhotoKit error", t.Context(), func(b *fake.Bridge) { b.DoneErr = photoKitErr }, exportID, ResourcePhoto, "", ErrExport, []string{resources, requestPhoto}},
		{"canceled", canceled, nil, exportID, ResourcePhoto, "", context.Canceled, nil},
		{"no directory", t.Context(), nil, exportID, ResourcePhoto, "missing", fs.ErrNotExist, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := exportFake(1 << 10)
			if tt.set != nil {
				tt.set(b)
			}
			dir := filepath.Join(t.TempDir(), tt.dir)
			err := (&Library{b}).Export(tt.ctx, tt.uuid, tt.kind, dir, "IMG_0001.JPG", ExportOptions{})
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			noFile(t, filepath.Join(dir, "IMG_0001.JPG"))
			wantCalls(t, b, tt.calls...)
		})
	}

	t.Run("PhotoKit error text", func(t *testing.T) {
		b, _ := exportFake(1 << 10)
		b.DoneErr = photoKitErr
		err := (&Library{b}).Export(t.Context(), exportID, ResourcePhoto, t.TempDir(), "IMG_0001.JPG", ExportOptions{})
		if err == nil || !strings.Contains(err.Error(), "PHPhotosErrorDomain") || !strings.Contains(err.Error(), "3164") {
			t.Errorf("err = %v, want the domain and code of the NSError", err)
		}
	})
	t.Run("overwritten file kept on an error", func(t *testing.T) {
		for _, tt := range []struct {
			kind    ResourceType
			doneErr error
			want    error
		}{
			{ResourcePhoto, photoKitErr, ErrExport},
			{ResourceFullSizePhoto, nil, ErrNoResource},
		} {
			b, _ := exportFake(1 << 10)
			b.DoneErr = tt.doneErr
			dir := t.TempDir()
			path := filepath.Join(dir, "IMG_0001.JPG")
			if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := (&Library{b}).Export(t.Context(), exportID, tt.kind, dir, "IMG_0001.JPG", ExportOptions{Overwrite: true}); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != "old" {
				t.Errorf("file = %q, %v; want it kept", got, err)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Errorf("%d entries in dir, %v; want the old file only", len(entries), err)
			}
		}
	})

	t.Run("hard link at name", func(t *testing.T) {
		b, data := exportFake(1 << 10)
		outside := filepath.Join(t.TempDir(), "x")
		if err := os.WriteFile(outside, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "IMG_0001.JPG")
		if err := os.Link(outside, path); err != nil {
			t.Fatal(err)
		}
		lib, o := &Library{b}, ExportOptions{Overwrite: true}
		b.DoneErr = photoKitErr
		if err := lib.Export(t.Context(), exportID, ResourcePhoto, dir, "IMG_0001.JPG", o); !errors.Is(err, ErrExport) {
			t.Errorf("failed Export: err = %v, want ErrExport", err)
		}
		b.DoneErr = nil
		if err := lib.Export(t.Context(), exportID, ResourcePhoto, dir, "IMG_0001.JPG", o); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
			t.Errorf("file = %d bytes, %v; want the %d bytes of the resource", len(got), err, len(data))
		}
		if got, err := os.ReadFile(outside); err != nil || string(got) != "old" {
			t.Errorf("linked file outside dir = %q, %v; want it kept", got, err)
		}
	})

	t.Run("write error", func(t *testing.T) {
		diskFull := errors.New("disk full")
		old := fileWrite
		fileWrite = func(*os.File, []byte) (int, error) { return 0, diskFull }
		t.Cleanup(func() { fileWrite = old })
		b, _ := exportFake(1 << 16)
		dir := t.TempDir()
		// The fake waits for Cancel after the failed write: without it Export
		// would wait for the deadline.
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		if err := (&Library{b}).Export(ctx, exportID, ResourcePhoto, dir, "IMG_0001.JPG", ExportOptions{}); !errors.Is(err, diskFull) {
			t.Errorf("err = %v, want the error of the write", err)
		}
		if ctx.Err() != nil {
			t.Errorf("Export returned at the deadline: the failed write did not cancel the request")
		}
		noFile(t, filepath.Join(dir, "IMG_0001.JPG"))
		wantCalls(t, b, resources, requestPhoto, "Cancel "+exportID)
	})

	// A one-chunk resource and a cancel from Progress put the cancel inside
	// the request whatever the load of the runner.
	t.Run("canceled, completion never comes", func(t *testing.T) {
		old := cancelGrace
		cancelGrace = 100 * time.Millisecond
		t.Cleanup(func() { cancelGrace = old })
		b, _ := exportFake(1)
		b.HoldRequest = make(chan struct{})
		t.Cleanup(func() { close(b.HoldRequest) })
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		start := time.Now()
		o := ExportOptions{Progress: func(float64) { cancel() }}
		if err := (&Library{b}).Export(ctx, exportID, ResourcePhoto, dir, "IMG_0001.JPG", o); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("Export returned after %v, want it within the grace after the cancel", d)
		}
		noFile(t, filepath.Join(dir, "IMG_0001.JPG"))
		wantCalls(t, b, resources, requestPhoto, "Cancel "+exportID)
	})

	t.Run("canceled, completion after the cancel", func(t *testing.T) {
		b, _ := exportFake(1)
		b.HoldRequest = make(chan struct{})
		var ended atomic.Bool
		go func() {
			for !slices.Contains(b.Calls(), "Cancel "+exportID) {
				time.Sleep(time.Millisecond)
			}
			time.Sleep(100 * time.Millisecond)
			ended.Store(true)
			close(b.HoldRequest)
		}()
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		o := ExportOptions{Progress: func(float64) { cancel() }}
		if err := (&Library{b}).Export(ctx, exportID, ResourcePhoto, dir, "IMG_0001.JPG", o); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		if !ended.Load() {
			t.Errorf("Export returned before PhotoKit ended the canceled request")
		}
		noFile(t, filepath.Join(dir, "IMG_0001.JPG"))
		wantCalls(t, b, resources, requestPhoto, "Cancel "+exportID)
	})
}

// Callers export in parallel: -race checks what the Exports share, the package
// variables and the bridge.
func TestExportRace(t *testing.T) {
	b, data := exportFake(1 << 14)
	lib, dir := &Library{b}, t.TempDir()
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 20 {
				name := fmt.Sprintf("IMG_%d_%d.JPG", g, i)
				if err := lib.Export(t.Context(), exportID, ResourcePhoto, dir, name, ExportOptions{}); err != nil {
					t.Errorf("Export(%s): %v", name, err)
				} else if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || !bytes.Equal(got, data) {
					t.Errorf("%s = %d bytes, %v; want %d", name, len(got), err, len(data))
				}
			}
		})
	}
	wg.Wait()
}
