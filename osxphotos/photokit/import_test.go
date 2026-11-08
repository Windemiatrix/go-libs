package photokit

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge/fake"
)

const (
	importID    = "77777777-7777-7777-7777-777777777777/L0/001"
	importAlbum = "88888888-8888-8888-8888-888888888888"
)

func importFiles(t *testing.T) (photo, video, raw string) {
	t.Helper()
	dir := t.TempDir()
	photo, video, raw = filepath.Join(dir, "IMG_0001.JPG"), filepath.Join(dir, "IMG_0001.MOV"), filepath.Join(dir, "IMG_0001.CR2")
	for _, p := range []string{photo, video, raw} {
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return photo, video, raw
}

func TestImport(t *testing.T) {
	photo, video, raw := importFiles(t)
	link := filepath.Join(filepath.Dir(photo), "IMG_0002.JPG")
	if err := os.Symlink(photo, link); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2001, 2, 3, 4, 5, 6, 250_000_000, time.UTC)

	for _, tt := range []struct {
		name string
		r    ImportRequest
		want bridge.ImportRecord
		call string
	}{
		{
			"photo",
			ImportRequest{Photo: photo, CreationDate: date, Location: &Location{Latitude: 52.5, Longitude: 13.4}, Favorite: true, Albums: []string{importAlbum}},
			bridge.ImportRecord{Photo: photo, HasCreationDate: true, CreationDate: 981173106.25, HasLocation: true, Latitude: 52.5, Longitude: 13.4, Favorite: true, Albums: []string{importAlbum}},
			"Import IMG_0001.JPG 1 albums",
		},
		{"video", ImportRequest{Video: video, Hidden: true}, bridge.ImportRecord{Video: video, Hidden: true}, "Import IMG_0001.MOV 0 albums"},
		{"Live Photo", ImportRequest{Photo: photo, PairedVideo: video}, bridge.ImportRecord{Photo: photo, PairedVideo: video}, "Import IMG_0001.JPG 0 albums"},
		{"RAW+JPEG", ImportRequest{Photo: photo, AlternatePhoto: raw}, bridge.ImportRecord{Photo: photo, AlternatePhoto: raw}, "Import IMG_0001.JPG 0 albums"},
		{"original filename", ImportRequest{Photo: photo, OriginalFilename: "IMG_0003.JPG"}, bridge.ImportRecord{Photo: photo, OriginalFilename: "IMG_0003.JPG"}, "Import IMG_0003.JPG 0 albums"},
		{"UTF-8 original filename", ImportRequest{Photo: photo, OriginalFilename: "IMG_caf\U000000e9.JPG"}, bridge.ImportRecord{Photo: photo, OriginalFilename: "IMG_caf\U000000e9.JPG"}, "Import IMG_caf\U000000e9.JPG 0 albums"},
		// PhotoKit reads through a link as through the file.
		{"symbolic link", ImportRequest{Photo: link}, bridge.ImportRecord{Photo: link}, "Import IMG_0002.JPG 0 albums"},
		{"location 0, 0", ImportRequest{Photo: photo, Location: &Location{}}, bridge.ImportRecord{Photo: photo, HasLocation: true}, "Import IMG_0001.JPG 0 albums"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &fake.Bridge{ImportID: importID}
			id, err := (&Library{b}).Import(t.Context(), tt.r)
			if err != nil || id != importID {
				t.Fatalf("Import = %q, %v; want %q", id, err, importID)
			}
			if got, _ := b.LastImport(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("record = %+v, want %+v", got, tt.want)
			}
			wantCalls(t, b, tt.call)
		})
	}

	t.Run("albums are copied", func(t *testing.T) {
		b := &fake.Bridge{ImportID: importID}
		albums := []string{importAlbum}
		if _, err := (&Library{b}).Import(t.Context(), ImportRequest{Photo: photo, Albums: albums}); err != nil {
			t.Fatal(err)
		}
		albums[0] = unknownID
		if got, _ := b.LastImport(); !reflect.DeepEqual(got.Albums, []string{importAlbum}) {
			t.Errorf("albums of the record = %q after the caller changed its slice, want a copy", got.Albums)
		}
	})

	t.Run("timeout of the bridge", func(t *testing.T) {
		b := &fake.Bridge{ImportID: importID}
		if _, err := (&Library{b}).Import(t.Context(), ImportRequest{Photo: photo}); err != nil {
			t.Fatal(err)
		}
		if _, d := b.LastImport(); d != time.Duration(math.MaxInt64) {
			t.Errorf("timeout = %v without a deadline, want math.MaxInt64: 0 would not wait at all", d)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
		defer cancel()
		if _, err := (&Library{b}).Import(ctx, ImportRequest{Photo: photo}); err != nil {
			t.Fatal(err)
		}
		if _, d := b.LastImport(); d <= 59*time.Minute || d > time.Hour {
			t.Errorf("timeout = %v, want the time left to the 1h deadline", d)
		}
	})

	missing := filepath.Join(filepath.Dir(photo), "missing.JPG")
	for _, tt := range []struct {
		name  string
		r     ImportRequest
		field string
	}{
		{"neither Photo nor Video", ImportRequest{}, "one of Photo or Video"},
		{"Photo and Video", ImportRequest{Photo: photo, Video: video}, "one of Photo or Video"},
		// The file exists relative to the directory of the test.
		{"relative path", ImportRequest{Photo: "import_test.go"}, "Photo"},
		{"no such file", ImportRequest{Video: missing}, "Video"},
		{"directory", ImportRequest{Photo: filepath.Dir(photo)}, "Photo"},
		{"NUL", ImportRequest{Photo: photo + "\x00b"}, "Photo"},
		{"PairedVideo without Photo", ImportRequest{Video: video, PairedVideo: video}, "PairedVideo"},
		{"AlternatePhoto without Photo", ImportRequest{Video: video, AlternatePhoto: raw}, "AlternatePhoto"},
		{"bad PairedVideo", ImportRequest{Photo: photo, PairedVideo: missing}, "PairedVideo"},
		{"bad AlternatePhoto", ImportRequest{Photo: photo, AlternatePhoto: "IMG_0001.CR2"}, "AlternatePhoto"},
		{"NUL in OriginalFilename", ImportRequest{Photo: photo, OriginalFilename: "IMG_0003\x00.JPG"}, "OriginalFilename"},
		{"OriginalFilename not UTF-8", ImportRequest{Photo: photo, OriginalFilename: "IMG_\xff.JPG"}, "OriginalFilename"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &fake.Bridge{ImportID: importID}
			id, err := (&Library{b}).Import(t.Context(), tt.r)
			if id != "" || !errors.Is(err, ErrImportFile) {
				t.Fatalf("Import = %q, %v; want ErrImportFile", id, err)
			}
			if got, want := err.Error(), ErrImportFile.Error()+": "+tt.field; got != want {
				t.Errorf("err = %q, want %q: the field only", got, want)
			}
			wantCalls(t, b)
		})
	}

	for _, album := range badIdentifiers {
		t.Run("album "+strconv.Quote(album), func(t *testing.T) {
			b := &fake.Bridge{ImportID: importID}
			id, err := (&Library{b}).Import(t.Context(), ImportRequest{Photo: photo, Albums: []string{importAlbum, album}})
			if id != "" || !errors.Is(err, ErrNotFound) || err.Error() != ErrNotFound.Error()+": album" {
				t.Errorf("Import = %q, %v; want ErrNotFound: album", id, err)
			}
			wantCalls(t, b)
		})
	}

	t.Run("canceled before the call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		b := &fake.Bridge{ImportID: importID}
		_, err := (&Library{b}).Import(ctx, ImportRequest{Photo: photo})
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrOutcomeUnknown) {
			t.Errorf("err = %v, want context.Canceled: nothing was changed", err)
		}
		// A bridge call started by mistake logs from its goroutine after the return.
		time.Sleep(20 * time.Millisecond)
		wantCalls(t, b)
	})

	bridgeErr := errors.New("bridge failed")
	for _, tt := range []struct {
		name      string
		id        string
		err       error
		want, not []error
		text      string
	}{
		{"no album", importID, bridge.ErrNoAlbum, []error{ErrNotFound}, []error{ErrImport}, "album"},
		{"user canceled", "", &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3072}, []error{ErrUserCanceled}, []error{ErrImport}, ""},
		{"3072 of another domain", "", &bridge.PhotoKitError{Domain: "NSCocoaErrorDomain", Code: 3072}, []error{ErrImport}, []error{ErrUserCanceled}, "NSCocoaErrorDomain code 3072"},
		{"PhotoKit error", "", &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3300}, []error{ErrImport}, nil, "PHPhotosErrorDomain code 3300"},
		{"bridge error", "", bridgeErr, []error{bridgeErr}, []error{ErrImport}, ""},
		{"bridge timeout", "", context.DeadlineExceeded, []error{ErrOutcomeUnknown, context.DeadlineExceeded}, nil, ""},
		{"no identifier", "", nil, []error{ErrImport}, nil, "no identifier"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &fake.Bridge{ImportID: tt.id, ImportErr: tt.err}
			id, err := (&Library{b}).Import(t.Context(), ImportRequest{Photo: photo})
			if id != "" {
				t.Errorf("id = %q, want none", id)
			}
			for _, w := range tt.want {
				if !errors.Is(err, w) {
					t.Errorf("err = %v, want %v", err, w)
				}
			}
			for _, n := range tt.not {
				if errors.Is(err, n) {
					t.Errorf("err = %v, want it not %v", err, n)
				}
			}
			if err == nil || !strings.Contains(err.Error(), tt.text) {
				t.Errorf("err = %v, want %q in it", err, tt.text)
			}
			wantCalls(t, b, "Import IMG_0001.JPG 0 albums")
		})
	}

	t.Run("answer after a wait", func(t *testing.T) {
		b := &fake.Bridge{ImportID: importID, HoldImport: make(chan struct{})}
		time.AfterFunc(20*time.Millisecond, func() { close(b.HoldImport) })
		if id, err := (&Library{b}).Import(t.Context(), ImportRequest{Photo: photo}); err != nil || id != importID {
			t.Errorf("Import = %q, %v; want %q: without a deadline the bridge waits for PhotoKit", id, err, importID)
		}
	})

	t.Run("deadline before the outcome", func(t *testing.T) {
		b := &fake.Bridge{ImportID: importID, HoldImport: make(chan struct{})}
		t.Cleanup(func() { close(b.HoldImport) })
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := (&Library{b}).Import(ctx, ImportRequest{Photo: photo})
		if !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want ErrOutcomeUnknown with context.DeadlineExceeded", err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("Import returned after %v, want it at the deadline", d)
		}
	})

	// The bridge would answer at the 2 s deadline: Import must not wait for it.
	t.Run("canceled before the outcome", func(t *testing.T) {
		b := &fake.Bridge{ImportID: importID, HoldImport: make(chan struct{})}
		t.Cleanup(func() { close(b.HoldImport) })
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		time.AfterFunc(10*time.Millisecond, cancel)
		start := time.Now()
		_, err := (&Library{b}).Import(ctx, ImportRequest{Photo: photo})
		if !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want ErrOutcomeUnknown with context.Canceled", err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("Import returned after %v, want it at the cancel", d)
		}
	})
}

// Not parallel: it counts the goroutines of the process.
func TestImportCancelInFlight(t *testing.T) {
	photo, _, _ := importFiles(t)
	b := &fake.Bridge{ImportID: importID, HoldImport: make(chan struct{})}
	before := runtime.NumGoroutine()
	// An Import that ignores ctx.Done() waits for the deadline and ends with
	// DeadlineExceeded, not Canceled.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	time.AfterFunc(10*time.Millisecond, cancel)
	_, err := (&Library{b}).Import(ctx, ImportRequest{Photo: photo})
	if !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want ErrOutcomeUnknown with context.Canceled", err)
	}
	close(b.HoldImport)
	for deadline := time.Now().Add(time.Second); runtime.NumGoroutine() > before; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines, %d before the import: the late answer was not dropped", runtime.NumGoroutine(), before)
		}
	}
}
