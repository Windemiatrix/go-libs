package sqlread

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// The public values of testdata/libraries/SOURCES.md.
var librarySchemas = map[string]string{
	"Test-13.0.0":       "photos8",
	"Test-Cloud-13.1":   "photos8",
	"Test-14.0.0":       "photos9",
	"Test-14.6.0":       "photos9_14_6",
	"Test-15.4.1":       "photos10",
	"Test-16.0.0-beta":  "photos11",
	"Test-26.1":         "photos11_1",
	"Test-27.0_DevBeta": "photos12",
	"synthetic":         "photos11_1",
}

func schemaByID(t *testing.T, id string) schema.Schema {
	t.Helper()
	for _, s := range schema.All() {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no schema %q", id)
	return schema.Schema{}
}

// The count is checked against the exported methods of *DB, so that a new
// reader is added here.
var readers = []struct {
	name string
	run  func(context.Context, *DB, schema.Schema) (int, error)
}{
	{"Assets", func(ctx context.Context, d *DB, s schema.Schema) (int, error) {
		r, err := d.Assets(ctx, s)
		return len(r), err
	}},
	{"Resources", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.Resources(ctx)
		return len(r), err
	}},
	{"Keywords", func(ctx context.Context, d *DB, s schema.Schema) (int, error) {
		r, err := d.Keywords(ctx, s)
		return len(r), err
	}},
	{"Faces", func(ctx context.Context, d *DB, s schema.Schema) (int, error) {
		r, err := d.Faces(ctx, s)
		return len(r), err
	}},
	{"Members", func(ctx context.Context, d *DB, s schema.Schema) (int, error) {
		r, err := d.Members(ctx, s)
		return len(r), err
	}},
	{"Albums", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.Albums(ctx)
		return len(r), err
	}},
	{"KeywordTitles", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.KeywordTitles(ctx)
		return len(r), err
	}},
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

type testDB struct {
	name, path string // path of Photos.sqlite
	schema     schema.Schema
}

func libraryDBs(t *testing.T) []testDB {
	t.Helper()
	var out []testDB
	for _, name := range testlib.Libraries(t) {
		id, ok := librarySchemas[name]
		if !ok {
			t.Fatalf("test library %s is not in librarySchemas", name)
		}
		lib := testlib.OpenPristine(t, name)
		out = append(out, testDB{name, filepath.Join(lib, "database", "Photos.sqlite"), schemaByID(t, id)})
	}
	return out
}

func openDB(t *testing.T, path string) *DB {
	t.Helper()
	d, err := Open(t.Context(), path, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func openRO(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestReaderColumns(t *testing.T) {
	t.Parallel()

	methods := reflect.TypeFor[*DB]().NumMethod() - len([]string{"Close", "Metadata", "CheckColumns"})
	if methods != len(readers) {
		t.Fatalf("*DB has %d exported reader methods, the table of readers has %d; add the new reader", methods, len(readers))
	}
	for _, lib := range libraryDBs(t) {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()
			d := openDB(t, lib.path)
			if err := d.CheckColumns(t.Context(), Columns(lib.schema)); err != nil {
				t.Errorf("CheckColumns(Columns): %v", err)
			}
			for _, r := range readers {
				if _, err := r.run(t.Context(), d, lib.schema); err != nil {
					t.Errorf("%s: %v", r.name, err)
				}
			}
		})
	}
}

func TestColumnsIncludeJoins(t *testing.T) {
	t.Parallel()

	c := Columns(schemaByID(t, "photos11_1"))
	want := map[string][]string{
		"ZASSET":                     {"ZMASTER", "ZCLOUDASSETGUID", "ZADJUSTMENTSSTATE", "ZHDRTYPE", "ZDEPTHTYPE", "ZVISIBILITYSTATE", "ZCLOUDOWNERHASHEDPERSONID", "ZMOMENTSHARE"},
		"ZADDITIONALASSETATTRIBUTES": {"ZASSET", "ZASSETDESCRIPTION", "ZORIGINALSTABLEHASH", "ZCAMERACAPTUREDEVICE", "ZTIMEZONENAME", "ZIMPORTEDBYDISPLAYNAME", "ZIMPORTEDBYBUNDLEIDENTIFIER", "ZUNMANAGEDADJUSTMENT"},
		"ZUNMANAGEDADJUSTMENT":       {"Z_PK", "ZADJUSTMENTFORMATIDENTIFIER"},
		"ZASSETDESCRIPTION":          {"Z_PK", "ZLONGDESCRIPTION"},
		"ZCLOUDMASTER":               {"Z_PK", "ZCLOUDLOCALSTATE", "ZCLOUDMASTERGUID"},
		"ZINTERNALRESOURCE":          {"ZASSET", "ZDATASTORESUBTYPE", "ZRESOURCETYPE", "ZFINGERPRINT"},
		"Z_1KEYWORDS":                {"Z_1ASSETATTRIBUTES", "Z_52KEYWORDS"},
		"ZKEYWORD":                   {"Z_PK", "ZTITLE"},
		"ZDETECTEDFACE":              {"ZASSETFORFACE", "ZPERSONFORFACE"},
		"ZPERSON":                    {"Z_PK", "ZFULLNAME"},
		"Z_33ASSETS":                 {"Z_33ALBUMS", "Z_3ASSETS", "Z_FOK_3ASSETS"},
		"ZGENERICALBUM":              {"Z_PK", "ZUUID", "ZKIND", "ZCUSTOMSORTKEY", "ZCUSTOMSORTASCENDING", "Z_FOK_PARENTFOLDER"},
	}
	for table, cols := range want {
		for _, col := range cols {
			if !contains(c[table], col) {
				t.Errorf("Columns has no %s.%s", table, col)
			}
		}
	}
	if len(c) != len(want) {
		t.Errorf("Columns has %d tables, want %d", len(c), len(want))
	}
	// Upstream reads ZSPATIALTYPE only where it exists (photosdb.py:1751-1759).
	if contains(c["ZASSET"], "ZSPATIALTYPE") {
		t.Error("Columns requires ZASSET.ZSPATIALTYPE")
	}
	for table, cols := range c {
		seen := map[string]bool{}
		for _, col := range cols {
			if seen[col] {
				t.Errorf("Columns: %s.%s twice", table, col)
			}
			seen[col] = true
		}
	}
}

func TestReadersRows(t *testing.T) {
	t.Parallel()

	for _, lib := range libraryDBs(t) {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()
			ro := openRO(t, lib.path)
			d := openDB(t, lib.path)
			n := lib.schema

			assets, err := d.Assets(t.Context(), n)
			if err != nil {
				t.Fatal(err)
			}
			if want := count(t, ro, "SELECT COUNT(*) FROM ZASSET"); int64(len(assets)) != want {
				t.Errorf("Assets: %d rows, want %d", len(assets), want)
			}
			noAttr := 0
			for i, a := range assets {
				if !a.HasAttributes {
					noAttr++
				}
				if i > 0 && assets[i-1].UUID.String > a.UUID.String {
					t.Errorf("Assets: row %d out of ZUUID order", i+1)
				}
			}
			if want := count(t, ro, "SELECT COUNT(*) FROM ZASSET a WHERE NOT EXISTS "+
				"(SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)"); int64(noAttr) != want {
				t.Errorf("Assets: %d without attributes, want %d", noAttr, want)
			}
			rows, err := ro.QueryContext(t.Context(), "SELECT a.ZUUID, u.ZADJUSTMENTFORMATIDENTIFIER FROM ZASSET a "+
				"LEFT JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK "+
				"LEFT JOIN ZUNMANAGEDADJUSTMENT u ON u.Z_PK = x.ZUNMANAGEDADJUSTMENT ORDER BY a.ZUUID, a.Z_PK")
			if err != nil {
				t.Fatal(err)
			}
			formats := 0
			for i := 0; rows.Next(); i++ {
				var uuid, format sql.NullString
				if err := rows.Scan(&uuid, &format); err != nil {
					t.Fatal(err)
				}
				if i >= len(assets) || assets[i].UUID != uuid || assets[i].AdjustmentFormat != format {
					t.Fatalf("Assets: row %d differs from the adjustment format of the same join", i+1)
				}
				if format.Valid {
					formats++
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			_ = rows.Close()
			t.Logf("Assets: %d with an adjustment format", formats)

			res, err := d.Resources(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if want := count(t, ro, "SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE ZASSET IS NOT NULL"); int64(len(res)) != want {
				t.Errorf("Resources: %d rows, want %d", len(res), want)
			}
			for i := 1; i < len(res); i++ {
				a, b := res[i-1], res[i]
				if a.AssetPK > b.AssetPK || (a.AssetPK == b.AssetPK && a.PK >= b.PK) {
					t.Errorf("Resources: row %d out of (ZASSET, Z_PK) order", i+1)
				}
			}

			members, err := d.Members(t.Context(), n)
			if err != nil {
				t.Fatal(err)
			}
			memberRows := " FROM " + n.MemberTable + " m JOIN ZGENERICALBUM g ON g.Z_PK = m." +
				n.MemberAlbumColumn + " WHERE g.ZKIND = 2 AND m." + n.MemberAssetColumn + " IS NOT NULL AND g.ZUUID IS NOT NULL"
			if want := count(t, ro, "SELECT COUNT(*)"+memberRows); int64(len(members)) != want {
				t.Errorf("Members: %d rows, want %d", len(members), want)
			}
			var ordered, orderSum int64
			for _, m := range members {
				if m.Order.Valid {
					ordered++
					orderSum += m.Order.Int64
				}
			}
			if want := count(t, ro, "SELECT COUNT(m."+n.MemberOrderColumn+")"+memberRows); ordered != want {
				t.Errorf("Members: %d with an order, want %d", ordered, want)
			}
			if want := count(t, ro, "SELECT COALESCE(SUM(m."+n.MemberOrderColumn+"), 0)"+memberRows); orderSum != want {
				t.Errorf("Members: sum of the orders %d, want %d", orderSum, want)
			}
			t.Logf("Members: %d of %d with an order", ordered, len(members))

			albums, err := d.Albums(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if want := count(t, ro, "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND IN (2, 3999, 4000) AND ZUUID IS NOT NULL"); int64(len(albums)) != want {
				t.Errorf("Albums: %d rows, want %d", len(albums), want)
			}
			for i := 1; i < len(albums); i++ {
				if albums[i-1].PK >= albums[i].PK {
					t.Errorf("Albums: row %d out of Z_PK order", i+1)
				}
			}
			var sums [3]int64
			for _, a := range albums {
				sums[0] += a.SortKey.Int64 * a.PK
				sums[1] += a.SortAscending.Int64 * a.PK
				sums[2] += a.ListOrder.Int64 * a.PK
			}
			for i, col := range []string{"ZCUSTOMSORTKEY", "ZCUSTOMSORTASCENDING", "Z_FOK_PARENTFOLDER"} {
				if want := count(t, ro, "SELECT COALESCE(SUM("+col+" * Z_PK), 0) FROM ZGENERICALBUM WHERE ZKIND IN (2, 3999, 4000) AND ZUUID IS NOT NULL"); sums[i] != want {
					t.Errorf("Albums: sum of %s * Z_PK %d, want %d", col, sums[i], want)
				}
			}

			kws, err := d.Keywords(t.Context(), n)
			if err != nil {
				t.Fatal(err)
			}
			if want := count(t, ro, "SELECT COUNT(*) FROM Z_1KEYWORDS j JOIN ZADDITIONALASSETATTRIBUTES x ON x.Z_PK = j.Z_1ASSETATTRIBUTES "+
				"JOIN ZKEYWORD k ON k.Z_PK = j."+n.KeywordColumn+" WHERE x.ZASSET IS NOT NULL"); int64(len(kws)) != want {
				t.Errorf("Keywords: %d rows, want %d", len(kws), want)
			}

			titles, err := d.KeywordTitles(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			wantTitles := zkeywordTitles(t, ro)
			sortTitles(titles)
			sortTitles(wantTitles)
			if !slices.Equal(titles, wantTitles) {
				t.Errorf("KeywordTitles: %d rows differ from the %d titles of SELECT ZTITLE FROM ZKEYWORD", len(titles), len(wantTitles))
			}

			faces, err := d.Faces(t.Context(), n)
			if err != nil {
				t.Fatal(err)
			}
			personFaceCounts(t, ro, n, faces)
		})
	}
}

func TestReadersMigrations(t *testing.T) {
	t.Parallel()

	t.Run("skipped asset", func(t *testing.T) {
		t.Parallel()
		lib := testlib.Open(t, "Test-16.0.0-beta")
		d := openDB(t, filepath.Join(lib, "database", "Photos.sqlite"))
		assets, err := d.Assets(t.Context(), schemaByID(t, "photos11"))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range assets {
			if a.Filename.String == "mig-skipped-asset.jpg" {
				found = true
				if a.HasAttributes {
					t.Error("mig-skipped-asset.jpg: HasAttributes true")
				}
			}
		}
		if !found {
			t.Fatal("no asset mig-skipped-asset.jpg")
		}
	})

	t.Run("burst frames out of albums", func(t *testing.T) {
		t.Parallel()
		lib := testlib.Open(t, "Test-13.0.0")
		path := filepath.Join(lib, "database", "Photos.sqlite")
		n := schemaByID(t, "photos8")
		d := openDB(t, path)
		members, err := d.Members(t.Context(), n)
		if err != nil {
			t.Fatal(err)
		}
		assets, err := d.Assets(t.Context(), n)
		if err != nil {
			t.Fatal(err)
		}
		frames := map[int64]string{}
		key := int64(0)
		for _, a := range assets {
			switch a.Title.String {
			case "mig:burst-selected", "mig:burst-none", "mig:burst-unselected", "mig:burst-default", "mig:burst-null":
				frames[a.PK] = a.Title.String
			case "mig:burst-key":
				key = a.PK
			}
		}
		if len(frames) != 5 || key == 0 {
			t.Fatalf("found %d frames and key %d, want 5 and a key frame", len(frames), key)
		}
		keyAlbums := 0
		for _, m := range members {
			if title, ok := frames[m.AssetPK]; ok {
				t.Errorf("%s is in an album", title)
			}
			if m.AssetPK == key {
				keyAlbums++
			}
		}
		if keyAlbums == 0 {
			t.Error("the key frame is in no album")
		}
	})
}

func TestReaderValueType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, stmt string
		run        func(context.Context, *DB, schema.Schema) (int, error)
	}{
		{"resource length", "UPDATE ZINTERNALRESOURCE SET ZDATALENGTH = 'SECRET-L' WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZINTERNALRESOURCE)", readers[1].run},
		{"asset width", "UPDATE ZASSET SET ZWIDTH = 'SECRET-W' WHERE Z_PK = 3", readers[0].run},
		{"asset HDR type", "UPDATE ZASSET SET ZHDRTYPE = 'SECRET-H' WHERE Z_PK = 3", readers[0].run},
		{"asset spatial type", "UPDATE ZASSET SET ZSPATIALTYPE = 'SECRET-S' WHERE Z_PK = 3", readers[0].run},
		{"member asset", "UPDATE Z_33ASSETS SET Z_3ASSETS = 'SECRET-M' WHERE rowid = (SELECT MIN(rowid) FROM Z_33ASSETS)", readers[4].run},
		{"member order", "UPDATE Z_33ASSETS SET Z_FOK_3ASSETS = 'SECRET-F' WHERE rowid = (SELECT MIN(rowid) FROM Z_33ASSETS)", readers[4].run},
		{"album parent", "UPDATE ZGENERICALBUM SET ZPARENTFOLDER = 'SECRET-P' WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZGENERICALBUM WHERE ZKIND = 2)", readers[5].run},
		{"album sort key", "UPDATE ZGENERICALBUM SET ZCUSTOMSORTKEY = 'SECRET-K' WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZGENERICALBUM WHERE ZKIND = 2)", readers[5].run},
		{"album sort ascending", "UPDATE ZGENERICALBUM SET ZCUSTOMSORTASCENDING = 'SECRET-A' WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZGENERICALBUM WHERE ZKIND = 2)", readers[5].run},
		{"album list order", "UPDATE ZGENERICALBUM SET Z_FOK_PARENTFOLDER = 'SECRET-O' WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZGENERICALBUM WHERE ZKIND = 2)", readers[5].run},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := syntheticDB(t)
			w := openWriter(t, path)
			exec(t, w, tt.stmt)
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			d := openDB(t, path)
			_, err := tt.run(t.Context(), d, schemaByID(t, "photos11_1"))
			if err == nil {
				t.Fatal("no error")
			}
			if !strings.Contains(err.Error(), ": row ") || !strings.Contains(err.Error(), "unexpected value type") {
				t.Errorf("error %q, want row <n>: unexpected value type", err)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Errorf("error %q holds the value", err)
			}
		})
	}
}

func TestReadersNull(t *testing.T) {
	t.Parallel()

	// Test-13.0.0 has faces of persons (testdata/golden/Test-13.0.0/query.json).
	t.Run("NULL ZFULLNAME", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(testlib.Open(t, "Test-13.0.0"), "database", "Photos.sqlite")
		w := openWriter(t, path)
		exec(t, w, "UPDATE ZPERSON SET ZFULLNAME = NULL")
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		s := schemaByID(t, "photos8")
		faces, err := openDB(t, path).Faces(t.Context(), s)
		if err != nil {
			t.Fatal(err)
		}
		if len(faces) == 0 {
			t.Fatal("Test-13.0.0 has no face of a person: the case is vacuous")
		}
		if unnamed := personFaceCounts(t, openRO(t, path), s, faces); unnamed != len(faces) {
			t.Errorf("Faces: %d with a NULL ZFULLNAME, want all %d", unnamed, len(faces))
		}
	})

	t.Run("NULL ZKEYWORD.ZTITLE", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(testlib.Open(t, "Test-13.0.0"), "database", "Photos.sqlite")
		w := openWriter(t, path)
		total := count(t, w, "SELECT COUNT(*) FROM ZKEYWORD")
		if total == 0 {
			t.Fatal("Test-13.0.0 has no keyword: the case is vacuous")
		}
		exec(t, w, "UPDATE ZKEYWORD SET ZTITLE = NULL WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZKEYWORD)")
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		titles, err := openDB(t, path).KeywordTitles(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		unnamed := 0
		for _, title := range titles {
			if !title.Valid {
				unnamed++
			}
		}
		if unnamed != 1 || int64(len(titles)) != total {
			t.Errorf("KeywordTitles: %d rows, %d NULL titles; want %d rows, 1 NULL title", len(titles), unnamed, total)
		}
	})

	path := syntheticDB(t)
	w := openWriter(t, path)
	exec(t, w,
		"UPDATE Z_33ASSETS SET Z_3ASSETS = NULL WHERE rowid = (SELECT MIN(rowid) FROM Z_33ASSETS)",
		"UPDATE Z_33ASSETS SET Z_33ALBUMS = NULL WHERE rowid = (SELECT MAX(rowid) FROM Z_33ASSETS)",
		"UPDATE Z_33ASSETS SET Z_FOK_3ASSETS = NULL",
		"UPDATE ZINTERNALRESOURCE SET ZASSET = NULL WHERE Z_PK = 1",
		"UPDATE ZASSET SET ZUUID = NULL, ZFILENAME = NULL, ZKIND = NULL, ZDATECREATED = NULL, ZHDRTYPE = NULL, ZDEPTHTYPE = NULL, "+
			"ZVISIBILITYSTATE = NULL, ZCLOUDOWNERHASHEDPERSONID = NULL, ZMOMENTSHARE = NULL, ZSPATIALTYPE = NULL WHERE Z_PK = 2",
		"UPDATE ZADDITIONALASSETATTRIBUTES SET ZORIGINALFILENAME = NULL, ZTIMEZONEOFFSET = NULL, ZCAMERACAPTUREDEVICE = NULL, "+
			"ZTIMEZONENAME = NULL, ZIMPORTEDBYDISPLAYNAME = NULL, ZIMPORTEDBYBUNDLEIDENTIFIER = NULL, ZUNMANAGEDADJUSTMENT = NULL WHERE ZASSET = 2",
		// The album keeps its ZUUID: without it Members would lose its rows.
		"UPDATE ZGENERICALBUM SET ZTITLE = NULL, ZPARENTFOLDER = NULL, ZTRASHEDSTATE = NULL, ZCREATIONDATE = NULL, ZCLOUDGUID = NULL, "+
			"ZCUSTOMSORTKEY = NULL, ZCUSTOMSORTASCENDING = NULL, Z_FOK_PARENTFOLDER = NULL "+
			"WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZGENERICALBUM WHERE ZKIND = 2)",
		"UPDATE ZGENERICALBUM SET ZUUID = NULL WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZGENERICALBUM WHERE ZKIND = 4000)",
	)
	ro := openRO(t, path)
	if n := count(t, ro, "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 4000 AND ZUUID IS NULL"); n != 1 {
		t.Fatalf("%d folders without ZUUID after the change, want 1", n)
	}
	wantMembers := count(t, ro, "SELECT COUNT(*) FROM Z_33ASSETS") - 2
	wantRes := count(t, ro, "SELECT COUNT(*) FROM ZINTERNALRESOURCE") - 1
	wantAlbums := count(t, ro, "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND IN (2, 3999, 4000)") - 1
	d := openDB(t, path)
	n := schemaByID(t, "photos11_1")
	for _, r := range readers {
		if _, err := r.run(t.Context(), d, n); err != nil {
			t.Errorf("%s: %v", r.name, err)
		}
	}
	m, _ := d.Members(t.Context(), n)
	if int64(len(m)) != wantMembers {
		t.Errorf("Members: %d, want %d", len(m), wantMembers)
	}
	for _, r := range m {
		if r.Order.Valid {
			t.Errorf("Members: Order %d, want NULL", r.Order.Int64)
			break
		}
	}
	if r, _ := d.Resources(t.Context()); int64(len(r)) != wantRes {
		t.Errorf("Resources: %d, want %d", len(r), wantRes)
	}
	if a, _ := d.Albums(t.Context()); int64(len(a)) != wantAlbums {
		t.Errorf("Albums: %d, want %d", len(a), wantAlbums)
	}
}

func zkeywordTitles(t *testing.T, ro *sql.DB) []sql.NullString {
	t.Helper()
	rows, err := ro.QueryContext(t.Context(), "SELECT ZTITLE FROM ZKEYWORD")
	if err != nil {
		t.Fatal(err)
	}
	var out []sql.NullString
	for rows.Next() {
		var title sql.NullString
		if err := rows.Scan(&title); err != nil {
			t.Fatal(err)
		}
		out = append(out, title)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	return out
}

func sortTitles(titles []sql.NullString) {
	key := func(v sql.NullString) string {
		if !v.Valid {
			return "N"
		}
		return "V" + v.String
	}
	slices.SortFunc(titles, func(a, b sql.NullString) int { return strings.Compare(key(a), key(b)) })
}

func personFaceCounts(t *testing.T, ro *sql.DB, s schema.Schema, faces []Face) (unnamed int) {
	t.Helper()
	join := " FROM ZDETECTEDFACE f JOIN ZPERSON p ON p.Z_PK = f." + s.FacePersonColumn + " WHERE f." + s.FaceAssetColumn + " IS NOT NULL"
	for _, f := range faces {
		if !f.FullName.Valid {
			unnamed++
		}
	}
	if want := count(t, ro, "SELECT COUNT(*)"+join); int64(len(faces)) != want {
		t.Errorf("Faces: %d rows, want %d", len(faces), want)
	}
	if want := count(t, ro, "SELECT COUNT(*)"+join+" AND p.ZFULLNAME IS NULL"); int64(unnamed) != want {
		t.Errorf("Faces: %d with a NULL ZFULLNAME, want %d", unnamed, want)
	}
	t.Logf("Faces: %d, %d with a NULL ZFULLNAME", len(faces), unnamed)
	return unnamed
}

func TestAssetsSpatialType(t *testing.T) {
	t.Parallel()

	t.Run("the column holds a value", func(t *testing.T) {
		t.Parallel()
		path := syntheticDB(t)
		w := openWriter(t, path)
		exec(t, w, "UPDATE ZASSET SET ZSPATIALTYPE = 7 WHERE Z_PK = 3")
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		assets, err := openDB(t, path).Assets(t.Context(), schemaByID(t, "photos11_1"))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range assets {
			if a.PK == 3 {
				found = true
				if a.SpatialType != (sql.NullInt64{Int64: 7, Valid: true}) {
					t.Errorf("SpatialType %v, want 7", a.SpatialType)
				}
			}
		}
		if !found {
			t.Fatal("no asset with Z_PK 3")
		}
	})
	t.Run("no column", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(testlib.OpenPristine(t, "Test-14.0.0"), "database", "Photos.sqlite")
		if n := count(t, openRO(t, path), "SELECT COUNT(*) FROM pragma_table_info('ZASSET') WHERE name = 'ZSPATIALTYPE'"); n != 0 {
			t.Fatal("Test-14.0.0 has ZASSET.ZSPATIALTYPE; the test needs a library without it")
		}
		assets, err := openDB(t, path).Assets(t.Context(), schemaByID(t, "photos9"))
		if err != nil {
			t.Fatal(err)
		}
		if len(assets) == 0 {
			t.Fatal("no assets")
		}
		for _, a := range assets {
			if a.SpatialType.Valid {
				t.Fatalf("Z_PK %d: SpatialType %v, want NULL", a.PK, a.SpatialType)
			}
		}
	})
}

func TestReadersEmpty(t *testing.T) {
	t.Parallel()

	path := syntheticDB(t)
	w := openWriter(t, path)
	exec(t, w,
		"DELETE FROM Z_33ASSETS",
		"DELETE FROM Z_1KEYWORDS",
		"DELETE FROM ZKEYWORD",
		"DELETE FROM ZDETECTEDFACE",
		"DELETE FROM ZINTERNALRESOURCE",
		"DELETE FROM ZADDITIONALASSETATTRIBUTES",
		"DELETE FROM ZASSET",
		"DELETE FROM ZGENERICALBUM",
	)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	d := openDB(t, path)
	for _, r := range readers {
		if n, err := r.run(t.Context(), d, schemaByID(t, "photos11_1")); n != 0 || err != nil {
			t.Errorf("%s: %d rows, %v; want 0, nil", r.name, n, err)
		}
	}
}

func TestReaderCanceled(t *testing.T) {
	t.Parallel()

	d := openDB(t, syntheticDB(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	n := schemaByID(t, "photos11_1")
	for _, r := range readers {
		if _, err := r.run(ctx, d, n); err == nil {
			t.Errorf("%s: no error with a canceled context", r.name)
		}
	}
}

type rawAggregate struct {
	subtype, rtype sql.NullInt64
	compact        sql.NullString
	rows           int64
}

// TestRAWResourceType holds the premise of the HasRAW rule of photosdb: a
// RAW resource (data store subtype 17) has ZRESOURCETYPE 4, and an original
// (1) or a Live video (18) never has it.
func TestRAWResourceType(t *testing.T) {
	t.Parallel()

	var raw17, raw17Type4 int64
	for _, lib := range libraryDBs(t) {
		ro := openRO(t, lib.path)
		rows, err := ro.QueryContext(t.Context(), "SELECT ZDATASTORESUBTYPE, ZRESOURCETYPE, CAST(ZCOMPACTUTI AS TEXT), COUNT(*) "+
			"FROM ZINTERNALRESOURCE WHERE ZDATASTORESUBTYPE IN (1, 3, 17, 18) GROUP BY 1, 2, 3 ORDER BY 1, 2, 3")
		if err != nil {
			t.Fatal(err)
		}
		var agg []rawAggregate
		for rows.Next() {
			var a rawAggregate
			if err := rows.Scan(&a.subtype, &a.rtype, &a.compact, &a.rows); err != nil {
				t.Fatal(err)
			}
			agg = append(agg, a)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
		for _, a := range agg {
			t.Logf("%s: subtype %v, type %v, compact %v: %d", lib.name, a.subtype.Int64, nullInt(a.rtype), nullStr(a.compact), a.rows)
		}
		pair3 := count(t, ro, "SELECT COUNT(DISTINCT r.ZASSET) FROM ZINTERNALRESOURCE r WHERE r.ZDATASTORESUBTYPE = 3 AND r.ZRESOURCETYPE = 4 "+
			"AND NOT EXISTS (SELECT 1 FROM ZINTERNALRESOURCE s WHERE s.ZASSET = r.ZASSET AND s.ZDATASTORESUBTYPE = 17)")
		t.Logf("%s: assets with subtype 3 and type 4 without 17: %d", lib.name, pair3)
		if bad := count(t, ro, "SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE ZDATASTORESUBTYPE IN (1, 18) AND ZRESOURCETYPE = 4"); bad != 0 {
			t.Errorf("%s: %d resources of subtype 1 or 18 have type 4", lib.name, bad)
		}
		if lib.name == "synthetic" {
			if want := int64(40); pair3 != want { // the RAW pairs of the synthetic library
				t.Errorf("synthetic: %d assets with subtype 3 and type 4, want %d", pair3, want)
			}
			continue
		}
		raw17 += count(t, ro, "SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE ZDATASTORESUBTYPE = 17")
		raw17Type4 += count(t, ro, "SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE ZDATASTORESUBTYPE = 17 AND ZRESOURCETYPE = 4")
	}
	t.Logf("libraries: %d resources of subtype 17, %d of them type 4", raw17, raw17Type4)
	if raw17 == 0 || raw17 != raw17Type4 {
		t.Errorf("%d resources of subtype 17, %d of them of type 4; want all of them, at least one", raw17, raw17Type4)
	}
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return "NULL"
	}
	return v.Int64
}

func nullStr(v sql.NullString) any {
	if !v.Valid {
		return "NULL"
	}
	return v.String
}
