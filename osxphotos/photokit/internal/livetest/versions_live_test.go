//go:build photokit_live

package livetest

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit"
)

// Metadata holds EXIF, GPS and maker data and Path a path inside the library:
// the tests print only counts.
func TestLiveVersions(t *testing.T) {
	lib := openLibrary(t)
	visible := visibleMarkerPhotos(t)
	// The Original of a RAW+JPEG may be the RAW (photokit.py:588-589).
	photos := filter(visible, func(p *photoinfo.PhotoInfo) bool { return p.IsPhoto && !p.HasRAW })
	if len(photos) == 0 {
		skipNone(t, "photo without a RAW")
	}
	x := &exporter{lib: lib, dir: t.TempDir(), taken: map[string]bool{}, exported: map[string]*photoinfo.PhotoInfo{}}
	local := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.Path != "" && !p.IsMissing })
	cloud := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.IsMissing && p.Path == "" })
	bySize := func(a, b *photoinfo.PhotoInfo) int { return cmp.Compare(a.OriginalFilesize, b.OriginalFilesize) }

	// First, before the subtests below download the originals: only a download
	// reports progress, and PhotoKit stops a request only there.
	t.Run("stop", func(t *testing.T) {
		if len(cloud) == 0 {
			skipNone(t, "photo whose original is only in iCloud")
		}
		p := slices.MaxFunc(cloud, bySize)
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var once sync.Once
		var calls atomic.Int64
		o := photokit.ExportOptions{Progress: func(float64) {
			calls.Add(1)
			once.Do(cancel)
		}}
		_, err := lib.ExportImage(ctx, p.UUID, photokit.VersionOriginal, dir, "IMG_0001.JPG", o)
		n := calls.Load()
		if n == 0 {
			skipNone(t, "image download that reports progress")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("ExportImage canceled at its first report of progress: %s, want context.Canceled", exportFailure(err))
		}
		emptyDir(t, dir)
		// One call may race the cancel, and the call that PhotoKit stops at reaches Progress too.
		if later := n - 1; later > 2 {
			t.Errorf("Progress called %d times after the cancel, want 2 at most", later)
		}
	})

	t.Run("original equals file", func(t *testing.T) {
		if len(local) == 0 {
			skipNone(t, "photo with its original on disk")
		}
		exported, differ, noUTI, badOrientation, otherUTI, oriented, otherOrientation := 0, 0, 0, 0, 0, 0, 0
		for _, p := range local {
			path, info, ok := x.exportImage(t, p, photokit.VersionOriginal)
			if !ok {
				continue
			}
			exported++
			if sum(t, path) != sum(t, p.Path) {
				differ++
			}
			if info.UTI == "" {
				noUTI++
			}
			if info.Orientation < 1 || info.Orientation > 8 {
				badOrientation++
			}
			if info.UTI != p.UTIOriginal {
				otherUTI++
			}
			if p.OriginalOrientation != 0 {
				oriented++
				if info.Orientation != p.OriginalOrientation {
					otherOrientation++
				}
			}
		}
		t.Logf("of %d originals on disk, %d exported, %d with a UTI other than UTIOriginal; of %d with an OriginalOrientation, %d with another orientation", len(local), exported, otherUTI, oriented, otherOrientation)
		if differ > 0 {
			t.Errorf("%d of %d exported Original versions differ from their file in the library", differ, exported)
		}
		if noUTI > 0 || badOrientation > 0 {
			t.Errorf("of %d exported Original versions, %d without a UTI, %d with an orientation outside 1 to 8", exported, noUTI, badOrientation)
		}
		// One photo may differ from photosdb; all of them point at a field of pk_go_image read from the wrong value.
		if otherUTI == exported && exported > 0 {
			t.Errorf("all %d exported Original versions have a UTI other than UTIOriginal", exported)
		}
		if otherOrientation == oriented && oriented > 0 {
			t.Errorf("all %d exported Original versions with an OriginalOrientation have another orientation", oriented)
		}
	})

	t.Run("current of an edited photo", func(t *testing.T) {
		edited := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.HasAdjustments })
		if len(edited) == 0 {
			skipNone(t, "edited photo")
		}
		exported, same, plainCurrent, plainOriginal := 0, 0, 0, 0
		for _, p := range edited {
			currentPath, _, okCurrent := x.exportImage(t, p, photokit.VersionCurrent)
			originalPath, _, okOriginal := x.exportImage(t, p, photokit.VersionOriginal)
			plainPath, _, okPlain := x.exportImage(t, p, photokit.VersionUnadjusted)
			if !okCurrent || !okOriginal || !okPlain {
				continue
			}
			exported++
			current, original, plain := sum(t, currentPath), sum(t, originalPath), sum(t, plainPath)
			if current == original {
				same++
			}
			if plain == current {
				plainCurrent++
			}
			if plain == original {
				plainOriginal++
			}
		}
		t.Logf("of %d edited photos, %d exported in the three versions, %d with an Unadjusted version equal to the Original", len(edited), exported, plainOriginal)
		if same > 0 || plainCurrent > 0 {
			t.Errorf("of %d edited photos exported in the three versions, %d with a Current version equal to the Original, %d with an Unadjusted version equal to the Current", exported, same, plainCurrent)
		}
	})

	t.Run("metadata", func(t *testing.T) {
		if len(local) == 0 {
			skipNone(t, "photo with its original on disk")
		}
		exported, none, keys, withPath, relative, degraded := 0, 0, 0, 0, 0, 0
		for _, p := range local {
			_, info, ok := x.exportImage(t, p, photokit.VersionCurrent)
			if !ok {
				continue
			}
			exported++
			if len(info.Metadata) == 0 {
				none++
			}
			keys = max(keys, len(info.Metadata))
			if info.Path != "" {
				withPath++
				if !filepath.IsAbs(info.Path) {
					relative++
				}
			}
			if info.Degraded {
				degraded++
			}
		}
		t.Logf("of %d Current versions of originals on disk, %d exported, %d with a Path; %d top-level keys of Metadata at most", len(local), exported, withPath, keys)
		if none > 0 || degraded > 0 || relative > 0 {
			t.Errorf("of %d exported Current versions of originals on disk, %d without Metadata, %d degraded, %d with a Path that is not absolute", exported, none, degraded, relative)
		}
	})

	// Progress is only an observation: TestLiveExport may have downloaded the
	// original already.
	t.Run("cloud", func(t *testing.T) {
		if len(cloud) == 0 {
			skipNone(t, "photo whose original is only in iCloud")
		}
		short, silent := 0, 0
		for _, p := range cloud {
			path, _, ok := x.exportImage(t, p, photokit.VersionOriginal)
			switch {
			case !ok:
			case p.OriginalFilesize > 0 && size(t, path) != p.OriginalFilesize:
				short++
			case x.lastProgressCalls == 0:
				silent++
			}
		}
		t.Logf("of %d originals only in iCloud, %d exported without a call of Progress", len(cloud), silent)
		if short > 0 {
			t.Errorf("%d of %d Original versions only in iCloud are not of the size of the original", short, len(cloud))
		}
	})

	t.Run("errors", func(t *testing.T) {
		p := photos[0]
		dir, o := t.TempDir(), photokit.ExportOptions{}
		const none = "00000000-0000-0000-0000-000000000000"
		if _, err := lib.ExportImage(t.Context(), none, photokit.VersionOriginal, dir, "IMG_0001.JPG", o); !errors.Is(err, photokit.ErrNotFound) {
			t.Errorf("ExportImage of an unknown UUID: %s, want ErrNotFound", exportFailure(err))
		}
		if _, err := lib.ExportImage(t.Context(), p.UUID, photokit.VersionOriginal, dir, "../x", o); !errors.Is(err, photokit.ErrInvalidName) {
			t.Errorf("ExportImage to ../x: %s, want ErrInvalidName", exportFailure(err))
		}
		emptyDir(t, dir)

		ctx, cancel := context.WithTimeout(t.Context(), exportTimeout)
		defer cancel()
		if _, err := lib.ExportImage(ctx, p.UUID, photokit.VersionOriginal, dir, "IMG_0001.JPG", o); err != nil {
			t.Fatalf("ExportImage: %s", exportFailure(err))
		}
		first := sum(t, filepath.Join(dir, "IMG_0001.JPG"))
		if _, err := lib.ExportImage(ctx, p.UUID, photokit.VersionOriginal, dir, "IMG_0001.JPG", o); !errors.Is(err, fs.ErrExist) {
			t.Errorf("ExportImage over an existing file: %s, want fs.ErrExist", exportFailure(err))
		}
		if sum(t, filepath.Join(dir, "IMG_0001.JPG")) != first {
			t.Error("ExportImage over an existing file without Overwrite changed it")
		}
	})

	t.Run("video", func(t *testing.T) {
		videos := filter(visible, func(p *photoinfo.PhotoInfo) bool { return p.IsMovie })
		if len(videos) == 0 {
			skipNone(t, "video")
		}
		dir := t.TempDir()
		if _, err := lib.ExportImage(t.Context(), videos[0].UUID, photokit.VersionOriginal, dir, "IMG_0001.MOV", photokit.ExportOptions{}); !errors.Is(err, photokit.ErrMediaType) {
			t.Errorf("ExportImage of a video: %s, want ErrMediaType", exportFailure(err))
		}
		emptyDir(t, dir)
	})

	// Only a smoke check: Fetch may end the shortest deadlines first, and an
	// original on disk reports no progress to stop at, so "stop" checks the stop.
	t.Run("deadline", func(t *testing.T) {
		p := slices.MaxFunc(photos, bySize)
		deadlines := []time.Duration{time.Millisecond, 2 * time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond}
		ended := 0
		var longest time.Duration
		for _, d := range deadlines {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), d)
			start := time.Now()
			_, err := lib.ExportImage(ctx, p.UUID, photokit.VersionOriginal, dir, "IMG_0001.JPG", photokit.ExportOptions{})
			longest = max(longest, time.Since(start)-d)
			cancel()
			switch {
			case err == nil:
				if n := size(t, filepath.Join(dir, "IMG_0001.JPG")); p.OriginalFilesize > 0 && n != p.OriginalFilesize {
					t.Errorf("the image export within %v is not of the size of the original", d)
				}
			case errors.Is(err, context.DeadlineExceeded):
				ended++
				emptyDir(t, dir)
			default:
				t.Errorf("ExportImage with a deadline of %v: %s, want nil or DeadlineExceeded", d, exportFailure(err))
			}
		}
		t.Logf("%d of %d image exports of the largest original ended at their deadline; the longest return after its deadline took %v", ended, len(deadlines), longest)
	})
}

// The error is printed by its class only, as by export.
func (x *exporter) exportImage(t *testing.T, p *photoinfo.PhotoInfo, v photokit.Version) (string, photokit.ImageInfo, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), exportTimeout)
	defer cancel()
	var progress atomic.Int64
	name := x.name(p.OriginalFilename)
	info, err := x.lib.ExportImage(ctx, p.UUID, v, x.dir, name, photokit.ExportOptions{Progress: func(float64) { progress.Add(1) }})
	if err != nil {
		t.Errorf("ExportImage of version %d: %s", v, exportFailure(err))
		return "", photokit.ImageInfo{}, false
	}
	x.lastProgressCalls = int(progress.Load())
	return filepath.Join(x.dir, name), info, true
}
