package photokit

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge/fake"
)

const (
	collectionFolder = "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA/L0/040"
	collectionAlbum  = "BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB/L0/040"
	collectionTop    = "CCCCCCCC-CCCC-CCCC-CCCC-CCCCCCCCCCCC/L0/040"
)

// A bad value holds collectionTop, so that a test sees an error that names it.
var badIdentifiers = []string{"", collectionTop + "\x00b", collectionTop + "\xff"}

func wantCollectionErr(t *testing.T, err error, want, not []error, text string, secrets ...string) {
	t.Helper()
	for _, w := range want {
		if !errors.Is(err, w) {
			t.Errorf("err = %v, want %v", err, w)
		}
	}
	for _, n := range not {
		if errors.Is(err, n) {
			t.Errorf("err = %v, want it not %v", err, n)
		}
	}
	if err == nil || !strings.HasSuffix(err.Error(), text) {
		t.Errorf("err = %v, want it to end in %q", err, text)
		return
	}
	for _, s := range secrets {
		if strings.Contains(err.Error(), s) {
			t.Errorf("err = %v, want no %q in it", err, s)
		}
	}
}

func TestCollections(t *testing.T) {
	records := []bridge.CollectionRecord{
		{LocalIdentifier: collectionFolder, Title: "Trips", IsFolder: true},
		{LocalIdentifier: collectionAlbum, Title: "2024", Parent: collectionFolder},
		{LocalIdentifier: collectionTop, Title: "Holidays"},
	}
	want := []Collection{
		{LocalIdentifier: collectionFolder, Title: "Trips", IsFolder: true},
		{LocalIdentifier: collectionAlbum, Title: "2024", Parent: collectionFolder},
		{LocalIdentifier: collectionTop, Title: "Holidays"},
	}
	b := &fake.Bridge{CollectionRecords: records}
	got, err := (&Library{b}).Collections(t.Context())
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Collections = %+v, %v; want %+v", got, err, want)
	}
	wantCalls(t, b, "Collections")

	t.Run("none", func(t *testing.T) {
		got, err := (&Library{&fake.Bridge{}}).Collections(t.Context())
		if err != nil || got != nil {
			t.Errorf("Collections = %+v, %v; want nil and no error", got, err)
		}
	})

	t.Run("error of the bridge", func(t *testing.T) {
		bridgeErr := errors.New("bridge failed")
		got, err := (&Library{&fake.Bridge{CollectionsErr: bridgeErr}}).Collections(t.Context())
		if got != nil || !errors.Is(err, bridgeErr) {
			t.Errorf("Collections = %+v, %v; want the error of the bridge", got, err)
		}
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		b := &fake.Bridge{CollectionRecords: records}
		if got, err := (&Library{b}).Collections(ctx); got != nil || !errors.Is(err, context.Canceled) {
			t.Errorf("Collections = %+v, %v; want context.Canceled", got, err)
		}
		wantCalls(t, b)
	})
}

var creates = map[string]func(*Library, context.Context, string, string) (string, error){
	"CreateFolder": (*Library).CreateFolder,
	"CreateAlbum":  (*Library).CreateAlbum,
}

func TestCreateCollection(t *testing.T) {
	for _, tt := range []struct {
		create, title, parent, call string
	}{
		{"CreateFolder", "Trips", "", "CreateCollection Trips  folder=true"},
		{"CreateAlbum", "2024", collectionFolder, "CreateCollection 2024 " + collectionFolder + " folder=false"},
		{"CreateAlbum", "2024/05-01 - Holidays", "", "CreateCollection 2024/05-01 - Holidays  folder=false"},
		{"CreateFolder", "Trips", collectionFolder, "CreateCollection Trips " + collectionFolder + " folder=true"},
	} {
		t.Run(tt.call, func(t *testing.T) {
			b := &fake.Bridge{CreatedID: collectionTop}
			id, err := creates[tt.create](&Library{b}, t.Context(), tt.title, tt.parent)
			if err != nil || id != collectionTop {
				t.Fatalf("%s = %q, %v; want %q", tt.create, id, err, collectionTop)
			}
			wantCalls(t, b, tt.call)
		})
	}

	for name, create := range creates {
		for _, title := range badIdentifiers {
			t.Run(name+" title "+strconv.Quote(title), func(t *testing.T) {
				b := &fake.Bridge{CreatedID: collectionTop}
				id, err := create(&Library{b}, t.Context(), title, "")
				if id != "" || !errors.Is(err, ErrInvalidTitle) || err.Error() != ErrInvalidTitle.Error() {
					t.Errorf("%s = %q, %v; want ErrInvalidTitle and no title", name, id, err)
				}
				wantCalls(t, b)
			})
		}
		for _, parent := range badIdentifiers[1:] {
			t.Run(name+" parent "+strconv.Quote(parent), func(t *testing.T) {
				b := &fake.Bridge{CreatedID: collectionTop}
				id, err := create(&Library{b}, t.Context(), "Trips", parent)
				if id != "" {
					t.Errorf("id = %q, want none", id)
				}
				wantCollectionErr(t, err, []error{ErrNotFound}, nil, "identifier", "Trips", collectionTop)
				wantCalls(t, b)
			})
		}
	}

	for _, tt := range []struct {
		name      string
		id        string
		err       error
		want, not []error
		text      string
	}{
		{"no parent", collectionTop, bridge.ErrNoAlbum, []error{ErrNotFound}, []error{ErrChange}, "parent"},
		{"user canceled", "", &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3072}, []error{ErrUserCanceled}, []error{ErrChange}, ""},
		{"PhotoKit error", "", &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3300}, []error{ErrChange}, nil, "PHPhotosErrorDomain code 3300"},
		{"no identifier", "", nil, []error{ErrChange}, nil, "no identifier"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &fake.Bridge{CreatedID: tt.id, ChangeErr: tt.err}
			id, err := (&Library{b}).CreateAlbum(t.Context(), "2024", collectionFolder)
			if id != "" {
				t.Errorf("id = %q, want none", id)
			}
			wantCollectionErr(t, err, tt.want, tt.not, tt.text, "2024", collectionFolder)
			wantCalls(t, b, "CreateCollection 2024 "+collectionFolder+" folder=false")
		})
	}

	t.Run("canceled before the call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		b := &fake.Bridge{CreatedID: collectionTop}
		_, err := (&Library{b}).CreateFolder(ctx, "Trips", "")
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrOutcomeUnknown) {
			t.Errorf("err = %v, want context.Canceled: nothing was changed", err)
		}
		// A bridge call started by mistake logs from its goroutine after the return.
		time.Sleep(20 * time.Millisecond)
		wantCalls(t, b)
	})

	t.Run("deadline before the outcome", func(t *testing.T) {
		b := &fake.Bridge{CreatedID: collectionTop, HoldChange: make(chan struct{})}
		t.Cleanup(func() { close(b.HoldChange) })
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := (&Library{b}).CreateFolder(ctx, "Trips", "")
		wantCollectionErr(t, err, []error{ErrOutcomeUnknown, context.DeadlineExceeded}, []error{ErrChange}, "", "Trips")
		if d := time.Since(start); d > time.Second {
			t.Errorf("CreateFolder returned after %v, want it at the deadline", d)
		}
	})

	// The bridge would answer at the 2 s deadline: CreateFolder must not wait for it.
	t.Run("canceled before the outcome", func(t *testing.T) {
		b := &fake.Bridge{CreatedID: collectionTop, HoldChange: make(chan struct{})}
		t.Cleanup(func() { close(b.HoldChange) })
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		time.AfterFunc(10*time.Millisecond, cancel)
		start := time.Now()
		_, err := (&Library{b}).CreateFolder(ctx, "Trips", "")
		wantCollectionErr(t, err, []error{ErrOutcomeUnknown, context.Canceled}, nil, "", "Trips")
		if d := time.Since(start); d > time.Second {
			t.Errorf("CreateFolder returned after %v, want it at the cancel", d)
		}
	})
}

func TestCollectionChange(t *testing.T) {
	bridgeErr := errors.New("bridge failed")
	for _, tt := range []struct {
		name      string
		err       error
		want, not []error
		text      string
	}{
		{"no album", bridge.ErrNoAlbum, []error{ErrNotFound}, []error{ErrChange, bridge.ErrNoAlbum}, "album"},
		{"no asset", bridge.ErrNoAsset, []error{ErrNotFound}, []error{ErrChange, bridge.ErrNoAsset}, "asset"},
		{"user canceled", &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3072}, []error{ErrUserCanceled}, []error{ErrChange}, ""},
		{"3072 of another domain", &bridge.PhotoKitError{Domain: "NSCocoaErrorDomain", Code: 3072}, []error{ErrChange}, []error{ErrUserCanceled}, "NSCocoaErrorDomain code 3072"},
		{"PhotoKit error", &bridge.PhotoKitError{Domain: "PHPhotosErrorDomain", Code: 3300}, []error{ErrChange}, nil, "PHPhotosErrorDomain code 3300"},
		{"bridge error", bridgeErr, []error{bridgeErr}, []error{ErrChange, ErrNotFound}, ""},
		{"bridge timeout", context.DeadlineExceeded, []error{ErrOutcomeUnknown, context.DeadlineExceeded}, []error{ErrChange}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, err := collectionChange(t.Context(), "album", nil, func(time.Duration) (string, error) { return collectionTop, tt.err })
			if id != "" {
				t.Errorf("id = %q, want none", id)
			}
			wantCollectionErr(t, err, tt.want, tt.not, tt.text)
		})
	}

	t.Run("success", func(t *testing.T) {
		id, err := collectionChange(t.Context(), "album", []string{collectionAlbum}, func(time.Duration) (string, error) { return collectionTop, nil })
		if err != nil || id != collectionTop {
			t.Errorf("collectionChange = %q, %v; want %q", id, err, collectionTop)
		}
	})
}

func TestCollectionTimeout(t *testing.T) {
	for name, call := range map[string]func(context.Context, *Library) error{
		"CreateFolder": func(ctx context.Context, l *Library) error {
			_, err := l.CreateFolder(ctx, "Trips", "")
			return err
		},
		"RenameCollection": func(ctx context.Context, l *Library) error { return l.RenameCollection(ctx, collectionAlbum, "2025") },
		"DeleteCollections": func(ctx context.Context, l *Library) error {
			return l.DeleteCollections(ctx, []string{collectionAlbum})
		},
		"AddToAlbum": func(ctx context.Context, l *Library) error {
			return l.AddToAlbum(ctx, collectionAlbum, []string{liveID})
		},
		"RemoveFromAlbum": func(ctx context.Context, l *Library) error {
			return l.RemoveFromAlbum(ctx, collectionAlbum, []string{liveID})
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := &fake.Bridge{CreatedID: collectionTop}
			if err := call(t.Context(), &Library{b}); err != nil {
				t.Fatal(err)
			}
			if d := b.LastTimeout(); d != time.Duration(math.MaxInt64) {
				t.Errorf("timeout = %v without a deadline, want math.MaxInt64: 0 would not wait at all", d)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
			defer cancel()
			if err := call(ctx, &Library{b}); err != nil {
				t.Fatal(err)
			}
			if d := b.LastTimeout(); d <= 59*time.Minute || d > time.Hour {
				t.Errorf("timeout = %v, want the time left to the 1h deadline", d)
			}
		})
	}
}

func TestRenameCollection(t *testing.T) {
	b := &fake.Bridge{}
	if err := (&Library{b}).RenameCollection(t.Context(), collectionAlbum, "2025"); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, b, "RenameCollection "+collectionAlbum+" 2025")

	for _, title := range badIdentifiers {
		t.Run("title "+strconv.Quote(title), func(t *testing.T) {
			b := &fake.Bridge{}
			if err := (&Library{b}).RenameCollection(t.Context(), collectionAlbum, title); !errors.Is(err, ErrInvalidTitle) || err.Error() != ErrInvalidTitle.Error() {
				t.Errorf("err = %v, want ErrInvalidTitle and no title", err)
			}
			wantCalls(t, b)
		})
	}
	for _, id := range badIdentifiers {
		t.Run("id "+strconv.Quote(id), func(t *testing.T) {
			b := &fake.Bridge{}
			err := (&Library{b}).RenameCollection(t.Context(), id, "2025")
			wantCollectionErr(t, err, []error{ErrNotFound}, nil, "identifier", "2025", collectionTop)
			wantCalls(t, b)
		})
	}

	t.Run("no collection", func(t *testing.T) {
		b := &fake.Bridge{ChangeErr: bridge.ErrNoAlbum}
		err := (&Library{b}).RenameCollection(t.Context(), collectionAlbum, "2025")
		wantCollectionErr(t, err, []error{ErrNotFound}, []error{ErrChange}, "collection", collectionAlbum, "2025")
	})
}

func TestDeleteCollections(t *testing.T) {
	b := &fake.Bridge{}
	if err := (&Library{b}).DeleteCollections(t.Context(), []string{collectionAlbum, collectionFolder}); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, b, "DeleteCollections 2")

	t.Run("none", func(t *testing.T) {
		b := &fake.Bridge{ChangeErr: bridge.ErrNoAlbum}
		for _, ids := range [][]string{nil, {}} {
			if err := (&Library{b}).DeleteCollections(t.Context(), ids); err != nil {
				t.Errorf("DeleteCollections(%q) = %v, want nil", ids, err)
			}
		}
		wantCalls(t, b)
	})

	for _, id := range badIdentifiers {
		t.Run("id "+strconv.Quote(id), func(t *testing.T) {
			b := &fake.Bridge{}
			err := (&Library{b}).DeleteCollections(t.Context(), []string{collectionAlbum, id})
			wantCollectionErr(t, err, []error{ErrNotFound}, nil, "identifier", collectionAlbum, collectionTop)
			wantCalls(t, b)
		})
	}

	t.Run("no collection", func(t *testing.T) {
		b := &fake.Bridge{ChangeErr: bridge.ErrNoAlbum}
		err := (&Library{b}).DeleteCollections(t.Context(), []string{collectionAlbum, unknownID})
		wantCollectionErr(t, err, []error{ErrNotFound}, []error{ErrChange}, "collection", collectionAlbum, unknownID)
	})

	t.Run("ids are copied", func(t *testing.T) {
		b := &fake.Bridge{}
		ids := []string{collectionAlbum, collectionFolder}
		if err := (&Library{b}).DeleteCollections(t.Context(), ids); err != nil {
			t.Fatal(err)
		}
		ids[0] = unknownID
		if got := b.LastIDs(); !reflect.DeepEqual(got, []string{collectionAlbum, collectionFolder}) {
			t.Errorf("ids of the bridge = %q after the caller changed its slice, want a copy", got)
		}
	})
}

func TestAlbumAssets(t *testing.T) {
	assets := []string{liveID, slowMoID + "/L0/001"}
	for _, tt := range []struct {
		name string
		call func(*Library, context.Context, string, []string) error
		log  string
	}{
		{"AddToAlbum", (*Library).AddToAlbum, "AlbumAssets " + collectionAlbum + " 2 add=true"},
		{"RemoveFromAlbum", (*Library).RemoveFromAlbum, "AlbumAssets " + collectionAlbum + " 2 add=false"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &fake.Bridge{}
			if err := tt.call(&Library{b}, t.Context(), collectionAlbum, assets); err != nil {
				t.Fatal(err)
			}
			wantCalls(t, b, tt.log)

			b = &fake.Bridge{ChangeErr: bridge.ErrNoAlbum}
			for _, none := range [][]string{nil, {}} {
				if err := tt.call(&Library{b}, t.Context(), collectionAlbum, none); err != nil {
					t.Errorf("%s(%q) = %v, want nil", tt.name, none, err)
				}
			}
			wantCalls(t, b)
		})
	}

	for _, id := range badIdentifiers {
		t.Run("asset "+strconv.Quote(id), func(t *testing.T) {
			b := &fake.Bridge{}
			err := (&Library{b}).AddToAlbum(t.Context(), collectionAlbum, []string{liveID, id})
			wantCollectionErr(t, err, []error{ErrNotFound}, nil, "identifier", collectionAlbum, liveID, collectionTop)
			wantCalls(t, b)
		})
		t.Run("album "+strconv.Quote(id), func(t *testing.T) {
			b := &fake.Bridge{}
			err := (&Library{b}).RemoveFromAlbum(t.Context(), id, assets)
			wantCollectionErr(t, err, []error{ErrNotFound}, nil, "identifier", liveID, collectionTop)
			wantCalls(t, b)
		})
	}

	t.Run("assets are copied", func(t *testing.T) {
		b := &fake.Bridge{}
		own := []string{liveID, slowMoID}
		if err := (&Library{b}).AddToAlbum(t.Context(), collectionAlbum, own); err != nil {
			t.Fatal(err)
		}
		own[0] = unknownID
		if got := b.LastIDs(); !reflect.DeepEqual(got, []string{liveID, slowMoID}) {
			t.Errorf("assets of the bridge = %q after the caller changed its slice, want a copy", got)
		}
	})

	for _, tt := range []struct {
		name string
		err  error
		text string
	}{
		{"no album", bridge.ErrNoAlbum, "album"},
		{"no asset", bridge.ErrNoAsset, "asset"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &fake.Bridge{ChangeErr: tt.err}
			err := (&Library{b}).AddToAlbum(t.Context(), collectionAlbum, []string{unknownID})
			wantCollectionErr(t, err, []error{ErrNotFound}, []error{ErrChange}, tt.text, collectionAlbum, unknownID)
		})
	}
}
