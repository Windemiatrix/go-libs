package photosdb

import (
	"database/sql"
	"maps"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

const (
	cr2UTI = "com.canon.cr2-raw-image"
	arwUTI = "com.sony.arw-raw-image"
	dngUTI = "com.adobe.raw-image"
)

// The cases of test_uti_for_compact_uti (tests/test_photoexporter_needs_download_raw.py:441-461);
// the integer 11 is a case of sqlread.TestResourcesCompactUTI.
func TestUTIForCompactUTI(t *testing.T) {
	t.Parallel()
	for code, want := range map[string]string{
		"1": "public.jpeg", "9": dngUTI, "10": arwUTI, "11": cr2UTI, "21": "com.fuji.raw-image",
		"36": "public.data", "37": "com.apple.photos.apple-adjustment-envelope", "39": "public.jpeg-xl",
		"_org.webmproject.webp": "org.webmproject.webp", "2": "", "99": "", "": "", "_": "",
	} {
		if got := utiForCompactUTI(code); got != want {
			t.Errorf("utiForCompactUTI(%q) = %q, want %q", code, got, want)
		}
	}
	for u, want := range map[string]string{cr2UTI: cr2UTI, "public.jpeg": "", "": ""} {
		if got := utiIfRaw(u); got != want {
			t.Errorf("utiIfRaw(%q) = %q, want %q", u, got, want)
		}
	}
}

// The RAW codes are written out from photosdb/photosdb_utils.py:239-252, apart
// from compactUTICodes, so that a wrong entry of the table fails; codes 9 to
// 22 are the RAW types (229-230).
func TestRawCompactUTICodes(t *testing.T) {
	t.Parallel()
	raw := map[string]string{
		"9": dngUTI, "10": arwUTI, "11": cr2UTI, "12": "com.canon.crw-raw-image",
		"13": "com.olympus.raw-image", "14": "com.panasonic.rw2-raw-image", "15": "com.panasonic.raw-image",
		"16": "com.pentax.raw-image", "17": "com.nikon.raw-image", "18": "com.samsung.raw-image",
		"19": "com.leafamerica.raw-image", "20": "com.hasselblad.3fr-raw-image", "21": "com.fuji.raw-image",
		"22": "com.hasselblad.fff-raw-image",
	}
	for i := 1; i <= 39; i++ {
		code := strconv.Itoa(i)
		if got := utiIfRaw(utiForCompactUTI(code)); got != raw[code] {
			t.Errorf("RAW type of code %s = %q, want %q", code, got, raw[code])
		}
	}
	if len(compactUTICodes) != 38 {
		t.Errorf("%d codes, want 38", len(compactUTICodes))
	}
}

func TestLoadRawUTI(t *testing.T) {
	t.Parallel()
	raw := func(pk, asset int64, code sql.NullString) sqlread.Resource {
		return sqlread.Resource{PK: pk, AssetPK: asset, Subtype: nullInt(resRAW), CompactUTI: code}
	}
	in := tables{
		assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2"), baseAsset(3, "A-3"), baseAsset(4, "A-4"),
			baseAsset(5, "A-5"), baseAsset(6, "A-6"), baseAsset(7, "A-7"), baseAsset(8, "A-8")},
		resources: []sqlread.Resource{
			raw(1, 1, nullString("11")),
			raw(2, 2, nullString("11")), raw(3, 2, nullString("21")),
			raw(4, 3, nullString("_com.example.x-raw-image")),
			raw(5, 4, nullString("1")),
			raw(6, 5, nullString("99")),
			raw(7, 6, sql.NullString{}),
			raw(8, 7, nullString("_")),
			{PK: 9, AssetPK: 8, Subtype: nullInt(resOriginal), CompactUTI: nullString("11")},
		},
	}
	c, err := load(in)
	if err != nil {
		t.Fatal(err)
	}
	for uuid, want := range map[string]string{
		"A-1": cr2UTI, "A-2": "com.fuji.raw-image", "A-3": "com.example.x-raw-image",
		"A-4": "", "A-5": "", "A-6": "", "A-7": "", "A-8": "",
	} {
		p := c.byUUID[uuid]
		if p.HasRAW != (uuid != "A-8") || p.UTIRaw != want {
			t.Errorf("%s: HasRAW %v, UTIRaw %q; want %q", uuid, p.HasRAW, p.UTIRaw, want)
		}
	}
	// A known code that is not a RAW type is not learned (photosdb/photosdb.py:2714).
	if want := map[string]string{"A-5": "99", "A-7": "_"}; !maps.Equal(c.rawCodes, want) {
		t.Errorf("rawCodes = %v, want %v", c.rawCodes, want)
	}
}

func TestLearnRawUTIs(t *testing.T) {
	t.Parallel()
	c := &catalog{rawCodes: map[string]string{}}
	add := func(uuid, code, pathRaw, utiRaw string) {
		p := &photoinfo.PhotoInfo{UUID: uuid, HasRAW: true, PathRaw: pathRaw, UTIRaw: utiRaw}
		c.photos = append(c.photos, p)
		if code != "" {
			c.rawCodes[uuid] = code
		}
	}
	add("A", "99", "/lib/originals/A/a_4.arw", "kept")
	add("B", "99", "/lib/originals/B/b_4.DNG", "kept")
	add("C", "99", "/lib/originals/C/c_4.dng", "kept")
	add("D", "99", "", "")
	// A tie of two types: the one met first wins, also when the other reaches the count first.
	add("E", "97", "/lib/originals/E/e_4.arw", "kept")
	add("F", "97", "/lib/originals/F/f_4.dng", "kept")
	add("G", "97", "/lib/originals/G/g_4.dng", "kept")
	add("H", "97", "/lib/originals/H/h_4.ARW", "kept")
	add("I", "97", "", "")
	add("J", "96", "/lib/originals/J/j_4.jpg", "kept")
	add("K", "96", "", "")
	add("L", "95", "", "")
	add("M", "", "", cr2UTI)
	learnRawUTIs(c)
	want := map[string]string{
		"A": "kept", "B": "kept", "C": "kept", "D": dngUTI,
		"E": "kept", "F": "kept", "G": "kept", "H": "kept", "I": arwUTI,
		"J": "kept", "K": "", "L": "", "M": cr2UTI,
	}
	for _, p := range c.photos {
		if p.UTIRaw != want[p.UUID] {
			t.Errorf("%s: UTIRaw %q, want %q", p.UUID, p.UTIRaw, want[p.UUID])
		}
	}
}

// The cases of tests/test_photoexporter_needs_download_raw.py:543-619 on a
// copy of Test-14.0.0: the photo marked mig:resources-original has its RAW
// file on disk, the one marked mig:resources-missing has none.
func TestRawUTIOpen(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, codeOnDisk, codeMissing string
		file                          bool
		wantOnDisk, wantMissing       string
	}{
		{"known code without the file", "21", "21", false, "com.fuji.raw-image", "com.fuji.raw-image"},
		{"unknown code learned from the file", "99", "99", true, arwUTI, arwUTI},
		{"unknown code without a file stays unknown", "99", "99", false, "", ""},
		{"a known code is not learned from the file", "11", "11", true, arwUTI, cr2UTI},
		{"known code of another type is not learned", "1", "1", true, arwUTI, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.Open(t, "Test-14.0.0")
			pk, onDisk, dir, file := marked(t, lib, "mig:resources-original")
			missingPK, missing, _, _ := marked(t, lib, "mig:resources-missing")
			insert := func(asset int64, code string) string {
				return "INSERT INTO ZINTERNALRESOURCE (Z_PK, Z_ENT, Z_OPT, ZASSET, ZDATASTORESUBTYPE, ZRESOURCETYPE, ZLOCALAVAILABILITY, ZCOMPACTUTI) " +
					"SELECT MAX(Z_PK) + 1, MAX(Z_ENT), 1, " + itoa(asset) + ", 17, 4, 1, '" + code + "' FROM ZINTERNALRESOURCE"
			}
			change(t, lib, insert(pk, tt.codeOnDisk), insert(missingPK, tt.codeMissing))
			if tt.file {
				writeFile(t, filepath.Join(lib, "originals", dir, stem(file)+"_4.ARW"))
			}
			db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			a, okA := db.Photo(onDisk)
			b, okB := db.Photo(missing)
			if !okA || !okB {
				t.Fatal("a marked photo is missing")
			}
			if !a.HasRAW || (a.PathRaw != "") != tt.file || a.UTIRaw != tt.wantOnDisk {
				t.Errorf("on disk: HasRAW %v, PathRaw %q, UTIRaw %q; want true, a path %v, %q", a.HasRAW, a.PathRaw, a.UTIRaw, tt.file, tt.wantOnDisk)
			}
			if !b.HasRAW || b.PathRaw != "" || b.UTIRaw != tt.wantMissing {
				t.Errorf("missing: HasRAW %v, PathRaw %q, UTIRaw %q; want true, \"\", %q", b.HasRAW, b.PathRaw, b.UTIRaw, tt.wantMissing)
			}
		})
	}
}

// The test libraries have no files: UTIRaw of a RAW is that of its code, and
// the oracle is the database (photoinfo.py:940-944). The messages hold photo
// indexes and counts, not the values of a library.
func TestRawUTILibraries(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	typed := 0
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				ro := roDB(t, lib.path)
				const oracle = "SELECT ZCOMPACTUTI FROM ZINTERNALRESOURCE WHERE ZASSET = ? AND ZDATASTORESUBTYPE = 17 ORDER BY Z_PK DESC LIMIT 1"
				raws, withType := 0, 0
				for i, p := range db.Photos() {
					if !p.HasRAW {
						continue
					}
					raws++
					var code sql.NullString
					if err := ro.QueryRowContext(t.Context(), oracle, p.PK).Scan(&code); err != nil {
						t.Fatalf("photo %d: the query of ZINTERNALRESOURCE failed", i)
					}
					if want := utiIfRaw(utiForCompactUTI(code.String)); p.UTIRaw != want {
						t.Errorf("photo %d: UTIRaw is not the type of the code of its RAW", i)
					}
					if p.UTIRaw != "" {
						withType++
					}
				}
				t.Logf("%d photos with a RAW, %d with its type", raws, withType)
				mu.Lock()
				defer mu.Unlock()
				typed += withType
			})
		}
	})
	if typed == 0 {
		t.Error("no RAW of any library has a type: the test is vacuous")
	}
}
