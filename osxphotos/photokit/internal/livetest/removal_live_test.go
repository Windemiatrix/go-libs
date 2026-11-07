//go:build photokit_live

package livetest

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit"
	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
)

// go test runs the tests of a package in the order of their files: the name
// of this file sorts after import_live_test.go, so that the removal comes after
// the imports it removes.

// The removal finds the imports of this run and of earlier ones by the prefix
// of their name. Three runs import at most three times maxImports: more means
// the marker album holds what the runs did not import.
const (
	removalPrefix = importPrefix
	maxRemovals   = 3 * maxImports
)

// In the deadline step the owner answers the dialog after lateTimeout, and the
// late answer may take effect up to lateWait after it.
const (
	lateTimeout = 30 * time.Second
	lateWait    = 2 * time.Minute
)

// Only a person answers the dialog of a deletion: the owner refuses the first,
// answers the second after lateTimeout and confirms the third.
func TestLiveRemoval(t *testing.T) {
	lib := openLibrary(t)
	db := reopenLibrary(t)
	own := ownPhotos(t, db)
	visible := filter(own, visibleOutsideTrash)
	hidden := filter(own, hiddenOutsideTrash)
	imported := count(own, func(p *photoinfo.PhotoInfo) bool {
		return slices.ContainsFunc(importedAssets, func(id string) bool {
			uuid, _, _ := strings.Cut(id, "/")
			return uuid == p.UUID
		})
	})
	t.Logf("%d photos with the prefix in the marker album: %d visible, %d hidden, %d in the trash; %d of them imported by this run",
		len(own), len(visible), len(hidden), count(own, func(p *photoinfo.PhotoInfo) bool { return p.InTrash }), imported)
	// With the Hidden album locked PhotoKit does not find a hidden asset.
	t.Logf("%d hidden photos with the prefix stay: delete them in Photos, album Hidden", len(hidden))
	// Only the album and the prefix together mark a photo of the runs.
	t.Logf("%d photos with the prefix outside the marker album stay: delete them in Photos", count(db.Photos(), func(p *photoinfo.PhotoInfo) bool {
		return withPrefix(p) && !p.InTrash && !slices.Contains(p.AlbumUUIDs, markerAlbumUUID)
	}))
	if len(visible) == 0 {
		t.Skip("no visible photo with the prefix in the marker album")
	}
	if len(visible) > maxRemovals {
		t.Fatalf("%d visible photos with the prefix in the marker album, more than the %d imports of three runs: delete the photos %s* of the album %s in Photos",
			len(visible), maxRemovals, removalPrefix, markerAlbum)
	}
	uuids := make([]string, len(visible))
	for i, p := range visible {
		uuids[i] = p.UUID
	}

	// Every call fails before the change: a dialog here is a broken check.
	t.Run("errors", func(t *testing.T) {
		ctx := changeContext(t)
		const none = "00000000-0000-0000-0000-000000000000"
		if err := lib.Delete(ctx, nil); err != nil {
			t.Errorf("Delete of no photo: %s", changeFailure(err))
		}
		secrets := append([]string{markerAlbumUUID, none, removalPrefix}, uuids...)
		for _, e := range []struct {
			name, role string
			err        error
		}{
			{"a missing photo", ": asset", lib.Delete(ctx, []string{none})},
			{"a photo and a missing one", ": asset", lib.Delete(ctx, []string{uuids[0], none})},
			{"an identifier with a NUL", ": identifier", lib.Delete(ctx, []string{"a\x00b"})},
		} {
			if !errors.Is(e.err, photokit.ErrNotFound) || !strings.HasSuffix(e.err.Error(), e.role) {
				t.Errorf("Delete of %s: %s, want ErrNotFound%s", e.name, changeFailure(e.err), e.role)
			}
			if e.err != nil && slices.ContainsFunc(secrets, func(s string) bool { return strings.Contains(e.err.Error(), s) }) {
				t.Errorf("the error of the Delete of %s names an identifier or the prefix", e.name)
			}
		}
		if !fetchable(t, lib, uuids[0]) {
			t.Error("Fetch does not find the photo of a failed Delete: the batch was not all or none")
		}
	})

	// A second dialog while the first is open would mix up the answers.
	if !t.Run("cancel", func(t *testing.T) {
		switch err := lib.Delete(changeContext(t), uuids[:1]); {
		case err == nil:
			t.Error("Delete returned no error where the owner was to refuse the dialog")
		case errors.Is(err, photokit.ErrOutcomeUnknown):
			t.Fatalf("the dialog of Delete was not answered: answer it in Photos; the photos %s* left over are deleted by the next run: %s", removalPrefix, changeFailure(err))
		case !errors.Is(err, photokit.ErrUserCanceled):
			t.Errorf("Delete refused in the dialog: %s, want ErrUserCanceled", changeFailure(err))
		case !fetchable(t, lib, uuids[0]):
			t.Error("Fetch does not find the photo of a refused Delete")
		}
	}) {
		return
	}

	// performChanges cannot be interrupted: the dialog stays open after the
	// deadline, and an answer to it may still delete the photo.
	t.Run("deadline", func(t *testing.T) {
		i := slices.IndexFunc(uuids, func(uuid string) bool { return fetchable(t, lib, uuid) })
		if i < 0 {
			t.Skip("no photo with the prefix is left to delete")
		}
		ctx, cancel := context.WithTimeout(t.Context(), lateTimeout)
		defer cancel()
		start := time.Now()
		err := lib.Delete(ctx, uuids[i:i+1])
		returned := time.Now()
		took := returned.Sub(start)
		t.Logf("Delete returned after %v", took.Round(time.Second))
		switch {
		case errors.Is(err, photokit.ErrUserCanceled):
			t.Fatalf("the dialog was refused before the deadline: answer it after %v, with Delete", lateTimeout)
		case err == nil:
			t.Fatalf("the dialog was confirmed before the deadline: answer it after %v, with Delete", lateTimeout)
		case !errors.Is(err, photokit.ErrOutcomeUnknown) || !errors.Is(err, context.DeadlineExceeded):
			t.Fatalf("Delete with a deadline of %v: %s, want ErrOutcomeUnknown with DeadlineExceeded", lateTimeout, changeFailure(err))
		case (took - lateTimeout).Abs() > 5*time.Second:
			t.Errorf("Delete returned after %v, want it at the deadline of %v", took.Round(time.Second), lateTimeout)
		}
		for end := returned.Add(lateWait); fetchable(t, lib, uuids[i]); time.Sleep(5 * time.Second) {
			if time.Now().After(end) {
				t.Logf("the asset is still there %v after the deadline: a late deletion was not observed; a dialog may still be open: answer it", time.Since(returned).Round(time.Second))
				return
			}
		}
		t.Logf("the late answer deleted the asset %v after the deadline", time.Since(returned).Round(time.Second))
	})

	t.Run("confirm", func(t *testing.T) {
		rest := filter(uuids, func(uuid string) bool { return fetchable(t, lib, uuid) })
		if len(rest) == 0 {
			t.Skip("no photo with the prefix is left to delete")
		}
		if err := lib.Delete(changeContext(t), rest); err != nil {
			t.Fatalf("Delete confirmed in the dialog: %s; a dialog may still be open: answer it, then delete the photos %s* of the album %s in Photos",
				changeFailure(err), removalPrefix, markerAlbum)
		}
		if n := count(rest, func(uuid string) bool { return fetchable(t, lib, uuid) }); n > 0 {
			t.Errorf("Fetch finds %d of %d photos after their Delete", n, len(rest))
		}
		// The hidden photos stay with the owner, so only the visible ones are awaited.
		after := ownPhotos(t, reopenUntil(t, func(db *photosdb.DB) bool { return count(ownPhotos(t, db), visibleOutsideTrash) == 0 }))
		left := count(after, visibleOutsideTrash)
		t.Logf("after Delete photosdb has %d photos with the prefix in the trash, %d hidden and %d visible outside the trash",
			count(after, func(p *photoinfo.PhotoInfo) bool { return p.InTrash }), count(after, hiddenOutsideTrash), left)
		if left > 0 {
			t.Errorf("photosdb has %d visible photos with the prefix outside the trash after their Delete; delete the photos %s* of the album %s in Photos",
				left, removalPrefix, markerAlbum)
		}
	})
}

// Both marks, the album and the prefix: the prefix alone could name a photo
// of the owner. The title of the album may change during the run, the UUID the
// guard read may not.
func ownPhotos(t *testing.T, db *photosdb.DB) []*photoinfo.PhotoInfo {
	t.Helper()
	a, ok := db.Album(markerAlbumUUID)
	if !ok {
		t.Fatalf("photosdb has no album %s: nothing is deleted", markerAlbum)
	}
	return filter(db.AlbumPhotos(a), withPrefix)
}

func withPrefix(p *photoinfo.PhotoInfo) bool {
	return strings.HasPrefix(p.OriginalFilename, removalPrefix)
}

// PhotoKit finds neither a hidden photo nor one in the trash.
func visibleOutsideTrash(p *photoinfo.PhotoInfo) bool { return !p.Hidden && !p.InTrash }

func hiddenOutsideTrash(p *photoinfo.PhotoInfo) bool { return p.Hidden && !p.InTrash }

// Fetch does not find a deleted asset: Recently Deleted is outside its default
// options.
func fetchable(t *testing.T, lib *photokit.Library, uuid string) bool {
	t.Helper()
	_, err := lib.Fetch(t.Context(), []string{uuid})
	if err != nil && !errors.Is(err, photokit.ErrNotFound) {
		t.Fatalf("Fetch of a photo with the prefix: %s", failure(err))
	}
	return err == nil
}
