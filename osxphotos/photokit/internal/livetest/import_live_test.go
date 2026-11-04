//go:build photokit_live

package livetest

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/export"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
)

// The removal at the end of the run finds the imports of this run and of
// earlier ones by the prefix of their name.
const (
	importPrefix = "photosync-livetest-"
	maxImports   = 4
)

// Hiding at creation may wait for the owner to answer a dialog.
const importTimeout = 2 * time.Minute

// PHPhotosErrorUserCancelled, and NSUserCancelledError of NSCocoaErrorDomain.
const userCancelled = 3072

// Far from the dates of the album, so that the date of the request is told
// from that of the file.
var importDate = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

// A point at sea, so that the location of the request is told from that of
// the file and names no place of the owner.
var importLocation = photokit.Location{Latitude: 1.5, Longitude: 2.5}

func TestLiveImport(t *testing.T) {
	lib := openLibrary(t)
	photos := visibleMarkerPhotos(t)
	x := &exporter{lib: lib, dir: t.TempDir(), taken: map[string]bool{}, exported: map[string]*photoinfo.PhotoInfo{}}
	im := &importer{lib: lib}

	var plain string
	t.Run("photo", func(t *testing.T) {
		p := firstOf(t, photos, "photo that is neither Live nor RAW", func(p *photoinfo.PhotoInfo) bool { return p.IsPhoto && !p.Live && !p.HasRAW })
		src, ok := x.export(t, p, photokit.ResourcePhoto, x.name(p.OriginalFilename))
		if !ok {
			return
		}
		plain = src
		a, err := im.add(t, photokit.ImportRequest{Photo: src, Favorite: true, Location: &importLocation}, false, false)
		if err != nil {
			t.Fatalf("Import of a photo: %s", importFailure(err))
		}
		got, ok := checkImported(t, lib, a, photokit.ResourcePhoto)
		if ok && (!got.Favorite || !got.IsPhoto) {
			t.Errorf("the imported photo: Favorite %t, IsPhoto %t, want both", got.Favorite, got.IsPhoto)
		}
		if l := got.Location; ok && (l == nil || math.Abs(l.Latitude-importLocation.Latitude) > 1e-5 || math.Abs(l.Longitude-importLocation.Longitude) > 1e-5) {
			t.Error("Location of the imported photo differs from the request")
		}
	})

	// A broken check would import the photo that PhotoKit takes, under the
	// prefix and in the marker album, where the removal finds it; the id is kept
	// in importedAssets.
	t.Run("errors", func(t *testing.T) {
		if plain == "" {
			t.Skip("no photo was exported for the import of a photo")
		}
		dir := t.TempDir()
		file := filepath.Join(dir, "IMG_0001.JPG")
		if err := os.WriteFile(file, []byte("not a photo"), 0o600); err != nil {
			t.Fatal("cannot write a file to import")
		}
		const none = "00000000-0000-0000-0000-000000000000"
		marker, name := []string{markerAlbumUUID}, importPrefix+"error.JPG"
		ctx, cancel := context.WithTimeout(t.Context(), importTimeout)
		defer cancel()
		for _, c := range []struct {
			name string
			r    photokit.ImportRequest
			want error
		}{
			{"a missing file", photokit.ImportRequest{Photo: filepath.Join(dir, "IMG_0002.JPG"), OriginalFilename: name, Albums: marker}, photokit.ErrImportFile},
			{"Photo and Video", photokit.ImportRequest{Photo: plain, Video: plain, OriginalFilename: name, Albums: marker}, photokit.ErrImportFile},
			{"a relative path", photokit.ImportRequest{Photo: "IMG_0001.JPG", OriginalFilename: name, Albums: marker}, photokit.ErrImportFile},
			{"an unknown album", photokit.ImportRequest{Photo: plain, OriginalFilename: name, Albums: []string{markerAlbumUUID, none}}, photokit.ErrNotFound},
			{"not a photo", photokit.ImportRequest{Photo: file, OriginalFilename: name, Albums: marker}, photokit.ErrImport},
		} {
			id, err := lib.Import(ctx, c.r)
			if err == nil {
				importedAssets = append(importedAssets, id)
			}
			// ErrImport is wanted from the NSError, which only PhotoKit gives.
			pk, isPK := errors.AsType[*bridge.PhotoKitError](err)
			if !errors.Is(err, c.want) || (errors.Is(c.want, photokit.ErrImport) && (!isPK || pk.Domain == "")) {
				t.Errorf("Import of %s: %s", c.name, importFailure(err))
			}
			if err != nil && slices.ContainsFunc([]string{dir, x.dir, filepath.Base(c.r.Photo), markerAlbumUUID, importPrefix}, func(s string) bool { return strings.Contains(err.Error(), s) }) {
				t.Errorf("the error of the import of %s names a path, a file, the album or the name of the request", c.name)
			}
		}
	})

	t.Run("Live", func(t *testing.T) {
		p := firstOf(t, photos, "Live Photo", func(p *photoinfo.PhotoInfo) bool { return p.Live })
		name := x.name(p.OriginalFilename)
		src, okPhoto := x.export(t, p, photokit.ResourcePhoto, name)
		video, okVideo := x.export(t, p, photokit.ResourcePairedVideo, export.Names(p, name).Live)
		if !okPhoto || !okVideo {
			return
		}
		a, err := im.add(t, photokit.ImportRequest{Photo: src, PairedVideo: video}, true, false)
		if err != nil {
			t.Fatalf("Import of a Live pair: %s", importFailure(err))
		}
		if got, ok := checkImported(t, lib, a, photokit.ResourcePhoto, photokit.ResourcePairedVideo); ok && !got.Live {
			t.Error("the imported Live pair is not Live")
		}
	})

	// With the RAW as original the Photo resource is the RAW, and the name of
	// export.Names would give the JPEG a RAW extension.
	t.Run("RAW", func(t *testing.T) {
		p := firstOf(t, photos, "RAW+JPEG photo with the JPEG as original", func(p *photoinfo.PhotoInfo) bool { return p.HasRAW && !p.RawOriginal })
		name := x.name(p.OriginalFilename)
		src, okPhoto := x.export(t, p, photokit.ResourcePhoto, name)
		raw, okRAW := x.export(t, p, photokit.ResourceAlternatePhoto, export.Names(p, name).RAW)
		if !okPhoto || !okRAW {
			return
		}
		a, err := im.add(t, photokit.ImportRequest{Photo: src, AlternatePhoto: raw}, false, true)
		if err != nil {
			t.Fatalf("Import of a RAW+JPEG pair: %s", importFailure(err))
		}
		if got, ok := checkImported(t, lib, a, photokit.ResourcePhoto, photokit.ResourceAlternatePhoto); ok && (got.RAWFilename == "" || got.UTIRaw == "") {
			t.Error("the imported RAW+JPEG pair has no RAWFilename or UTIRaw")
		}
	})

	// An observation: hiding an existing asset asks the user, and Go maps only
	// the refusal of PHPhotosErrorDomain to ErrUserCanceled. Fetch does not find
	// a hidden asset; photosdb checks Hidden below.
	t.Run("hidden", func(t *testing.T) {
		if plain == "" {
			t.Skip("no photo was exported for the import of a photo")
		}
		start := time.Now()
		a, err := im.add(t, photokit.ImportRequest{Photo: plain, Hidden: true}, false, false)
		t.Logf("Import with Hidden returned after %v", time.Since(start).Round(time.Second))
		switch pk, isPK := errors.AsType[*bridge.PhotoKitError](err); {
		case errors.Is(err, photokit.ErrUserCanceled):
			t.Log("hiding at creation needs a dialog; refused with NSError domain PHPhotosErrorDomain")
		case isPK && pk.Code == userCancelled:
			t.Logf("hiding at creation needs a dialog; refused with NSError domain %s", pk.Domain)
		// The asset may be created later, outside importedAssets: the count of
		// the marker album below then differs.
		case errors.Is(err, photokit.ErrOutcomeUnknown):
			t.Logf("Import with Hidden ended before its outcome: look for %s* in the Hidden album", importPrefix)
		case err != nil:
			t.Errorf("Import with Hidden: %s", importFailure(err))
		default:
			if _, err := lib.Fetch(t.Context(), []string{a.id}); !errors.Is(err, photokit.ErrNotFound) {
				t.Errorf("Fetch of the asset imported hidden: not ErrNotFound (error: %t)", err != nil)
			}
		}
	})

	db := reopenWithImports(t, importedAssets)

	t.Run("photosdb after import", func(t *testing.T) {
		// AlbumPhotos holds hidden photos too. An import that ended before its
		// outcome, so is not in importedAssets, changes the count.
		prefixed := func(p *photoinfo.PhotoInfo) bool { return strings.HasPrefix(p.OriginalFilename, importPrefix) }
		if n, want := count(db.AlbumPhotos(markerOf(db.AlbumInfo())), prefixed), count(markerPhotos, prefixed)+len(importedAssets); n != want {
			t.Errorf("the marker album holds %d photos named with the prefix, want %d", n, want)
		}
		if len(im.done) == 0 {
			t.Skip("no import succeeded")
		}
		type pair struct {
			want importedAsset
			got  *photoinfo.PhotoInfo
		}
		var pairs []pair
		for _, a := range im.done {
			uuid, _, _ := strings.Cut(a.id, "/")
			if q, ok := db.Photo(uuid); ok {
				pairs = append(pairs, pair{a, q})
			}
		}
		if len(pairs) < len(im.done) {
			t.Errorf("%d of %d imported assets are not in photosdb", len(im.done)-len(pairs), len(im.done))
		}
		for _, c := range []struct {
			field   string
			differs func(pair) bool
		}{
			{"OriginalFilename", func(p pair) bool { return p.got.OriginalFilename != p.want.name }},
			{"Date", func(p pair) bool { return !withinSecond(p.got.Date, importDate) }},
			{"AlbumUUIDs", func(p pair) bool { return !slices.Contains(p.got.AlbumUUIDs, markerAlbumUUID) }},
			{"Live", func(p pair) bool { return p.got.Live != p.want.live }},
			{"HasRAW", func(p pair) bool { return p.got.HasRAW != p.want.raw }},
			{"Hidden", func(p pair) bool { return p.got.Hidden != p.want.hidden }},
		} {
			if n := count(pairs, c.differs); n > 0 {
				t.Errorf("%s differs from the import for %d of %d imported assets", c.field, n, len(pairs))
			}
		}
		// An observation: iCloud may give the GUIDs only after an upload.
		t.Logf("of %d imported assets, %d with a CloudAssetGUID, %d with a CloudMasterGUID", len(pairs),
			count(pairs, func(p pair) bool { return p.got.CloudAssetGUID != "" }),
			count(pairs, func(p pair) bool { return p.got.CloudMasterGUID != "" }))
	})
}

type importedAsset struct {
	id, name          string
	live, raw, hidden bool
}

type importer struct {
	lib      *photokit.Library
	attempts int
	done     []importedAsset
}

// An import that failed or ended at its deadline may still have created its
// asset, so the limit counts attempts. Every import goes to the marker album,
// where the guard and the removal find it.
func (im *importer) add(t *testing.T, r photokit.ImportRequest, live, raw bool) (importedAsset, error) {
	t.Helper()
	if im.attempts == maxImports {
		t.Fatalf("more than %d imports in one run", maxImports)
	}
	im.attempts++
	r.Albums = []string{markerAlbumUUID}
	r.CreationDate = importDate
	r.OriginalFilename = fmt.Sprintf("%s%d%s", importPrefix, im.attempts, filepath.Ext(r.Photo))
	ctx, cancel := context.WithTimeout(t.Context(), importTimeout)
	defer cancel()
	id, err := im.lib.Import(ctx, r)
	// Photos copies the files of the request, and must leave them.
	for _, path := range []string{r.Photo, r.PairedVideo, r.AlternatePhoto} {
		if _, statErr := os.Stat(path); path != "" && statErr != nil {
			t.Error("Import moved or removed a file of the request")
		}
	}
	if err != nil {
		return importedAsset{}, err
	}
	importedAssets = append(importedAssets, id)
	a := importedAsset{id: id, name: r.OriginalFilename, live: live, raw: raw, hidden: r.Hidden}
	im.done = append(im.done, a)
	return a, nil
}

// photosdb may show an import late.
func reopenWithImports(t *testing.T, ids []string) *photosdb.DB {
	t.Helper()
	end := time.Now().Add(30 * time.Second)
	for {
		db := reopenLibrary(t)
		missing := slices.ContainsFunc(ids, func(id string) bool {
			uuid, _, _ := strings.Cut(id, "/")
			_, ok := db.Photo(uuid)
			return !ok
		})
		if !missing || time.Now().After(end) {
			return db
		}
		time.Sleep(2 * time.Second)
	}
}

func checkImported(t *testing.T, lib *photokit.Library, a importedAsset, kinds ...photokit.ResourceType) (photokit.Asset, bool) {
	t.Helper()
	found, err := lib.Fetch(t.Context(), []string{a.id})
	if err != nil {
		t.Errorf("Fetch of the imported asset: %s", failure(err))
		return photokit.Asset{}, false
	}
	got := found[0]
	if got.OriginalFilename != a.name {
		t.Error("OriginalFilename of the imported asset differs from the request")
	}
	if got.Date == nil || !withinSecond(*got.Date, importDate) {
		t.Error("Date of the imported asset is not within a second of CreationDate")
	}
	res, err := lib.Resources(t.Context(), a.id)
	if err != nil {
		t.Errorf("Resources of the imported asset: %s", failure(err))
	}
	for _, k := range kinds {
		if !slices.ContainsFunc(res, func(r photokit.Resource) bool { return r.Type == k }) {
			t.Errorf("the imported asset has no resource of type %d", k)
		}
	}
	return got, true
}

func firstOf(t *testing.T, photos []*photoinfo.PhotoInfo, kind string, match func(*photoinfo.PhotoInfo) bool) *photoinfo.PhotoInfo {
	t.Helper()
	i := slices.IndexFunc(photos, match)
	if i < 0 {
		skipNone(t, kind)
	}
	return photos[i]
}

// As exportFailure: the classes of the error and the domain and code of an
// NSError only.
func importFailure(err error) string {
	nsError := "none"
	if pk, ok := errors.AsType[*bridge.PhotoKitError](err); ok {
		nsError = pk.Error()
	}
	return fmt.Sprintf("%s (ErrImportFile: %t, ErrImport: %t, ErrUserCanceled: %t, ErrOutcomeUnknown: %t, NSError: %s)",
		failure(err), errors.Is(err, photokit.ErrImportFile), errors.Is(err, photokit.ErrImport),
		errors.Is(err, photokit.ErrUserCanceled), errors.Is(err, photokit.ErrOutcomeUnknown), nsError)
}
