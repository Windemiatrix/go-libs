package photosdb

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/golden"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func burstFrame(uuid, burst string, pick int) *photoinfo.PhotoInfo {
	p := &photoinfo.PhotoInfo{UUID: uuid, Burst: true, BurstUUID: burst, BurstPickType: pick}
	p.BurstSelected = pick&photoinfo.BurstPickSelected != 0
	p.BurstKey = pick&photoinfo.BurstPickKey != 0
	return p
}

func burstUUIDs(photos []*photoinfo.PhotoInfo) []string {
	var out []string
	for _, p := range photos {
		out = append(out, p.UUID)
	}
	return out
}

// The photos must be in UUID order, as Open reads them.
func burstDB(photos ...*photoinfo.PhotoInfo) *DB {
	db := &DB{photos: photos, byUUID: map[string]*photoinfo.PhotoInfo{}, bursts: indexBursts(photos)}
	for _, p := range photos {
		db.byUUID[p.UUID] = p
	}
	return db
}

// A pick type with neither the selected nor the key bit.
const pickUnselected = 2

func burstCatalog() *DB {
	const (
		fourFrames   = "key-selected-unselected-trashed"
		twoKeys      = "two-keys"
		noKey        = "no-key"
		trashedKeyID = "trashed-key"
		keySelected  = "key-selected"
	)
	trashed := burstFrame("A4", fourFrames, 0)
	trashed.InTrash = true
	trashedKey := burstFrame("T1", trashedKeyID, photoinfo.BurstPickKey)
	trashedKey.InTrash = true
	return burstDB(
		burstFrame("A1", fourFrames, photoinfo.BurstPickKey),
		burstFrame("A2", fourFrames, photoinfo.BurstPickSelected),
		burstFrame("A3", fourFrames, pickUnselected),
		trashed,
		burstFrame("C1", twoKeys, photoinfo.BurstPickKey),
		burstFrame("C2", twoKeys, photoinfo.BurstPickKey),
		burstFrame("C3", twoKeys, photoinfo.BurstPickSelected),
		burstFrame("D1", noKey, photoinfo.BurstPickSelected),
		burstFrame("D2", noKey, photoinfo.BurstPickSelected),
		burstFrame("E1", "", photoinfo.BurstPickKey),
		burstFrame("E2", "", photoinfo.BurstPickSelected),
		burstFrame("F1", "solo", photoinfo.BurstPickSelected),
		burstFrame("F2", "solo-key", photoinfo.BurstPickKey),
		&photoinfo.PhotoInfo{UUID: "N1"},
		trashedKey,
		burstFrame("T2", trashedKeyID, photoinfo.BurstPickSelected),
		burstFrame("U1", keySelected, photoinfo.BurstPickKey),
		burstFrame("U2", keySelected, photoinfo.BurstPickSelected),
		&photoinfo.PhotoInfo{UUID: "X1", BurstUUID: fourFrames, BurstKey: true},
	)
}

func TestBurstPhotos(t *testing.T) {
	t.Parallel()

	db := burstCatalog()
	tests := []struct {
		uuid string
		want []string
	}{
		{"A1", []string{"A2", "A3", "A4"}},
		{"A2", []string{"A1", "A3", "A4"}},
		{"A3", []string{"A1", "A2", "A4"}},
		{"A4", []string{"A1", "A2", "A3"}},
		{"C3", []string{"C1", "C2"}},
		{"E2", []string{"E1"}},
		{"F1", nil},
		{"N1", nil},
		{"X1", nil},
	}
	for _, tt := range tests {
		got := db.BurstPhotos(db.byUUID[tt.uuid])
		if !sameStringsNilAware(burstUUIDs(got), tt.want) {
			t.Errorf("BurstPhotos(%s) = %q, want %q", tt.uuid, burstUUIDs(got), tt.want)
		}
		for _, p := range got {
			if p != db.byUUID[p.UUID] {
				t.Errorf("BurstPhotos(%s): %s is a copy, not the value of the catalog", tt.uuid, p.UUID)
			}
		}
	}
}

func TestBurstKeyPhoto(t *testing.T) {
	t.Parallel()

	db := burstCatalog()
	tests := []struct {
		uuid string
		want string
	}{
		{"A1", "A1"},
		{"A2", "A1"},
		{"A3", "A1"},
		{"A4", "A1"},
		{"C2", "C2"},
		{"C3", "C1"},
		{"D1", ""},
		{"E2", "E1"},
		{"F1", ""},
		{"F2", "F2"},
		{"N1", ""},
		{"T2", "T1"}, // upstream searches burst_photos: the trash is not skipped
		{"U2", "U1"}, // nor are the unselected frames
		{"X1", ""},
	}
	for _, tt := range tests {
		got, ok := db.BurstKeyPhoto(db.byUUID[tt.uuid])
		if want := db.byUUID[tt.want]; got != want || ok != (tt.want != "") {
			t.Errorf("BurstKeyPhoto(%s) = %v, %v; want %q", tt.uuid, got, ok, tt.want)
		}
	}
}

func TestPhotosByUUID(t *testing.T) {
	t.Parallel()

	db := burstCatalog()
	tests := []struct {
		name  string
		uuids []string
		want  []string
	}{
		{"order, repeat, unknown, trashed", []string{"A3", "NONE", "A1", "A3", "A4"}, []string{"A3", "A1", "A3", "A4"}},
		{"unknown only", []string{"NONE", ""}, nil},
		{"no UUIDs", nil, nil},
	}
	for _, tt := range tests {
		got := db.PhotosByUUID(tt.uuids)
		if !slices.Equal(burstUUIDs(got), tt.want) {
			t.Errorf("%s: PhotosByUUID(%q) = %q, want %q", tt.name, tt.uuids, burstUUIDs(got), tt.want)
		}
		for _, p := range got {
			if p != db.byUUID[p.UUID] {
				t.Errorf("%s: %s is a copy, not the value of the catalog", tt.name, p.UUID)
			}
		}
	}
}

func TestBurstsGolden(t *testing.T) {
	t.Parallel()

	dir := testlib.RepoDir(t, "golden")
	frames := 0
	for _, lib := range goldenLibraries(t) {
		t.Run(lib.name, func(t *testing.T) {
			db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			ref, err := golden.Load(dir, lib.name)
			if err != nil {
				t.Fatal(err)
			}
			var uuids []string
			for _, obj := range slices.Concat(ref.Query, ref.Deleted) {
				var uuid string
				var want []string
				if json.Unmarshal(obj["uuid"], &uuid) != nil || json.Unmarshal(obj["burst_photos"], &want) != nil {
					t.Fatal("uuid or burst_photos of an object is not a string or a list of strings")
				}
				uuids = append(uuids, uuid)
				p, ok := db.Photo(uuid)
				if !ok {
					t.Errorf("Photo(%s) found nothing", uuid)
					continue
				}
				slices.Sort(want)
				if got := burstUUIDs(db.BurstPhotos(p)); !slices.Equal(got, want) {
					t.Errorf("photo %s: BurstPhotos %q, burst_photos %q", uuid, got, want)
				}
				frames += len(want)
			}
			if got := burstUUIDs(db.PhotosByUUID(uuids)); !slices.Equal(got, uuids) {
				t.Error("PhotosByUUID of the UUIDs of the objects returns other photos, or in another order")
			}
		})
	}
	if frames == 0 {
		t.Error("no object of any library has burst_photos")
	}
}
