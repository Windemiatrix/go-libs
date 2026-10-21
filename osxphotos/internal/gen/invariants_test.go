package gen

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// invariant is a query that returns the number of rows that break a rule.
// real means the rule holds in any Photos library, not only in a generated
// one; such rules are also checked on the osxphotos test library Test-26.1.
type invariant struct {
	name  string
	query string
	real  bool
}

const quickCheck = "quick-check" // checked by a loop over the tables, not by one query

// uuidGlob matches an upper-case UUID.
var uuidGlob = strings.Join([]string{
	strings.Repeat("[0-9A-F]", 8), strings.Repeat("[0-9A-F]", 4), strings.Repeat("[0-9A-F]", 4),
	strings.Repeat("[0-9A-F]", 4), strings.Repeat("[0-9A-F]", 12),
}, "-")

// rootTables maps the tables the generator writes to the root entity of
// their rows (the table of a Core Data root entity is "Z" + its upper-case name).
var rootTables = []struct{ table, root string }{
	{"ZADDITIONALASSETATTRIBUTES", "AdditionalAssetAttributes"},
	{"ZASSET", "Asset"},
	{"ZCLOUDMASTER", "CloudMaster"},
	{"ZGENERICALBUM", "GenericAlbum"},
	{"ZINTERNALRESOURCE", "InternalResource"},
	{"ZSHARE", "Share"},
}

// violations sums the conditions over the rows of table; a condition that
// is NULL counts as kept, so each one must be written NULL-safe.
func violations(table string, conds ...string) string {
	parts := make([]string, len(conds))
	for i, c := range conds {
		parts[i] = "IFNULL(SUM(IFNULL(" + c + ", 0) != 0), 0)"
	}
	return "SELECT " + strings.Join(parts, " + ") + " FROM " + table
}

// notNullColumns lists the columns, as {table, column}, that the generator
// always fills and that upstream reads without a NULL check.
func notNullColumns(sc schema) [][2]string {
	var out [][2]string
	add := func(table string, cols ...string) {
		for _, c := range cols {
			out = append(out, [2]string{table, c})
		}
	}
	add("ZASSET", "ZDEPTHTYPE", "ZHIDDEN", "ZFAVORITE", "ZTRASHEDSTATE", "ZKIND", "ZKINDSUBTYPE",
		"ZVISIBILITYSTATE", "ZAVALANCHEPICKTYPE", "ZHDRTYPE", "ZSAVEDASSETTYPE", "ZSYNDICATIONSTATE",
		"ZDATECREATED", "ZADDEDDATE", "ZFILENAME", "ZDIRECTORY", "ZUNIFORMTYPEIDENTIFIER")
	add("ZADDITIONALASSETATTRIBUTES", "ZORIGINALFILENAME", "ZORIGINALFILESIZE", sc.fingerprint)
	add("ZINTERNALRESOURCE", "ZVERSION", "ZRECIPEID", "ZCOMPACTUTI")
	add("ZGENERICALBUM", "ZTRASHEDSTATE")
	return out
}

func invariants(sc schema) []invariant {
	fp, adj := sc.fingerprint, sc.adjustments
	mt, ma, ms := sc.members, sc.memberAlbum, sc.memberAsset
	libraryScope := sc.entities["LibraryScope"].ent

	// Extension of the original file name and of ZFILENAME by UTI.
	originalExt := "CASE a.ZUNIFORMTYPEIDENTIFIER WHEN 'public.jpeg' THEN 'JPG' WHEN 'public.heic' THEN 'HEIC' " +
		"WHEN 'com.adobe.raw-image' THEN 'DNG' WHEN 'public.png' THEN 'PNG' " +
		"WHEN 'com.apple.quicktime-movie' THEN 'MOV' WHEN 'public.mpeg-4' THEN 'MP4' ELSE '?' END"

	var notNull []string
	for _, tc := range notNullColumns(sc) {
		notNull = append(notNull, "(SELECT COUNT(*) FROM "+tc[0]+" WHERE "+tc[1]+" IS NULL)")
	}

	// roots maps every Z_ENT of Z_PRIMARYKEY to the name of its root entity.
	const roots = "WITH RECURSIVE up(ent, cur) AS (SELECT Z_ENT, Z_ENT FROM Z_PRIMARYKEY " +
		"UNION ALL SELECT up.ent, k.Z_SUPER FROM up JOIN Z_PRIMARYKEY k ON k.Z_ENT = up.cur WHERE k.Z_SUPER != 0), " +
		"roots(ent, root) AS (SELECT up.ent, k.Z_NAME FROM up JOIN Z_PRIMARYKEY k ON k.Z_ENT = up.cur WHERE k.Z_SUPER = 0) "
	var entParts, maxAtLeast, maxEqual []string
	for _, rt := range rootTables {
		entParts = append(entParts, "(SELECT COUNT(*) FROM "+rt.table+" t LEFT JOIN roots r ON r.ent = t.Z_ENT "+
			"WHERE r.root IS NOT '"+rt.root+"')")
		rootRow := "FROM Z_PRIMARYKEY WHERE Z_NAME = '" + rt.root + "' AND Z_SUPER = 0"
		maxPK := "(SELECT IFNULL(MAX(Z_PK), 0) FROM " + rt.table + ")"
		maxAtLeast = append(maxAtLeast, "(SELECT COUNT(*) != 1 "+rootRow+")",
			"(SELECT COUNT(*) "+rootRow+" AND Z_MAX < "+maxPK+")")
		maxEqual = append(maxEqual, "(SELECT COUNT(*) "+rootRow+" AND Z_MAX != "+maxPK+")")
	}

	return []invariant{
		{name: "attrs-per-asset", real: true, query: "SELECT " +
			"(SELECT COUNT(*) FROM (SELECT a.Z_PK FROM ZASSET a LEFT JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK " +
			"GROUP BY a.Z_PK HAVING COUNT(x.Z_PK) != 1)) + " +
			"(SELECT COUNT(*) FROM ZASSET a LEFT JOIN ZADDITIONALASSETATTRIBUTES x ON x.Z_PK = a.ZADDITIONALATTRIBUTES " +
			"WHERE x.Z_PK IS NULL OR x.ZASSET IS NOT a.Z_PK) + " +
			"(SELECT COUNT(*) FROM ZADDITIONALASSETATTRIBUTES x LEFT JOIN ZASSET a ON a.Z_PK = x.ZASSET WHERE a.Z_PK IS NULL)"},
		{name: "kinds", real: true, query: "SELECT COUNT(*) FROM ZASSET WHERE IFNULL(ZKIND, -1) NOT IN (0, 1)"},
		{name: "kind-subtypes", query: "SELECT COUNT(*) FROM ZASSET WHERE NOT IFNULL((ZKIND = 0 AND ZKINDSUBTYPE IN (0, 2, 10)) " +
			"OR (ZKIND = 1 AND ZKINDSUBTYPE = 0), 0)"},
		{name: "names", query: violations("ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK",
			"NOT IFNULL(x.ZORIGINALFILENAME LIKE '%.' || "+originalExt+", 0)",
			"IFNULL(a.ZFILENAME = x.ZORIGINALFILENAME, 1)",
			"NOT IFNULL(a.ZDIRECTORY = substr(a.ZFILENAME, 1, 1) AND a.ZFILENAME LIKE a.ZUUID || '.%', 0)")},
		// SUM of no rows is NULL: IFNULL keeps an empty table from hiding the others.
		{name: "uuids", real: true, query: "SELECT " +
			"(SELECT COUNT(*) - COUNT(DISTINCT ZUUID) + IFNULL(SUM(NOT IFNULL(ZUUID GLOB '" + uuidGlob + "', 0)), 0) FROM ZASSET) + " +
			"(SELECT COUNT(*) - COUNT(DISTINCT ZUUID) + IFNULL(SUM(NOT IFNULL(ZUUID GLOB '" + uuidGlob + "', 0)), 0) FROM ZGENERICALBUM)"},
		{name: "cloud-guids", query: "SELECT " +
			"(SELECT COUNT(*) - COUNT(DISTINCT ZCLOUDASSETGUID) + IFNULL(SUM(ZCLOUDASSETGUID IS NULL), 0) FROM ZASSET) + " +
			"(SELECT COUNT(*) - COUNT(DISTINCT ZCLOUDMASTERGUID) + IFNULL(SUM(ZCLOUDMASTERGUID IS NULL), 0) FROM ZCLOUDMASTER) + " +
			"(SELECT COUNT(*) - COUNT(DISTINCT ZCLOUDGUID) + IFNULL(SUM(ZCLOUDGUID IS NULL), 0) FROM ZGENERICALBUM) + " +
			"(SELECT COUNT(*) FROM ZASSET a JOIN ZCLOUDMASTER m ON m.Z_PK = a.ZMASTER WHERE a.ZCLOUDASSETGUID = m.ZCLOUDMASTERGUID)"},
		{name: "master", query: "SELECT COUNT(*) FROM ZASSET a LEFT JOIN ZCLOUDMASTER m ON m.Z_PK = a.ZMASTER " +
			"WHERE m.Z_PK IS NULL OR m.ZCLOUDLOCALSTATE IS NOT 3"},
		{name: "original-count", real: true, query: "SELECT COUNT(*) FROM (SELECT a.Z_PK FROM ZASSET a " +
			"LEFT JOIN ZINTERNALRESOURCE r ON r.ZASSET = a.Z_PK AND r.ZDATASTORESUBTYPE = 1 GROUP BY a.Z_PK HAVING COUNT(r.Z_PK) != 1)"},
		{name: "original-size", real: true, query: "SELECT COUNT(*) FROM ZASSET a " +
			"JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK " +
			"JOIN ZINTERNALRESOURCE r ON r.ZASSET = a.Z_PK AND r.ZDATASTORESUBTYPE = 1 WHERE x.ZORIGINALFILESIZE IS NOT r.ZDATALENGTH"},
		// Not real: in Test-26.1 most original resources have no fingerprint.
		{name: "original-fingerprint", query: "SELECT COUNT(*) FROM ZASSET a " +
			"JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK " +
			"JOIN ZINTERNALRESOURCE r ON r.ZASSET = a.Z_PK AND r.ZDATASTORESUBTYPE = 1 WHERE x." + fp + " IS NOT r.ZFINGERPRINT"},
		// Assets of the same content come in pairs of one UTI and one size;
		// the exact number of pairs is checked by the profile tests.
		{name: "same-content", query: "SELECT COUNT(*) FROM (SELECT x." + fp + " FROM ZADDITIONALASSETATTRIBUTES x " +
			"JOIN ZASSET a ON a.Z_PK = x.ZASSET GROUP BY x." + fp + " HAVING COUNT(*) > 2 " +
			"OR COUNT(DISTINCT a.ZUNIFORMTYPEIDENTIFIER) > 1 OR COUNT(DISTINCT x.ZORIGINALFILESIZE) > 1)"},
		{name: "live-video", query: "SELECT COUNT(*) FROM (SELECT IFNULL(a.ZKINDSUBTYPE = 2, 0) AS live, COUNT(r.Z_PK) AS n " +
			"FROM ZASSET a LEFT JOIN ZINTERNALRESOURCE r ON r.ZASSET = a.Z_PK AND r.ZRESOURCETYPE = 3 AND r.ZDATASTORESUBTYPE = 18 " +
			"GROUP BY a.Z_PK) WHERE (live AND n != 1) OR (NOT live AND n != 0)"},
		{name: "raw-pair", query: "SELECT COUNT(*) FROM (SELECT r.ZASSET FROM ZINTERNALRESOURCE r " +
			"LEFT JOIN ZASSET a ON a.Z_PK = r.ZASSET WHERE r.ZDATASTORESUBTYPE = 3 " +
			"GROUP BY r.ZASSET HAVING COUNT(*) > 1 OR MAX(IFNULL(a.ZKIND, -1)) != 0 OR MIN(IFNULL(a.ZKIND, -1)) != 0)"},
		{name: "not-null", query: "SELECT " + strings.Join(notNull, " + ")},
		{name: "flags-dates", query: violations("ZASSET",
			"IFNULL(ZHIDDEN, 0) NOT IN (0, 1) OR IFNULL(ZFAVORITE, 0) NOT IN (0, 1) OR IFNULL(ZTRASHEDSTATE, 0) NOT IN (0, 1)",
			"ZHIDDEN = 1 AND ZTRASHEDSTATE = 1",
			"ZDATECREATED > ZADDEDDATE",
			"(ZTRASHEDDATE IS NOT NULL) != IFNULL(ZTRASHEDSTATE = 1, 0) OR ZTRASHEDDATE < ZADDEDDATE",
			"(ZADJUSTMENTTIMESTAMP IS NOT NULL) != IFNULL("+adj+" > 0, 0)")},
		{name: "bursts", query: "SELECT " +
			"(SELECT COUNT(*) FROM (SELECT ZAVALANCHEUUID FROM ZASSET WHERE ZAVALANCHEUUID IS NOT NULL GROUP BY ZAVALANCHEUUID " +
			"HAVING COUNT(*) < 2 OR SUM(ZVISIBILITYSTATE = 0) = 0)) + " +
			"(SELECT COUNT(*) FROM ZASSET WHERE IFNULL(ZVISIBILITYSTATE, -1) NOT IN (0, 2) " +
			"OR (ZVISIBILITYSTATE = 2 AND ZAVALANCHEUUID IS NULL))"},
		// upstream fails unless there is exactly one root folder
		{name: "album-root", real: true, query: "SELECT " +
			"(SELECT COUNT(*) != 1 FROM ZGENERICALBUM WHERE ZKIND = 3999) + " +
			"(SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 3999 AND ZPARENTFOLDER IS NOT NULL)"},
		{name: "album-tree", query: "SELECT COUNT(*) FROM ZGENERICALBUM g LEFT JOIN ZGENERICALBUM p ON p.Z_PK = g.ZPARENTFOLDER " +
			"WHERE (g.ZKIND = 4000 AND IFNULL(p.ZKIND, 0) != 3999) OR (g.ZKIND = 2 AND IFNULL(p.ZKIND, 0) NOT IN (3999, 4000)) " +
			"OR IFNULL(g.ZKIND, 0) NOT IN (2, 3999, 4000)"},
		{name: "membership-refs", real: true, query: "SELECT COUNT(*) FROM " + mt + " m " +
			"LEFT JOIN ZASSET a ON a.Z_PK = m." + ms + " LEFT JOIN ZGENERICALBUM g ON g.Z_PK = m." + ma +
			" WHERE a.Z_PK IS NULL OR g.Z_PK IS NULL"},
		{name: "membership", query: "SELECT " +
			"(SELECT COUNT(*) FROM " + mt + " m JOIN ZASSET a ON a.Z_PK = m." + ms + " JOIN ZGENERICALBUM g ON g.Z_PK = m." + ma +
			" WHERE g.ZKIND IS NOT 2 OR a.ZVISIBILITYSTATE IS NOT 0) + " +
			"(SELECT COUNT(*) FROM ZGENERICALBUM g LEFT JOIN (SELECT m." + ma + " AS album, MIN(a.ZDATECREATED) AS lo, " +
			"MAX(a.ZDATECREATED) AS hi FROM " + mt + " m JOIN ZASSET a ON a.Z_PK = m." + ms + " GROUP BY m." + ma + ") s " +
			"ON s.album = g.Z_PK WHERE g.ZSTARTDATE IS NOT s.lo OR g.ZENDDATE IS NOT s.hi)"},
		{name: "primary-keys", real: true, query: roots + "SELECT " + strings.Join(slices.Concat(entParts, maxAtLeast), " + ")},
		{name: "primary-keys-max", query: "SELECT " + strings.Join(maxEqual, " + ")},
		{name: "library-scope", query: "SELECT COUNT(*) FROM ZASSET a " +
			"LEFT JOIN ZSHARE s ON s.Z_PK = a.ZLIBRARYSCOPE AND s.Z_ENT = " + fmt.Sprint(libraryScope) +
			" WHERE (a.ZLIBRARYSCOPE IS NOT NULL AND s.Z_PK IS NULL) " +
			"OR ((a.ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE IS 1) != (a.ZLIBRARYSCOPE IS NOT NULL))"},
		{name: "derived", real: true, query: "SELECT COUNT(*) FROM ZASSET a " +
			"JOIN ZADDITIONALASSETATTRIBUTES x ON x.Z_PK = a.ZADDITIONALATTRIBUTES " +
			"WHERE a.ZDERIVEDCAMERACAPTUREDEVICE IS NOT x.ZCAMERACAPTUREDEVICE"},
		{name: "metadata", real: true, query: "SELECT ABS(COUNT(*) - 1) + IFNULL(SUM(typeof(Z_PLIST) != 'blob'), 0) FROM Z_METADATA"},
		// Not real: the plist of a real library is binary.
		{name: "metadata-model", query: "SELECT COUNT(*) FROM Z_METADATA WHERE IFNULL(instr(" +
			"replace(replace(replace(CAST(Z_PLIST AS TEXT), char(10), ''), char(9), ''), ' ', ''), " +
			"'<key>PLModelVersion</key><integer>" + fmt.Sprint(sc.model) + "</integer>'), 0) = 0"},
		{name: quickCheck, real: true},
	}
}

// checkInvariants returns the number of violations of every invariant; a
// query that fails or returns NULL counts as one violation.
func checkInvariants(ctx context.Context, db *sql.DB, sc schema) map[string]int64 {
	out := map[string]int64{}
	for _, inv := range invariants(sc) {
		n, err := inv.count(ctx, db)
		if err != nil {
			n = 1
		}
		out[inv.name] = n
	}
	return out
}

func (inv invariant) count(ctx context.Context, db *sql.DB) (int64, error) {
	if inv.name == quickCheck {
		return quickCheckTables(ctx, db)
	}
	var n sql.NullInt64
	if err := db.QueryRowContext(ctx, inv.query).Scan(&n); err != nil {
		return 0, err
	}
	// NULL is not 0: a sum over an empty table would hide every violation.
	if !n.Valid {
		return 0, fmt.Errorf("invariant %s: the query returns NULL", inv.name)
	}
	return n.Int64, nil
}

// quickCheckTables runs quick_check on every ordinary table, as checkDB of
// testlib does (virtual tables are skipped: SQLite 3.44+ checks them too) and
// returns the number of tables that fail.
func quickCheckTables(ctx context.Context, db *sql.DB) (int64, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL TABLE%' ORDER BY name")
	if err != nil {
		return 0, err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return 0, err
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	var bad int64
	for _, table := range tables {
		var res string
		if err := db.QueryRowContext(ctx, "SELECT quick_check FROM pragma_quick_check(?)", table).Scan(&res); err != nil || res != "ok" {
			bad++
		}
	}
	return bad, nil
}

// assertInvariants fails the test for every invariant that does not hold;
// with realOnly only the real ones are checked.
func assertInvariants(t testing.TB, db *sql.DB, sc schema, realOnly bool) {
	t.Helper()
	for _, inv := range invariants(sc) {
		if realOnly && !inv.real {
			continue
		}
		n, err := inv.count(t.Context(), db)
		switch {
		case err != nil:
			t.Errorf("invariant %s: %v", inv.name, err)
		case n != 0:
			t.Errorf("invariant %s: %d violations", inv.name, n)
		}
	}
}

func TestInvariantsHold(t *testing.T) {
	sc := schemas[DefaultSchema]
	tests := []struct {
		name string
		cfg  Config
	}{
		{"default 300", func() Config { c := DefaultConfig(); c.Assets = 300; return c }()},
		{"default 2000", func() Config { c := DefaultConfig(); c.Assets = 2000; return c }()},
		{"zero shares", zeroConfig(100)},
		{"empty", func() Config { c := DefaultConfig(); c.Assets = 0; return c }()},
		{"other seed", func() Config { c := DefaultConfig(); c.Assets = 500; c.Seed = 42; return c }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, db := generate(t, tt.cfg)
			assertInvariants(t, db, sc, false)
		})
	}
}

// TestInvariantsCatch corrupts a generated database one statement at a time
// and checks that the invariant the statement breaks reports it.
func TestInvariantsCatch(t *testing.T) {
	sc := schemas[DefaultSchema]
	cfg := DefaultConfig()
	cfg.Assets = 300
	dir, db := generate(t, cfg)
	if got := checkInvariants(t.Context(), db, sc); slices.ContainsFunc(slices.Collect(maps.Values(got)), func(n int64) bool { return n != 0 }) {
		t.Fatalf("the generated database breaks invariants: %v", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	pristine, err := os.ReadFile(dbPath(dir))
	if err != nil {
		t.Fatal(err)
	}

	fp, adj := sc.fingerprint, sc.adjustments
	mt, ma, ms, mo := sc.members, sc.memberAlbum, sc.memberAsset, sc.memberOrder
	const (
		firstAlbum  = "(SELECT MIN(Z_PK) FROM ZGENERICALBUM WHERE ZKIND = 2)"
		firstVideo  = "(SELECT MIN(Z_PK) FROM ZASSET WHERE ZKIND = 1)"
		firstPhoto  = "(SELECT MIN(Z_PK) FROM ZASSET WHERE ZKIND = 0 AND ZKINDSUBTYPE = 0 AND ZAVALANCHEUUID IS NULL)"
		firstRAW    = "(SELECT MIN(ZASSET) FROM ZINTERNALRESOURCE WHERE ZDATASTORESUBTYPE = 3)"
		firstDNG    = "(SELECT MIN(Z_PK) FROM ZASSET WHERE ZUNIFORMTYPEIDENTIFIER = 'com.adobe.raw-image')"
		firstShared = "(SELECT MIN(Z_PK) FROM ZASSET WHERE ZLIBRARYSCOPE IS NOT NULL)"
		masterOf1   = "(SELECT ZMASTER FROM ZASSET WHERE Z_PK = 1)"
		attrsOf1    = "(SELECT ZADDITIONALATTRIBUTES FROM ZASSET WHERE Z_PK = 1)"
		invisible   = "(SELECT MIN(Z_PK) FROM ZASSET WHERE ZVISIBILITYSTATE = 2)"
	)
	tests := []struct {
		invariant string
		corrupt   string // empty for the page corruption of quick-check
	}{
		{"attrs-per-asset", "DELETE FROM ZADDITIONALASSETATTRIBUTES WHERE Z_PK = " + attrsOf1},
		{"attrs-per-asset", "UPDATE ZADDITIONALASSETATTRIBUTES SET ZASSET = 999999 WHERE Z_PK = 1"},
		{"attrs-per-asset", "UPDATE ZADDITIONALASSETATTRIBUTES SET ZASSET = 2 WHERE Z_PK = " + attrsOf1},
		{"kinds", "UPDATE ZASSET SET ZKIND = 7 WHERE Z_PK = 1"},
		{"kind-subtypes", "UPDATE ZASSET SET ZKINDSUBTYPE = 10 WHERE Z_PK = " + firstVideo},
		{"names", "UPDATE ZADDITIONALASSETATTRIBUTES SET ZORIGINALFILENAME = 'IMG_0001.XYZ' WHERE Z_PK = 1"},
		// The original name of a DNG asset becomes "<ZUUID>.dng": LIKE '%.DNG'
		// ignores case, so only the rule ZFILENAME != original breaks.
		{"names", "UPDATE ZADDITIONALASSETATTRIBUTES SET ZORIGINALFILENAME = (SELECT a.ZFILENAME FROM ZASSET a " +
			"WHERE a.Z_PK = ZADDITIONALASSETATTRIBUTES.ZASSET) WHERE ZASSET = " + firstDNG},
		{"names", "UPDATE ZASSET SET ZDIRECTORY = 'x' WHERE Z_PK = 1"},
		{"uuids", "UPDATE ZASSET SET ZUUID = (SELECT ZUUID FROM ZASSET WHERE Z_PK = 2) WHERE Z_PK = 1"},
		{"uuids", "UPDATE ZASSET SET ZUUID = lower(ZUUID) WHERE Z_PK = 1"},
		{"uuids", "UPDATE ZGENERICALBUM SET ZUUID = (SELECT ZUUID FROM ZGENERICALBUM WHERE Z_PK = 1) WHERE Z_PK = 2"},
		{"uuids", "UPDATE ZGENERICALBUM SET ZUUID = lower(ZUUID) WHERE Z_PK = " + firstAlbum},
		{"cloud-guids", "UPDATE ZASSET SET ZCLOUDASSETGUID = (SELECT ZCLOUDASSETGUID FROM ZASSET WHERE Z_PK = 2) WHERE Z_PK = 1"},
		{"cloud-guids", "UPDATE ZASSET SET ZCLOUDASSETGUID = NULL WHERE Z_PK = 1"},
		{"cloud-guids", "UPDATE ZCLOUDMASTER SET ZCLOUDMASTERGUID = (SELECT ZCLOUDMASTERGUID FROM ZCLOUDMASTER " +
			"WHERE Z_PK = " + masterOf1 + ") WHERE Z_PK = " + masterOf1 + " + 1"},
		{"cloud-guids", "UPDATE ZCLOUDMASTER SET ZCLOUDMASTERGUID = NULL WHERE Z_PK = " + masterOf1},
		{"cloud-guids", "UPDATE ZGENERICALBUM SET ZCLOUDGUID = (SELECT ZCLOUDGUID FROM ZGENERICALBUM WHERE Z_PK = 1) WHERE Z_PK = 2"},
		{"cloud-guids", "UPDATE ZGENERICALBUM SET ZCLOUDGUID = NULL WHERE Z_PK = " + firstAlbum},
		{"cloud-guids", "UPDATE ZCLOUDMASTER SET ZCLOUDMASTERGUID = (SELECT ZCLOUDASSETGUID FROM ZASSET WHERE Z_PK = 1) " +
			"WHERE Z_PK = " + masterOf1},
		{"master", "UPDATE ZCLOUDMASTER SET ZCLOUDLOCALSTATE = 1 WHERE Z_PK = " + masterOf1},
		{"master", "UPDATE ZASSET SET ZMASTER = NULL WHERE Z_PK = 1"},
		{"original-count", "DELETE FROM ZINTERNALRESOURCE WHERE ZASSET = 1 AND ZDATASTORESUBTYPE = 1"},
		{"original-size", "UPDATE ZADDITIONALASSETATTRIBUTES SET ZORIGINALFILESIZE = ZORIGINALFILESIZE + 1 WHERE Z_PK = 1"},
		{"original-fingerprint", "UPDATE ZINTERNALRESOURCE SET ZFINGERPRINT = 'x' WHERE ZASSET = 1 AND ZDATASTORESUBTYPE = 1"},
		{"same-content", "UPDATE ZADDITIONALASSETATTRIBUTES SET " + fp + " = (SELECT " + fp +
			" FROM ZADDITIONALASSETATTRIBUTES WHERE Z_PK = 1) WHERE Z_PK IN (2, 3)"},
		{"live-video", "DELETE FROM ZINTERNALRESOURCE WHERE ZDATASTORESUBTYPE = 18"},
		{"live-video", "UPDATE ZASSET SET ZKINDSUBTYPE = 2 WHERE Z_PK = " + firstPhoto},
		{"raw-pair", "INSERT INTO ZINTERNALRESOURCE (ZASSET, ZDATASTORESUBTYPE, ZRESOURCETYPE, ZVERSION, ZRECIPEID, ZCOMPACTUTI) " +
			"VALUES (" + firstVideo + ", 3, 4, 0, 0, 11)"},
		{"raw-pair", "INSERT INTO ZINTERNALRESOURCE (ZASSET, ZDATASTORESUBTYPE, ZRESOURCETYPE, ZVERSION, ZRECIPEID, ZCOMPACTUTI) " +
			"VALUES (" + firstRAW + ", 3, 4, 0, 0, 12)"},
		{"flags-dates", "UPDATE ZASSET SET ZFAVORITE = 3 WHERE Z_PK = 1"},
		// The update of ZHIDDEN fires a trigger that calls a Core Data function (stand-in of testlib).
		{"flags-dates", "UPDATE ZASSET SET ZHIDDEN = 1 WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZASSET WHERE ZTRASHEDSTATE = 1)"},
		{"flags-dates", "UPDATE ZASSET SET ZADDEDDATE = ZDATECREATED - 1 WHERE Z_PK = 1"},
		{"flags-dates", "UPDATE ZASSET SET ZTRASHEDDATE = ZADDEDDATE WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZASSET WHERE ZTRASHEDSTATE = 0)"},
		{"flags-dates", "UPDATE ZASSET SET ZADJUSTMENTTIMESTAMP = NULL WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZASSET WHERE " + adj + " > 0)"},
		{"bursts", "UPDATE ZASSET SET ZAVALANCHEUUID = 'X' WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZASSET WHERE ZAVALANCHEUUID IS NOT NULL)"},
		// The update of ZVISIBILITYSTATE fires a trigger that calls a Core Data function (stand-in of testlib).
		{"bursts", "UPDATE ZASSET SET ZVISIBILITYSTATE = 2 WHERE Z_PK = " + firstPhoto},
		{"album-root", "INSERT INTO ZGENERICALBUM (Z_PK, Z_ENT, Z_OPT, ZKIND, ZTRASHEDSTATE) VALUES (999999, 41, 1, 3999, 0)"},
		{"album-root", "UPDATE ZGENERICALBUM SET ZPARENTFOLDER = 2 WHERE ZKIND = 3999"},
		{"album-tree", "UPDATE ZGENERICALBUM SET ZPARENTFOLDER = NULL WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZGENERICALBUM WHERE ZKIND = 4000)"},
		{"album-tree", "UPDATE ZGENERICALBUM SET ZKIND = 1505 WHERE Z_PK = " + firstAlbum},
		{"membership-refs", "INSERT INTO " + mt + " (" + ma + ", " + ms + ", " + mo + ") VALUES (" + firstAlbum + ", 999999, 1)"},
		{"membership-refs", "INSERT INTO " + mt + " (" + ma + ", " + ms + ", " + mo + ") VALUES (999999, 1, 1)"},
		{"membership", "UPDATE ZGENERICALBUM SET ZSTARTDATE = ZSTARTDATE - 1 WHERE Z_PK = (SELECT MIN(" + ma + ") FROM " + mt + ")"},
		{"membership", "UPDATE ZGENERICALBUM SET ZENDDATE = 0 WHERE Z_PK = " + firstAlbum},
		{"membership", "INSERT INTO " + mt + " (" + ma + ", " + ms + ", " + mo + ") VALUES (" + firstAlbum + ", " + invisible + ", 1)"},
		// An album with members becomes a folder: its rows now point at a
		// folder, and its dates still match them.
		{"membership", "UPDATE ZGENERICALBUM SET ZKIND = 4000 WHERE Z_PK = (SELECT MIN(" + ma + ") FROM " + mt + ")"},
		{"primary-keys", "UPDATE Z_PRIMARYKEY SET Z_MAX = 0 WHERE Z_NAME = 'Asset'"},
		{"primary-keys", "UPDATE ZASSET SET Z_ENT = 33 WHERE Z_PK = 1"},
		{"primary-keys", "UPDATE ZCLOUDMASTER SET Z_ENT = 999 WHERE Z_PK = " + masterOf1},
		{"primary-keys", "DELETE FROM Z_PRIMARYKEY WHERE Z_NAME = 'InternalResource'"},
		{"primary-keys-max", "UPDATE Z_PRIMARYKEY SET Z_MAX = Z_MAX + 1 WHERE Z_NAME = 'GenericAlbum'"},
		{"library-scope", "UPDATE ZASSET SET ZLIBRARYSCOPE = 99 WHERE Z_PK = " + firstShared},
		{"library-scope", "UPDATE ZASSET SET ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE = 0 WHERE Z_PK = " + firstShared},
		{"derived", "UPDATE ZASSET SET ZDERIVEDCAMERACAPTUREDEVICE = 1 WHERE Z_PK = 1"},
		{"metadata", "UPDATE Z_METADATA SET Z_PLIST = CAST(Z_PLIST AS TEXT)"},
		{"metadata", "INSERT INTO Z_METADATA (Z_VERSION, Z_UUID) VALUES (2, 'X')"},
		{"metadata-model", fmt.Sprintf("UPDATE Z_METADATA SET Z_PLIST = CAST(replace(CAST(Z_PLIST AS TEXT), '%d', '1') AS BLOB)", sc.model)},
		{quickCheck, ""},
	}
	for _, tc := range notNullColumns(sc) {
		tests = append(tests, struct{ invariant, corrupt string }{"not-null", "UPDATE " + tc[0] + " SET " + tc[1] + " = NULL WHERE Z_PK = 1"})
	}

	covered := map[string]bool{}
	for _, tt := range tests {
		covered[tt.invariant] = true
	}
	for _, inv := range invariants(sc) {
		if !covered[inv.name] {
			t.Errorf("invariant %s has no corruption case", inv.name)
		}
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%02d-%s", i, tt.invariant), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "Photos.sqlite")
			if err := os.WriteFile(path, pristine, 0o600); err != nil { //nolint:gosec // G703: a file in t.TempDir()
				t.Fatal(err)
			}
			if tt.corrupt == "" {
				corruptTablePage(t, path, "ZASSET")
			} else {
				exec(t, openDB(t, path), tt.corrupt)
			}
			db := openDB(t, path)
			if tt.corrupt != "" {
				// The query itself must run: a failing query would count as a
				// violation and hide a rule that misses the corruption.
				i := slices.IndexFunc(invariants(sc), func(inv invariant) bool { return inv.name == tt.invariant })
				if _, err := invariants(sc)[i].count(t.Context(), db); err != nil {
					t.Fatalf("invariant %s: %v", tt.invariant, err)
				}
			}
			got := checkInvariants(t.Context(), db, sc)
			if got[tt.invariant] == 0 {
				t.Errorf("%s: invariant %s does not see the corruption (all: %v)", tt.corrupt, tt.invariant, got)
			}
		})
	}
}

// TestInvariantsCatchEmpty: in a library without assets the sums over the
// empty tables do not hide a broken root folder.
func TestInvariantsCatchEmpty(t *testing.T) {
	t.Parallel()
	sc := schemas[DefaultSchema]
	cfg := DefaultConfig()
	cfg.Assets = 0
	for _, tt := range []struct{ invariant, corrupt string }{
		{"uuids", "UPDATE ZGENERICALBUM SET ZUUID = lower(ZUUID)"},
		{"cloud-guids", "UPDATE ZGENERICALBUM SET ZCLOUDGUID = NULL"},
	} {
		t.Run(tt.invariant, func(t *testing.T) {
			t.Parallel()
			_, db := generate(t, cfg)
			exec(t, db, tt.corrupt)
			i := slices.IndexFunc(invariants(sc), func(inv invariant) bool { return inv.name == tt.invariant })
			n, err := invariants(sc)[i].count(t.Context(), db)
			if err != nil || n == 0 {
				t.Errorf("%s: invariant %s gives %d, %v; want violations", tt.corrupt, tt.invariant, n, err)
			}
		})
	}
}

// TestInvariantNull: a query that returns NULL is an error, not 0 violations.
func TestInvariantNull(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Assets = 0
	_, db := generate(t, cfg)
	if n, err := (invariant{name: "null", query: "SELECT SUM(Z_PK) FROM ZASSET"}).count(t.Context(), db); err == nil {
		t.Errorf("a NULL result gives %d violations and no error", n)
	}
}

// corruptTablePage overwrites the root page of table with 0xFF bytes in the
// closed database file.
func corruptTablePage(t *testing.T, path, table string) {
	t.Helper()
	db := openDB(t, path)
	page := count(t, db, "SELECT rootpage FROM sqlite_master WHERE type = 'table' AND name = '"+table+"'")
	size := count(t, db, "PRAGMA page_size")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	junk := make([]byte, size)
	for i := range junk {
		junk[i] = 0xff
	}
	if _, err := f.WriteAt(junk, (page-1)*size); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestTestLibrary checks the schema table against Z_PRIMARYKEY of the
// osxphotos test library Test-26.1 (the same schema, an earlier model) and
// the real invariants against that library.
func TestTestLibrary(t *testing.T) {
	lib := testlib.OpenPristine(t, "Test-26.1")
	db := openDB(t, dbPath(lib))
	sc := schemas["photos11_1"]
	for _, name := range slices.Sorted(maps.Keys(sc.entities)) {
		e := sc.entities[name]
		got := scalar(t, db, "SELECT Z_ENT || '/' || Z_SUPER FROM Z_PRIMARYKEY WHERE Z_NAME = '"+name+"'")
		if want := fmt.Sprintf("%d/%d", e.ent, e.super); got != want {
			t.Errorf("entity %s: Z_ENT/Z_SUPER in Test-26.1 %s, in the schema table %s", name, got, want)
		}
	}
	assertInvariants(t, db, sc, true)
}
