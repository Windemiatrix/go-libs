//go:build !(darwin && cgo)

package bridge

import (
	"errors"
	"testing"
	"time"
)

func TestStubNotSupported(t *testing.T) {
	b := New()
	if _, err := b.AuthorizationStatus(); !errors.Is(err, ErrNotSupported) {
		t.Errorf("AuthorizationStatus: err = %v, want ErrNotSupported", err)
	}
	if _, err := b.RequestAuthorization(time.Minute); !errors.Is(err, ErrNotSupported) {
		t.Errorf("RequestAuthorization: err = %v, want ErrNotSupported", err)
	}
	if got, err := b.FetchAssets([]string{"id"}); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("FetchAssets = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.FetchBurst("id", true); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("FetchBurst = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.AssetResources("id"); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("AssetResources = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.RequestResource("id", 1, nil); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("RequestResource = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.RequestImage("id", 2, nil); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("RequestImage = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.RequestVideo("id", 1, nil); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("RequestVideo = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.ExportComposition("id", 0, "/x/render.mov", nil); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("ExportComposition = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.Import(ImportRecord{Photo: "/IMG_0001.JPG"}, time.Minute); got != "" || !errors.Is(err, ErrNotSupported) {
		t.Errorf("Import = %q, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.Collections(); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("Collections = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.CreateCollection("Trips", "", true, time.Minute); got != "" || !errors.Is(err, ErrNotSupported) {
		t.Errorf("CreateCollection = %q, %v; want ErrNotSupported", got, err)
	}
	if err := b.RenameCollection("id", "Trips", time.Minute); !errors.Is(err, ErrNotSupported) {
		t.Errorf("RenameCollection: err = %v, want ErrNotSupported", err)
	}
	if err := b.DeleteCollections([]string{"id"}, time.Minute); !errors.Is(err, ErrNotSupported) {
		t.Errorf("DeleteCollections: err = %v, want ErrNotSupported", err)
	}
	if err := b.AlbumAssets("id", []string{"id"}, true, time.Minute); !errors.Is(err, ErrNotSupported) {
		t.Errorf("AlbumAssets: err = %v, want ErrNotSupported", err)
	}
	if err := b.DeleteAssets([]string{"id"}, time.Minute); !errors.Is(err, ErrNotSupported) {
		t.Errorf("DeleteAssets: err = %v, want ErrNotSupported", err)
	}
	if err := b.UpdateAsset("id", PatchRecord{HasFavorite: true}, time.Minute); !errors.Is(err, ErrNotSupported) {
		t.Errorf("UpdateAsset: err = %v, want ErrNotSupported", err)
	}
	if got, err := b.CloudIdentifiers([]string{"id"}); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("CloudIdentifiers = %v, %v; want ErrNotSupported", got, err)
	}
	if got, err := b.LocalIdentifiers([]string{"id"}); got != nil || !errors.Is(err, ErrNotSupported) {
		t.Errorf("LocalIdentifiers = %v, %v; want ErrNotSupported", got, err)
	}
}
