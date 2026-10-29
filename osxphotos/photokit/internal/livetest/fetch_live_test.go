//go:build photokit_live

package livetest

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit"
)

// The run log is read by others: the tests print field names and counts,
// never a UUID or a file name. A kind of photo the marker album lacks is a
// t.Skip, so that it shows in the summary of the run.

func TestLiveFetch(t *testing.T) {
	lib := openLibrary(t)
	photos := untrashedMarkerPhotos(t)
	assets := fetchMarker(t, lib, photos)

	t.Run("fields", func(t *testing.T) {
		for _, c := range []struct {
			field string
			set   func(*photoinfo.PhotoInfo) bool
			same  func(photokit.Asset, *photoinfo.PhotoInfo) bool
		}{
			{"OriginalFilename", func(p *photoinfo.PhotoInfo) bool { return p.OriginalFilename != "" }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.OriginalFilename == p.OriginalFilename }},
			{"IsPhoto", func(p *photoinfo.PhotoInfo) bool { return p.IsPhoto }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.IsPhoto == p.IsPhoto }},
			{"IsMovie", func(p *photoinfo.PhotoInfo) bool { return p.IsMovie }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.IsMovie == p.IsMovie }},
			{"Live", func(p *photoinfo.PhotoInfo) bool { return p.Live }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.Live == p.Live }},
			{"Favorite", func(p *photoinfo.PhotoInfo) bool { return p.Favorite }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.Favorite == p.Favorite }},
			{"Hidden", func(p *photoinfo.PhotoInfo) bool { return p.Hidden }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.Hidden == p.Hidden }},
			{"PixelWidth", func(p *photoinfo.PhotoInfo) bool { return p.Width != 0 }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.PixelWidth == p.Width }},
			{"PixelHeight", func(p *photoinfo.PhotoInfo) bool { return p.Height != 0 }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.PixelHeight == p.Height }},
			{"Date", func(p *photoinfo.PhotoInfo) bool { return !p.Date.IsZero() }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool {
				return a.Date != nil && withinSecond(*a.Date, p.Date)
			}},
			// PHAsset.modificationDate is ZMODIFICATIONDATE; photosdb DateModified is the time of the last edit.
			{"DateModified", func(p *photoinfo.PhotoInfo) bool { return p.DateModifiedRaw != nil }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool {
				return (a.DateModified == nil) == (p.DateModifiedRaw == nil) &&
					(a.DateModified == nil || withinSecond(*a.DateModified, *p.DateModifiedRaw))
			}},
			{"BurstID", func(p *photoinfo.PhotoInfo) bool { return p.BurstUUID != "" }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.BurstID == p.BurstUUID }},
			{"HasAdjustments", func(p *photoinfo.PhotoInfo) bool { return p.HasAdjustments }, func(a photokit.Asset, p *photoinfo.PhotoInfo) bool { return a.HasAdjustments == p.HasAdjustments }},
		} {
			// A field that is zero for every photo would agree with any asset.
			t.Logf("%s: set in photosdb for %d of %d photos", c.field, count(photos, c.set), len(photos))
			if n := count(photos, func(p *photoinfo.PhotoInfo) bool { return !c.same(assets[p.UUID], p) }); n > 0 {
				t.Errorf("%s differs from photosdb for %d of %d photos", c.field, n, len(photos))
			}
		}
	})

	t.Run("location", func(t *testing.T) {
		located := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.Latitude != nil && p.Longitude != nil })
		if len(located) == 0 {
			skipNone(t, "photo with a location")
		}
		n := count(located, func(p *photoinfo.PhotoInfo) bool {
			l := assets[p.UUID].Location
			return l == nil || math.Abs(l.Latitude-*p.Latitude) > 1e-5 || math.Abs(l.Longitude-*p.Longitude) > 1e-5
		})
		if n > 0 {
			t.Errorf("Location differs from photosdb for %d of %d photos with a location", n, len(located))
		}
	})

	t.Run("video duration", func(t *testing.T) {
		// ExifInfo describes the original file; an edit can change the duration PhotoKit gives.
		videos := filter(photos, func(p *photoinfo.PhotoInfo) bool {
			return p.IsMovie && !p.HasAdjustments && p.ExifInfo.Duration != nil
		})
		if len(videos) == 0 {
			skipNone(t, "unedited video with a duration")
		}
		n := count(videos, func(p *photoinfo.PhotoInfo) bool {
			want := time.Duration(*p.ExifInfo.Duration * float64(time.Second))
			return (assets[p.UUID].Duration - want).Abs() >= 10*time.Millisecond
		})
		if n > 0 {
			t.Errorf("Duration differs from photosdb by 10 ms or more for %d of %d videos", n, len(videos))
		}
	})

	t.Run("hidden", func(t *testing.T) {
		hidden := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.Hidden })
		if len(hidden) == 0 {
			skipNone(t, "hidden photo")
		}
		if n := count(hidden, func(p *photoinfo.PhotoInfo) bool { return !assets[p.UUID].Hidden }); n > 0 {
			t.Errorf("%d of %d hidden photos found but not hidden", n, len(hidden))
		}
	})

	t.Run("RAW", func(t *testing.T) {
		raws := filter(photos, func(p *photoinfo.PhotoInfo) bool { return p.HasRAW })
		if len(raws) == 0 {
			skipNone(t, "RAW+JPEG photo")
		}
		n := count(raws, func(p *photoinfo.PhotoInfo) bool {
			a := assets[p.UUID]
			return a.UTIRaw == "" || a.RAWFilename == ""
		})
		if n > 0 {
			t.Errorf("%d of %d RAW+JPEG photos have no UTIRaw or RAWFilename", n, len(raws))
		}
	})

	t.Run("resources", func(t *testing.T) {
		a := assets[photos[0].UUID]
		res, err := lib.Resources(t.Context(), photos[0].UUID)
		if err != nil {
			t.Fatalf("Resources: %s", failure(err))
		}
		want := photokit.ResourceVideo
		if a.IsPhoto {
			want = photokit.ResourcePhoto
		}
		if !slices.ContainsFunc(res, func(r photokit.Resource) bool { return r.Type == want && r.OriginalFilename == a.OriginalFilename }) {
			t.Errorf("none of %d resources is of type %d with the OriginalFilename of the asset", len(res), want)
		}
	})

	bursts := fetchBursts(t, lib, photos)
	t.Run("burst", func(t *testing.T) {
		if len(bursts) == 0 {
			skipNone(t, "burst photo")
		}
		for _, b := range bursts {
			if b.all < b.frames || b.picked > b.frames {
				t.Errorf("a burst of %d frames in photosdb: FetchBurst found %d with all, %d without", b.frames, b.all, b.picked)
			}
		}
	})
	t.Run("burst with an unselected frame", func(t *testing.T) {
		unpicked := filter(bursts, func(b burst) bool { return b.unpicked })
		if len(unpicked) == 0 {
			skipNone(t, "burst with an unselected frame")
		}
		for _, b := range unpicked {
			if b.picked >= b.all {
				t.Errorf("a burst with an unselected frame: FetchBurst found %d with all, %d without; want fewer without", b.all, b.picked)
			}
		}
	})
}

func TestLiveFetchNotFound(t *testing.T) {
	lib := openLibrary(t)
	const none = "00000000-0000-0000-0000-000000000000"
	if _, err := lib.Fetch(t.Context(), []string{none}); !errors.Is(err, photokit.ErrNotFound) {
		t.Error("Fetch of an unknown UUID: not ErrNotFound")
	}
	if _, err := lib.FetchBurst(t.Context(), "nope", true); !errors.Is(err, photokit.ErrNotFound) {
		t.Error("FetchBurst of an unknown burst: not ErrNotFound")
	}
	if _, err := lib.Resources(t.Context(), none); !errors.Is(err, photokit.ErrNotFound) {
		t.Error("Resources of an unknown UUID: not ErrNotFound")
	}
	if _, err := lib.Resources(t.Context(), ""); !errors.Is(err, photokit.ErrNotFound) {
		t.Error("Resources of an empty UUID: not ErrNotFound")
	}
}

func openLibrary(t *testing.T) *photokit.Library {
	t.Helper()
	lib, err := photokit.Open()
	if err != nil {
		t.Fatalf("Open: %s", failure(err))
	}
	return lib
}

// The error of PhotoKit may hold the text of an NSException, which can name an
// identifier: only its class is printed.
func failure(err error) string {
	return fmt.Sprintf("failed (ErrNotFound: %t, ErrMediaType: %t, ErrNotAuthorized: %t)",
		errors.Is(err, photokit.ErrNotFound), errors.Is(err, photokit.ErrMediaType), errors.Is(err, photokit.ErrNotAuthorized))
}

func skipNone(t *testing.T, kind string) {
	t.Helper()
	t.Skipf("no %s among the first %d untrashed photos of the marker album", kind, maxFetched)
}

// PhotoKit does not find an asset in the trash.
func untrashedMarkerPhotos(t *testing.T) []*photoinfo.PhotoInfo {
	t.Helper()
	photos := filter(markerPhotos, func(p *photoinfo.PhotoInfo) bool { return !p.InTrash })
	if len(photos) == 0 {
		t.Skip("no photo outside the trash in the marker album")
	}
	return photos[:min(len(photos), maxFetched)]
}

const maxFetched = 20

// The plain UUIDs of photosdb, without the suffix of a LocalIdentifier, are
// what Fetch must find.
func fetchMarker(t *testing.T, lib *photokit.Library, photos []*photoinfo.PhotoInfo) map[string]photokit.Asset {
	t.Helper()
	uuids := make([]string, len(photos))
	for i, p := range photos {
		uuids[i] = p.UUID
	}
	found, err := lib.Fetch(t.Context(), uuids)
	if err != nil {
		t.Fatalf("Fetch of %d photos: %s", len(uuids), failure(err))
	}
	assets := make(map[string]photokit.Asset, len(found))
	for _, a := range found {
		uuid, _, _ := strings.Cut(a.LocalIdentifier, "/")
		assets[uuid] = a
	}
	missing := count(photos, func(p *photoinfo.PhotoInfo) bool {
		_, ok := assets[p.UUID]
		return !ok
	})
	if len(found) != len(uuids) || missing > 0 {
		t.Fatalf("Fetch of %d photos found %d assets, %d photos without an asset of their UUID", len(uuids), len(found), missing)
	}
	return assets
}

type burst struct {
	burstFrames
	all, picked int
}

func fetchBursts(t *testing.T, lib *photokit.Library, photos []*photoinfo.PhotoInfo) []burst {
	t.Helper()
	var bursts []burst
	seen := map[string]bool{}
	for _, p := range photos {
		if !p.Burst || seen[p.BurstUUID] {
			continue
		}
		seen[p.BurstUUID] = true
		b := burst{burstFrames: markerBursts[p.UUID]}
		all, err := lib.FetchBurst(t.Context(), p.BurstUUID, true)
		if err != nil {
			t.Fatalf("FetchBurst with all: %s", failure(err))
		}
		picked, err := lib.FetchBurst(t.Context(), p.BurstUUID, false)
		if err != nil {
			t.Fatalf("FetchBurst without all: %s", failure(err))
		}
		b.all, b.picked = len(all), len(picked)
		bursts = append(bursts, b)
	}
	return bursts
}

func withinSecond(a, b time.Time) bool { return a.Sub(b).Abs() < time.Second }

func filter[T any](s []T, keep func(T) bool) []T {
	var out []T
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func count[T any](s []T, match func(T) bool) int { return len(filter(s, match)) }
