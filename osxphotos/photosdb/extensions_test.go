package photosdb

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

// The cases of this file are the migrations of testdata/libraries: each
// migration marks the assets it changes with a title "mig:<case>" and states
// what it expects in its header.

type migrated struct {
	db, pristine *DB
	byTitle      map[string]*photoInfo
	path         string
}

func openMigrated(t *testing.T, name string) *migrated {
	t.Helper()
	m := &migrated{path: testlib.Open(t, name), byTitle: map[string]*photoInfo{}}
	var err error
	if m.db, err = Open(t.Context(), m.path, Options{TempDir: t.TempDir()}); err != nil {
		t.Fatalf("%s: Open: %v", name, err)
	}
	if m.pristine, err = Open(t.Context(), testlib.OpenPristine(t, name), Options{TempDir: t.TempDir()}); err != nil {
		t.Fatalf("%s: Open of the pristine copy: %v", name, err)
	}
	for _, p := range m.db.Photos() {
		if strings.HasPrefix(p.Title, "mig:") {
			m.byTitle[p.Title] = p
		}
	}
	return m
}

func (m *migrated) get(t *testing.T, marker string) *photoInfo {
	t.Helper()
	p, ok := m.byTitle["mig:"+marker]
	if !ok {
		t.Fatalf("no asset marked mig:%s", marker)
	}
	return p
}

func dateIs(p *time.Time, y int, mo time.Month, d int) bool {
	return p != nil && p.Equal(time.Date(y, mo, d, 0, 0, 0, 0, time.UTC))
}

func TestExtensionsCloudGUID(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Test-13.0.0", "Test-26.1"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := openMigrated(t, name)
			if p := m.get(t, "asset-cloud-guid-1"); p.CloudAssetGUID != "TEST-GUID-0001" || !p.IsCloudAsset {
				t.Errorf("guid-1: %q, %v", p.CloudAssetGUID, p.IsCloudAsset)
			}
			for _, mk := range []string{"asset-cloud-guid-dup-a", "asset-cloud-guid-dup-b"} {
				if p := m.get(t, mk); p.CloudAssetGUID != "TEST-GUID-0002" || !p.IsCloudAsset {
					t.Errorf("%s: %q, %v", mk, p.CloudAssetGUID, p.IsCloudAsset)
				}
			}
			n := 0
			for _, p := range m.db.Photos() {
				if p.CloudAssetGUID != "" || p.IsCloudAsset {
					n++
				}
			}
			if n != 3 || len(m.db.Photos()) != len(m.pristine.Photos()) {
				t.Errorf("%d assets with a cloud GUID, want 3; %d photos, pristine %d", n,
					len(m.db.Photos()), len(m.pristine.Photos()))
			}
		})
	}
}

func TestExtensionsCloud(t *testing.T) {
	t.Parallel()
	m := openMigrated(t, "Test-Cloud-13.1")
	for _, mk := range []string{"asset-cloud-guid-null", "asset-cloud-guid-empty"} {
		if p := m.get(t, mk); p.CloudAssetGUID != "" || p.IsCloudAsset {
			t.Errorf("%s: %q, %v; want \"\", false", mk, p.CloudAssetGUID, p.IsCloudAsset)
		}
	}
	if p := m.get(t, "cloud-master-local"); p.InCloud == nil || *p.InCloud || p.CloudMasterGUID != "TEST-MASTER-GUID-0001" {
		t.Errorf("cloud-master-local: InCloud %v, CloudMasterGUID %q", p.InCloud, p.CloudMasterGUID)
	}
	if p := m.get(t, "cloud-master-none"); p.InCloud != nil || p.CloudMasterGUID != "" {
		t.Errorf("cloud-master-none: InCloud %v, CloudMasterGUID %q", p.InCloud, p.CloudMasterGUID)
	}
}

func TestExtensionsFingerprint(t *testing.T) {
	t.Parallel()
	// Both fingerprint columns: ZMASTERFINGERPRINT of photos8 and
	// ZORIGINALSTABLEHASH of the later schemas.
	for _, name := range []string{"Test-13.0.0", "Test-26.1", "Test-27.0_DevBeta"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := openMigrated(t, name)
			if p := m.get(t, "fingerprint-value"); p.Fingerprint != "TEST-FP-0001" {
				t.Errorf("value: %q; want TEST-FP-0001", p.Fingerprint)
			}
			if p := m.get(t, "fingerprint-null"); p.Fingerprint != "" {
				t.Errorf("null: %q; want empty", p.Fingerprint)
			}
		})
	}
}

func TestExtensionsResources(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Test-14.0.0", "Test-26.1"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := openMigrated(t, name)
			p := m.get(t, "resources-original")
			if p.IsMissing || !slices.ContainsFunc(p.Resources, func(r photoResource) bool {
				return r.Subtype == 1 && r.Size == 1000001 && r.LocalAvailability == 1 && r.RemoteAvailability == 1
			}) {
				t.Errorf("original: IsMissing %v, Resources %+v", p.IsMissing, p.Resources)
			}
			p = m.get(t, "resources-missing")
			if !p.IsMissing || p.Path != "" || !slices.ContainsFunc(p.Resources, func(r photoResource) bool {
				return r.Subtype == 1 && r.LocalAvailability == 0
			}) {
				t.Errorf("missing: IsMissing %v, Path %q, Resources %+v", p.IsMissing, p.Path, p.Resources)
			}
			if name != "Test-14.0.0" {
				return
			}
			p = m.get(t, "resources-raw")
			if !p.HasRAW || !slices.ContainsFunc(p.Resources, func(r photoResource) bool { return r.Subtype == 17 && r.Size == 2000002 }) {
				t.Errorf("raw: HasRAW %v, Resources %+v", p.HasRAW, p.Resources)
			}
		})
	}
}

func TestExtensionsSharedLibraryAndNulls(t *testing.T) {
	t.Parallel()
	m := openMigrated(t, "Test-14.6.0")
	if p := m.get(t, "shared-library-member"); !p.SharedLibrary || p.LibraryScopeShareState != 2 || p.LibraryScope != 7 {
		t.Errorf("member: %v, %d, %d", p.SharedLibrary, p.LibraryScopeShareState, p.LibraryScope)
	}
	if p := m.get(t, "shared-library-null"); p.SharedLibrary || p.LibraryScopeShareState != 0 || p.LibraryScope != 0 {
		t.Errorf("null: %v, %d, %d", p.SharedLibrary, p.LibraryScopeShareState, p.LibraryScope)
	}
	p := m.get(t, "tzoffset-null")
	pristine, ok := m.pristine.Photo(p.UUID)
	// The name still gives the zone, as photos_datetime does with tzoffset None (photos_datetime.py:50-59).
	// UTC would pass with no zone of the name: Date without an offset is in UTC.
	if p.TZOffset != nil || p.TZName == "" || p.TZName == "UTC" || p.Date.Location().String() != p.TZName ||
		!ok || !p.Date.Equal(pristine.Date) {
		t.Errorf("tzoffset-null: TZOffset %v, Date %v; pristine %v", p.TZOffset, p.Date, pristine)
	}
	for _, mk := range []string{"original-filename-null", "original-filename-empty"} {
		if p := m.get(t, mk); p.Filename == "" || p.OriginalFilename != p.Filename {
			t.Errorf("%s: OriginalFilename %q, Filename %q", mk, p.OriginalFilename, p.Filename)
		}
	}
	p = m.get(t, "date-created-null")
	if _, off := p.Date.Zone(); !p.Date.Equal(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)) || off != 0 {
		t.Errorf("date-created-null: Date %v", p.Date)
	}
}

func TestExtensionsDates(t *testing.T) {
	t.Parallel()
	m := openMigrated(t, "Test-16.0.0-beta")
	if p := m.get(t, "date-modified-raw-value"); !dateIs(p.DateModifiedRaw, 2023, 1, 1) || !dateIs(p.DateModified, 2023, 1, 2) {
		t.Errorf("value: DateModifiedRaw %v, DateModified %v", p.DateModifiedRaw, p.DateModified)
	}
	if p := m.get(t, "date-modified-raw-null"); p.DateModifiedRaw != nil {
		t.Errorf("null: DateModifiedRaw %v", p.DateModifiedRaw)
	}

	m14 := openMigrated(t, "Test-14.0.0")
	p := m14.get(t, "trash")
	if !p.InTrash || !dateIs(p.DateTrashed, 2023, 1, 1) || !slices.Contains(m14.db.Photos(), p) {
		t.Errorf("trash: InTrash %v, DateTrashed %v", p.InTrash, p.DateTrashed)
	}
}

func TestExtensionsBurst(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Test-13.0.0", "Test-26.1"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := openMigrated(t, name)
			key := m.get(t, "burst-key")
			keyAlbums := lookupUUIDs(m.db.AlbumInfoOf(key))
			if len(keyAlbums) == 0 {
				t.Fatal("burst-key has no album outside the trash")
			}
			selected := m.db.Query(query.Options{})
			for _, tt := range []struct {
				marker                      string
				pick                        int
				isKey, selected, unselected bool
			}{
				{"burst-key", 16, true, false, false},
				{"burst-selected", 8, false, true, false},
				{"burst-none", 0, false, false, false},
				{"burst-unselected", 2, false, false, true},
				{"burst-default", 4, false, false, true},
				{"burst-null", 0, false, false, false},
			} {
				p := m.get(t, tt.marker)
				queried := slices.Contains(selected, p)
				if !p.Burst || p.BurstUUID != "TEST-BURST-0001" || p.BurstPickType != tt.pick || p.BurstKey != tt.isKey ||
					p.BurstSelected != tt.selected || queried == tt.unselected {
					t.Errorf("%s: Burst %v %q, pick %d, key %v, selected %v, in Query %v", tt.marker, p.Burst, p.BurstUUID,
						p.BurstPickType, p.BurstKey, p.BurstSelected, queried)
				}
				if !tt.isKey && len(p.AlbumUUIDs) != 0 {
					t.Errorf("%s: AlbumUUIDs %q, want none", tt.marker, p.AlbumUUIDs)
				}
				if got := lookupUUIDs(m.db.BurstAlbumInfoOf(p)); !slices.Equal(got, keyAlbums) {
					t.Errorf("%s: BurstAlbumInfoOf %q, want %q", tt.marker, got, keyAlbums)
				}
			}
		})
	}
}

func TestExtensionsSyndication(t *testing.T) {
	t.Parallel()
	m := openMigrated(t, "Test-15.4.1")
	if p := m.get(t, "syndicated-not-saved"); !p.Syndicated || p.SavedToLibrary {
		t.Errorf("not-saved: %v, %v", p.Syndicated, p.SavedToLibrary)
	}
	if p := m.get(t, "syndicated-saved"); !p.Syndicated || !p.SavedToLibrary {
		t.Errorf("saved: %v, %v", p.Syndicated, p.SavedToLibrary)
	}
	if p := m.get(t, "shared-album-photo"); !p.Shared {
		t.Error("shared-album-photo: Shared false")
	}
}

func TestExtensionsSkippedAsset(t *testing.T) {
	t.Parallel()
	m := openMigrated(t, "Test-16.0.0-beta")
	src := roDB(t, m.path)
	skipped := count(t, src, withoutAttributes)
	if skipped < 1 || count(t, src, "SELECT COUNT(*) FROM ZASSET WHERE ZFILENAME = 'mig-skipped-asset.jpg'") != 1 {
		t.Fatalf("%d assets without attributes; the marked asset is missing", skipped)
	}
	if got := m.db.Info().SkippedAssets; got != skipped {
		t.Errorf("SkippedAssets = %d, want %d", got, skipped)
	}
	if got, want := len(m.db.Photos()), count(t, src, "SELECT COUNT(*) FROM ZASSET")-skipped; got != want {
		t.Errorf("%d photos, want %d", got, want)
	}
	rows, err := src.QueryContext(t.Context(), "SELECT a.ZUUID FROM ZASSET a WHERE NOT EXISTS "+
		"(SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			t.Fatal(err)
		}
		if _, ok := m.db.Photo(uuid); ok {
			t.Errorf("Photo(%s) of an asset without attributes: found", uuid)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestExtensionsUntouched(t *testing.T) {
	t.Parallel()
	for _, name := range testlib.Libraries(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := openMigrated(t, name)
			for _, p := range m.db.Photos() {
				if strings.HasPrefix(p.Title, "mig:") {
					continue
				}
				want, ok := m.pristine.Photo(p.UUID)
				if !ok || !reflect.DeepEqual(p, want) {
					t.Errorf("%s: an unmarked photo differs from the pristine copy:\n got %+v\nwant %+v", p.UUID, *p, want)
				}
			}
		})
	}
}
