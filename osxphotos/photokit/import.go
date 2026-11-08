package photokit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// ImportRequest describes one asset to create. Exactly one of Photo and Video
// is the original file; PairedVideo makes a Live Photo of Photo, and
// AlternatePhoto the RAW of a RAW+JPEG pair. Every path is an absolute path of
// a regular file, which Photos copies. OriginalFilename names the original in
// Photos ("" keeps the name of the file). A zero CreationDate leaves the date
// to Photos, a nil Location leaves none. Albums, a UUID or a LocalIdentifier
// per album, are those the asset is added to in the same change.
// extension: not in osxphotos
type ImportRequest struct {
	Photo, Video, PairedVideo, AlternatePhoto string
	OriginalFilename                          string
	CreationDate                              time.Time
	Location                                  *Location
	Favorite, Hidden                          bool
	Albums                                    []string
}

// Import creates one asset from r in one change of PhotoKit and returns its
// LocalIdentifier, as Fetch gives it. An import shows no dialog, a Hidden one
// too; a refusal of the user would be ErrUserCanceled. Before any call, a bad
// file or name is ErrImportFile and an album that is empty, has a NUL or is not
// UTF-8 ErrNotFound; so is, before the change, one not in the library or that
// cannot take an asset. A failure of PhotoKit is ErrImport. A ctx done before
// the call is ctx.Err(), and nothing was changed; a ctx done before the outcome
// is ErrOutcomeUnknown with ctx.Err(), and the request goes on in PhotoKit, as
// a change cannot be canceled. Without a deadline in ctx Import waits as long
// as PhotoKit does. The asset of a Hidden request is not found by Fetch.
// extension: not in osxphotos
func (l *Library) Import(ctx context.Context, r ImportRequest) (string, error) {
	if err := checkImportFiles(r); err != nil {
		return "", err
	}
	// The bridge takes an album as UTF-8; C would cut one at a NUL and add the asset to another.
	if slices.ContainsFunc(r.Albums, func(a string) bool { return !isCString(a) }) {
		return "", fmt.Errorf("%w: album", ErrNotFound)
	}
	rec := importRecord(r)
	id, err := change(ctx, func(timeout time.Duration) (string, error) { return l.b.Import(rec, timeout) })
	_, failed := errors.AsType[*bridge.PhotoKitError](err)
	switch {
	case errors.Is(err, bridge.ErrNoAlbum):
		return "", fmt.Errorf("%w: album", ErrNotFound)
	case failed:
		return "", fmt.Errorf("%w: %w", ErrImport, err)
	case err != nil:
		return "", err
	case id == "":
		return "", fmt.Errorf("%w: no identifier", ErrImport)
	}
	return id, nil
}

// The paths and the name are the data of the caller: an error names the field only.
func checkImportFiles(r ImportRequest) error {
	if (r.Photo == "") == (r.Video == "") {
		return fmt.Errorf("%w: one of Photo or Video", ErrImportFile)
	}
	for _, f := range []struct {
		name, path string
		withPhoto  bool
	}{
		{"Photo", r.Photo, false},
		{"Video", r.Video, false},
		{"PairedVideo", r.PairedVideo, true},
		{"AlternatePhoto", r.AlternatePhoto, true},
	} {
		if f.path != "" && ((f.withPhoto && r.Photo == "") || !isAbsFile(f.path)) {
			return fmt.Errorf("%w: %s", ErrImportFile, f.name)
		}
	}
	// C would cut the name at a NUL, and one that is not UTF-8 gives no NSString:
	// Photos would keep the name of the file.
	if r.OriginalFilename != "" && !isCString(r.OriginalFilename) {
		return fmt.Errorf("%w: OriginalFilename", ErrImportFile)
	}
	return nil
}

// os.Stat refuses a NUL, at which C would cut the path; PhotoKit reads through
// a symbolic link as through the file.
func isAbsFile(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	fi, err := os.Stat(path) //nolint:gosec // G304: the path is given by the caller and only checked
	return err == nil && fi.Mode().IsRegular()
}

// The bridge may read Albums after Import returned on ctx, so they are copied.
func importRecord(r ImportRequest) bridge.ImportRecord {
	rec := bridge.ImportRecord{
		Photo: r.Photo, Video: r.Video, PairedVideo: r.PairedVideo, AlternatePhoto: r.AlternatePhoto,
		OriginalFilename: r.OriginalFilename, Favorite: r.Favorite, Hidden: r.Hidden, Albums: slices.Clone(r.Albums),
	}
	if !r.CreationDate.IsZero() {
		rec.HasCreationDate, rec.CreationDate = true, float64(r.CreationDate.UnixMicro())/1e6
	}
	if r.Location != nil {
		rec.HasLocation, rec.Latitude, rec.Longitude = true, r.Location.Latitude, r.Location.Longitude
	}
	return rec
}

// PHPhotosErrorUserCancelled, the code of a refused dialog.
const userCanceled = 3072

// performChanges cannot be interrupted: after ctx is done the change may still
// complete, and the buffer lets the goroutine end when the bridge returns.
func change(ctx context.Context, call func(timeout time.Duration) (string, error)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Without a deadline the bridge waits as long as PhotoKit does; 0 would not wait at all.
	timeout := time.Duration(math.MaxInt64)
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}
	type result struct {
		id  string
		err error
	}
	done := make(chan result, 1)
	go func() {
		id, err := call(timeout)
		done <- result{id, err}
	}()
	var r result
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("%w: %w", ErrOutcomeUnknown, ctx.Err())
	case r = <-done:
	}
	pk, _ := errors.AsType[*bridge.PhotoKitError](r.err)
	switch {
	case errors.Is(r.err, context.DeadlineExceeded):
		return "", fmt.Errorf("%w: %w", ErrOutcomeUnknown, r.err)
	case pk != nil && pk.Domain == "PHPhotosErrorDomain" && pk.Code == userCanceled:
		return "", ErrUserCanceled
	}
	return r.id, r.err
}
