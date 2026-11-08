//go:build photokit_live

package livetest

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/export"
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

// A video without edits has one file, which every version gives; the Current
// version of a slow-motion video is an AVComposition without a file, which
// ExportVideo renders (photokit.py:876-884).
func TestLiveVideoVersions(t *testing.T) {
	lib := openLibrary(t)
	visible := visibleMarkerPhotos(t)
	videos := filter(visible, func(p *photoinfo.PhotoInfo) bool { return p.IsMovie })
	if len(videos) == 0 {
		skipNone(t, "video")
	}
	x := &exporter{lib: lib, dir: t.TempDir(), taken: map[string]bool{}, exported: map[string]*photoinfo.PhotoInfo{}}

	t.Run("video current equals original", func(t *testing.T) {
		plain := filter(videos, func(p *photoinfo.PhotoInfo) bool {
			return !p.HasAdjustments && !p.SlowMo && p.Path != "" && !p.IsMissing
		})
		if len(plain) == 0 {
			skipNone(t, "video without edits with its original on disk")
		}
		exported, differ := 0, 0
		for _, p := range plain {
			for _, v := range []photokit.Version{photokit.VersionCurrent, photokit.VersionUnadjusted, photokit.VersionOriginal} {
				path, err := x.exportVideo(t, p, v)
				if err != nil {
					t.Errorf("ExportVideo of version %d: %s", v, exportFailure(err))
					continue
				}
				exported++
				if sum(t, path) != sum(t, p.Path) {
					differ++
				}
			}
		}
		t.Logf("of %d videos without edits on disk, %d versions exported", len(plain), exported)
		if differ > 0 {
			t.Errorf("%d of %d exported versions of videos without edits differ from their file in the library", differ, exported)
		}
	})

	// The Current version of an edited video may be an AVComposition without a
	// file: only counted.
	t.Run("current of an edited video", func(t *testing.T) {
		edited := filter(videos, func(p *photoinfo.PhotoInfo) bool {
			return p.HasAdjustments && !p.SlowMo && p.Path != "" && !p.IsMissing
		})
		if len(edited) == 0 {
			skipNone(t, "edited video with its original on disk")
		}
		exported, notFile, noFile, same, plainOriginal := 0, 0, 0, 0, 0
		for _, p := range edited {
			originalPath, err := x.exportVideo(t, p, photokit.VersionOriginal)
			if err != nil {
				t.Errorf("ExportVideo of the Original version of an edited video: %s", exportFailure(err))
				continue
			}
			exported++
			original := sum(t, originalPath)
			if original != sum(t, p.Path) {
				notFile++
			}
			if plainPath, err := x.exportVideo(t, p, photokit.VersionUnadjusted); err != nil {
				t.Errorf("ExportVideo of the Unadjusted version of an edited video: %s", exportFailure(err))
			} else if sum(t, plainPath) == original {
				plainOriginal++
			}
			currentPath, err := x.exportVideo(t, p, photokit.VersionCurrent)
			switch {
			case errors.Is(err, photokit.ErrNoResource):
				noFile++
			case err != nil:
				t.Errorf("ExportVideo of the Current version of an edited video: %s, want nil or ErrNoResource", exportFailure(err))
			case sum(t, currentPath) == original:
				same++
			}
		}
		t.Logf("of %d edited videos on disk, %d exported in the Original version, %d with an Unadjusted version equal to the Original, %d whose Current version has no file", len(edited), exported, plainOriginal, noFile)
		if notFile > 0 || same > 0 {
			t.Errorf("of %d edited videos exported in the Original version, %d differ from their file in the library, %d with a Current version equal to the Original", exported, notFile, same)
		}
	})

	// Only a smoke check, as "deadline" of TestLiveVersions: Fetch may end the
	// shortest deadlines first.
	t.Run("video deadline", func(t *testing.T) {
		p := slices.MaxFunc(videos, func(a, b *photoinfo.PhotoInfo) int { return cmp.Compare(a.OriginalFilesize, b.OriginalFilesize) })
		deadlines := []time.Duration{time.Millisecond, 2 * time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond}
		ended := 0
		for _, d := range deadlines {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), d)
			err := lib.ExportVideo(ctx, p.UUID, photokit.VersionOriginal, dir, "IMG_0001.MOV", photokit.ExportOptions{})
			cancel()
			switch {
			case err == nil:
				if n := size(t, filepath.Join(dir, "IMG_0001.MOV")); p.OriginalFilesize > 0 && n != p.OriginalFilesize {
					t.Errorf("the video export within %v is not of the size of the original", d)
				}
			case errors.Is(err, context.DeadlineExceeded):
				ended++
				emptyDir(t, dir)
			default:
				t.Errorf("ExportVideo with a deadline of %v: %s, want nil or DeadlineExceeded", d, exportFailure(err))
			}
		}
		t.Logf("%d of %d video exports of the largest original ended at their deadline", ended, len(deadlines))
	})

	// The Current version is a new encoding of the composition
	// (photokit.py:799-806), so it differs from the file in the library, which
	// the other versions give.
	t.Run("slow motion", func(t *testing.T) {
		slow := filter(videos, func(p *photoinfo.PhotoInfo) bool { return p.SlowMo })
		if len(slow) == 0 {
			skipNone(t, "slow-motion video")
		}
		rendered, empty, notMovie, sameAsFile, compared, differ := 0, 0, 0, 0, 0, 0
		var longest time.Duration
		renders := map[string]time.Duration{}
		for _, p := range slow {
			dir := t.TempDir()
			// Upstream names the render <stem>.mov (photokit.py:954).
			name := x.name(p.OriginalFilename)
			name = strings.TrimSuffix(name, filepath.Ext(name)) + ".mov"
			ctx, cancel := context.WithTimeout(t.Context(), exportTimeout)
			start := time.Now()
			err := lib.ExportVideo(ctx, p.UUID, photokit.VersionCurrent, dir, name, photokit.ExportOptions{})
			cancel()
			if err != nil {
				t.Errorf("ExportVideo of the Current version of a slow-motion video: %s", exportFailure(err))
				continue
			}
			renders[p.UUID] = time.Since(start)
			longest = max(longest, renders[p.UUID])
			rendered++
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Errorf("%d entries in the directory after the export, want 1 (read error: %t)", len(entries), err != nil)
			}
			path := filepath.Join(dir, name)
			switch {
			case size(t, path) == 0:
				empty++
			case string(head(t, path)[4:]) != "ftyp":
				notMovie++
			case p.Path != "" && !p.IsMissing && sum(t, path) == sum(t, p.Path):
				sameAsFile++
			}
			// The renders of a library may not fit on the disk together, as its Live Photos (sumRemove).
			if err := os.Remove(path); err != nil {
				t.Fatal("cannot remove an exported file")
			}
			if p.Path == "" || p.IsMissing {
				continue
			}
			for _, v := range []photokit.Version{photokit.VersionUnadjusted, photokit.VersionOriginal} {
				other, err := x.exportVideo(t, p, v)
				if err != nil {
					t.Errorf("ExportVideo of version %d of a slow-motion video: %s", v, exportFailure(err))
					continue
				}
				compared++
				if sumRemove(t, other) != sum(t, p.Path) {
					differ++
				}
			}
		}
		t.Logf("the Current version of %d slow-motion videos rendered, the longest in %v; %d Unadjusted and Original versions of those on disk compared with their file", rendered, longest, compared)
		if empty > 0 || notMovie > 0 || sameAsFile > 0 {
			t.Errorf("of %d rendered Current versions of slow-motion videos, %d empty, %d without ftyp, %d equal to the file in the library", rendered, empty, notMovie, sameAsFile)
		}
		if differ > 0 {
			t.Errorf("%d of %d exported Unadjusted and Original versions of slow-motion videos differ from their file in the library", differ, compared)
		}

		// The video on disk whose render took longest, so that the 2 s before the
		// cancel go to the render, not to a download from iCloud.
		t.Run("cancel", func(t *testing.T) {
			local := filter(slow, func(p *photoinfo.PhotoInfo) bool { return renders[p.UUID] > 0 && p.Path != "" && !p.IsMissing })
			if len(local) == 0 {
				skipNone(t, "rendered slow-motion video on disk")
			}
			p := slices.MaxFunc(local, func(a, b *photoinfo.PhotoInfo) int { return cmp.Compare(renders[a.UUID], renders[b.UUID]) })
			whole := renders[p.UUID]
			// From 10 s on, the rest of the render after the cancel, which ExportVideo
			// waits for without cancelExport, exceeds half of the render by 3 s.
			if whole < 10*time.Second {
				t.Skipf("the longest render of a slow-motion video on disk took %v, too short to tell cancelExport from the end of the render", whole)
			}
			dir := t.TempDir()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			timer := time.AfterFunc(2*time.Second, cancel)
			defer timer.Stop()
			start := time.Now()
			err := lib.ExportVideo(ctx, p.UUID, photokit.VersionCurrent, dir, "IMG_0001.mov", photokit.ExportOptions{})
			late := time.Since(start) - 2*time.Second
			if err == nil {
				t.Skip("the render ended before the cancel")
			}
			t.Logf("ExportVideo of a render of %v canceled 2 s after its start returned %v after the cancel", whole, late)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("ExportVideo of the Current version of a slow-motion video canceled 2 s after its start: %s, want context.Canceled", exportFailure(err))
			}
			// Without cancelExport ExportVideo waits for the end of the render, or for its grace of 30 s.
			if late > 10*time.Second || late > whole/2 {
				t.Errorf("ExportVideo returned %v after the cancel of a render of %v, want 10 s and half of the render at most", late, whole)
			}
			emptyDir(t, dir)
		})
	})

	t.Run("video errors", func(t *testing.T) {
		dir, o := t.TempDir(), photokit.ExportOptions{}
		const none = "00000000-0000-0000-0000-000000000000"
		if err := lib.ExportVideo(t.Context(), none, photokit.VersionOriginal, dir, "IMG_0001.MOV", o); !errors.Is(err, photokit.ErrNotFound) {
			t.Errorf("ExportVideo of an unknown UUID: %s, want ErrNotFound", exportFailure(err))
		}
		if err := lib.ExportVideo(t.Context(), videos[0].UUID, photokit.VersionOriginal, dir, "../x", o); !errors.Is(err, photokit.ErrInvalidName) {
			t.Errorf("ExportVideo to ../x: %s, want ErrInvalidName", exportFailure(err))
		}
		if photos := filter(visible, func(p *photoinfo.PhotoInfo) bool { return p.IsPhoto }); len(photos) > 0 {
			if err := lib.ExportVideo(t.Context(), photos[0].UUID, photokit.VersionOriginal, dir, "IMG_0001.MOV", o); !errors.Is(err, photokit.ErrMediaType) {
				t.Errorf("ExportVideo of a photo: %s, want ErrMediaType", exportFailure(err))
			}
		}
		emptyDir(t, dir)
	})
}

// photosdb gives the files of the resources that ExportLivePhoto reads: Path and
// PathLive the originals, PathEdited and PathEditedLive the renders of an edit.
func TestLiveLivePhotoVersions(t *testing.T) {
	lib := openLibrary(t)
	visible := visibleMarkerPhotos(t)
	lives := filter(visible, func(p *photoinfo.PhotoInfo) bool {
		return p.Live && !p.IsMissing && p.Path != "" && p.PathLive != ""
	})
	if len(lives) == 0 {
		skipNone(t, "Live Photo with its photo and video on disk")
	}
	x := &exporter{lib: lib, dir: t.TempDir(), taken: map[string]bool{}, exported: map[string]*photoinfo.PhotoInfo{}}

	t.Run("unedited", func(t *testing.T) {
		plain := filter(lives, func(p *photoinfo.PhotoInfo) bool { return !p.HasAdjustments })
		if len(plain) == 0 {
			skipNone(t, "Live Photo without edits with its photo and video on disk")
		}
		exported, differ := 0, 0
		for _, p := range plain {
			for _, half := range []struct {
				kind photokit.ResourceType
				file string
			}{{photokit.ResourcePhoto, p.Path}, {photokit.ResourcePairedVideo, p.PathLive}} {
				for _, v := range []photokit.Version{photokit.VersionCurrent, photokit.VersionUnadjusted, photokit.VersionOriginal} {
					if got, ok := x.exportLive(t, p, v, half.kind); ok {
						exported++
						if got != sum(t, half.file) {
							differ++
						}
					}
				}
			}
		}
		t.Logf("of %d Live Photos without edits on disk, %d versions of a photo or a video exported", len(plain), exported)
		if exported == 0 {
			t.Skip("no version of a Live Photo without edits exported")
		}
		if differ > 0 {
			t.Errorf("%d of %d exported versions of Live Photos without edits differ from their file in the library", differ, exported)
		}
	})

	t.Run("edited", func(t *testing.T) {
		edited := filter(lives, func(p *photoinfo.PhotoInfo) bool { return p.HasAdjustments && p.PathEdited != "" })
		if len(edited) == 0 {
			skipNone(t, "edited Live Photo with a rendered photo on disk")
		}
		exported, notRender, sameAsOriginal, videoDiffer, noEditedVideo := 0, 0, 0, 0, 0
		for _, p := range edited {
			current, okCurrent := x.exportLive(t, p, photokit.VersionCurrent, photokit.ResourcePhoto)
			original, okOriginal := x.exportLive(t, p, photokit.VersionOriginal, photokit.ResourcePhoto)
			video, okVideo := x.exportLive(t, p, photokit.VersionCurrent, photokit.ResourcePairedVideo)
			if !okCurrent || !okOriginal || !okVideo {
				continue
			}
			exported++
			if current != sum(t, p.PathEdited) {
				notRender++
			}
			if current == original {
				sameAsOriginal++
			}
			wantVideo := p.PathEditedLive
			if wantVideo == "" {
				noEditedVideo++
				wantVideo = p.PathLive
			}
			if video != sum(t, wantVideo) {
				videoDiffer++
			}
		}
		t.Logf("of %d edited Live Photos with a rendered photo on disk, %d exported, %d without a rendered video on disk", len(edited), exported, noEditedVideo)
		if exported == 0 {
			t.Skip("no edited Live Photo exported")
		}
		if notRender > 0 || sameAsOriginal > 0 || videoDiffer > 0 {
			t.Errorf("of %d edited Live Photos exported, %d with a Current photo other than the render in the library, %d with a Current photo equal to the Original, %d with a Current video other than the rendered video or, without one, the original video", exported, notRender, sameAsOriginal, videoDiffer)
		}
	})

	// Only counted: a photo of VersionUnadjusted other than that of
	// PHImageManager would point at the AdjustmentBasePhoto resource.
	t.Run("compared with ExportImage", func(t *testing.T) {
		compared, sameCurrent, sameUnadjusted := 0, 0, 0
		for _, p := range lives {
			live, okLive := x.exportLive(t, p, photokit.VersionCurrent, photokit.ResourcePhoto)
			image, okImage := x.imageSum(t, p, photokit.VersionCurrent)
			livePlain, okLivePlain := x.exportLive(t, p, photokit.VersionUnadjusted, photokit.ResourcePhoto)
			imagePlain, okImagePlain := x.imageSum(t, p, photokit.VersionUnadjusted)
			if !okLive || !okImage || !okLivePlain || !okImagePlain {
				continue
			}
			compared++
			if live == image {
				sameCurrent++
			}
			if livePlain == imagePlain {
				sameUnadjusted++
			}
		}
		t.Logf("of %d Live Photos, %d compared with ExportImage: the photo of ExportLivePhoto equals that of ExportImage in %d for Current, in %d for Unadjusted", len(lives), compared, sameCurrent, sameUnadjusted)
		if compared == 0 {
			t.Skip("no Live Photo exported by both ExportLivePhoto and ExportImage")
		}
	})

	t.Run("live errors", func(t *testing.T) {
		p := lives[0]
		dir, o := t.TempDir(), photokit.ExportOptions{}
		const none = "00000000-0000-0000-0000-000000000000"
		if err := lib.ExportLivePhoto(t.Context(), p.UUID, photokit.VersionOriginal, photokit.ResourceVideo, dir, "IMG_0001.MOV", o); !errors.Is(err, photokit.ErrNoResource) {
			t.Errorf("ExportLivePhoto of a Video resource: %s, want ErrNoResource", exportFailure(err))
		}
		if stills := filter(visible, func(q *photoinfo.PhotoInfo) bool { return q.IsPhoto && !q.Live }); len(stills) > 0 {
			if err := lib.ExportLivePhoto(t.Context(), stills[0].UUID, photokit.VersionOriginal, photokit.ResourcePhoto, dir, "IMG_0001.JPG", o); !errors.Is(err, photokit.ErrMediaType) {
				t.Errorf("ExportLivePhoto of a photo that is not a Live Photo: %s, want ErrMediaType", exportFailure(err))
			}
		}
		if err := lib.ExportLivePhoto(t.Context(), none, photokit.VersionOriginal, photokit.ResourcePhoto, dir, "IMG_0001.JPG", o); !errors.Is(err, photokit.ErrNotFound) {
			t.Errorf("ExportLivePhoto of an unknown UUID: %s, want ErrNotFound", exportFailure(err))
		}
		if err := lib.ExportLivePhoto(t.Context(), p.UUID, photokit.VersionOriginal, photokit.ResourcePhoto, dir, "../x", o); !errors.Is(err, photokit.ErrInvalidName) {
			t.Errorf("ExportLivePhoto to ../x: %s, want ErrInvalidName", exportFailure(err))
		}
		emptyDir(t, dir)
	})
}

// The error is printed by its class only, as by export. The file is hashed and
// removed at once: the Live Photos of a library may not fit on the disk twice.
func (x *exporter) exportLive(t *testing.T, p *photoinfo.PhotoInfo, v photokit.Version, kind photokit.ResourceType) ([sha256.Size]byte, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), exportTimeout)
	defer cancel()
	name := x.name(p.OriginalFilename)
	if kind == photokit.ResourcePairedVideo {
		name = export.Names(p, name).Live
	}
	if err := x.lib.ExportLivePhoto(ctx, p.UUID, v, kind, x.dir, name, photokit.ExportOptions{}); err != nil {
		t.Errorf("ExportLivePhoto of version %d, resource type %d: %s", v, kind, exportFailure(err))
		return [sha256.Size]byte{}, false
	}
	return sumRemove(t, filepath.Join(x.dir, name)), true
}

func (x *exporter) imageSum(t *testing.T, p *photoinfo.PhotoInfo, v photokit.Version) ([sha256.Size]byte, bool) {
	t.Helper()
	path, _, ok := x.exportImage(t, p, v)
	if !ok {
		return [sha256.Size]byte{}, false
	}
	return sumRemove(t, path), true
}

// The error of os names the path, so it is not printed.
func sumRemove(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	s := sum(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal("cannot remove an exported file")
	}
	return s
}

// The error goes to the caller, which prints it by its class only, as export
// does: the Current version of a video may have no file.
func (x *exporter) exportVideo(t *testing.T, p *photoinfo.PhotoInfo, v photokit.Version) (string, error) {
	ctx, cancel := context.WithTimeout(t.Context(), exportTimeout)
	defer cancel()
	name := x.name(p.OriginalFilename)
	return filepath.Join(x.dir, name), x.lib.ExportVideo(ctx, p.UUID, v, x.dir, name, photokit.ExportOptions{})
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
