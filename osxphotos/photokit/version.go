package photokit

import (
	"context"
	"fmt"

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
// not returned. A Live Photo gives its still, where upstream exports it
// through LivePhotoAsset.export (1095-1182). An asset that is not a photo is
// ErrMediaType; a v other than the three constants ErrInvalidVersion; no such
// asset ErrNotFound; no data of the version ErrNoResource; a failure of
// PhotoKit ErrExport with its domain and code; when ctx is done the request is
// canceled (cancelImageRequest:) and ExportImage returns ctx.Err(). Upstream
// names the file by the UTI (593, 596); here the caller does, with
// export.EditedName and uti.PreferredExtension. After an error the ImageInfo
// is the zero value. Until the Objective-C side of RequestImage lands,
// ExportImage on macOS returns an error that wraps ErrNotSupported.
func (l *Library) ExportImage(ctx context.Context, uuid string, v Version, dir, name string, o ExportOptions) (ImageInfo, error) {
	s := &imageSink{}
	err := writeExport(ctx, dir, name, o, func() error {
		assets, err := l.Fetch(ctx, []string{uuid})
		if err != nil {
			return err
		}
		if a := assets[0]; !a.IsPhoto {
			return fmt.Errorf("%w: %d", ErrMediaType, a.MediaType)
		}
		if v < VersionCurrent || v > VersionOriginal {
			return fmt.Errorf("%w: %d", ErrInvalidVersion, v)
		}
		return nil
	}, func(f *fileSink) (bridge.Request, error) {
		s.fileSink = f
		return l.b.RequestImage(uuid, int(v), s)
	})
	if err != nil {
		return ImageInfo{}, err
	}
	r := s.record
	return ImageInfo{UTI: r.UTI, Orientation: r.Orientation, Degraded: r.Degraded, Path: r.Path, Metadata: metadata(r.Metadata)}, nil
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
