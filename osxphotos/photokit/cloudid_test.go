package photokit

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge/fake"
)

// B has no cloud identifier, as an asset of a library without iCloud.
var cloudPairs = []bridge.IdentifierPair{{Local: "A/L0/001", Cloud: "cA"}, {Local: "B/L0/001"}, {Local: "C/L0/001", Cloud: "cC"}}

func TestCloudIdentifiers(t *testing.T) {
	// The pair of AB comes first, so that a match by a prefix without "/" would
	// give it to A.
	b := &fake.Bridge{Pairs: append([]bridge.IdentifierPair{{Local: "AB/L0/001", Cloud: "cAB"}}, cloudPairs...)}
	got, err := (&Library{b}).CloudIdentifiers(t.Context(), []string{"A", "B", "C/L0/001", "D", "A", "AB/L0/001"})
	if want := map[string]string{"A": "cA", "C/L0/001": "cC", "AB/L0/001": "cAB"}; err != nil || !maps.Equal(got, want) {
		t.Errorf("CloudIdentifiers = %v, %v; want %v by the identifiers as given", got, err, want)
	}
	wantCalls(t, b, "CloudIdentifiers 6")

	got, err = (&Library{b}).CloudIdentifiers(t.Context(), []string{"B", "D"})
	if err != nil || got == nil || len(got) > 0 {
		t.Errorf("CloudIdentifiers of assets without a cloud identifier = %v, %v; want an empty map", got, err)
	}
}

func TestLocalIdentifiers(t *testing.T) {
	// The pair of cAB comes first, so that a match by prefix would give it to cA.
	b := &fake.Bridge{Pairs: append([]bridge.IdentifierPair{{Local: "E/L0/001", Cloud: "cAB"}}, cloudPairs...)}
	got, err := (&Library{b}).LocalIdentifiers(t.Context(), []string{"cAB", "cA", "cC", "cX"})
	if want := map[string]string{"cAB": "E/L0/001", "cA": "A/L0/001", "cC": "C/L0/001"}; err != nil || !maps.Equal(got, want) {
		t.Errorf("LocalIdentifiers = %v, %v; want %v", got, err, want)
	}
	wantCalls(t, b, "LocalIdentifiers 4")
}

func TestIdentifiersErrors(t *testing.T) {
	bridgeErr := errors.New("bridge failed")
	for name, call := range map[string]func(*Library, context.Context, []string) (map[string]string, error){
		"CloudIdentifiers": (*Library).CloudIdentifiers,
		"LocalIdentifiers": (*Library).LocalIdentifiers,
	} {
		t.Run(name, func(t *testing.T) {
			b := &fake.Bridge{Pairs: cloudPairs}
			for _, none := range [][]string{nil, {}} {
				if got, err := call(&Library{b}, t.Context(), none); got != nil || err != nil {
					t.Errorf("%s(%q) = %v, %v; want nil, nil", name, none, got, err)
				}
			}
			for _, id := range badIdentifiers {
				got, err := call(&Library{b}, t.Context(), []string{"cA", id})
				if got != nil {
					t.Errorf("%s(%q) = %v, want nil", name, id, got)
				}
				wantCollectionErr(t, err, []error{ErrNotFound}, []error{ErrChange}, ": identifier", collectionTop)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			for _, ids := range [][]string{{"cA"}, nil} {
				if got, err := call(&Library{b}, ctx, ids); got != nil || !errors.Is(err, context.Canceled) {
					t.Errorf("%s(%q) with a canceled ctx = %v, %v; want context.Canceled", name, ids, got, err)
				}
			}
			wantCalls(t, b)

			b.PairsErr = bridgeErr
			got, err := call(&Library{b}, t.Context(), []string{"cA"})
			if got != nil || !errors.Is(err, bridgeErr) || errors.Is(err, ErrChange) {
				t.Errorf("%s = %v, %v; want the error of the bridge as it is: nothing is changed", name, got, err)
			}
			wantCalls(t, b, name+" 1")
		})
	}
}
