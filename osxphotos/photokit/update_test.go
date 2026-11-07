package photokit

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge/fake"
)

func TestUpdate(t *testing.T) {
	asset := liveID + "/L0/001"
	date := time.Date(2001, 2, 3, 4, 5, 6, 250_500_000, time.UTC)
	for _, tt := range []struct {
		name string
		p    AssetPatch
		want bridge.PatchRecord
		call string
	}{
		{
			"all",
			AssetPatch{CreationDate: new(date), Favorite: new(true), Hidden: new(false), Location: &Location{Latitude: 52.5, Longitude: 13.4}},
			bridge.PatchRecord{HasCreationDate: true, CreationDate: 981173106.2505, HasFavorite: true, Favorite: true, HasHidden: true, HasLocation: true, Latitude: 52.5, Longitude: 13.4},
			"date favorite hidden location",
		},
		{"date", AssetPatch{CreationDate: new(date)}, bridge.PatchRecord{HasCreationDate: true, CreationDate: 981173106.2505}, "date"},
		{"Favorite false", AssetPatch{Favorite: new(false)}, bridge.PatchRecord{HasFavorite: true}, "favorite"},
		{"Hidden true", AssetPatch{Hidden: new(true)}, bridge.PatchRecord{HasHidden: true, Hidden: true}, "hidden"},
		{"location 0, 0", AssetPatch{Location: &Location{}}, bridge.PatchRecord{HasLocation: true}, "location"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &fake.Bridge{}
			if err := (&Library{b}).Update(t.Context(), asset, tt.p); err != nil {
				t.Fatal(err)
			}
			wantCalls(t, b, "UpdateAsset "+asset+" "+tt.call)
			if got := b.LastPatch(); got != tt.want {
				t.Errorf("record = %+v, want %+v", got, tt.want)
			}
		})
	}

	t.Run("empty patch", func(t *testing.T) {
		b := &fake.Bridge{ChangeErr: bridge.ErrNoAsset}
		for _, id := range []string{asset, ""} {
			if err := (&Library{b}).Update(t.Context(), id, AssetPatch{}); err != nil {
				t.Errorf("Update(%q) of an empty patch = %v, want nil", id, err)
			}
		}
		wantCalls(t, b)
	})

	for _, id := range badIdentifiers {
		t.Run("id "+strconv.Quote(id), func(t *testing.T) {
			b := &fake.Bridge{}
			err := (&Library{b}).Update(t.Context(), id, AssetPatch{Favorite: new(true)})
			wantCollectionErr(t, err, []error{ErrNotFound}, []error{ErrChange}, ": identifier", collectionTop)
			wantCalls(t, b)
		})
	}

	for _, tt := range []struct {
		name      string
		err       error
		want, not []error
		text      string
	}{
		{"no asset", bridge.ErrNoAsset, []error{ErrNotFound}, []error{ErrChange, bridge.ErrNoAsset}, ": asset"},
		{"user canceled", &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3072}, []error{ErrUserCanceled}, []error{ErrChange, ErrNotFound}, ""},
		{"PhotoKit error", &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3300}, []error{ErrChange}, []error{ErrNotFound, ErrUserCanceled}, "PHPhotosErrorDomain code 3300"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &fake.Bridge{ChangeErr: tt.err}
			err := (&Library{b}).Update(t.Context(), asset, AssetPatch{Hidden: new(true)})
			wantCollectionErr(t, err, tt.want, tt.not, tt.text, liveID)
			wantCalls(t, b, "UpdateAsset "+asset+" hidden")
		})
	}

	t.Run("timeout of the bridge", func(t *testing.T) {
		b := &fake.Bridge{}
		if err := (&Library{b}).Update(t.Context(), asset, AssetPatch{Favorite: new(true)}); err != nil {
			t.Fatal(err)
		}
		if d := b.LastTimeout(); d != time.Duration(math.MaxInt64) {
			t.Errorf("timeout = %v without a deadline, want math.MaxInt64: 0 would not wait at all", d)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
		defer cancel()
		if err := (&Library{b}).Update(ctx, asset, AssetPatch{Favorite: new(true)}); err != nil {
			t.Fatal(err)
		}
		if d := b.LastTimeout(); d <= 59*time.Minute || d > time.Hour {
			t.Errorf("timeout = %v, want the time left to the 1h deadline", d)
		}
	})

	t.Run("canceled before the call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		b := &fake.Bridge{}
		err := (&Library{b}).Update(ctx, asset, AssetPatch{Favorite: new(true)})
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrOutcomeUnknown) {
			t.Errorf("err = %v, want context.Canceled: nothing was changed", err)
		}
		// A bridge call started by mistake logs from its goroutine after the return.
		time.Sleep(20 * time.Millisecond)
		wantCalls(t, b)
	})

	t.Run("deadline before the outcome", func(t *testing.T) {
		b := &fake.Bridge{HoldChange: make(chan struct{})}
		t.Cleanup(func() { close(b.HoldChange) })
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := (&Library{b}).Update(ctx, asset, AssetPatch{Hidden: new(true)})
		wantCollectionErr(t, err, []error{ErrOutcomeUnknown, context.DeadlineExceeded}, []error{ErrChange}, "", liveID)
		if d := time.Since(start); d > time.Second {
			t.Errorf("Update returned after %v, want it at the deadline", d)
		}
	})

	// The bridge would answer at the 2 s deadline: Update must not wait for it.
	t.Run("canceled before the outcome", func(t *testing.T) {
		b := &fake.Bridge{HoldChange: make(chan struct{})}
		t.Cleanup(func() { close(b.HoldChange) })
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		time.AfterFunc(10*time.Millisecond, cancel)
		start := time.Now()
		err := (&Library{b}).Update(ctx, asset, AssetPatch{Hidden: new(true)})
		wantCollectionErr(t, err, []error{ErrOutcomeUnknown, context.Canceled}, []error{ErrChange}, "", liveID)
		if d := time.Since(start); d > time.Second {
			t.Errorf("Update returned after %v, want it at the cancel", d)
		}
	})
}
