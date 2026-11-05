//go:build photokit_live

package livetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
)

// The removal finds the collections of this run and of earlier ones by the
// prefix of their title, the prefix of the imports, so that the owner removes
// both alike. Of the seven creations, four must succeed and three must fail.
const (
	collectionPrefix = importPrefix
	maxCollections   = 7
)

// No change here should raise a dialog; one that does waits for the owner.
const collectionTimeout = 2 * time.Minute

// The marker album is only read: the run changes its own folders and albums
// and adds to them photos of the marker album.
func TestLiveCollections(t *testing.T) {
	lib := openLibrary(t)
	photos := visibleMarkerPhotos(t)
	var uuids []string
	for _, p := range photos[:min(len(photos), 3)] {
		uuids = append(uuids, p.UUID)
	}
	// With one photo, a RemoveFromAlbum that removes all would pass.
	if len(uuids) < 2 {
		skipNone(t, "second photo that is not hidden")
	}
	c := &collector{lib: lib}
	t.Cleanup(func() { t.Logf("%d collections of this and earlier runs removed at the end of the run", c.remove(t)) })

	t.Run("list", func(t *testing.T) {
		all := c.list(t)
		albums := filter(all, func(x photokit.Collection) bool { return !x.IsFolder })
		db := reopenLibrary(t)
		inDB := count(albums, func(x photokit.Collection) bool {
			uuid, _, _ := strings.Cut(x.LocalIdentifier, "/")
			_, ok := db.Album(uuid)
			return ok
		})
		t.Logf("Collections: %d folders, %d albums, %d of them albums of photosdb; photosdb: %d user albums, %d shared albums",
			len(all)-len(albums), len(albums), inDB, len(db.AlbumInfo()), len(db.SharedAlbumInfo()))
		if len(albums) != len(db.AlbumInfo()) {
			t.Errorf("Collections lists %d albums, photosdb %d user albums", len(albums), len(db.AlbumInfo()))
		}
		i := slices.IndexFunc(all, func(x photokit.Collection) bool { return x.Title == markerAlbum })
		if i < 0 || !strings.HasPrefix(all[i].LocalIdentifier, markerAlbumUUID) {
			t.Errorf("Collections lists no album %s whose LocalIdentifier starts with its UUID", markerAlbum)
			return
		}
		t.Logf("the LocalIdentifier of an album is its UUID followed by %q", strings.TrimPrefix(all[i].LocalIdentifier, markerAlbumUUID))
	})

	if !t.Run("removal of earlier runs", func(t *testing.T) {
		t.Logf("%d collections with the prefix found from earlier runs", c.remove(t))
	}) {
		return
	}

	var folderID, albumID string
	if !t.Run("folder and album", func(t *testing.T) {
		ctx := changeContext(t)
		folderID = c.made(t, collectionPrefix+"folder", "", true)
		albumID = c.made(t, collectionPrefix+"album", folderID, false)
		succeed(t, "RenameCollection of the folder", lib.RenameCollection(ctx, folderID, collectionPrefix+"folder-2"))
		succeed(t, "RenameCollection of the album", lib.RenameCollection(ctx, albumID, collectionPrefix+"album-2"))
		want := []photokit.Collection{
			{LocalIdentifier: folderID, Title: collectionPrefix + "folder-2", IsFolder: true},
			{LocalIdentifier: albumID, Title: collectionPrefix + "album-2", Parent: folderID},
		}
		if got := filter(c.list(t), prefixed); !slices.Equal(got, want) {
			t.Errorf("Collections lists %s, want the folder at the top and the album in it, both renamed", shape(got, folderID))
		}
	}) {
		return
	}

	t.Run("bare UUID", func(t *testing.T) {
		uuid, _, _ := strings.Cut(albumID, "/")
		if uuid == albumID {
			t.Skip("the LocalIdentifier of the album is its UUID")
		}
		switch err := lib.RenameCollection(changeContext(t), uuid, collectionPrefix+"album-2"); {
		case err == nil:
			t.Log("a bare UUID resolves")
		case errors.Is(err, photokit.ErrNotFound):
			t.Log("a bare UUID does not resolve: callers pass the LocalIdentifier")
		default:
			t.Errorf("RenameCollection by the bare UUID: %s", changeFailure(err))
		}
	})

	if !t.Run("assets", func(t *testing.T) {
		ctx := changeContext(t)
		// The photo named twice covers the deduplication that pk_import shares,
		// on which the count of the found assets rests.
		succeed(t, "AddToAlbum", lib.AddToAlbum(ctx, albumID, append(slices.Clone(uuids), uuids[0])))
		a := ownAlbum(reopenUntil(t, holding(len(uuids))))
		if a == nil {
			t.Fatal("photosdb has no album of the new title")
		}
		uuid, _, _ := strings.Cut(albumID, "/")
		for _, f := range []struct {
			field   string
			differs bool
		}{
			{"UUID", a.UUID != uuid},
			{"FolderNames", !slices.Equal(a.FolderNames, []string{collectionPrefix + "folder-2"})},
			{"PhotoUUIDs", !slices.Equal(a.PhotoUUIDs, slices.Sorted(slices.Values(uuids)))},
		} {
			if f.differs {
				t.Errorf("%s of the album in photosdb differs from the changes", f.field)
			}
		}
		succeed(t, "RemoveFromAlbum", lib.RemoveFromAlbum(ctx, albumID, uuids[:1]))
		if a = ownAlbum(reopenUntil(t, holding(len(uuids)-1))); a == nil || !slices.Equal(a.PhotoUUIDs, slices.Sorted(slices.Values(uuids[1:]))) {
			t.Error("after RemoveFromAlbum of one photo, the album in photosdb does not hold the others only")
		}
	}) {
		return
	}

	// A broken check would change only the collections of the run, which the
	// removal finds by the prefix or by the identifier.
	t.Run("errors", func(t *testing.T) {
		ctx := changeContext(t)
		const none = "00000000-0000-0000-0000-000000000000"
		before := len(c.list(t))
		title := collectionPrefix + "error"
		_, emptyTitle := c.create(t, "", "", false)
		_, noParent := c.create(t, title, none, false)
		_, albumParent := c.create(t, title, albumID, false)
		secrets := append([]string{albumID, folderID, markerAlbumUUID, none, collectionPrefix}, uuids...)
		// Every code of the bridge for a missing object is ErrNotFound: only the
		// role at the end of the error tells them apart.
		for _, e := range []struct {
			name      string
			err, want error
			role      string
		}{
			{"CreateAlbum with an empty title", emptyTitle, photokit.ErrInvalidTitle, ""},
			{"CreateAlbum in a missing folder", noParent, photokit.ErrNotFound, ": parent"},
			{"CreateAlbum in an album", albumParent, photokit.ErrNotFound, ": parent"},
			{"RenameCollection of a missing collection", lib.RenameCollection(ctx, none, title), photokit.ErrNotFound, ": collection"},
			{"DeleteCollections of the folder and a missing collection", lib.DeleteCollections(ctx, []string{folderID, none}), photokit.ErrNotFound, ": collection"},
			{"AddToAlbum to a missing album", lib.AddToAlbum(ctx, none, uuids), photokit.ErrNotFound, ": album"},
			{"AddToAlbum to a folder", lib.AddToAlbum(ctx, folderID, uuids), photokit.ErrNotFound, ": album"},
			{"AddToAlbum of a photo and a missing one", lib.AddToAlbum(ctx, albumID, []string{uuids[0], none}), photokit.ErrNotFound, ": asset"},
		} {
			if !errors.Is(e.err, e.want) || !strings.HasSuffix(e.err.Error(), e.role) {
				t.Errorf("%s: %s, want %v%s", e.name, changeFailure(e.err), e.want, e.role)
			}
			if e.err != nil && slices.ContainsFunc(secrets, func(s string) bool { return strings.Contains(e.err.Error(), s) }) {
				t.Errorf("the error of %s names an identifier or a title", e.name)
			}
		}
		if n := len(c.list(t)); n != before {
			t.Errorf("Collections lists %d collections after the failed changes, %d before", n, before)
		}
		// photosdb may show a wrong add of uuids[0] late: a passing run waits
		// for it.
		if a := ownAlbum(reopenUntil(t, holding(len(uuids)))); a == nil || !slices.Equal(a.PhotoUUIDs, slices.Sorted(slices.Values(uuids[1:]))) {
			t.Error("the failed changes changed the album in photosdb")
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := changeContext(t)
		// The album alone first: deleting its folder could take it along. It is
		// named twice, which PhotoKit must not see.
		succeed(t, "DeleteCollections of the album", lib.DeleteCollections(ctx, []string{albumID, albumID}))
		if got := filter(c.list(t), prefixed); len(got) != 1 || got[0].LocalIdentifier != folderID {
			t.Errorf("after DeleteCollections of the album, Collections lists %s, want the folder only", shape(got, folderID))
		}
		succeed(t, "DeleteCollections of the folder", lib.DeleteCollections(ctx, []string{folderID}))
		if n := count(c.list(t), prefixed); n > 0 {
			t.Errorf("Collections lists %d collections with the prefix after their deletion", n)
		}
		if n := ownCount(reopenUntil(t, func(db *photosdb.DB) bool { return ownCount(db) == 0 })); n > 0 {
			t.Errorf("photosdb has %d albums and folders with the prefix after their deletion", n)
		}
		const albumTitle = collectionPrefix + "album-of-deleted-folder"
		folder := c.made(t, collectionPrefix+"deleted-folder", "", true)
		album := c.made(t, albumTitle, folder, false)
		succeed(t, "DeleteCollections of a folder that holds an album", lib.DeleteCollections(ctx, []string{folder}))
		all := c.list(t)
		listed := func(id string) bool {
			return slices.ContainsFunc(all, func(x photokit.Collection) bool { return x.LocalIdentifier == id })
		}
		// Neither Collections nor the removal reaches an album that outlives its
		// folder; photosdb tells it from a deleted one.
		db := reopenUntil(t, func(db *photosdb.DB) bool { return ownCount(db) == 0 })
		inDB := slices.ContainsFunc(db.AlbumInfo(), func(a *albuminfo.AlbumInfo) bool { return a.Title == albumTitle })
		t.Logf("after the deletion of a folder, the album that was in it: listed by Collections %t, in photosdb %t", listed(album), inDB)
		if listed(folder) {
			t.Error("Collections lists the deleted folder")
		}
		if inDB && !listed(album) {
			t.Errorf("photosdb holds the album of the deleted folder, which Collections does not list; delete the folders and albums %s* in Photos", collectionPrefix)
		}
	})
}

// collector keeps what the run created for the removal at its end. A
// creation that failed or ended at its deadline may still have created its
// collection, so the limit counts attempts.
type collector struct {
	lib      *photokit.Library
	attempts int
	created  []string
}

func (c *collector) create(t *testing.T, title, parent string, folder bool) (string, error) {
	t.Helper()
	if c.attempts == maxCollections {
		t.Fatalf("more than %d creations of a collection in one run", maxCollections)
	}
	c.attempts++
	create := c.lib.CreateAlbum
	if folder {
		create = c.lib.CreateFolder
	}
	id, err := create(changeContext(t), title, parent)
	if err == nil {
		c.created = append(c.created, id)
	}
	return id, err
}

func (c *collector) made(t *testing.T, title, parent string, folder bool) string {
	t.Helper()
	id, err := c.create(t, title, parent, folder)
	succeed(t, "creation of "+title, err)
	return id
}

func (c *collector) list(t *testing.T) []photokit.Collection {
	t.Helper()
	all, err := c.lib.Collections(t.Context())
	if err != nil {
		t.Fatalf("Collections: %s", changeFailure(err))
	}
	return all
}

// DeleteCollections fails all when one is gone, so the collections are found
// again in Collections: by the prefix, or by the identifier for one whose
// title has none. The context is not that of t, which ends before its cleanup.
func (c *collector) remove(t *testing.T) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), collectionTimeout)
	defer cancel()
	all, err := c.lib.Collections(ctx)
	if err != nil {
		t.Errorf("Collections before the removal: %s; delete the folders and albums %s* in Photos", changeFailure(err), collectionPrefix)
		return 0
	}
	var ids []string
	for _, x := range all {
		if prefixed(x) || slices.Contains(c.created, x.LocalIdentifier) {
			ids = append(ids, x.LocalIdentifier)
		}
	}
	// A deleted folder may take what it holds along, and a collection of the
	// owner moved into a folder of a run must stay.
	if slices.ContainsFunc(all, func(x photokit.Collection) bool {
		return slices.Contains(ids, x.Parent) && !slices.Contains(ids, x.LocalIdentifier)
	}) {
		t.Errorf("a folder %s* holds a collection without the prefix; move it out and delete the folders and albums %s* in Photos", collectionPrefix, collectionPrefix)
		return 0
	}
	if err := c.lib.DeleteCollections(ctx, ids); err != nil {
		t.Errorf("DeleteCollections of %d collections: %s; delete the folders and albums %s* in Photos", len(ids), changeFailure(err), collectionPrefix)
		return 0
	}
	return len(ids)
}

func changeContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(t.Context(), collectionTimeout)
	t.Cleanup(cancel)
	return ctx
}

func succeed(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %s", what, changeFailure(err))
	}
}

func prefixed(x photokit.Collection) bool { return strings.HasPrefix(x.Title, collectionPrefix) }

// The titles are those of the run; an identifier is printed as its place only.
func shape(cs []photokit.Collection, folderID string) string {
	parts := []string{fmt.Sprintf("%d collections with the prefix", len(cs))}
	for _, x := range cs {
		kind, place := "album", "in another folder"
		if x.IsFolder {
			kind = "folder"
		}
		switch x.Parent {
		case "":
			place = "at the top"
		case folderID:
			place = "in the folder"
		}
		parts = append(parts, fmt.Sprintf("%s %q %s", kind, x.Title, place))
	}
	return strings.Join(parts, "; ")
}

// Only this run titles an album so.
func ownAlbum(db *photosdb.DB) *albuminfo.AlbumInfo {
	albums := db.AlbumInfo()
	if i := slices.IndexFunc(albums, func(a *albuminfo.AlbumInfo) bool { return a.Title == collectionPrefix+"album-2" }); i >= 0 {
		return albums[i]
	}
	return nil
}

func holding(n int) func(*photosdb.DB) bool {
	return func(db *photosdb.DB) bool {
		a := ownAlbum(db)
		return a != nil && len(a.PhotoUUIDs) == n
	}
}

// The folders of the run are at the top level.
func ownCount(db *photosdb.DB) int {
	return count(db.AlbumInfo(), func(a *albuminfo.AlbumInfo) bool { return strings.HasPrefix(a.Title, collectionPrefix) }) +
		count(db.FolderInfo(), func(f *albuminfo.FolderInfo) bool { return strings.HasPrefix(f.Title, collectionPrefix) })
}

// photosdb may show a change late, as reopenWithImports waits for an import.
func reopenUntil(t *testing.T, done func(*photosdb.DB) bool) *photosdb.DB {
	t.Helper()
	end := time.Now().Add(30 * time.Second)
	for {
		db := reopenLibrary(t)
		if done(db) || time.Now().After(end) {
			return db
		}
		time.Sleep(2 * time.Second)
	}
}

// As importFailure: the classes of the error and the domain and code of an
// NSError only.
func changeFailure(err error) string {
	if err == nil {
		return "no error"
	}
	nsError := "none"
	if pk, ok := errors.AsType[*bridge.PhotoKitError](err); ok {
		nsError = pk.Error()
	}
	return fmt.Sprintf("%s (ErrInvalidTitle: %t, ErrChange: %t, ErrUserCanceled: %t, ErrOutcomeUnknown: %t, NSError: %s)",
		failure(err), errors.Is(err, photokit.ErrInvalidTitle), errors.Is(err, photokit.ErrChange),
		errors.Is(err, photokit.ErrUserCanceled), errors.Is(err, photokit.ErrOutcomeUnknown), nsError)
}
