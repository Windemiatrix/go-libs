package photokit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// Collection is a user folder (PHCollectionList), which holds folders and
// albums, or album (PHAssetCollection), which holds assets. Parent is the
// LocalIdentifier of the enclosing folder, "" at the top level. Smart albums
// and the albums of shared libraries are not user collections.
// extension: not in osxphotos
type Collection struct {
	LocalIdentifier, Title, Parent string
	IsFolder                       bool
}

// Collections returns every user folder and album, a parent before its
// children, in the order of PhotoKit. ctx is checked only before the call.
// extension: not in osxphotos
func (l *Library) Collections(ctx context.Context) ([]Collection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	records, err := l.b.Collections()
	var out []Collection
	for _, r := range records {
		out = append(out, Collection(r))
	}
	return out, err
}

// CreateFolder makes a folder titled title in parent ("" is the top level) in
// one change and returns its LocalIdentifier. A bad title is ErrInvalidTitle,
// a parent that is missing or no folder ErrNotFound, a failure of PhotoKit
// ErrChange, a refusal of the user ErrUserCanceled. No dialog is expected. As
// for Import, a ctx done before the call is ctx.Err() and nothing was changed,
// and a ctx done before the outcome is ErrOutcomeUnknown with ctx.Err().
// extension: not in osxphotos
func (l *Library) CreateFolder(ctx context.Context, title, parent string) (string, error) {
	return l.create(ctx, title, parent, true)
}

// CreateAlbum is CreateFolder for an album.
// extension: not in osxphotos
func (l *Library) CreateAlbum(ctx context.Context, title, parent string) (string, error) {
	return l.create(ctx, title, parent, false)
}

// RenameCollection sets the title of the folder or album id. The errors are
// those of CreateFolder; a collection that cannot be renamed, such as a smart
// album, is ErrNotFound.
// extension: not in osxphotos
func (l *Library) RenameCollection(ctx context.Context, id, title string) error {
	if !isCString(title) {
		return ErrInvalidTitle
	}
	_, err := collectionChange(ctx, "collection", []string{id}, func(timeout time.Duration) (string, error) {
		return "", l.b.RenameCollection(id, title, timeout)
	})
	return err
}

// DeleteCollections removes the folders and albums ids, not their assets, in
// one change, all or none; an album inside a deleted folder is deleted with
// it. A change that deletes a folder asks the user once; one that deletes
// only albums shows no dialog. Before the change, and without a dialog, a
// collection that is missing or cannot be deleted is ErrNotFound. A refusal of
// the user is ErrUserCanceled and nothing was deleted. A ctx done before the
// outcome is ErrOutcomeUnknown with ctx.Err(), as for Delete: the change may
// still take effect, and with a folder the dialog stays open and a later
// answer may still delete, so read the collections again before the next
// change. An empty ids changes nothing; the other errors are as for
// CreateFolder.
// extension: not in osxphotos
func (l *Library) DeleteCollections(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	ids = slices.Clone(ids)
	_, err := collectionChange(ctx, "collection", ids, func(timeout time.Duration) (string, error) {
		return "", l.b.DeleteCollections(ids, timeout)
	})
	return err
}

// AddToAlbum adds the assets, UUIDs or LocalIdentifiers as for Fetch, to the
// album in one change, all or none: an album that is missing or cannot take
// the change (a smart or a shared album) and an asset not found (a hidden one
// too) are ErrNotFound. An empty assets changes nothing. The other errors are
// those of CreateFolder.
// extension: not in osxphotos
func (l *Library) AddToAlbum(ctx context.Context, album string, assets []string) error {
	return l.albumAssets(ctx, album, assets, true)
}

// RemoveFromAlbum takes the assets out of the album as AddToAlbum adds them.
// extension: not in osxphotos
func (l *Library) RemoveFromAlbum(ctx context.Context, album string, assets []string) error {
	return l.albumAssets(ctx, album, assets, false)
}

func (l *Library) create(ctx context.Context, title, parent string, folder bool) (string, error) {
	if !isCString(title) {
		return "", ErrInvalidTitle
	}
	var ids []string
	if parent != "" {
		ids = []string{parent}
	}
	id, err := collectionChange(ctx, "parent", ids, func(timeout time.Duration) (string, error) {
		return l.b.CreateCollection(title, parent, folder, timeout)
	})
	if err == nil && id == "" {
		err = fmt.Errorf("%w: no identifier", ErrChange)
	}
	return id, err
}

func (l *Library) albumAssets(ctx context.Context, album string, assets []string, add bool) error {
	if len(assets) == 0 {
		return nil
	}
	ids := append([]string{album}, assets...)
	_, err := collectionChange(ctx, "album", ids, func(timeout time.Duration) (string, error) {
		return "", l.b.AlbumAssets(album, ids[1:], add, timeout)
	})
	return err
}

// Callers pass copies: the bridge may read them after the return on ctx. C
// would cut an identifier at a NUL and change another object. An error names
// the role only: the identifiers and the titles are the data of the caller.
func collectionChange(ctx context.Context, what string, ids []string, call func(timeout time.Duration) (string, error)) (string, error) {
	if slices.ContainsFunc(ids, func(id string) bool { return !isCString(id) }) {
		return "", fmt.Errorf("%w: identifier", ErrNotFound)
	}
	id, err := change(ctx, call)
	_, failed := errors.AsType[*bridge.PhotoKitError](err)
	switch {
	case errors.Is(err, bridge.ErrNoAlbum):
		return "", fmt.Errorf("%w: %s", ErrNotFound, what)
	case errors.Is(err, bridge.ErrNoAsset):
		return "", fmt.Errorf("%w: asset", ErrNotFound)
	case failed:
		return "", fmt.Errorf("%w: %w", ErrChange, err)
	case err != nil:
		return "", err
	}
	return id, nil
}

func isCString(s string) bool {
	return s != "" && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
