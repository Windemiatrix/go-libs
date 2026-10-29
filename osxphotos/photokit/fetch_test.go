package photokit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge/fake"
)

const (
	liveID    = "11111111-1111-1111-1111-111111111111"
	slowMoID  = "22222222-2222-2222-2222-222222222222"
	frameID   = "33333333-3333-3333-3333-333333333333"
	burstID   = "44444444-4444-4444-4444-444444444444"
	unknownID = "99999999-9999-9999-9999-999999999999"
)

var (
	created  = time.Date(2024, 5, 1, 12, 30, 0, 0, time.UTC)
	modified = time.Date(2024, 5, 2, 8, 0, 0, 0, time.UTC)

	livePhoto = bridge.AssetRecord{
		LocalIdentifier:  liveID + "/L0/001",
		MediaType:        1,
		MediaSubtypes:    1 << 3,
		SourceType:       1,
		PixelWidth:       4032,
		PixelHeight:      3024,
		CreationDate:     new(created),
		ModificationDate: new(modified),
		Latitude:         new(52.5),
		Longitude:        new(13.4),
		Favorite:         true,
	}
	wantLive = Asset{
		LocalIdentifier: liveID + "/L0/001",
		MediaType:       MediaTypeImage,
		Subtypes:        1 << 3,
		IsPhoto:         true,
		Live:            true,
		SourceType:      1,
		PixelWidth:      4032,
		PixelHeight:     3024,
		Date:            new(created),
		DateModified:    new(modified),
		Location:        &Location{Latitude: 52.5, Longitude: 13.4},
		Favorite:        true,
	}

	slowMo = bridge.AssetRecord{
		LocalIdentifier: slowMoID + "/L0/001",
		MediaType:       2,
		MediaSubtypes:   1 << 17,
		SourceType:      1,
		PixelWidth:      1920,
		PixelHeight:     1080,
		Duration:        3 * time.Second,
		Hidden:          true,
	}
	wantSlowMo = Asset{
		LocalIdentifier: slowMoID + "/L0/001",
		MediaType:       MediaTypeVideo,
		Subtypes:        1 << 17,
		IsMovie:         true,
		SlowMo:          true,
		SourceType:      1,
		PixelWidth:      1920,
		PixelHeight:     1080,
		Duration:        3 * time.Second,
		Hidden:          true,
	}

	frame     = bridge.AssetRecord{LocalIdentifier: frameID + "/L0/001", MediaType: 1, BurstIdentifier: burstID}
	wantFrame = Asset{LocalIdentifier: frameID + "/L0/001", MediaType: MediaTypeImage, IsPhoto: true, BurstID: burstID, Burst: true}
)

func newFake() *fake.Bridge {
	return &fake.Bridge{Assets: []bridge.AssetRecord{livePhoto, slowMo, frame}}
}

func TestFetch(t *testing.T) {
	for _, tt := range []struct {
		name string
		ids  []string
		want []Asset
	}{
		{"in the order of PhotoKit", []string{slowMoID, liveID}, []Asset{wantLive, wantSlowMo}},
		{"one identifier", []string{slowMoID}, []Asset{wantSlowMo}},
		{"an unknown one left out", []string{unknownID, liveID}, []Asset{wantLive}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newFake()
			got, err := (&Library{b}).Fetch(t.Context(), tt.ids)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Fetch(%q) = %+v, %v; want %+v", tt.ids, got, err, tt.want)
			}
			if want := []string{"FetchAssets " + strings.Join(tt.ids, " ")}; !slices.Equal(b.Calls(), want) {
				t.Errorf("calls = %q, want %q", b.Calls(), want)
			}
		})
	}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	bridgeErr := errors.New("bridge failed")
	for _, tt := range []struct {
		name  string
		ctx   context.Context
		b     *fake.Bridge
		ids   []string
		want  error
		calls int
	}{
		{"none found", t.Context(), newFake(), []string{unknownID}, ErrNotFound, 1},
		{"no identifier", t.Context(), newFake(), nil, ErrNotFound, 0},
		{"canceled", canceled, newFake(), []string{liveID}, context.Canceled, 0},
		{"canceled, no identifier", canceled, newFake(), nil, context.Canceled, 0},
		{"bridge error", t.Context(), &fake.Bridge{FetchErr: bridgeErr}, []string{liveID}, bridgeErr, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&Library{tt.b}).Fetch(tt.ctx, tt.ids)
			if got != nil || !errors.Is(err, tt.want) {
				t.Errorf("Fetch = %v, %v; want %v", got, err, tt.want)
			}
			if calls := tt.b.Calls(); len(calls) != tt.calls {
				t.Errorf("calls = %q, want %d", calls, tt.calls)
			}
		})
	}

	// _asset_factory raises for any other media type, audio included
	// (photokit.py:1309-1319).
	for _, mt := range []int{0, 3, 4} {
		other := bridge.AssetRecord{LocalIdentifier: frameID + "/L0/001", MediaType: mt}
		b := &fake.Bridge{Assets: []bridge.AssetRecord{livePhoto, other}}
		got, err := (&Library{b}).Fetch(t.Context(), []string{liveID, frameID})
		if got != nil || !errors.Is(err, ErrMediaType) || !strings.HasSuffix(err.Error(), fmt.Sprintf(": %d", mt)) {
			t.Errorf("Fetch with media type %d = %v, %v; want ErrMediaType with the number", mt, got, err)
		}
	}
}

// A Live Photo is taken before its media type is looked at
// (photokit.py:1312-1313); the media type stays as PhotoKit gives it.
func TestFetchLivePhotoOfAnyMediaType(t *testing.T) {
	for _, tt := range []struct {
		raw  int
		want Asset
	}{
		{0, Asset{Subtypes: 1 << 3, Live: true}},
		{3, Asset{MediaType: MediaTypeAudio, IsAudio: true, Subtypes: 1 << 3, Live: true}},
		{7, Asset{MediaType: 7, Subtypes: 1 << 3, Live: true}},
	} {
		got, err := asset(bridge.AssetRecord{MediaType: tt.raw, MediaSubtypes: 1 << 3})
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("asset(media type %d, Live) = %+v, %v; want %+v", tt.raw, got, err, tt.want)
		}
	}
}

func TestAssetHalfLocation(t *testing.T) {
	for _, r := range []bridge.AssetRecord{{MediaType: 1, Latitude: new(1.0)}, {MediaType: 1, Longitude: new(1.0)}} {
		if a, err := asset(r); err != nil || a.Location != nil {
			t.Errorf("asset(%+v) = location %+v, %v; want no location", r, a.Location, err)
		}
	}
}

func TestFetchBurst(t *testing.T) {
	for _, all := range []bool{true, false} {
		b := newFake()
		got, err := (&Library{b}).FetchBurst(t.Context(), burstID, all)
		if err != nil || !reflect.DeepEqual(got, []Asset{wantFrame}) {
			t.Errorf("FetchBurst(%t) = %+v, %v; want %+v", all, got, err, wantFrame)
		}
		if want := []string{fmt.Sprintf("FetchBurst %s %t", burstID, all)}; !slices.Equal(b.Calls(), want) {
			t.Errorf("calls = %q, want %q", b.Calls(), want)
		}
	}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	bridgeErr := errors.New("bridge failed")
	for _, tt := range []struct {
		name  string
		ctx   context.Context
		b     *fake.Bridge
		id    string
		want  error
		calls int
	}{
		{"no such burst", t.Context(), newFake(), unknownID, ErrNotFound, 1},
		{"no identifier", t.Context(), newFake(), "", ErrNotFound, 0},
		{"canceled", canceled, newFake(), burstID, context.Canceled, 0},
		{"canceled, no identifier", canceled, newFake(), "", context.Canceled, 0},
		{"bridge error", t.Context(), &fake.Bridge{FetchErr: bridgeErr}, burstID, bridgeErr, 1},
		{"media type", t.Context(), &fake.Bridge{Assets: []bridge.AssetRecord{{MediaType: 3, BurstIdentifier: burstID}}}, burstID, ErrMediaType, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&Library{tt.b}).FetchBurst(tt.ctx, tt.id, true)
			if got != nil || !errors.Is(err, tt.want) {
				t.Errorf("FetchBurst = %v, %v; want %v", got, err, tt.want)
			}
			if calls := tt.b.Calls(); len(calls) != tt.calls {
				t.Errorf("calls = %q, want %d", calls, tt.calls)
			}
		})
	}
}

// The bits are those of PHAssetMediaSubtype (photokit.py:364-409).
func TestAssetFlags(t *testing.T) {
	names := []string{"Panorama", "HDR", "Screenshot", "Live", "Portrait", "Streamed", "SlowMo", "TimeLapse", "ScreenRecording"}
	flags := func(a Asset) []string {
		var on []string
		for i, set := range []bool{a.Panorama, a.HDR, a.Screenshot, a.Live, a.Portrait, a.Streamed, a.SlowMo, a.TimeLapse, a.ScreenRecording} {
			if set {
				on = append(on, names[i])
			}
		}
		return on
	}
	const all = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<16 | 1<<17 | 1<<18 | 1<<19
	for _, tt := range []struct {
		bits uint64
		want []string
	}{
		{0, nil},
		{1 << 0, []string{"Panorama"}},
		{1 << 1, []string{"HDR"}},
		{1 << 2, []string{"Screenshot"}},
		{1 << 3, []string{"Live"}},
		{1 << 4, []string{"Portrait"}},
		{1 << 16, []string{"Streamed"}},
		{1 << 17, []string{"SlowMo"}},
		{1 << 18, []string{"TimeLapse"}},
		{1 << 19, []string{"ScreenRecording"}},
		{all, names},
		{1<<5 | 1<<10 | 1<<21, nil},
	} {
		a, err := asset(bridge.AssetRecord{MediaType: 1, MediaSubtypes: tt.bits})
		if err != nil || a.Subtypes != tt.bits || !slices.Equal(flags(a), tt.want) {
			t.Errorf("asset(subtypes %#x) = flags %q, subtypes %#x, %v; want %q", tt.bits, flags(a), a.Subtypes, err, tt.want)
		}
	}
}
