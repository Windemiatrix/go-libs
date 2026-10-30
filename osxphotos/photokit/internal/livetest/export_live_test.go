//go:build photokit_live

package livetest

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/export"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
)

// An original kept only in iCloud is downloaded; upstream waits 300 s (photokit.py:196-203).
const exportTimeout = 5 * time.Minute

// ZDATASTORESUBTYPE of an original resource (photosdb.py:2447).
const originalSubtype = 1

func TestLiveExport(t *testing.T) {
	lib := openLibrary(t)
	photos := untrashedMarkerPhotos(t)
	x := &exporter{lib: lib, dir: t.TempDir(), taken: map[string]bool{}, exported: map[string]*photoinfo.PhotoInfo{}}
	// A subtest of hidden photos would export them a second time: the subtests of
	// originals count them instead.
	hidden := func(p *photoinfo.PhotoInfo) bool { return p.Hidden }

	t.Run("local original", func(t *testing.T) {
		local := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.Path != "" && !p.IsMissing })
		if len(local) == 0 {
			skipNone(t, "photo with its original on disk")
		}
		differ := 0
		for _, p := range local {
			if path, ok := x.export(t, p, originalType(p), x.name(p.OriginalFilename)); ok && sum(t, path) != sum(t, p.Path) {
				differ++
			}
		}
		t.Logf("%d local originals exported, %d of them hidden", len(local), count(local, hidden))
		if differ > 0 {
			t.Errorf("%d of %d exported originals differ from their file in the library", differ, len(local))
		}
	})

	var cloud []*photoinfo.PhotoInfo
	t.Run("cloud original", func(t *testing.T) {
		cloud = filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.IsMissing && p.Path == "" })
		if len(cloud) == 0 {
			skipNone(t, "photo whose original is only in iCloud")
		}
		empty, short, silent := 0, 0, 0
		for _, p := range cloud {
			path, ok := x.export(t, p, originalType(p), x.name(p.OriginalFilename))
			switch {
			case !ok:
			case size(t, path) == 0:
				empty++
			case p.OriginalFilesize > 0 && size(t, path) != p.OriginalFilesize:
				short++
			case x.lastProgressCalls == 0:
				silent++
			}
		}
		t.Logf("%d originals only in iCloud exported, %d of them hidden", len(cloud), count(cloud, hidden))
		if empty > 0 || short > 0 || silent > 0 {
			t.Errorf("of %d originals only in iCloud, %d exported empty, %d not of the size of the original, %d without a call of Progress", len(cloud), empty, short, silent)
		}
	})

	t.Run("RAW", func(t *testing.T) {
		raws := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.HasRAW })
		if len(raws) == 0 {
			skipNone(t, "RAW+JPEG photo")
		}
		empty, differ, jpeg := 0, 0, 0
		for _, p := range raws {
			path, ok := x.export(t, p, photokit.ResourceAlternatePhoto, export.Names(p, x.name(p.OriginalFilename)).RAW)
			switch {
			case !ok:
			case size(t, path) == 0:
				empty++
			case p.PathRaw != "" && sum(t, path) != sum(t, p.PathRaw):
				differ++
			case p.PathRaw == "" && string(head(t, path)[:2]) == "\xff\xd8":
				jpeg++
			}
		}
		if empty > 0 || differ > 0 || jpeg > 0 {
			t.Errorf("of %d RAW exports, %d empty, %d differ from the RAW file in the library, %d a JPEG", len(raws), empty, differ, jpeg)
		}
	})

	t.Run("Live after an edit", func(t *testing.T) {
		edited := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.Live && p.HasAdjustments })
		if len(edited) == 0 {
			skipNone(t, "edited Live Photo")
		}
		notOriginal, notEdited := 0, 0
		for _, p := range edited {
			photo, okPhoto := x.export(t, p, photokit.ResourcePhoto, x.name(p.OriginalFilename))
			full, okFull := x.export(t, p, photokit.ResourceFullSizePhoto, x.name(export.EditedName(p)))
			if !okPhoto || !okFull {
				continue
			}
			if p.Path != "" && sum(t, photo) != sum(t, p.Path) {
				notOriginal++
			}
			if sum(t, full) == sum(t, photo) {
				notEdited++
			}
		}
		if notOriginal > 0 || notEdited > 0 {
			t.Errorf("of %d edited Live Photos, %d with a Photo resource other than the original, %d with a FullSizePhoto equal to it", len(edited), notOriginal, notEdited)
		}
	})

	t.Run("Live video", func(t *testing.T) {
		lives := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.Live })
		if len(lives) == 0 {
			skipNone(t, "Live Photo")
		}
		empty, differ, notVideo := 0, 0, 0
		for _, p := range lives {
			path, ok := x.export(t, p, photokit.ResourcePairedVideo, export.Names(p, x.name(p.OriginalFilename)).Live)
			switch {
			case !ok:
			case size(t, path) == 0:
				empty++
			case p.PathLive != "" && sum(t, path) != sum(t, p.PathLive):
				differ++
			case p.PathLive == "" && string(head(t, path)[4:]) != "ftyp":
				notVideo++
			}
		}
		if empty > 0 || differ > 0 || notVideo > 0 {
			t.Errorf("of %d Live videos, %d empty, %d differ from the video in the library, %d without ftyp", len(lives), empty, differ, notVideo)
		}
	})

	// An expired deadline ends Export in Resources, before the bridge
	// (fetch.go:35-37); the shortest of these may too, so a count above 0 does
	// not prove that a cancel reached PhotoKit.
	t.Run("deadline", func(t *testing.T) {
		p := slices.MaxFunc(photos, func(a, b *photoinfo.PhotoInfo) int { return cmp.Compare(a.OriginalFilesize, b.OriginalFilesize) })
		deadlines := []time.Duration{time.Millisecond, 2 * time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond}
		ended := 0
		for _, d := range deadlines {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), d)
			err := lib.Export(ctx, p.UUID, originalType(p), dir, "IMG_0001.JPG", photokit.ExportOptions{})
			cancel()
			switch {
			case err == nil:
				if n := size(t, filepath.Join(dir, "IMG_0001.JPG")); p.OriginalFilesize > 0 && n != p.OriginalFilesize {
					t.Errorf("the export within %v is not of the size of the original", d)
				}
			case errors.Is(err, context.DeadlineExceeded):
				ended++
				emptyDir(t, dir)
			default:
				t.Errorf("Export with a deadline of %v: %s, want nil or DeadlineExceeded", d, exportFailure(err))
			}
		}
		t.Logf("%d of %d exports of the largest original ended at their deadline", ended, len(deadlines))
		if ended == 0 {
			t.Skip("no export of the largest original ended at its deadline; the cancel was not reached")
		}
	})

	t.Run("errors", func(t *testing.T) {
		p := photos[0]
		dir, o := t.TempDir(), photokit.ExportOptions{}
		const none = "00000000-0000-0000-0000-000000000000"
		if err := lib.Export(t.Context(), none, photokit.ResourcePhoto, dir, "IMG_0001.JPG", o); !errors.Is(err, photokit.ErrNotFound) {
			t.Errorf("Export of an unknown UUID: %s, want ErrNotFound", exportFailure(err))
		}
		// Export finds no resource of the kind before the bridge (export.go:62-64):
		// PK_NO_RESOURCE is covered by TestCodeError only.
		if plain := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.IsPhoto && !p.HasRAW }); len(plain) > 0 {
			if err := lib.Export(t.Context(), plain[0].UUID, photokit.ResourceAlternatePhoto, dir, "IMG_0001.CR2", o); !errors.Is(err, photokit.ErrNoResource) {
				t.Errorf("Export of the RAW of a photo without one: %s, want ErrNoResource", exportFailure(err))
			}
		}
		if err := lib.Export(t.Context(), p.UUID, originalType(p), dir, "../x", o); !errors.Is(err, photokit.ErrInvalidName) {
			t.Errorf("Export to ../x: %s, want ErrInvalidName", exportFailure(err))
		}
		if _, err := os.Lstat(filepath.Join(dir, "..", "x")); !errors.Is(err, fs.ErrNotExist) {
			t.Error("Export to ../x left a file beside the directory")
		}
		emptyDir(t, dir)

		ctx, cancel := context.WithTimeout(t.Context(), exportTimeout)
		defer cancel()
		path := filepath.Join(dir, "IMG_0001.JPG")
		if err := lib.Export(ctx, p.UUID, originalType(p), dir, "IMG_0001.JPG", o); err != nil {
			t.Fatalf("Export: %s", exportFailure(err))
		}
		first := sum(t, path)
		if err := lib.Export(ctx, p.UUID, originalType(p), dir, "IMG_0001.JPG", o); !errors.Is(err, fs.ErrExist) {
			t.Errorf("Export over an existing file: %s, want fs.ErrExist", exportFailure(err))
		}
		if sum(t, path) != first {
			t.Error("Export over an existing file without Overwrite changed it")
		}
		if err := lib.Export(ctx, p.UUID, originalType(p), dir, "IMG_0001.JPG", photokit.ExportOptions{Overwrite: true}); err != nil {
			t.Errorf("Export with Overwrite: %s", exportFailure(err))
		} else if sum(t, path) != first {
			t.Error("Export with Overwrite wrote other bytes of the same resource")
		}
	})

	// The read of TestMain came before any export; this one shows what the exports changed.
	db := reopenLibrary(t)

	t.Run("identifiers after export", func(t *testing.T) {
		if len(x.exported) == 0 {
			t.Skip("no export succeeded")
		}
		// An empty GUID equals an empty one whatever the export did.
		exported := slices.Collect(maps.Values(x.exported))
		t.Logf("of %d exported photos, %d with a CloudAssetGUID, %d with a CloudMasterGUID", len(exported),
			count(exported, func(p *photoinfo.PhotoInfo) bool { return p.CloudAssetGUID != "" }),
			count(exported, func(p *photoinfo.PhotoInfo) bool { return p.CloudMasterGUID != "" }))
		changed := count(exported, func(p *photoinfo.PhotoInfo) bool {
			q, ok := db.Photo(p.UUID)
			return !ok || q.CloudAssetGUID != p.CloudAssetGUID || q.CloudMasterGUID != p.CloudMasterGUID
		})
		if changed > 0 {
			t.Errorf("%d of %d exported photos lost their UUID or changed a cloud GUID", changed, len(exported))
		}
	})

	// An observation: Photos may keep a downloaded original or not.
	t.Run("availability after download", func(t *testing.T) {
		if len(cloud) == 0 {
			skipNone(t, "photo whose original is only in iCloud")
		}
		present, local := 0, 0
		for _, p := range cloud {
			q, ok := db.Photo(p.UUID)
			if !ok {
				continue
			}
			if !q.IsMissing {
				present++
			}
			if slices.ContainsFunc(q.Resources, func(r photoinfo.Resource) bool {
				return r.Subtype == originalSubtype && r.LocalAvailability == 1
			}) {
				local++
			}
		}
		t.Logf("of %d originals only in iCloud before the export: %d not missing now, %d with a local original resource", len(cloud), present, local)
	})
}

// One directory for all exports, so that IncrementName numbers the names as
// osxphotos export does in one destination.
type exporter struct {
	lib      *photokit.Library
	dir      string
	taken    map[string]bool
	exported map[string]*photoinfo.PhotoInfo

	lastProgressCalls int
}

func (x *exporter) name(name string) string {
	name, _ = export.IncrementName(name, 0, x.taken)
	x.taken[export.StemKey(name)] = true
	return name
}

func (x *exporter) export(t *testing.T, p *photoinfo.PhotoInfo, kind photokit.ResourceType, name string) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), exportTimeout)
	defer cancel()
	// Progress runs on a queue of PhotoKit.
	var mu sync.Mutex
	var got []float64
	o := photokit.ExportOptions{Progress: func(f float64) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, f)
	}}
	if err := x.lib.Export(ctx, p.UUID, kind, x.dir, name, o); err != nil {
		t.Errorf("Export of a resource of type %d: %s", kind, exportFailure(err))
		return "", false
	}
	mu.Lock()
	defer mu.Unlock()
	// PhotoKit need not call Progress for a resource it has on disk.
	if len(got) > 0 && (got[0] < 0 || !slices.IsSorted(got) || got[len(got)-1] != 1) {
		t.Errorf("Progress of an export of a resource of type %d: %d values, not rising from 0 or more to 1", kind, len(got))
	}
	x.lastProgressCalls = len(got)
	x.exported[p.UUID] = p
	return filepath.Join(x.dir, name), true
}

func originalType(p *photoinfo.PhotoInfo) photokit.ResourceType {
	if p.IsMovie {
		return photokit.ResourceVideo
	}
	return photokit.ResourcePhoto
}

// The error of Export may wrap a *fs.PathError, which names the file: as in
// failure, only its class and the domain and code of an NSError are printed.
func exportFailure(err error) string {
	nsError := "none"
	var pk *bridge.PhotoKitError
	if errors.As(err, &pk) {
		nsError = pk.Error()
	}
	return fmt.Sprintf("%s (ErrNoResource: %t, ErrInvalidName: %t, ErrExport: %t, NSError: %s, DeadlineExceeded: %t, ErrExist: %t)",
		failure(err), errors.Is(err, photokit.ErrNoResource), errors.Is(err, photokit.ErrInvalidName), errors.Is(err, photokit.ErrExport),
		nsError, errors.Is(err, context.DeadlineExceeded), errors.Is(err, fs.ErrExist))
}

// The errors of os name the path, so they are not printed. A video may not fit
// in memory, and a killed binary leaves its copies in TMPDIR.
func sum(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal("cannot read a file to hash it")
	}
	defer f.Close() //nolint:errcheck // opened only to be read
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal("cannot read a file to hash it")
	}
	return [sha256.Size]byte(h.Sum(nil))
}

// A file shorter than 8 bytes leaves zeros, which match no magic.
func head(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal("cannot read an exported file")
	}
	defer f.Close() //nolint:errcheck // opened only to be read
	b := make([]byte, 8)
	_, _ = io.ReadFull(f, b)
	return b
}

func size(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal("cannot stat an exported file")
	}
	return fi.Size()
}

func emptyDir(t *testing.T, dir string) {
	t.Helper()
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("%d files left in the directory of a failed export (read error: %t)", len(entries), err != nil)
	}
}

func reopenLibrary(t *testing.T) *photosdb.DB {
	t.Helper()
	path, err := systemLibraryPath()
	if err != nil {
		t.Fatal("livetest: no home directory")
	}
	// As in TestMain: an interrupted Open still removes its copy of the database.
	ctx, stop := signal.NotifyContext(t.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := photosdb.Open(ctx, path, photosdb.Options{})
	if err != nil {
		t.Fatal(openFailure(err))
	}
	return db
}
