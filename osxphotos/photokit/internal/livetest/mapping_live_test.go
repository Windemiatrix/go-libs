//go:build photokit_live

package livetest

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photokit"
)

// go test runs the tests of a package in the order of their files: the name
// of this file sorts after import_live_test.go, whose assets it maps, and
// before patch_live_test.go, which hides one of them.

// A cloud identifier names an asset in the iCloud of the owner: only counts
// are printed. The test only reads.
func TestLiveMapping(t *testing.T) {
	lib := openLibrary(t)

	// First, so that a failed mapping below does not hide them; these are the
	// only live calls of LocalIdentifiers on a library without iCloud.
	t.Run("errors", func(t *testing.T) {
		const none = "00000000-0000-0000-0000-000000000000"
		for _, m := range []struct {
			name string
			call func(context.Context, []string) (map[string]string, error)
		}{{"CloudIdentifiers", lib.CloudIdentifiers}, {"LocalIdentifiers", lib.LocalIdentifiers}} {
			if got, err := m.call(t.Context(), []string{none}); err != nil || got == nil || len(got) > 0 {
				t.Errorf("%s of an unknown identifier: %d identifiers, %s; want an empty map", m.name, len(got), changeFailure(err))
			}
			if _, err := m.call(t.Context(), []string{"a\x00b"}); !errors.Is(err, photokit.ErrNotFound) || !strings.HasSuffix(err.Error(), ": identifier") {
				t.Errorf("%s of an identifier with a NUL: %s, want ErrNotFound: identifier", m.name, changeFailure(err))
			}
			if got, err := m.call(t.Context(), nil); got != nil || err != nil {
				t.Errorf("%s of no identifier: %d identifiers, %s; want nil", m.name, len(got), changeFailure(err))
			}
		}
	})

	photos := visibleMarkerPhotos(t)
	var marker []string
	for _, p := range photos {
		marker = append(marker, p.UUID)
	}
	clouds := map[string]string{}
	for _, set := range []struct {
		what string
		ids  []string
	}{
		{"marker photos", marker},
		// CloudIdentifiers never finds the asset imported hidden.
		{"visible assets imported by this run", filter(importedAssets, func(id string) bool { return fetchable(t, lib, id) })},
	} {
		found, err := lib.CloudIdentifiers(t.Context(), set.ids)
		if err != nil {
			t.Fatalf("CloudIdentifiers of the %s: %s", set.what, changeFailure(err))
		}
		t.Logf("of %d %s %d have a cloud identifier", len(set.ids), set.what, len(found))
		for id, cloud := range found {
			uuid, _, _ := strings.Cut(id, "/")
			clouds[uuid] = cloud
		}
	}

	// Whether PHCloudIdentifier needs an iCloud sign-in is not known, so none
	// is an observation, told from a broken bridge by the GUIDs of photosdb.
	if len(clouds) == 0 {
		t.Skipf("no cloud identifiers, while %d of %d marker photos have a CloudAssetGUID in photosdb: the library is not in iCloud, or PhotoKit gives none; the comparison with photosdb and the mapping back are not checked",
			count(photos, func(p *photoinfo.PhotoInfo) bool { return p.CloudAssetGUID != "" }), len(photos))
	}

	// An observation: what PHCloudIdentifier is among the columns of photosdb.
	db := reopenWithImports(t, importedAssets)
	agree := func(guid func(*photoinfo.PhotoInfo) string, same func(cloud, guid string) bool) int {
		n := 0
		for uuid, cloud := range clouds {
			if q, ok := db.Photo(uuid); ok && guid(q) != "" && same(cloud, guid(q)) {
				n++
			}
		}
		return n
	}
	asset := func(q *photoinfo.PhotoInfo) string { return q.CloudAssetGUID }
	master := func(q *photoinfo.PhotoInfo) string { return q.CloudMasterGUID }
	equal := func(a, b string) bool { return a == b }
	t.Logf("the cloud identifier equals CloudAssetGUID of photosdb for %d of %d assets, CloudMasterGUID for %d", agree(asset, equal), len(clouds), agree(master, equal))
	t.Logf("the cloud identifier contains CloudAssetGUID of photosdb for %d of %d assets, CloudMasterGUID for %d", agree(asset, strings.Contains), len(clouds), agree(master, strings.Contains))

	back, err := lib.LocalIdentifiers(t.Context(), slices.Collect(maps.Values(clouds)))
	if err != nil {
		t.Fatalf("LocalIdentifiers: %s", changeFailure(err))
	}
	if n := count(slices.Collect(maps.Keys(clouds)), func(uuid string) bool { return !strings.HasPrefix(back[clouds[uuid]], uuid+"/") }); n > 0 {
		t.Errorf("LocalIdentifiers gives no LocalIdentifier of the asset for %d of %d cloud identifiers", n, len(clouds))
	}
}
