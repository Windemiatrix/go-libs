//go:build photokit_live

package livetest

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
)

// go test runs the tests of a package in the order of their files: the name
// of this file sorts after import_live_test.go, whose assets it changes, and
// before removal_live_test.go, which deletes them.

// A change that failed or ended at its deadline may still take effect, so the
// limit counts attempts.
const maxPatches = 4

// Only an asset imported by this run is changed, never a photo of the owner.
// The owner confirms the dialog of hiding and refuses any other: only a
// refusal tells a dialog of Hidden false from none, and a dialog of properties
// is then an error.
func TestLivePatch(t *testing.T) {
	lib := openLibrary(t)
	i := slices.IndexFunc(importedAssets, func(id string) bool { return fetchable(t, lib, id) })
	if i < 0 {
		t.Skip("no asset imported by this run is visible")
	}
	id := importedAssets[i]
	uuid, _, _ := strings.Cut(id, "/")
	attempts := 0
	update := func(t *testing.T, target string, p photokit.AssetPatch) error {
		t.Helper()
		if attempts == maxPatches {
			t.Fatalf("more than %d changes of an asset in one run", maxPatches)
		}
		attempts++
		return lib.Update(changeContext(t), target, p)
	}

	// By the bare UUID, which a caller takes from photosdb; the other subtests
	// pass the LocalIdentifier.
	t.Run("properties", func(t *testing.T) {
		before, err := lib.Fetch(t.Context(), []string{id})
		if err != nil {
			t.Fatalf("Fetch of the asset before Update: %s", failure(err))
		}
		// The opposite of the current value, so that a Favorite left unwritten shows.
		favorite := !before[0].Favorite
		date := importDate.Add(48 * time.Hour)
		// At sea as importLocation, and apart from it, so that the change shows.
		location := photokit.Location{Latitude: 3.5, Longitude: 4.5}
		succeed(t, "Update of the date, Favorite and the location", update(t, uuid, photokit.AssetPatch{CreationDate: &date, Favorite: &favorite, Location: &location}))
		near := func(got *float64, want float64) bool { return got != nil && math.Abs(*got-want) <= 1e-5 }
		found, err := lib.Fetch(t.Context(), []string{id})
		if err != nil {
			t.Fatalf("Fetch of the changed asset: %s", failure(err))
		}
		a := found[0]
		for _, f := range []struct {
			field   string
			differs bool
		}{
			{"Date", a.Date == nil || !withinSecond(*a.Date, date)},
			{"Favorite", a.Favorite != favorite},
			{"Location", a.Location == nil || !near(&a.Location.Latitude, location.Latitude) || !near(&a.Location.Longitude, location.Longitude)},
		} {
			if f.differs {
				t.Errorf("%s of the asset after Update differs from the patch", f.field)
			}
		}
		db := reopenUntil(t, func(db *photosdb.DB) bool {
			q, ok := db.Photo(uuid)
			return ok && withinSecond(q.Date, date)
		})
		q, ok := db.Photo(uuid)
		if !ok {
			t.Fatal("photosdb has no photo of the changed asset")
		}
		for _, f := range []struct {
			field   string
			differs bool
		}{
			{"Date", !withinSecond(q.Date, date)},
			{"Favorite", q.Favorite != favorite},
			{"Latitude", !near(q.Latitude, location.Latitude)},
			{"Longitude", !near(q.Longitude, location.Longitude)},
		} {
			if f.differs {
				t.Errorf("%s of the photo in photosdb differs from the patch", f.field)
			}
		}
	})

	// Every call fails before the change: a dialog here is a broken check.
	t.Run("errors", func(t *testing.T) {
		ctx := changeContext(t)
		const none = "00000000-0000-0000-0000-000000000000"
		if err := lib.Update(ctx, id, photokit.AssetPatch{}); err != nil {
			t.Errorf("Update of an empty patch: %s", changeFailure(err))
		}
		secrets := []string{id, markerAlbumUUID, none, importPrefix}
		for _, e := range []struct {
			name, role string
			err        error
		}{
			{"a missing asset", ": asset", lib.Update(ctx, none, photokit.AssetPatch{Favorite: new(true)})},
			{"an identifier with a NUL", ": identifier", lib.Update(ctx, "a\x00b", photokit.AssetPatch{Favorite: new(true)})},
		} {
			if !errors.Is(e.err, photokit.ErrNotFound) || !strings.HasSuffix(e.err.Error(), e.role) {
				t.Errorf("Update of %s: %s, want ErrNotFound%s", e.name, changeFailure(e.err), e.role)
			}
			if e.err != nil && slices.ContainsFunc(secrets, func(s string) bool { return strings.Contains(e.err.Error(), s) }) {
				t.Errorf("the error of the Update of %s names an identifier or the prefix", e.name)
			}
		}
	})

	// An observation: pk_import writes only hidden YES, as a write even of NO
	// may ask the user. The time tells a dialog confirmed by mistake.
	pending := false
	t.Run("unhide-visible", func(t *testing.T) {
		start := time.Now()
		err := update(t, id, photokit.AssetPatch{Hidden: new(false)})
		t.Logf("Hidden false on a visible asset: Update returned after %v", time.Since(start).Round(time.Second))
		switch pk, isPK := errors.AsType[*bridge.PhotoKitError](err); {
		case err == nil:
			t.Log("Hidden false on a visible asset: no dialog")
		case errors.Is(err, photokit.ErrUserCanceled):
			t.Log("Hidden false on a visible asset asked the user; refused")
		case isPK && pk.Code == userCancelled:
			t.Logf("Hidden false on a visible asset asked the user; refused with NSError domain %s", pk.Domain)
		case errors.Is(err, photokit.ErrOutcomeUnknown):
			pending = true
			t.Log("Hidden false on a visible asset ended before its outcome: a dialog may be open")
		default:
			t.Errorf("Update with Hidden false: %s", changeFailure(err))
		}
	})

	// With the Hidden album locked PhotoKit does not find a hidden asset, so the
	// asset stays hidden after the run.
	t.Run("hide", func(t *testing.T) {
		// A second dialog while the first is open would mix up the answers.
		if pending {
			t.Skip("a dialog of Hidden false may still be open: answer it")
		}
		start := time.Now()
		err := update(t, id, photokit.AssetPatch{Hidden: new(true)})
		t.Logf("hiding an asset: Update returned after %v", time.Since(start).Round(time.Second))
		switch {
		case errors.Is(err, photokit.ErrUserCanceled):
			t.Error("Update returned ErrUserCanceled where the owner was to confirm the dialog")
			return
		case errors.Is(err, photokit.ErrOutcomeUnknown):
			t.Logf("Update with Hidden ended before its outcome: answer the dialog; look for %s* in the Hidden album", importPrefix)
			return
		case err != nil:
			t.Errorf("Update with Hidden: %s", changeFailure(err))
			return
		}
		if fetchable(t, lib, id) {
			t.Error("Fetch finds the asset after Update with Hidden")
		}
		err = update(t, id, photokit.AssetPatch{Hidden: new(false)})
		switch {
		case err == nil:
			t.Log("the hidden asset was shown again through PhotoKit: the Hidden album is unlocked")
		case errors.Is(err, photokit.ErrNotFound):
			t.Log("a hidden asset is not found by Update: it cannot be shown again through PhotoKit")
		default:
			t.Logf("Update with Hidden false of the hidden asset: %s", changeFailure(err))
		}
		hidden := err != nil
		shows := func(db *photosdb.DB) bool {
			q, ok := db.Photo(uuid)
			return ok && q.Hidden == hidden
		}
		if !shows(reopenUntil(t, shows)) {
			t.Errorf("photosdb does not show the asset with Hidden %t after the Updates", hidden)
		}
		if hidden {
			t.Log("1 asset hidden by this run stays: delete it in Photos, album Hidden")
		}
	})
}
