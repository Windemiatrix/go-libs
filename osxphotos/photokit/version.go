package photokit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// Version is PHImageRequestOptionsVersion, the version of an asset that a
// request asks for (photokit.py:79-87, 703-708): VersionCurrent with its
// edits, VersionUnadjusted the highest quality without them, VersionOriginal
// the original. The values are those of PhotoKit.
type Version int

// The values of PHImageRequestOptionsVersion.
const (
	VersionCurrent Version = iota
	VersionUnadjusted
	VersionOriginal
)

// ImageInfo is what requestImageDataAndOrientationForAsset gives besides the
// bytes (photokit.py:498-559, 721-736): UTI is dataUTI; Orientation the EXIF
// orientation of the data; Degraded PHImageResultIsDegradedKey; Path the file
// PhotoKit read (PHImageFileURLKey), "" when the key is absent, where path()
// and url() raise KeyError (534, 543-544); Metadata the properties of
// CGImageSourceCopyPropertiesAtIndex as a property list decodes them, nil
// when they cannot be serialized, exceed 1 MiB, do not decode or are not a
// dictionary. Metadata holds the EXIF, GPS and maker data of the file, and
// Path a path inside the library: a caller that logs must print neither.
type ImageInfo struct {
	UTI         string
	Orientation int
	Degraded    bool
	Path        string
	Metadata    map[string]any
}

// ExportImage writes the data of version v of the photo uuid into the file
// name of dir, as Export does (name, dir, o, the mode 0600, the removal on an
// error are those of Export), and returns the ImageInfo of the data:
// PhotoAsset.export of a photo that is not RAW (photokit.py:622-631, 646-655)
// together with metadata(), uti(), path(), orientation() and degraded
// (498-559), which upstream gets from the same request (684-750), with
// networkAccessAllowed (712) and HighQualityFormat (715-716); url() is Path as
// a file URL and is not returned. A Live Photo gives its still;
// LivePhotoAsset.export (1184-1276) is ExportLivePhoto. An asset that is not a
// photo is ErrMediaType; a v other than the three constants ErrInvalidVersion;
// no such asset ErrNotFound; no data of the version ErrNoResource; a failure of
// PhotoKit ErrExport with its domain and code; when ctx is done PhotoKit stops
// the request at its next report of progress, and ExportImage returns ctx.Err()
// once PhotoKit ends it, 30 s later at most, as Export does: a request that
// reports no progress (read from disk, or a stalled download) is not stopped.
// Upstream names the file by the UTI (628, 631); here the caller does, with
// export.EditedName and uti.PreferredExtension. After an error the ImageInfo
// is the zero value.
func (l *Library) ExportImage(ctx context.Context, uuid string, v Version, dir, name string, o ExportOptions) (ImageInfo, error) {
	s := &imageSink{}
	err := writeExport(dir, name, o, func() error {
		_, err := l.checkVersion(ctx, uuid, func(a Asset) bool { return a.IsPhoto }, v)
		return err
	}, func(f *fileSink) error {
		s.fileSink = f
		return awaitRequest(ctx, f, func() (bridge.Request, error) { return l.b.RequestImage(uuid, int(v), s) })
	})
	if err != nil {
		return ImageInfo{}, err
	}
	r := s.record
	return ImageInfo{UTI: r.UTI, Orientation: r.Orientation, Degraded: r.Degraded, Path: r.Path, Metadata: metadata(r.Metadata)}, nil
}

// ExportVideo writes the file of version v of the video uuid into the file
// name of dir, as Export does (name, dir, o.Overwrite, the mode 0600, the
// removal on an error are those of Export): VideoAsset.export
// (photokit.py:918-981). requestAVAssetForVideo, with networkAccessAllowed and
// HighQualityFormat (1054-1064), gives the URL of a file PhotoKit holds, which
// is read, never changed, and copied (979). VersionUnadjusted asks for the
// original, as upstream does (1056-1061): PHVideoRequestOptionsVersion has
// Current and Original only.
//
// The current version of a slow-motion video is an AVComposition without a
// file: it is rendered through AVAssetExportSession with
// AVAssetExportPresetHighestQuality into a QuickTime movie, as upstream does
// (943-951, 983-1031, 866-873), so its bytes are a new encoding and may differ
// between calls; the caller names the file .mov, as upstream does (1021). When
// PhotoKit gives that version as a file instead, the file is copied as that of
// any other video, where upstream raises PhotoKitMediaTypeError (1005-1009); a
// current AVAsset that is neither a composition of two tracks nor a file is
// ErrMediaType (1005-1009). The render goes into a directory that ExportVideo
// creates inside dir with mode 0700 and removes before it returns; the file
// name is then written as by every export. A failed render is ErrExport with
// the domain and code of the error of the session (893-895).
//
// Another video whose request gives no file, or a file that does not exist or
// is not a regular file (965-966, 971-972), is ErrNoResource; a file that
// cannot be opened is another error, as FileUtil.copy raises (979). No error
// names a file. An asset that is not a video is ErrMediaType; a v other than
// the three constants ErrInvalidVersion (1047-1052); no such asset ErrNotFound;
// a failure of PhotoKit ErrExport with its domain and code. When ctx is done
// the request or the render is canceled and ExportVideo returns ctx.Err() once
// PhotoKit or the session ends it, 30 s later at most, or, during the copy,
// before its next MiB. o.Progress is not called. Upstream names the file by the
// extension of the file of PhotoKit (973-974); here the caller does, with
// export.Names and export.EditedName, whose suffix is that of the render
// (PathEdited).
func (l *Library) ExportVideo(ctx context.Context, uuid string, v Version, dir, name string, o ExportOptions) error {
	var path string
	var slowMo bool
	return writeExport(dir, name, o, func() error {
		a, err := l.checkVersion(ctx, uuid, func(a Asset) bool { return a.IsMovie }, v)
		slowMo = a.SlowMo
		return err
	}, func(s *fileSink) error {
		// Upstream renders only the current version of a slow-motion video (photokit.py:943).
		if slowMo && v == VersionCurrent {
			return l.exportComposition(ctx, uuid, dir, s)
		}
		// PHVideoRequestOptionsVersion has Current (0) and Original (1) only;
		// upstream maps the versions the same way (photokit.py:1056-1061).
		version := 1
		if v == VersionCurrent {
			version = 0
		}
		err := awaitRequest(ctx, s, func() (bridge.Request, error) {
			return l.b.RequestVideo(uuid, version, func(p string) { path = p })
		})
		if err != nil {
			return err
		}
		return copyVideo(ctx, path, s)
	})
}

// ExportLivePhoto writes the photo (ResourcePhoto) or the paired video
// (ResourcePairedVideo) of version v of the Live Photo uuid into the file name
// of dir, as Export writes a resource (name, dir, o, the mode 0600, the
// removal on an error, the download from iCloud and the cancel by ctx are
// those of Export): LivePhotoAsset.export (photokit.py:1184-1276), one half
// per call. VersionCurrent gives the render of an edit of that half,
// ResourceFullSizePhoto or ResourceFullSizePairedVideo, when the asset has
// one, otherwise the resource of kind, where upstream asks PhotoKit for the
// PHLivePhoto (1104-1170, 1228-1230). VersionOriginal and VersionUnadjusted
// give the resource of kind, as upstream reads the resources of the asset for
// them (1231-1232); of several resources of the kind the first is taken, where
// upstream keeps the last (1236-1240). Another kind, or an asset without the resource, is
// ErrNoResource; an asset that is not a Live Photo ErrMediaType; a v other than
// the three constants ErrInvalidVersion; no such asset ErrNotFound; a failure
// of PhotoKit ErrExport with its domain and code. Upstream names the two files
// by their UTIs (1247-1258); here the caller does.
func (l *Library) ExportLivePhoto(ctx context.Context, uuid string, v Version, kind ResourceType, dir, name string, o ExportOptions) error {
	// Upstream exports these two resources of a Live Photo only (photokit.py:1236-1240).
	if kind != ResourcePhoto && kind != ResourcePairedVideo {
		return fmt.Errorf("%w: %d", ErrNoResource, kind)
	}
	var typ ResourceType
	return writeExport(dir, name, o, func() error {
		if _, err := l.checkVersion(ctx, uuid, func(a Asset) bool { return a.Live }, v); err != nil {
			return err
		}
		res, err := l.Resources(ctx, uuid)
		if err != nil {
			return err
		}
		typ = liveResource(res, v, kind)
		if !slices.ContainsFunc(res, func(r Resource) bool { return r.Type == typ }) {
			return fmt.Errorf("%w: %d", ErrNoResource, typ)
		}
		return nil
	}, func(s *fileSink) error {
		return awaitRequest(ctx, s, func() (bridge.Request, error) { return l.b.RequestResource(uuid, int(typ), s) })
	})
}

// For VersionCurrent upstream reads the resources of the PHLivePhoto
// (photokit.py:1104-1170, 1228-1230), whose result handler PhotoKit may deliver
// on the main queue, which nothing runs in a Go program. The asset holds the
// same resources: PhotoKit keeps the render of an edit beside the original as
// a FullSize resource, and an edit may redraw one half only, so each half is
// chosen alone. The other versions give the original, as upstream reads the
// resources of the asset for them (1231-1232).
func liveResource(res []Resource, v Version, kind ResourceType) ResourceType {
	render := ResourceFullSizePhoto
	if kind == ResourcePairedVideo {
		render = ResourceFullSizePairedVideo
	}
	if v == VersionCurrent && slices.ContainsFunc(res, func(r Resource) bool { return r.Type == render }) {
		return render
	}
	return kind
}

// Upstream checks the version when it requests (photokit.py:703-708, 1047-1052),
// after the class of the asset (1403-1413).
func (l *Library) checkVersion(ctx context.Context, uuid string, class func(Asset) bool, v Version) (Asset, error) {
	assets, err := l.Fetch(ctx, []string{uuid})
	if err != nil {
		return Asset{}, err
	}
	a := assets[0]
	if !class(a) {
		return Asset{}, fmt.Errorf("%w: %d", ErrMediaType, a.MediaType)
	}
	if v < VersionCurrent || v > VersionOriginal {
		return Asset{}, fmt.Errorf("%w: %d", ErrInvalidVersion, v)
	}
	return a, nil
}

// A variable, so that a test can fail it: dir, where the file name has just
// been created, is writable when it is called.
var mkdirTemp = os.MkdirTemp

// _export_slow_mo (photokit.py:983-1031). AVAssetExportSession refuses a file
// that exists, and name exists already, so the session writes into a directory
// of its own; the copy gives the render the rules of every export. The errors
// of os name the directory, so only their cause is kept.
func (l *Library) exportComposition(ctx context.Context, uuid, dir string, s *fileSink) error {
	tmp, err := mkdirTemp(dir, ".osxphotos-*")
	if err != nil {
		return fmt.Errorf("photokit: cannot create the directory of the render: %w", errors.Unwrap(err))
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // the result is decided; a failed removal leaves only the hidden directory
	out := filepath.Join(tmp, "render.mov")
	var file string
	// 0 is PHVideoRequestOptionsVersionCurrent, the version upstream renders (photokit.py:1004).
	err = awaitRequest(ctx, s, func() (bridge.Request, error) {
		return l.b.ExportComposition(uuid, 0, out, func(p string) { file = p })
	})
	switch {
	case errors.Is(err, bridge.ErrNoComposition):
		return fmt.Errorf("%w: not a slow-motion composition", ErrMediaType)
	case err != nil:
		return err
	case file != "":
		// extension: not in osxphotos. PhotoKit gives the current version of some
		// slow-motion videos as a file, where upstream raises
		// PhotoKitMediaTypeError (photokit.py:1005-1009).
		return copyVideo(ctx, file, s)
	}
	return copyVideo(ctx, out, s)
}

// Upstream checks is_file() (photokit.py:971-972) and lets the copy raise
// (979). The errors of os name the path, which is inside the library, so only
// their cause is kept. A video may take long to copy, so ctx is checked
// between chunks.
func copyVideo(ctx context.Context, path string, s *fileSink) error {
	fi, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%w: %w", ErrNoResource, errors.Unwrap(err))
	case err == nil && !fi.Mode().IsRegular():
		return ErrNoResource
	}
	f, err := os.Open(path) //nolint:gosec // G304: the file of the AVURLAsset that PhotoKit gave, opened only to be read
	if err != nil {
		return fmt.Errorf("photokit: cannot open the file of the video: %w", errors.Unwrap(err))
	}
	defer f.Close() //nolint:errcheck // opened only to be read
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := f.Read(buf)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("photokit: cannot read the file of the video: %w", errors.Unwrap(err))
		}
		if err := s.Write(buf[:n]); err != nil {
			return err
		}
	}
}

// Upstream keeps the dictionary itself (photokit.py:728-732); here it crosses
// the bridge as the binary list NSPropertyListSerialization writes from the
// properties of ImageIO, a tree, so 1 MiB bounds the decoding without the
// object and expansion limits that photosdb needs for lists read from a
// database.
func metadata(b []byte) map[string]any {
	// An empty document decodes as an empty dictionary in the text format of howett.net/plist.
	if len(b) == 0 || len(b) > 1<<20 {
		return nil
	}
	var v any
	if _, err := plist.Unmarshal(b, &v); err != nil {
		return nil
	}
	m, _ := v.(map[string]any)
	return m
}

// PhotoKit gives the record before Done, and ExportImage reads it only after
// Done: the channel orders them.
type imageSink struct {
	*fileSink
	record bridge.ImageRecord
}

func (s *imageSink) Info(r bridge.ImageRecord) { s.record = r }
