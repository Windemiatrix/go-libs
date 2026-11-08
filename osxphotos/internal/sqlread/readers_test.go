package sqlread

import (
	"cmp"
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
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
	{"Shares", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.Shares(ctx)
		return len(r), err
	}},
	{"CollectionShares", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		albums, members, err := d.CollectionShares(ctx)
		return len(albums) + len(members), err
	}},
	{"Participants", func(ctx context.Context, d *DB, s schema.Schema) (int, error) {
		r, err := d.Participants(ctx, s)
		return len(r), err
	}},
	{"FingerprintAvailability", func(ctx context.Context, d *DB, s schema.Schema) (int, error) {
		r, err := d.FingerprintAvailability(ctx, s)
		return len(r), err
	}},
	{"Comments", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.Comments(ctx)
		return len(r), err
	}},
	{"Exif", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.Exif(ctx)
		return len(r), err
	}},
	{"ReverseLocations", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.ReverseLocations(ctx)
		return len(r), err
	}},
	{"Persons", func(ctx context.Context, d *DB, s schema.Schema) (int, error) {
		r, err := d.Persons(ctx, s)
		return len(r), err
	}},
	{"UserFeedback", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.UserFeedback(ctx)
		return len(r), err
	}},
	{"Moments", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.Moments(ctx)
		return len(r), err
	}},
	{"Scores", func(ctx context.Context, d *DB, s schema.Schema) (int, error) {
		r, err := d.Scores(ctx, s)
		return len(r), err
	}},
	{"CloudMetadata", func(ctx context.Context, d *DB, _ schema.Schema) (int, error) {
		r, err := d.CloudMetadata(ctx)
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

	// SearchTerms and LeoTerms read the search databases, not Photos.sqlite: search_test.go runs them.
	methods := reflect.TypeFor[*DB]().NumMethod() - len([]string{"Close", "Metadata", "CheckColumns", "SearchTerms", "LeoTerms"})
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
		"ZASSET":                     {"ZMASTER", "ZCLOUDASSETGUID", "ZADJUSTMENTSSTATE", "ZHDRTYPE", "ZDEPTHTYPE", "ZVISIBILITYSTATE", "ZCLOUDOWNERHASHEDPERSONID", "ZMOMENTSHARE", "ZIMPORTSESSION", "ZMOMENT", "ZOVERALLAESTHETICSCORE", "ZCURATIONSCORE", "ZPROMOTIONSCORE", "ZHIGHLIGHTVISIBILITYSCORE"},
		"ZCOMPUTEDASSETATTRIBUTES":   {"Z_PK", "ZASSET", "ZFAILURESCORE", "ZLOWLIGHT", "ZPLEASANTREFLECTIONSSCORE", "ZWELLTIMEDSHOTSCORE"},
		"ZADDITIONALASSETATTRIBUTES": {"ZASSET", "ZASSETDESCRIPTION", "ZORIGINALSTABLEHASH", "ZCAMERACAPTUREDEVICE", "ZTIMEZONENAME", "ZIMPORTEDBYDISPLAYNAME", "ZIMPORTEDBYBUNDLEIDENTIFIER", "ZUNMANAGEDADJUSTMENT", "ZREVERSELOCATIONDATA"},
		"ZUNMANAGEDADJUSTMENT":       {"Z_PK", "ZADJUSTMENTFORMATIDENTIFIER"},
		"ZASSETDESCRIPTION":          {"Z_PK", "ZLONGDESCRIPTION"},
		"ZCLOUDMASTER":               {"Z_PK", "ZCLOUDLOCALSTATE", "ZCLOUDMASTERGUID"},
		"ZCLOUDMASTERMEDIAMETADATA":  {"Z_PK", "ZCLOUDMASTER", "ZDATA"},
		"ZEXTENDEDATTRIBUTES":        {"Z_PK", "ZASSET", "ZFLASHFIRED", "ZISO", "ZFPS", "ZLENSMODEL", "ZDATECREATED", "ZTIMEZONEOFFSET", "ZTIMEZONENAME"},
		"ZINTERNALRESOURCE":          {"ZASSET", "ZDATASTORESUBTYPE", "ZRESOURCETYPE", "ZFINGERPRINT"},
		"Z_1KEYWORDS":                {"Z_1ASSETATTRIBUTES", "Z_52KEYWORDS"},
		"ZKEYWORD":                   {"Z_PK", "ZTITLE"},
		"ZDETECTEDFACE":              {"Z_PK", "ZUUID", "ZASSETFORFACE", "ZPERSONFORFACE", "ZAGETYPE", "ZEYEMAKEUPTYPE", "ZEYESSTATE", "ZFACIALHAIRTYPE", "ZGENDERTYPE", "ZGLASSESTYPE", "ZHAIRCOLORTYPE", "ZHASSMILE", "ZISINTRASH", "ZLIPMAKEUPTYPE", "ZMANUAL", "ZSMILETYPE", "ZSOURCEHEIGHT", "ZSOURCEWIDTH", "ZCENTERX", "ZCENTERY", "ZQUALITY", "ZROLL", "ZSIZE"},
		"ZPERSON":                    {"Z_PK", "ZPERSONUUID", "ZFULLNAME", "ZDISPLAYNAME", "ZFACECOUNT", "ZKEYFACE", "ZTYPE", "ZMANUALORDER"},
		"ZUSERFEEDBACK":              {"Z_PK", "ZPERSON", "ZTYPE"},
		"Z_33ASSETS":                 {"Z_33ALBUMS", "Z_3ASSETS", "Z_FOK_3ASSETS"},
		"ZMOMENT":                    {"Z_PK", "ZTIMEZONEOFFSET", "ZAPPROXIMATELATITUDE", "ZAPPROXIMATELONGITUDE", "ZSTARTDATE", "ZENDDATE", "ZREPRESENTATIVEDATE", "ZMODIFICATIONDATE", "ZTITLE", "ZSUBTITLE"},
		"ZGENERICALBUM": {"Z_PK", "ZUUID", "ZKIND", "ZCUSTOMSORTKEY", "ZCUSTOMSORTASCENDING", "Z_FOK_PARENTFOLDER",
			"ZCLOUDOWNERHASHEDPERSONID", "ZCLOUDOWNERFIRSTNAME", "ZCLOUDOWNERLASTNAME", "ZCLOUDOWNERFULLNAME"},
		"ZCLOUDSHAREDALBUMINVITATIONRECORD": {"ZINVITEEHASHEDPERSONID", "ZINVITEEFIRSTNAME", "ZINVITEELASTNAME", "ZINVITEEFULLNAME"},
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
	// Photos 11 and later have no such columns (_photosdb_process_scoreinfo.py:62-76).
	for _, col := range []string{"ZBEHAVIORALSCORE", "ZINTERACTIONSCORE"} {
		if contains(c["ZCOMPUTEDASSETATTRIBUTES"], col) {
			t.Errorf("Columns requires ZCOMPUTEDASSETATTRIBUTES.%s with Photos 11", col)
		}
		if !contains(Columns(schemaByID(t, "photos10"))["ZCOMPUTEDASSETATTRIBUTES"], col) {
			t.Errorf("Columns has no ZCOMPUTEDASSETATTRIBUTES.%s with Photos 10", col)
		}
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
			imported := 0
			for _, a := range assets {
				if a.ImportUUID.Valid {
					imported++
				}
			}
			if want := count(t, ro, "SELECT COUNT(g.ZUUID) FROM ZASSET a JOIN ZGENERICALBUM g ON g.Z_PK = a.ZIMPORTSESSION"); int64(imported) != want {
				t.Errorf("Assets: %d with an import session, want %d", imported, want)
			}

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
				n.MemberAlbumColumn + " WHERE g.ZKIND IN (2, 1505, 1508) AND m." + n.MemberAssetColumn + " IS NOT NULL AND g.ZUUID IS NOT NULL"
			if want := count(t, ro, "SELECT COUNT(*)"+memberRows); int64(len(members)) != want {
				t.Errorf("Members: %d rows, want %d", len(members), want)
			}
			var ordered, orderSum, ofProjects, ofShared int64
			for _, m := range members {
				if m.Order.Valid {
					ordered++
					orderSum += m.Order.Int64
				}
				switch m.Kind {
				case KindProject:
					ofProjects++
				case KindSharedAlbum:
					ofShared++
				case KindUserAlbum:
				default:
					t.Errorf("Members: Kind %d", m.Kind)
				}
			}
			if want := count(t, ro, "SELECT COUNT(*)"+memberRows+" AND g.ZKIND = 1508"); ofProjects != want {
				t.Errorf("Members: %d of projects, want %d", ofProjects, want)
			}
			if want := count(t, ro, "SELECT COUNT(*)"+memberRows+" AND g.ZKIND = 1505"); ofShared != want {
				t.Errorf("Members: %d of shared albums, want %d", ofShared, want)
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
			if want := count(t, ro, "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND IN (2, 1505, 1506, 1508, 3999, 4000) AND ZUUID IS NOT NULL"); int64(len(albums)) != want {
				t.Errorf("Albums: %d rows, want %d", len(albums), want)
			}
			for i := 1; i < len(albums); i++ {
				if albums[i-1].PK >= albums[i].PK {
					t.Errorf("Albums: row %d out of Z_PK order", i+1)
				}
			}
			var withParent int64
			for _, a := range albums {
				if a.ParentUUID.Valid {
					withParent++
				}
			}
			if want := count(t, ro, "SELECT COUNT(p.ZUUID) FROM ZGENERICALBUM g JOIN ZGENERICALBUM p ON p.Z_PK = g.ZPARENTFOLDER "+
				"WHERE g.ZKIND IN (2, 1505, 1506, 1508, 3999, 4000) AND g.ZUUID IS NOT NULL"); withParent != want {
				t.Errorf("Albums: %d with the ZUUID of a parent, want %d", withParent, want)
			}
			byPK := map[int64]string{}
			for _, a := range albums {
				byPK[a.PK] = a.UUID
			}
			for _, a := range albums {
				if u, ok := byPK[a.Parent.Int64]; a.Parent.Valid && ok && a.ParentUUID.String != u {
					t.Errorf("Albums: row %d: ParentUUID is not the ZUUID of its ZPARENTFOLDER row", a.PK)
				}
			}
			var sums [3]int64
			for _, a := range albums {
				sums[0] += a.SortKey.Int64 * a.PK
				sums[1] += a.SortAscending.Int64 * a.PK
				sums[2] += a.ListOrder.Int64 * a.PK
			}
			for i, col := range []string{"ZCUSTOMSORTKEY", "ZCUSTOMSORTASCENDING", "Z_FOK_PARENTFOLDER"} {
				if want := count(t, ro, "SELECT COALESCE(SUM("+col+" * Z_PK), 0) FROM ZGENERICALBUM WHERE ZKIND IN (2, 1505, 1506, 1508, 3999, 4000) AND ZUUID IS NOT NULL"); sums[i] != want {
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
		{"share scope type", "UPDATE ZSHARE SET ZSCOPETYPE = 'SECRET-T' WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZSHARE)", readers[7].run},
		{"share creation date", "UPDATE ZSHARE SET ZCREATIONDATE = 'SECRET-D' WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZSHARE)", readers[7].run},
		// reference/006_shared-album.sql gives the synthetic library a CollectionShare.
		{"collection share sort key", "UPDATE ZSHARE SET ZCUSTOMSORTKEY = 'SECRET-K' " +
			"WHERE Z_ENT = (SELECT Z_ENT FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare')", readers[8].run},
		// An INSERT, so that the row exists whether the library has persons or not.
		// ZTYPE, not ZFACECOUNT: a trigger of ZPERSON rewrites ZFACECOUNT on insert.
		{"person type", "INSERT INTO ZPERSON (Z_PK, ZTYPE) VALUES (999999, 'SECRET-C')", readers[14].run},
		{"feedback person", "INSERT INTO ZUSERFEEDBACK (Z_PK, ZPERSON) VALUES (999999, 'SECRET-U')", readers[15].run},
		{"moment start", "INSERT INTO ZMOMENT (Z_PK, ZSTARTDATE) VALUES (999999, 'SECRET-D')", readers[16].run},
		{"moment timezone", "INSERT INTO ZMOMENT (Z_PK, ZTIMEZONEOFFSET) VALUES (999997, 'SECRET-Z')", readers[16].run},
		{"score value", "INSERT INTO ZCOMPUTEDASSETATTRIBUTES (Z_PK, ZASSET, ZFAILURESCORE) VALUES (999999, 3, 'SECRET-S')", readers[17].run},
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
	wantAlbums := count(t, ro, "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND IN (2, 1505, 1506, 1508, 3999, 4000)") - 1
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
	join := " FROM ZDETECTEDFACE f JOIN ZASSET a ON a.Z_PK = f." + s.FaceAssetColumn + " JOIN ZPERSON p ON p.Z_PK = f." + s.FacePersonColumn
	for _, f := range faces {
		if !f.FullName.Valid {
			unnamed++
		}
	}
	if want := count(t, ro, "SELECT COUNT(*)"+join); int64(len(faces)) != want {
		t.Errorf("Faces: %d rows, want %d", len(faces), want)
	}
	if want := count(t, ro, "SELECT COUNT(*)"+join+" WHERE p.ZFULLNAME IS NULL"); int64(unnamed) != want {
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

func TestExifRows(t *testing.T) {
	t.Parallel()

	var total atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range libraryDBs(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				ro := openRO(t, lib.path)
				rows, err := openDB(t, lib.path).Exif(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				const from = " FROM ZEXTENDEDATTRIBUTES WHERE ZASSET IS NOT NULL"
				if want := count(t, ro, "SELECT COUNT(*)"+from); int64(len(rows)) != want {
					t.Errorf("Exif: %d rows, want %d", len(rows), want)
				}
				var flash, isoSum, makes, dates int64
				for i, r := range rows {
					if i > 0 && (rows[i-1].AssetPK > r.AssetPK || (rows[i-1].AssetPK == r.AssetPK && rows[i-1].PK >= r.PK)) {
						t.Errorf("Exif: row %d out of (ZASSET, Z_PK) order", i+1)
					}
					if r.FlashFired.Valid && r.FlashFired.Int64 == 1 {
						flash++
					}
					isoSum += r.ISO.Int64
					if r.CameraMake.Valid {
						makes++
					}
					if r.DateCreated.Valid {
						dates++
					}
				}
				for _, c := range []struct {
					name, query string
					got         int64
				}{
					{"flash fired", "SELECT COUNT(*)" + from + " AND ZFLASHFIRED = 1", flash},
					{"sum of ISO", "SELECT CAST(COALESCE(SUM(ZISO), 0) AS INTEGER)" + from, isoSum},
					{"camera makes", "SELECT COUNT(ZCAMERAMAKE)" + from, makes},
					{"creation dates", "SELECT COUNT(ZDATECREATED)" + from, dates},
				} {
					if want := count(t, ro, c.query); c.got != want {
						t.Errorf("Exif: %s %d, want %d", c.name, c.got, want)
					}
				}
				t.Logf("Exif: %d rows, %d with a camera make, %d with a date", len(rows), makes, dates)
				total.Add(int64(len(rows)))
			})
		}
	})
	if total.Load() == 0 {
		t.Error("no library has a ZEXTENDEDATTRIBUTES row: the test is vacuous")
	}
}

func TestExifNull(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testlib.Open(t, "Test-13.0.0"), "database", "Photos.sqlite")
	w := openWriter(t, path)
	total := count(t, w, "SELECT COUNT(*) FROM ZEXTENDEDATTRIBUTES WHERE ZASSET IS NOT NULL")
	if total < 2 {
		t.Fatalf("Test-13.0.0 has %d ZEXTENDEDATTRIBUTES rows: the case is vacuous", total)
	}
	last := count(t, w, "SELECT MAX(Z_PK) FROM ZEXTENDEDATTRIBUTES")
	exec(t, w,
		"UPDATE ZEXTENDEDATTRIBUTES SET ZASSET = NULL WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZEXTENDEDATTRIBUTES WHERE ZASSET IS NOT NULL)",
		"UPDATE ZEXTENDEDATTRIBUTES SET ZFLASHFIRED = NULL, ZISO = NULL, ZAPERTURE = NULL, ZCAMERAMAKE = NULL, ZDATECREATED = NULL, "+
			"ZTIMEZONEOFFSET = NULL, ZTIMEZONENAME = NULL WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZEXTENDEDATTRIBUTES)",
	)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	rows, err := openDB(t, path).Exif(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(rows)) != total-1 {
		t.Errorf("Exif: %d rows, want %d: the row without ZASSET is no row of an asset", len(rows), total-1)
	}
	found := false
	for i, r := range rows {
		if r.PK != last {
			continue
		}
		found = true
		if r.FlashFired.Valid || r.ISO.Valid || r.Aperture.Valid || r.CameraMake.Valid || r.DateCreated.Valid || r.TZOffset.Valid || r.TZName.Valid {
			t.Errorf("Exif: row %d keeps a value in a column set to NULL", i+1)
		}
	}
	if !found {
		t.Error("Exif: the row with NULL columns is missing")
	}
}

func TestReverseLocationRows(t *testing.T) {
	t.Parallel()

	var total atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range libraryDBs(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				rows, err := openDB(t, lib.path).ReverseLocations(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				// The archives of the test libraries are under the bound, so the
				// length of each row is that of its Data.
				want := testlib.Rows(t, openRO(t, lib.path), "SELECT ZASSET || ':' || length(CAST(ZREVERSELOCATIONDATA AS BLOB)) "+
					"FROM ZADDITIONALASSETATTRIBUTES WHERE ZASSET IS NOT NULL AND ZREVERSELOCATIONDATA IS NOT NULL ORDER BY ZASSET, Z_PK")
				got := make([]string, len(rows))
				empty := 0
				for i, r := range rows {
					got[i] = strconv.FormatInt(r.AssetPK, 10) + ":" + strconv.Itoa(len(r.Data))
					if len(r.Data) == 0 {
						empty++
					}
				}
				if !slices.Equal(got, want) {
					t.Errorf("ReverseLocations: %d rows, want the %d rows of the query in (ZASSET, Z_PK) order", len(got), len(want))
				}
				if empty > 0 {
					t.Errorf("ReverseLocations: %d rows without Data", empty)
				}
				t.Logf("ReverseLocations: %d rows", len(rows))
				total.Add(int64(len(rows)))
			})
		}
	})
	if total.Load() == 0 {
		t.Error("no library has a ZREVERSELOCATIONDATA: the test is vacuous")
	}

	// The bound is that of photosdb, which refuses a larger property list.
	t.Run("an archive over 1 MiB", func(t *testing.T) {
		t.Parallel()
		path := syntheticDB(t)
		w := openWriter(t, path)
		exec(t, w,
			// An archive without an asset is no row of an asset.
			"UPDATE ZADDITIONALASSETATTRIBUTES SET ZREVERSELOCATIONDATA = X'01', ZASSET = NULL WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZADDITIONALASSETATTRIBUTES)",
			"UPDATE ZADDITIONALASSETATTRIBUTES SET ZREVERSELOCATIONDATA = zeroblob(1048576) WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZADDITIONALASSETATTRIBUTES WHERE ZASSET IS NOT NULL)",
			"UPDATE ZADDITIONALASSETATTRIBUTES SET ZREVERSELOCATIONDATA = zeroblob(1048577) WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZADDITIONALASSETATTRIBUTES WHERE ZASSET IS NOT NULL)",
		)
		want := count(t, w, "SELECT COUNT(*) FROM ZADDITIONALASSETATTRIBUTES WHERE ZASSET IS NOT NULL AND ZREVERSELOCATIONDATA IS NOT NULL")
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		rows, err := openDB(t, path).ReverseLocations(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		mib, null := 0, 0
		for _, r := range rows {
			switch {
			case r.Data == nil:
				null++
			case len(r.Data) == 1048576:
				mib++
			}
		}
		if int64(len(rows)) != want || mib != 1 || null != 1 {
			t.Errorf("%d rows, %d of 1 MiB, %d without Data; want %d, 1, 1", len(rows), mib, null, want)
		}
	})
}

func realSum(t *testing.T, db *sql.DB, query string) float64 {
	t.Helper()
	var v float64
	if err := db.QueryRowContext(t.Context(), query).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

// As quote() of SQLite, so that NULL and ” differ.
func sqlQuote(v sql.NullString) string {
	if !v.Valid {
		return "NULL"
	}
	return "'" + strings.ReplaceAll(v.String, "'", "''") + "'"
}

// Some integer and real columns of persons and faces may be 0 in every test
// library, and a swap of two such columns in a Scan changes no sum: this copy
// of Test-13.0.0 gives each column a value of its own.
func withValues(t *testing.T) testDB {
	t.Helper()
	s := schemaByID(t, librarySchemas["Test-13.0.0"])
	path := filepath.Join(testlib.OpenPristine(t, "Test-13.0.0"), "database", "Photos.sqlite")
	w := openWriter(t, path)
	exec(t, w,
		"UPDATE ZPERSON SET ZFACECOUNT = 3, ZTYPE = 5, ZMANUALORDER = 7 WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZPERSON)",
		"UPDATE ZDETECTEDFACE SET ZAGETYPE = 2, ZEYEMAKEUPTYPE = 3, ZEYESSTATE = 4, ZFACIALHAIRTYPE = 5, ZGENDERTYPE = 6, ZGLASSESTYPE = 7, "+
			"ZHAIRCOLORTYPE = 8, ZHASSMILE = 9, ZISINTRASH = 10, ZLIPMAKEUPTYPE = 11, ZMANUAL = 12, ZSMILETYPE = 13, ZSOURCEHEIGHT = 14, "+
			"ZSOURCEWIDTH = 15, ZCENTERX = 0.125, ZCENTERY = 0.25, ZQUALITY = 0.375, ZROLL = 0.5, ZSIZE = 0.625 "+
			"WHERE Z_PK = (SELECT MIN(f.Z_PK) FROM ZDETECTEDFACE f JOIN ZASSET a ON a.Z_PK = f."+s.FaceAssetColumn+
			" JOIN ZPERSON p ON p.Z_PK = f."+s.FacePersonColumn+")",
	)
	if count(t, w, "SELECT COUNT(*) FROM ZPERSON WHERE ZFACECOUNT = 3 AND ZTYPE = 5 AND ZMANUALORDER = 7") != 1 ||
		count(t, w, "SELECT COUNT(*) FROM ZDETECTEDFACE WHERE ZAGETYPE = 2 AND ZSMILETYPE = 13 AND ZSIZE = 0.625") != 1 {
		t.Fatal("withValues: an UPDATE changed no row of Test-13.0.0")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return testDB{"Test-13.0.0 with values", path, s}
}

// The oracle is the database. A swap of two columns in the Scan changes a sum
// weighted by Z_PK or the text of a row; withValues makes the sums differ.
func TestPersonRows(t *testing.T) {
	t.Parallel()

	ints := []string{"ZFACECOUNT", "ZKEYFACE", "ZTYPE", "ZMANUALORDER"}
	var people, keyed atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range append(libraryDBs(t), withValues(t)) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				ro := openRO(t, lib.path)
				d := openDB(t, lib.path)
				rows, err := d.Persons(t.Context(), lib.schema)
				if err != nil {
					t.Fatal(err)
				}
				if want := count(t, ro, "SELECT COUNT(*) FROM ZPERSON"); int64(len(rows)) != want {
					t.Errorf("Persons: %d rows, want %d", len(rows), want)
				}
				var withKey int64
				var sums [4]int64
				texts := make([]string, len(rows))
				for i, r := range rows {
					if i > 0 && rows[i-1].PK >= r.PK {
						t.Errorf("Persons: row %d out of Z_PK order", i+1)
					}
					if r.KeyAssetUUID.Valid {
						withKey++
					}
					for k, v := range []sql.NullInt64{r.FaceCount, r.KeyFace, r.Type, r.ManualOrder} {
						sums[k] += v.Int64 * r.PK
					}
					texts[i] = strconv.FormatInt(r.PK, 10) + "|" + sqlQuote(r.UUID) + "|" + sqlQuote(r.FullName) + "|" +
						sqlQuote(r.DisplayName) + "|" + sqlQuote(r.KeyAssetUUID)
				}
				for i, col := range ints {
					if want := count(t, ro, "SELECT CAST(COALESCE(SUM("+col+" * Z_PK), 0) AS INTEGER) FROM ZPERSON"); sums[i] != want {
						t.Errorf("Persons: sum of %s * Z_PK %d, want %d", col, sums[i], want)
					}
				}
				wantTexts := testlib.Rows(t, ro, "SELECT p.Z_PK || '|' || quote(p.ZPERSONUUID) || '|' || quote(p.ZFULLNAME) || '|' || "+
					"quote(p.ZDISPLAYNAME) || '|' || quote(a.ZUUID) FROM ZPERSON p LEFT JOIN ZDETECTEDFACE kf ON kf.Z_PK = p.ZKEYFACE "+
					"LEFT JOIN ZASSET a ON a.Z_PK = kf."+lib.schema.FaceAssetColumn+" ORDER BY p.Z_PK")
				if !slices.Equal(texts, wantTexts) {
					t.Errorf("Persons: %d rows, the %d rows of the query differ in a text column", len(texts), len(wantTexts))
				}
				if want := count(t, ro, "SELECT COUNT(a.ZUUID) FROM ZPERSON p JOIN ZDETECTEDFACE kf ON kf.Z_PK = p.ZKEYFACE "+
					"JOIN ZASSET a ON a.Z_PK = kf."+lib.schema.FaceAssetColumn); withKey != want {
					t.Errorf("Persons: %d with the asset of the key face, want %d", withKey, want)
				}

				feedback, err := d.UserFeedback(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if want := count(t, ro, "SELECT COUNT(*) FROM ZUSERFEEDBACK WHERE ZPERSON IS NOT NULL"); int64(len(feedback)) != want {
					t.Errorf("UserFeedback: %d rows, want %d", len(feedback), want)
				}
				var typeSum, personSum int64
				for i, f := range feedback {
					if i > 0 && (feedback[i-1].PersonPK > f.PersonPK || (feedback[i-1].PersonPK == f.PersonPK && feedback[i-1].PK >= f.PK)) {
						t.Errorf("UserFeedback: row %d out of (ZPERSON, Z_PK) order", i+1)
					}
					typeSum += f.Type.Int64 * f.PK
					personSum += f.PersonPK * f.PK
				}
				const from = " FROM ZUSERFEEDBACK WHERE ZPERSON IS NOT NULL"
				if want := count(t, ro, "SELECT CAST(COALESCE(SUM(ZTYPE * Z_PK), 0) AS INTEGER)"+from); typeSum != want {
					t.Errorf("UserFeedback: sum of ZTYPE * Z_PK %d, want %d", typeSum, want)
				}
				if want := count(t, ro, "SELECT CAST(COALESCE(SUM(ZPERSON * Z_PK), 0) AS INTEGER)"+from); personSum != want {
					t.Errorf("UserFeedback: sum of ZPERSON * Z_PK %d, want %d", personSum, want)
				}
				t.Logf("Persons: %d, %d with the asset of the key face; UserFeedback: %d", len(rows), withKey, len(feedback))
				people.Add(int64(len(rows)))
				keyed.Add(withKey)
			})
		}
	})
	if people.Load() == 0 || keyed.Load() == 0 {
		t.Errorf("%d persons, %d with a key face in all libraries; want some of both: the test is vacuous", people.Load(), keyed.Load())
	}

	// The libraries need not hold a person with two ZUSERFEEDBACK rows, so the
	// rows are written.
	t.Run("feedback", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(testlib.OpenPristine(t, "Test-13.0.0"), "database", "Photos.sqlite")
		w := openWriter(t, path)
		a, b := count(t, w, "SELECT MIN(Z_PK) FROM ZPERSON"), count(t, w, "SELECT MAX(Z_PK) FROM ZPERSON")
		if a == b {
			t.Fatal("Test-13.0.0 has fewer than two persons: the case is vacuous")
		}
		pa, pb := strconv.FormatInt(a, 10), strconv.FormatInt(b, 10)
		exec(t, w,
			"DELETE FROM ZUSERFEEDBACK",
			"INSERT INTO ZUSERFEEDBACK (Z_PK, ZPERSON, ZTYPE) VALUES (1, "+pb+", NULL), (2, "+pa+", 0), (3, "+pa+", 1), (4, NULL, 1)",
		)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		got, err := openDB(t, path).UserFeedback(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		want := []Feedback{
			{PK: 2, PersonPK: a, Type: sql.NullInt64{Int64: 0, Valid: true}},
			{PK: 3, PersonPK: a, Type: sql.NullInt64{Int64: 1, Valid: true}},
			{PK: 1, PersonPK: b},
		}
		if !slices.Equal(got, want) {
			t.Errorf("UserFeedback = %+v, want %+v", got, want)
		}
	})
}

func TestFaceRows(t *testing.T) {
	t.Parallel()

	var total atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range append(libraryDBs(t), withValues(t)) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				ro := openRO(t, lib.path)
				s := lib.schema
				faces, err := openDB(t, lib.path).Faces(t.Context(), s)
				if err != nil {
					t.Fatal(err)
				}
				from := " FROM ZDETECTEDFACE f JOIN ZASSET a ON a.Z_PK = f." + s.FaceAssetColumn + " JOIN ZPERSON p ON p.Z_PK = f." + s.FacePersonColumn
				if want := count(t, ro, "SELECT COUNT(*)"+from); int64(len(faces)) != want {
					t.Errorf("Faces: %d rows, want %d", len(faces), want)
				}
				dangling := count(t, ro, "SELECT COUNT(*) FROM ZDETECTEDFACE f JOIN ZPERSON p ON p.Z_PK = f."+s.FacePersonColumn+
					" WHERE f."+s.FaceAssetColumn+" IS NOT NULL") - int64(len(faces))
				t.Logf("Faces: %d, %d more with an asset without a ZASSET row", len(faces), dangling)

				ints := []struct {
					col string
					get func(Face) int64
				}{
					{"f." + s.FaceAssetColumn, func(f Face) int64 { return f.AssetPK }},
					{"f." + s.FacePersonColumn, func(f Face) int64 { return f.PersonPK }},
					{"f.ZAGETYPE", func(f Face) int64 { return f.AgeType.Int64 }},
					{"f.ZEYEMAKEUPTYPE", func(f Face) int64 { return f.EyeMakeupType.Int64 }},
					{"f.ZEYESSTATE", func(f Face) int64 { return f.EyeState.Int64 }},
					{"f.ZFACIALHAIRTYPE", func(f Face) int64 { return f.FacialHairType.Int64 }},
					{"f.ZGENDERTYPE", func(f Face) int64 { return f.GenderType.Int64 }},
					{"f.ZGLASSESTYPE", func(f Face) int64 { return f.GlassesType.Int64 }},
					{"f.ZHAIRCOLORTYPE", func(f Face) int64 { return f.HairColorType.Int64 }},
					{"f.ZHASSMILE", func(f Face) int64 { return f.HasSmile.Int64 }},
					{"f.ZISINTRASH", func(f Face) int64 { return f.InTrash.Int64 }},
					{"f.ZLIPMAKEUPTYPE", func(f Face) int64 { return f.LipMakeupType.Int64 }},
					{"f.ZMANUAL", func(f Face) int64 { return f.Manual.Int64 }},
					{"f.ZSMILETYPE", func(f Face) int64 { return f.SmileType.Int64 }},
					{"f.ZSOURCEHEIGHT", func(f Face) int64 { return f.SourceHeight.Int64 }},
					{"f.ZSOURCEWIDTH", func(f Face) int64 { return f.SourceWidth.Int64 }},
				}
				reals := []struct {
					col string
					get func(Face) float64
				}{
					{"f.ZCENTERX", func(f Face) float64 { return f.CenterX.Float64 }},
					{"f.ZCENTERY", func(f Face) float64 { return f.CenterY.Float64 }},
					{"f.ZQUALITY", func(f Face) float64 { return f.Quality.Float64 }},
					{"f.ZROLL", func(f Face) float64 { return f.Roll.Float64 }},
					{"f.ZSIZE", func(f Face) float64 { return f.Size.Float64 }},
				}
				intSums, realSums := make([]int64, len(ints)), make([]float64, len(reals))
				texts := make([]string, len(faces))
				for i, f := range faces {
					if i > 0 && faces[i-1].PK >= f.PK {
						t.Errorf("Faces: row %d out of Z_PK order", i+1)
					}
					for k, c := range ints {
						intSums[k] += c.get(f) * f.PK
					}
					for k, c := range reals {
						realSums[k] += c.get(f) * float64(f.PK)
					}
					texts[i] = strconv.FormatInt(f.PK, 10) + "|" + sqlQuote(f.UUID) + "|" + sqlQuote(f.AssetUUID) + "|" +
						sqlQuote(f.PersonUUID) + "|" + sqlQuote(f.FullName)
				}
				for k, c := range ints {
					if want := count(t, ro, "SELECT CAST(COALESCE(SUM("+c.col+" * f.Z_PK), 0) AS INTEGER)"+from); intSums[k] != want {
						t.Errorf("Faces: sum of %s * Z_PK %d, want %d", c.col, intSums[k], want)
					}
				}
				for k, c := range reals {
					want := realSum(t, ro, "SELECT COALESCE(SUM("+c.col+" * f.Z_PK), 0.0)"+from)
					if math.Abs(realSums[k]-want) > 1e-6*(1+math.Abs(want)) {
						t.Errorf("Faces: sum of %s * Z_PK %v, want %v", c.col, realSums[k], want)
					}
				}
				wantTexts := testlib.Rows(t, ro, "SELECT f.Z_PK || '|' || quote(f.ZUUID) || '|' || quote(a.ZUUID) || '|' || "+
					"quote(p.ZPERSONUUID) || '|' || quote(p.ZFULLNAME)"+from+" ORDER BY f.Z_PK")
				if !slices.Equal(texts, wantTexts) {
					t.Errorf("Faces: %d rows, the %d rows of the query differ in a text column", len(texts), len(wantTexts))
				}
				total.Add(int64(len(faces)))
			})
		}
	})
	if total.Load() == 0 {
		t.Error("no library has a face with an asset and a person: the test is vacuous")
	}
}

// Upstream joins ZASSET (_photosdb_process_faceinfo.py:228-230): a face whose
// asset has no row is no face of an asset.
func TestFaceRowsWithoutAsset(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testlib.Open(t, "Test-13.0.0"), "database", "Photos.sqlite")
	s := schemaByID(t, "photos8")
	w := openWriter(t, path)
	from := " FROM ZDETECTEDFACE f JOIN ZASSET a ON a.Z_PK = f." + s.FaceAssetColumn + " JOIN ZPERSON p ON p.Z_PK = f." + s.FacePersonColumn
	total := count(t, w, "SELECT COUNT(*)"+from)
	if total == 0 {
		t.Fatal("Test-13.0.0 has no face of a person with an asset: the case is vacuous")
	}
	exec(t, w, "UPDATE ZDETECTEDFACE SET "+s.FaceAssetColumn+" = 999999999 WHERE Z_PK = (SELECT MIN(f.Z_PK)"+from+")")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	faces, err := openDB(t, path).Faces(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(faces)) != total-1 {
		t.Errorf("Faces: %d rows, want %d: the face whose asset has no row is left out", len(faces), total-1)
	}
	personFaceCounts(t, openRO(t, path), s, faces)
}

// The oracle is the database: a swap of two columns in the Scan changes a sum
// weighted by Z_PK or the text of a row.
func TestMomentRows(t *testing.T) {
	t.Parallel()

	ints := []struct {
		col string
		get func(Moment) int64
	}{
		{"m.Z_PK", func(m Moment) int64 { return m.PK }},
		{"m.ZTIMEZONEOFFSET", func(m Moment) int64 { return m.TZOffset.Int64 }},
	}
	reals := []struct {
		col string
		get func(Moment) float64
	}{
		{"m.ZAPPROXIMATELATITUDE", func(m Moment) float64 { return m.Latitude.Float64 }},
		{"m.ZAPPROXIMATELONGITUDE", func(m Moment) float64 { return m.Longitude.Float64 }},
		{"m.ZSTARTDATE", func(m Moment) float64 { return m.Start.Float64 }},
		{"m.ZENDDATE", func(m Moment) float64 { return m.End.Float64 }},
		{"m.ZREPRESENTATIVEDATE", func(m Moment) float64 { return m.Representative.Float64 }},
		{"m.ZMODIFICATIONDATE", func(m Moment) float64 { return m.Modified.Float64 }},
	}
	var moments, assigned atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range libraryDBs(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				ro := openRO(t, lib.path)
				d := openDB(t, lib.path)
				rows, err := d.Moments(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if want := count(t, ro, "SELECT COUNT(*) FROM ZMOMENT"); int64(len(rows)) != want {
					t.Errorf("Moments: %d rows, want %d", len(rows), want)
				}
				intSums, realSums := make([]int64, len(ints)), make([]float64, len(reals))
				texts := make([]string, len(rows))
				for i, m := range rows {
					if i > 0 && rows[i-1].PK >= m.PK {
						t.Errorf("Moments: row %d out of Z_PK order", i+1)
					}
					for k, c := range ints {
						intSums[k] += c.get(m) * m.PK
					}
					for k, c := range reals {
						realSums[k] += c.get(m) * float64(m.PK)
					}
					texts[i] = strconv.FormatInt(m.PK, 10) + "|" + sqlQuote(m.Title) + "|" + sqlQuote(m.Subtitle)
				}
				for k, c := range ints {
					if want := count(t, ro, "SELECT CAST(COALESCE(SUM("+c.col+" * m.Z_PK), 0) AS INTEGER) FROM ZMOMENT m"); intSums[k] != want {
						t.Errorf("Moments: sum of %s * Z_PK %d, want %d", c.col, intSums[k], want)
					}
				}
				for k, c := range reals {
					want := realSum(t, ro, "SELECT COALESCE(SUM("+c.col+" * m.Z_PK), 0.0) FROM ZMOMENT m")
					if math.Abs(realSums[k]-want) > 1e-6*(1+math.Abs(want)) {
						t.Errorf("Moments: sum of %s * Z_PK %v, want %v", c.col, realSums[k], want)
					}
				}
				wantTexts := testlib.Rows(t, ro, "SELECT m.Z_PK || '|' || quote(m.ZTITLE) || '|' || quote(m.ZSUBTITLE) FROM ZMOMENT m ORDER BY m.Z_PK")
				if !slices.Equal(texts, wantTexts) {
					t.Errorf("Moments: %d rows, the %d rows of the query differ in a text column", len(texts), len(wantTexts))
				}

				assets, err := d.Assets(t.Context(), lib.schema)
				if err != nil {
					t.Fatal(err)
				}
				var inMoment, momentSum int64
				for _, a := range assets {
					if a.MomentPK.Valid {
						inMoment++
						momentSum += a.MomentPK.Int64 * a.PK
					}
				}
				if want := count(t, ro, "SELECT COUNT(*) FROM ZASSET WHERE ZMOMENT IS NOT NULL"); inMoment != want {
					t.Errorf("Assets: %d with a moment, want %d", inMoment, want)
				}
				if want := count(t, ro, "SELECT CAST(COALESCE(SUM(ZMOMENT * Z_PK), 0) AS INTEGER) FROM ZASSET"); momentSum != want {
					t.Errorf("Assets: sum of ZMOMENT * Z_PK %d, want %d", momentSum, want)
				}
				t.Logf("Moments: %d, %d assets in a moment", len(rows), inMoment)
				moments.Add(int64(len(rows)))
				assigned.Add(inMoment)
			})
		}
	})
	if moments.Load() == 0 || assigned.Load() == 0 {
		t.Errorf("%d moments, %d assets in a moment in all libraries; want some of both: the test is vacuous", moments.Load(), assigned.Load())
	}
}

// A column of its own value in each position: a swap of two columns in the
// Scan shows, which the libraries may not (latitude and longitude are often
// both -180).
func TestMomentRowValues(t *testing.T) {
	t.Parallel()

	path := syntheticDB(t)
	w := openWriter(t, path)
	exec(t, w,
		"INSERT INTO ZMOMENT (Z_PK, ZTIMEZONEOFFSET, ZAPPROXIMATELATITUDE, ZAPPROXIMATELONGITUDE, ZSTARTDATE, ZENDDATE, "+
			"ZREPRESENTATIVEDATE, ZMODIFICATIONDATE, ZTITLE, ZSUBTITLE) "+
			"VALUES (999998, 3600, 1.5, 2.5, 10.25, 20.5, 30.75, 40, 'Beach Day', 'Summer')",
		"INSERT INTO ZMOMENT (Z_PK) VALUES (999999)",
		"UPDATE ZASSET SET ZMOMENT = 999998 WHERE Z_PK = 3",
	)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	d := openDB(t, path)

	rows, err := d.Moments(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	f := func(v float64) sql.NullFloat64 { return sql.NullFloat64{Float64: v, Valid: true} }
	s := func(v string) sql.NullString { return sql.NullString{String: v, Valid: true} }
	want := []Moment{
		{
			PK: 999998, TZOffset: sql.NullInt64{Int64: 3600, Valid: true}, Latitude: f(1.5), Longitude: f(2.5),
			Start: f(10.25), End: f(20.5), Representative: f(30.75), Modified: f(40), Title: s("Beach Day"), Subtitle: s("Summer"),
		},
		{PK: 999999},
	}
	if len(rows) < len(want) || !slices.Equal(rows[len(rows)-len(want):], want) {
		t.Errorf("Moments: the last %d of %d rows differ from the inserted ones", len(want), len(rows))
	}

	assets, err := d.Assets(t.Context(), schemaByID(t, "photos11_1"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range assets {
		if a.PK == 3 {
			found = true
			if want := (sql.NullInt64{Int64: 999998, Valid: true}); a.MomentPK != want {
				t.Errorf("MomentPK %v, want %v", a.MomentPK, want)
			}
		}
	}
	if !found {
		t.Fatal("no asset with Z_PK 3")
	}
}

// until10: Photos 11 and later have no such column
// (_photosdb_process_scoreinfo.py:62-76).
var scoreFields = []struct {
	col     string
	until10 bool
	get     func(Score) sql.NullFloat64
}{
	{"a.ZOVERALLAESTHETICSCORE", false, func(r Score) sql.NullFloat64 { return r.Overall }},
	{"a.ZCURATIONSCORE", false, func(r Score) sql.NullFloat64 { return r.Curation }},
	{"a.ZPROMOTIONSCORE", false, func(r Score) sql.NullFloat64 { return r.Promotion }},
	{"a.ZHIGHLIGHTVISIBILITYSCORE", false, func(r Score) sql.NullFloat64 { return r.HighlightVisibility }},
	{"c.ZBEHAVIORALSCORE", true, func(r Score) sql.NullFloat64 { return r.Behavioral }},
	{"c.ZFAILURESCORE", false, func(r Score) sql.NullFloat64 { return r.Failure }},
	{"c.ZHARMONIOUSCOLORSCORE", false, func(r Score) sql.NullFloat64 { return r.HarmoniousColor }},
	{"c.ZIMMERSIVENESSSCORE", false, func(r Score) sql.NullFloat64 { return r.Immersiveness }},
	{"c.ZINTERACTIONSCORE", true, func(r Score) sql.NullFloat64 { return r.Interaction }},
	{"c.ZINTERESTINGSUBJECTSCORE", false, func(r Score) sql.NullFloat64 { return r.InterestingSubject }},
	{"c.ZINTRUSIVEOBJECTPRESENCESCORE", false, func(r Score) sql.NullFloat64 { return r.IntrusiveObjectPresence }},
	{"c.ZLIVELYCOLORSCORE", false, func(r Score) sql.NullFloat64 { return r.LivelyColor }},
	{"c.ZLOWLIGHT", false, func(r Score) sql.NullFloat64 { return r.LowLight }},
	{"c.ZNOISESCORE", false, func(r Score) sql.NullFloat64 { return r.Noise }},
	{"c.ZPLEASANTCAMERATILTSCORE", false, func(r Score) sql.NullFloat64 { return r.PleasantCameraTilt }},
	{"c.ZPLEASANTCOMPOSITIONSCORE", false, func(r Score) sql.NullFloat64 { return r.PleasantComposition }},
	{"c.ZPLEASANTLIGHTINGSCORE", false, func(r Score) sql.NullFloat64 { return r.PleasantLighting }},
	{"c.ZPLEASANTPATTERNSCORE", false, func(r Score) sql.NullFloat64 { return r.PleasantPattern }},
	{"c.ZPLEASANTPERSPECTIVESCORE", false, func(r Score) sql.NullFloat64 { return r.PleasantPerspective }},
	{"c.ZPLEASANTPOSTPROCESSINGSCORE", false, func(r Score) sql.NullFloat64 { return r.PleasantPostProcessing }},
	{"c.ZPLEASANTREFLECTIONSSCORE", false, func(r Score) sql.NullFloat64 { return r.PleasantReflection }},
	{"c.ZPLEASANTSYMMETRYSCORE", false, func(r Score) sql.NullFloat64 { return r.PleasantSymmetry }},
	{"c.ZSHARPLYFOCUSEDSUBJECTSCORE", false, func(r Score) sql.NullFloat64 { return r.SharplyFocusedSubject }},
	{"c.ZTASTEFULLYBLURREDSCORE", false, func(r Score) sql.NullFloat64 { return r.TastefullyBlurred }},
	{"c.ZWELLCHOSENSUBJECTSCORE", false, func(r Score) sql.NullFloat64 { return r.WellChosenSubject }},
	{"c.ZWELLFRAMEDSUBJECTSCORE", false, func(r Score) sql.NullFloat64 { return r.WellFramedSubject }},
	{"c.ZWELLTIMEDSHOTSCORE", false, func(r Score) sql.NullFloat64 { return r.WellTimedShot }},
}

// The oracle is the database: a swap of two columns in the Scan changes a sum
// weighted by Z_PK.
func TestScoreRows(t *testing.T) {
	t.Parallel()

	var rowCount, overall, failure atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range libraryDBs(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				ro := openRO(t, lib.path)
				rows, err := openDB(t, lib.path).Scores(t.Context(), lib.schema)
				if err != nil {
					t.Fatal(err)
				}
				const from = " FROM ZCOMPUTEDASSETATTRIBUTES c JOIN ZASSET a ON a.Z_PK = c.ZASSET"
				if want := count(t, ro, "SELECT COUNT(*)"+from); int64(len(rows)) != want {
					t.Errorf("Scores: %d rows, want %d", len(rows), want)
				}
				var pkSum, assetSum int64
				sums, valid := make([]float64, len(scoreFields)), make([]int64, len(scoreFields))
				for i, r := range rows {
					if i > 0 && (rows[i-1].AssetPK > r.AssetPK || (rows[i-1].AssetPK == r.AssetPK && rows[i-1].PK >= r.PK)) {
						t.Errorf("Scores: row %d out of (ZASSET, Z_PK) order", i+1)
					}
					pkSum += r.PK
					assetSum += r.AssetPK * r.PK
					for k, f := range scoreFields {
						v := f.get(r)
						sums[k] += v.Float64 * float64(r.PK)
						if v.Valid {
							valid[k]++
						}
					}
				}
				if want := count(t, ro, "SELECT COALESCE(SUM(c.Z_PK), 0)"+from); pkSum != want {
					t.Errorf("Scores: sum of Z_PK %d, want %d", pkSum, want)
				}
				if want := count(t, ro, "SELECT COALESCE(SUM(c.ZASSET * c.Z_PK), 0)"+from); assetSum != want {
					t.Errorf("Scores: sum of ZASSET * Z_PK %d, want %d", assetSum, want)
				}
				for k, f := range scoreFields {
					if f.until10 && lib.schema.PhotosVersion > 10 {
						if valid[k] != 0 {
							t.Errorf("Scores: %d rows with %s, want none from Photos 11 on", valid[k], f.col)
						}
						continue
					}
					if want := count(t, ro, "SELECT COUNT("+f.col+")"+from); valid[k] != want {
						t.Errorf("Scores: %d rows with %s, want %d", valid[k], f.col, want)
					}
					want := realSum(t, ro, "SELECT COALESCE(SUM("+f.col+" * c.Z_PK), 0.0)"+from)
					if math.Abs(sums[k]-want) > 1e-6*(1+math.Abs(want)) {
						t.Errorf("Scores: sum of %s * Z_PK %v, want %v", f.col, sums[k], want)
					}
				}
				var nOverall, nFailure int64
				for _, r := range rows {
					if r.Overall.Float64 != 0 {
						nOverall++
					}
					if r.Failure.Float64 != 0 {
						nFailure++
					}
				}
				t.Logf("Scores: %d rows, %d with Overall, %d with Failure", len(rows), nOverall, nFailure)
				rowCount.Add(int64(len(rows)))
				overall.Add(nOverall)
				failure.Add(nFailure)
			})
		}
	})
	if rowCount.Load() == 0 || overall.Load() == 0 || failure.Load() == 0 {
		t.Errorf("%d rows, %d with Overall, %d with Failure in all libraries; want some of each: the test is vacuous",
			rowCount.Load(), overall.Load(), failure.Load())
	}
}

// Upstream joins ZASSET (_photosdb_process_scoreinfo.py:97-98): a row whose
// asset has no row, or that has no ZASSET, is no score of an asset. The three
// rows of assets have a Z_PK against the order of their ZASSET, which the
// libraries may not have.
func TestScoreRowsWithoutAsset(t *testing.T) {
	t.Parallel()

	const name = "Test-15.4.1"
	path := filepath.Join(testlib.OpenPristine(t, name), "database", "Photos.sqlite")
	w := openWriter(t, path)
	total := count(t, w, "SELECT COUNT(*) FROM ZCOMPUTEDASSETATTRIBUTES c JOIN ZASSET a ON a.Z_PK = c.ZASSET")
	first := strconv.FormatInt(count(t, w, "SELECT MIN(Z_PK) FROM ZASSET"), 10)
	last := strconv.FormatInt(count(t, w, "SELECT MAX(Z_PK) FROM ZASSET"), 10)
	if first == last {
		t.Fatalf("%s has one asset: the case is vacuous", name)
	}
	exec(t, w, "INSERT INTO ZCOMPUTEDASSETATTRIBUTES (Z_PK, ZASSET) VALUES (999990, "+last+"), (999991, "+first+"), (999992, "+first+"), "+
		"(999993, 999999999), (999994, NULL)")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	rows, err := openDB(t, path).Scores(t.Context(), schemaByID(t, librarySchemas[name]))
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(rows)) != total+3 {
		t.Errorf("Scores: %d rows, want %d: a row without an asset is left out", len(rows), total+3)
	}
	for i := 1; i < len(rows); i++ {
		if a, b := rows[i-1], rows[i]; a.AssetPK > b.AssetPK || (a.AssetPK == b.AssetPK && a.PK >= b.PK) {
			t.Errorf("Scores: row %d out of (ZASSET, Z_PK) order", i+1)
		}
	}
}

// A value of its own in each column: a swap of two columns in the Scan shows,
// which the libraries may not (many scores are 0 or equal).
func TestScoreRowValues(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Test-15.4.1", "Test-26.1"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := schemaByID(t, librarySchemas[name])
			path := filepath.Join(testlib.OpenPristine(t, name), "database", "Photos.sqlite")
			w := openWriter(t, path)
			const from = " FROM ZCOMPUTEDASSETATTRIBUTES c JOIN ZASSET a ON a.Z_PK = c.ZASSET"
			pk := count(t, w, "SELECT COALESCE(MIN(c.Z_PK), 0)"+from)
			if pk == 0 {
				t.Fatalf("%s has no score row: the case is vacuous", name)
			}
			asset := count(t, w, "SELECT c.ZASSET"+from+" WHERE c.Z_PK = "+strconv.FormatInt(pk, 10))
			var ofAsset, ofScore []string
			for k, f := range scoreFields {
				if f.until10 && s.PhotosVersion > 10 {
					continue
				}
				set := f.col[2:] + " = " + strconv.FormatFloat(float64(k+1)/100, 'g', -1, 64)
				if f.col[0] == 'a' {
					ofAsset = append(ofAsset, set)
				} else {
					ofScore = append(ofScore, set)
				}
			}
			exec(t, w,
				"UPDATE ZASSET SET "+strings.Join(ofAsset, ", ")+" WHERE Z_PK = "+strconv.FormatInt(asset, 10),
				"UPDATE ZCOMPUTEDASSETATTRIBUTES SET "+strings.Join(ofScore, ", ")+" WHERE Z_PK = "+strconv.FormatInt(pk, 10),
			)
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			rows, err := openDB(t, path).Scores(t.Context(), s)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, r := range rows {
				if r.PK != pk {
					continue
				}
				found = true
				if r.AssetPK != asset {
					t.Errorf("AssetPK %d, want %d", r.AssetPK, asset)
				}
				for k, f := range scoreFields {
					want := sql.NullFloat64{Float64: float64(k+1) / 100, Valid: true}
					if f.until10 && s.PhotosVersion > 10 {
						want = sql.NullFloat64{}
					}
					if got := f.get(r); got != want {
						t.Errorf("%s = %v, want %v", f.col, got, want)
					}
				}
			}
			if !found {
				t.Errorf("no row with Z_PK %d", pk)
			}
		})
	}
}

// Upstream joins ZCLOUDMASTER and ZASSET (photoinfo.py:1747-1753): the rows are
// those with a value whose master an asset has. Data is kept only for a blob.
const cloudMetaFrom = " FROM ZCLOUDMASTERMEDIAMETADATA md JOIN ZCLOUDMASTER m ON m.Z_PK = md.ZCLOUDMASTER JOIN ZASSET a ON a.ZMASTER = m.Z_PK" +
	" WHERE md.ZDATA IS NOT NULL"

func cloudMetaOrdered(rows []CloudMetadata) bool {
	return slices.IsSortedFunc(rows, func(a, b CloudMetadata) int {
		return cmp.Or(cmp.Compare(a.AssetPK, b.AssetPK), cmp.Compare(a.PK, b.PK))
	})
}

// The oracle is the database: the two plain sums catch a swap of the keys in
// the Scan, the sum of the products a row paired with another asset.
func TestCloudMetadataRows(t *testing.T) {
	t.Parallel()

	var total atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range libraryDBs(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				ro := openRO(t, lib.path)
				rows, err := openDB(t, lib.path).CloudMetadata(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if !cloudMetaOrdered(rows) {
					t.Error("CloudMetadata: rows out of (asset, Z_PK) order")
				}
				var pkSum, assetSum, weighted, size int64
				for _, r := range rows {
					pkSum += r.PK
					assetSum += r.AssetPK
					weighted += r.AssetPK * r.PK
					size += int64(len(r.Data))
				}
				// The data of the libraries are blobs under 1 MiB, so every row keeps its value.
				for _, c := range []struct {
					name, query string
					got         int64
				}{
					{"rows", "SELECT COUNT(*)" + cloudMetaFrom, int64(len(rows))},
					{"sum of Z_PK", "SELECT COALESCE(SUM(md.Z_PK), 0)" + cloudMetaFrom, pkSum},
					{"sum of the asset Z_PK", "SELECT COALESCE(SUM(a.Z_PK), 0)" + cloudMetaFrom, assetSum},
					{"sum of Z_PK * asset Z_PK", "SELECT COALESCE(SUM(md.Z_PK * a.Z_PK), 0)" + cloudMetaFrom, weighted},
					{"bytes", "SELECT COALESCE(SUM(CASE WHEN typeof(md.ZDATA) = 'blob' THEN length(md.ZDATA) END), 0)" + cloudMetaFrom, size},
				} {
					if want := count(t, ro, c.query); c.got != want {
						t.Errorf("CloudMetadata: %s %d, want %d", c.name, c.got, want)
					}
				}
				// Measured: the rows and bytes of the photos that photosdb keeps are a part of these.
				if lib.name == "Test-Cloud-13.1" && (len(rows) < 23 || size < 45046) {
					t.Errorf("CloudMetadata: %d rows and %d bytes, want 23 and 45046 or more", len(rows), size)
				}
				t.Logf("CloudMetadata: %d rows, %d bytes", len(rows), size)
				total.Add(int64(len(rows)))
			})
		}
	})
	if total.Load() == 0 {
		t.Error("no library has a ZCLOUDMASTERMEDIAMETADATA row of an asset: the test is vacuous")
	}

	// The libraries may hold no row, so the rows are written.
	t.Run("bounds and joins", func(t *testing.T) {
		t.Parallel()
		path := syntheticDB(t)
		w := openWriter(t, path)
		master := func(aggregate string) string {
			return "(SELECT " + aggregate + "(m.Z_PK) FROM ZCLOUDMASTER m JOIN ZASSET a ON a.ZMASTER = m.Z_PK)"
		}
		const (
			insert = "INSERT INTO ZCLOUDMASTERMEDIAMETADATA (Z_PK, ZCLOUDMASTER, ZDATA) VALUES "
			xml    = `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict/></plist>`
		)
		exec(t, w,
			// A master that no asset has, and a row without a master.
			"INSERT INTO ZCLOUDMASTER (Z_PK) VALUES (900099)",
			insert+"(900003, 900099, X'01')",
			insert+"(900004, NULL, X'01')",
			insert+"(900001, "+master("MIN")+", zeroblob(1048576))",
			insert+"(900002, "+master("MAX")+", zeroblob(1048577))",
			// plistlib.loads of an integer or a str raises upstream (photoinfo.py:1757-1759): the row stays, without Data.
			insert+"(900005, "+master("MIN")+", 424242)",
			insert+"(900006, "+master("MIN")+", '"+xml+"')",
			insert+"(900007, "+master("MIN")+", CAST('"+xml+"' AS BLOB))",
		)
		want := count(t, w, "SELECT COUNT(*)"+cloudMetaFrom)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		rows, err := openDB(t, path).CloudMetadata(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		mib, null, plist := 0, 0, 0
		for _, r := range rows {
			switch {
			case r.Data == nil:
				null++
			case len(r.Data) == 1048576:
				mib++
			case string(r.Data) == xml:
				plist++
			}
		}
		if int64(len(rows)) != want || mib != 1 || null != 3 || plist != 1 {
			t.Errorf("%d rows, %d of 1 MiB, %d without Data, %d of a property list; want %d, 1, 3, 1", len(rows), mib, null, plist, want)
		}
	})

	// The assets are the first three of the library: one master has two of them,
	// and the order of the assets contradicts that of the rows.
	t.Run("a master of several assets", func(t *testing.T) {
		t.Parallel()
		path := syntheticDB(t)
		w := openWriter(t, path)
		asset := func(n int) string {
			return "(SELECT Z_PK FROM ZASSET ORDER BY Z_PK LIMIT 1 OFFSET " + strconv.Itoa(n) + ")"
		}
		const insert = "INSERT INTO ZCLOUDMASTERMEDIAMETADATA (Z_PK, ZCLOUDMASTER, ZDATA) VALUES "
		exec(t, w,
			"INSERT INTO ZCLOUDMASTER (Z_PK) VALUES (900050), (900051)",
			"UPDATE ZASSET SET ZMASTER = 900051 WHERE Z_PK = "+asset(0),
			"UPDATE ZASSET SET ZMASTER = 900050 WHERE Z_PK = "+asset(1),
			"UPDATE ZASSET SET ZMASTER = 900050 WHERE Z_PK = "+asset(2),
			insert+"(900060, 900050, X'0102')",
			insert+"(900061, 900051, X'0304')",
		)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		rows, err := openDB(t, path).CloudMetadata(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !cloudMetaOrdered(rows) {
			t.Error("rows out of (asset, Z_PK) order")
		}
		var shared []CloudMetadata
		for _, r := range rows {
			if r.PK == 900060 {
				shared = append(shared, r)
			}
		}
		if len(shared) != 2 {
			t.Fatalf("%d rows of the master of two assets, want 2", len(shared))
		}
		switch want := []byte{1, 2}; {
		case !slices.Equal(shared[0].Data, want) || !slices.Equal(shared[1].Data, want):
			t.Errorf("the rows of one master hold %v and %v, want %v", shared[0].Data, shared[1].Data, want)
		case &shared[0].Data[0] != &shared[1].Data[0]:
			t.Error("the rows of one master hold a copy each")
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
		"DELETE FROM ZPERSON",
		"DELETE FROM ZUSERFEEDBACK",
		"DELETE FROM ZINTERNALRESOURCE",
		"DELETE FROM ZEXTENDEDATTRIBUTES",
		"DELETE FROM ZADDITIONALASSETATTRIBUTES",
		"DELETE FROM ZASSET",
		"DELETE FROM ZMOMENT",
		"DELETE FROM ZGENERICALBUM",
		"DELETE FROM ZSHARE",
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
