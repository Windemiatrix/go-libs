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
	editedID  = "55555555-5555-5555-5555-555555555555"
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
			want := []string{"FetchAssets " + strings.Join(tt.ids, " ")}
			for _, a := range tt.want {
				want = append(want, "AssetResources "+a.LocalIdentifier)
			}
			if !slices.Equal(b.Calls(), want) {
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
		if want := []string{fmt.Sprintf("FetchBurst %s %t", burstID, all), "AssetResources " + wantFrame.LocalIdentifier}; !slices.Equal(b.Calls(), want) {
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

// Upstream takes the first resource of a type: the second Photo, Video and
// AlternatePhoto resources here must not win (photokit.py:310-352, 484-493).
func TestResources(t *testing.T) {
	edited := bridge.AssetRecord{LocalIdentifier: editedID + "/L0/001", MediaType: 1}
	b := &fake.Bridge{
		Assets: []bridge.AssetRecord{livePhoto, slowMo, frame, edited},
		Resources: map[string][]bridge.ResourceRecord{
			livePhoto.LocalIdentifier: {
				{Type: 5, OriginalFilename: "FullSizeRender.heic", UTI: "public.heic"},
				{Type: 1, OriginalFilename: "IMG_0001.JPG", UTI: "public.jpeg"},
				{Type: 4, OriginalFilename: "IMG_0001.CR2", UTI: "com.canon.cr2-raw-image"},
				{Type: 1, OriginalFilename: "IMG_0002.JPG", UTI: "public.jpeg"},
				{Type: 4, OriginalFilename: "IMG_0002.DNG", UTI: "com.adobe.raw-image"},
				{Type: 7, OriginalFilename: "Adjustments.plist", UTI: "com.apple.property-list"},
				{Type: 13},
			},
			slowMo.LocalIdentifier: {
				{Type: 1, OriginalFilename: "IMG_0003.JPG", UTI: "public.jpeg"},
				{Type: 2, OriginalFilename: "IMG_0003.MOV", UTI: "com.apple.quicktime-movie"},
				{Type: 4, OriginalFilename: "IMG_0003.CR2", UTI: "com.canon.cr2-raw-image"},
				{Type: 2, OriginalFilename: "IMG_0004.MOV", UTI: "com.apple.quicktime-movie"},
				{Type: 7, OriginalFilename: "Adjustments.plist", UTI: "com.apple.property-list"},
			},
			edited.LocalIdentifier: {{Type: 7, OriginalFilename: "Adjustments.plist", UTI: "com.apple.property-list"}},
		},
	}
	lib := &Library{b}

	photo := wantLive
	photo.OriginalFilename, photo.RAWFilename, photo.UTIRaw, photo.HasAdjustments = "IMG_0001.JPG", "IMG_0001.CR2", "com.canon.cr2-raw-image", true
	video := wantSlowMo
	video.OriginalFilename, video.HasAdjustments = "IMG_0003.MOV", true
	wantEdited := Asset{LocalIdentifier: editedID + "/L0/001", MediaType: MediaTypeImage, IsPhoto: true, HasAdjustments: true}
	want := []Asset{photo, video, wantFrame, wantEdited}
	got, err := lib.Fetch(t.Context(), []string{liveID, slowMoID, frameID, editedID})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Fetch = %+v, %v; want %+v", got, err, want)
	}

	// original_filename takes the Video resource of anything but a photo, and
	// hasadjustments only AdjustmentData (photokit.py:310-322, 336-352).
	for _, tt := range []struct {
		r    bridge.AssetRecord
		res  []bridge.ResourceRecord
		want Asset
	}{
		{bridge.AssetRecord{MediaSubtypes: 1 << 3}, []bridge.ResourceRecord{{Type: 1, OriginalFilename: "A"}, {Type: 2, OriginalFilename: "B"}}, Asset{Subtypes: 1 << 3, Live: true, OriginalFilename: "B"}},
		{bridge.AssetRecord{MediaType: 1}, []bridge.ResourceRecord{{Type: 5}, {Type: 8}}, Asset{MediaType: MediaTypeImage, IsPhoto: true}},
	} {
		a, err := asset(tt.r)
		withResources(&a, tt.res)
		if err != nil || !reflect.DeepEqual(a, tt.want) {
			t.Errorf("asset %+v with resources %+v = %+v, %v; want %+v", tt.r, tt.res, a, err, tt.want)
		}
	}

	wantResources := []Resource{
		{ResourceFullSizePhoto, "FullSizeRender.heic", "public.heic"},
		{ResourcePhoto, "IMG_0001.JPG", "public.jpeg"},
		{ResourceAlternatePhoto, "IMG_0001.CR2", "com.canon.cr2-raw-image"},
		{ResourcePhoto, "IMG_0002.JPG", "public.jpeg"},
		{ResourceAlternatePhoto, "IMG_0002.DNG", "com.adobe.raw-image"},
		{ResourceAdjustmentData, "Adjustments.plist", "com.apple.property-list"},
		{Type: 13},
	}
	if res, err := lib.Resources(t.Context(), livePhoto.LocalIdentifier); err != nil || !reflect.DeepEqual(res, wantResources) {
		t.Errorf("Resources = %+v, %v; want %+v", res, err, wantResources)
	}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	bridgeErr := errors.New("bridge failed")
	for _, tt := range []struct {
		name  string
		ctx   context.Context
		b     *fake.Bridge
		want  error
		calls int
	}{
		{"no such asset", t.Context(), &fake.Bridge{}, ErrNotFound, 1},
		{"no resource", t.Context(), &fake.Bridge{Resources: map[string][]bridge.ResourceRecord{unknownID: {}}}, ErrNotFound, 1},
		{"canceled", canceled, &fake.Bridge{}, context.Canceled, 0},
		{"bridge error", t.Context(), &fake.Bridge{ResourcesErr: bridgeErr}, bridgeErr, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res, err := (&Library{tt.b}).Resources(tt.ctx, unknownID)
			if res != nil || !errors.Is(err, tt.want) {
				t.Errorf("Resources = %v, %v; want %v", res, err, tt.want)
			}
			if calls := tt.b.Calls(); len(calls) != tt.calls {
				t.Errorf("calls = %q, want %d", calls, tt.calls)
			}
		})
	}

	failing := &fake.Bridge{Assets: []bridge.AssetRecord{livePhoto}, ResourcesErr: bridgeErr}
	if got, err := (&Library{failing}).Fetch(t.Context(), []string{liveID}); got != nil || !errors.Is(err, bridgeErr) {
		t.Errorf("Fetch with a failing AssetResources = %v, %v; want %v", got, err, bridgeErr)
	}
}
