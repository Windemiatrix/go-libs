package photokit

import (
	"context"
	"fmt"
	"slices"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// Fetch returns the assets of uuids that the library has, hidden ones
// included, in the order of PhotoKit (fetch_uuid_list, photokit.py:1219-1246;
// fetch_uuid is Fetch with one uuid, 1248-1264). A uuid is the UUID of
// photosdb or a LocalIdentifier. A uuid the library does not have is left
// out; no uuid, or none found, is ErrNotFound. An asset that is neither an
// image, a video nor a Live Photo fails the whole fetch with ErrMediaType.
// ctx is checked only before the first call: PhotoKit answers the fetch and
// the resources of each asset at once and cannot be interrupted.
func (l *Library) Fetch(ctx context.Context, uuids []string) ([]Asset, error) {
	return l.fetch(ctx, len(uuids) > 0, func() ([]bridge.AssetRecord, error) { return l.b.FetchAssets(uuids) })
}

// FetchBurst returns the assets of the burst burstID, hidden ones included:
// with all, every frame; without, the frames the user selected and the key
// photo (fetch_burst_uuid, photokit.py:1266-1294). An empty burstID, or no
// such burst, is ErrNotFound; ErrMediaType and ctx are as for Fetch.
func (l *Library) FetchBurst(ctx context.Context, burstID string, all bool) ([]Asset, error) {
	return l.fetch(ctx, burstID != "", func() ([]bridge.AssetRecord, error) { return l.b.FetchBurst(burstID, all) })
}

// Resources returns the resources of the asset uuid in the order of PhotoKit
// (_resources, photokit.py:769-772). uuid and ctx are as for Fetch; no such
// asset is ErrNotFound.
func (l *Library) Resources(ctx context.Context, uuid string) ([]Resource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	records, err := l.b.AssetResources(uuid)
	if err != nil {
		return nil, err
	}
	// PhotoKit gives every asset at least one resource, so none is no asset.
	if len(records) == 0 {
		return nil, ErrNotFound
	}
	out := make([]Resource, len(records))
	for i, r := range records {
		out[i] = Resource{ResourceType(r.Type), r.OriginalFilename, r.UTI}
	}
	return out, nil
}

// Without an identifier upstream asks PhotoKit all the same and fails on the
// empty result; the call is not made here.
func (l *Library) fetch(ctx context.Context, given bool, call func() ([]bridge.AssetRecord, error)) ([]Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !given {
		return nil, ErrNotFound
	}
	records, err := call()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrNotFound
	}
	out := make([]Asset, len(records))
	for i, r := range records {
		if out[i], err = asset(r); err != nil {
			return nil, err
		}
		var res []bridge.ResourceRecord
		if res, err = l.b.AssetResources(r.LocalIdentifier); err != nil {
			return nil, err
		}
		withResources(&out[i], res)
	}
	return out, nil
}

// Upstream returns the first resource of a type (photokit.py:310-334,
// 484-493): walked backward, an earlier one overwrites a later one.
func withResources(a *Asset, res []bridge.ResourceRecord) {
	original := ResourceVideo
	if a.IsPhoto {
		original = ResourcePhoto
	}
	for _, r := range slices.Backward(res) {
		switch t := ResourceType(r.Type); {
		case t == original:
			a.OriginalFilename = r.OriginalFilename
		case t == ResourceAlternatePhoto && a.IsPhoto:
			a.RAWFilename, a.UTIRaw = r.OriginalFilename, r.UTI
		case t == ResourceAdjustmentData:
			a.HasAdjustments = true
		}
	}
}

// _asset_factory looks at the Live bit before the media type
// (photokit.py:1309-1319).
func asset(r bridge.AssetRecord) (Asset, error) {
	t, s := MediaType(r.MediaType), r.MediaSubtypes
	if s&subtypeLive == 0 && t != MediaTypeImage && t != MediaTypeVideo {
		return Asset{}, fmt.Errorf("%w: %d", ErrMediaType, r.MediaType)
	}
	a := Asset{
		LocalIdentifier: r.LocalIdentifier,
		MediaType:       t,
		Subtypes:        s,
		IsPhoto:         t == MediaTypeImage,
		IsMovie:         t == MediaTypeVideo,
		IsAudio:         t == MediaTypeAudio,
		Panorama:        s&subtypePanorama != 0,
		HDR:             s&subtypeHDR != 0,
		Screenshot:      s&subtypeScreenshot != 0,
		ScreenRecording: s&subtypeScreenRecording != 0,
		Live:            s&subtypeLive != 0,
		Streamed:        s&subtypeStreamed != 0,
		SlowMo:          s&subtypeHighFrameRate != 0,
		TimeLapse:       s&subtypeTimelapse != 0,
		Portrait:        s&subtypeDepthEffect != 0,
		BurstID:         r.BurstIdentifier,
		Burst:           r.BurstIdentifier != "",
		SourceType:      r.SourceType,
		PixelWidth:      r.PixelWidth,
		PixelHeight:     r.PixelHeight,
		Date:            r.CreationDate,
		DateModified:    r.ModificationDate,
		Duration:        r.Duration,
		Favorite:        r.Favorite,
		Hidden:          r.Hidden,
	}
	if r.Latitude != nil && r.Longitude != nil {
		a.Location = &Location{Latitude: *r.Latitude, Longitude: *r.Longitude}
	}
	return a, nil
}
