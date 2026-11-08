package photokit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// Version is PHImageRequestOptionsVersion, the version of an asset that a
// request asks for (photokit.py:77-85, 644-649): VersionCurrent with its
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
// bytes (photokit.py:466-527, 662-677): UTI is dataUTI; Orientation the EXIF
// orientation of the data; Degraded PHImageResultIsDegradedKey; Path the file
// PhotoKit read (PHImageFileURLKey), "" when the key is absent, where path()
// and url() raise KeyError (502, 511-512); Metadata the properties of
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
// PhotoAsset.export of a photo that is not RAW (photokit.py:587-605) together
// with metadata(), uti(), path(), orientation() and degraded (466-527), which
// upstream gets from the same request (625-691), with networkAccessAllowed
// (653) and HighQualityFormat (656-657); url() is Path as a file URL and is
// not returned. A Live Photo gives its still; LivePhotoAsset.export
// (1095-1182) is ExportLivePhoto. An asset that is not a photo is
// ErrMediaType; a v other than the three constants ErrInvalidVersion; no such
// asset ErrNotFound; no data of the version ErrNoResource; a failure of
// PhotoKit ErrExport with its domain and code; when ctx is done PhotoKit stops
// the request at its next report of progress, and ExportImage returns ctx.Err()
// once PhotoKit ends it, 30 s later at most, as Export does: a request that
// reports no progress (read from disk, or a stalled download) is not stopped.
// Upstream names the file by the UTI (593, 596); here the caller does, with
// export.EditedName and uti.PreferredExtension. After an error the ImageInfo
// is the zero value.
func (l *Library) ExportImage(ctx context.Context, uuid string, v Version, dir, name string, o ExportOptions) (ImageInfo, error) {
	s := &imageSink{}
	err := writeExport(dir, name, o, func() error {
		return l.checkVersion(ctx, uuid, func(a Asset) bool { return a.IsPhoto }, v)
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
// removal on an error are those of Export): VideoAsset.export without its
// slow-motion branch (photokit.py:851-914). requestAVAssetForVideo, with
// networkAccessAllowed and HighQualityFormat (987-992), gives the URL of a file
// PhotoKit holds, which is read, never changed, and copied (912).
// VersionUnadjusted asks for the original: PHVideoRequestOptionsVersion has
// Current and Original only. A video whose request gives no file, such as a
// slow-motion video of VersionCurrent (an AVComposition, which upstream exports
// through AVAssetExportSession, 876-884), or one that does not exist or is not
// a regular file (904-905) is ErrNoResource; a file that cannot be opened is
// another error, as FileUtil.copy raises (912). No error names the file. An
// asset that is not a video is ErrMediaType; a v other than the three
// constants ErrInvalidVersion (980-985); no such asset ErrNotFound; a failure
// of PhotoKit ErrExport with its domain and code. When ctx is done the request
// is canceled and ExportVideo returns ctx.Err() once PhotoKit ends it, 30 s
// later at most, or, during the copy, before its next MiB. o.Progress is not
// called. Upstream names the file by the extension of the file of PhotoKit
// (906-907); here the caller does, with export.Names and export.EditedName,
// whose suffix is that of the render (PathEdited).
func (l *Library) ExportVideo(ctx context.Context, uuid string, v Version, dir, name string, o ExportOptions) error {
	var path string
	return writeExport(dir, name, o, func() error {
		return l.checkVersion(ctx, uuid, func(a Asset) bool { return a.IsMovie }, v)
	}, func(s *fileSink) error {
		// PHVideoRequestOptionsVersion has Current (0) and Original (1) only;
		// upstream passes the image constants as they are (photokit.py:989): its
		// Unadjusted (1) is Original, and its Original (2) is no value of the enum,
		// so it is sent as Original.
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
// those of Export): LivePhotoAsset.export (photokit.py:1095-1182), one half
// per call. VersionCurrent gives the render of an edit of that half,
// ResourceFullSizePhoto or ResourceFullSizePairedVideo, when the asset has
// one, otherwise the resource of kind, which VersionOriginal and
// VersionUnadjusted give, where upstream asks PhotoKit for the PHLivePhoto of
// the version (1032-1081). Another kind, or an asset without the resource, is
// ErrNoResource; an asset that is not a Live Photo ErrMediaType; a v other than
// the three constants ErrInvalidVersion; no such asset ErrNotFound; a failure
// of PhotoKit ErrExport with its domain and code. Upstream names the two files
// by their UTIs (1153-1164); here the caller does.
func (l *Library) ExportLivePhoto(ctx context.Context, uuid string, v Version, kind ResourceType, dir, name string, o ExportOptions) error {
	// Upstream exports these two resources of a Live Photo only (photokit.py:1142-1146).
	if kind != ResourcePhoto && kind != ResourcePairedVideo {
		return fmt.Errorf("%w: %d", ErrNoResource, kind)
	}
	var typ ResourceType
	return writeExport(dir, name, o, func() error {
		if err := l.checkVersion(ctx, uuid, func(a Asset) bool { return a.Live }, v); err != nil {
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

// Upstream reads the resources of the PHLivePhoto of the version
// (photokit.py:1032-1081), whose result handler PhotoKit may deliver on the
// main queue, which nothing runs in a Go program. The asset holds the same
// resources: PhotoKit keeps the render of an edit beside the original as a
// FullSize resource, and an edit may redraw one half only, so each half is
// chosen alone. VersionUnadjusted gives the original, as for ExportVideo: no
// resource type is known to hold the unadjusted half.
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

// Upstream checks the version when it requests (photokit.py:644-649, 980-985),
// after the class of the asset (1309-1319).
func (l *Library) checkVersion(ctx context.Context, uuid string, class func(Asset) bool, v Version) error {
	assets, err := l.Fetch(ctx, []string{uuid})
	if err != nil {
		return err
	}
	if a := assets[0]; !class(a) {
		return fmt.Errorf("%w: %d", ErrMediaType, a.MediaType)
	}
	if v < VersionCurrent || v > VersionOriginal {
		return fmt.Errorf("%w: %d", ErrInvalidVersion, v)
	}
	return nil
}

// Upstream checks is_file() (photokit.py:904-905) and lets the copy raise
// (912). The errors of os name the path, which is inside the library, so only
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

// Upstream keeps the dictionary itself (photokit.py:669-673); here it crosses
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
