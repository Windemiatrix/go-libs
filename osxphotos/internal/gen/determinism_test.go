package gen

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"testing"
)

// dump returns the SHA-256 of the schema and of the content of every
// ordinary table (values with their types, rows by rowid), keyed by
// "sqlite_master" or the table name.
func dump(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	out := map[string]string{}
	out["sqlite_master"] = hashRows(t, db, "SELECT type, name, sql FROM sqlite_master ORDER BY type, name")
	for _, table := range rowStrings(t, db,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL TABLE%' ORDER BY name") {
		out[table] = hashRows(t, db, `SELECT * FROM "`+table+`" ORDER BY rowid`)
	}
	return out
}

func hashRows(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		for _, v := range vals {
			s := fmt.Sprint(v)
			if b, ok := v.([]byte); ok {
				s = hex.EncodeToString(b)
			}
			_, _ = fmt.Fprintf(h, "%T:%s\x00", v, s)
		}
		_, _ = h.Write([]byte{'\n'})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func diffTables(a, b map[string]string) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(a)) {
		if a[k] != b[k] {
			out = append(out, k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(b)) {
		if _, ok := a[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}

func TestDeterministic(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Assets = 500
	_, db1 := generate(t, cfg)
	_, db2 := generate(t, cfg)
	if diff := diffTables(dump(t, db1), dump(t, db2)); len(diff) > 0 {
		t.Errorf("the same config gives different tables: %v", diff)
	}

	cfg.Seed++
	_, db3 := generate(t, cfg)
	diff := diffTables(dump(t, db1), dump(t, db3))
	for _, table := range []string{"ZASSET", "ZADDITIONALASSETATTRIBUTES", "ZINTERNALRESOURCE", "ZGENERICALBUM", "Z_METADATA"} {
		if !slices.Contains(diff, table) {
			t.Errorf("another seed gives the same %s", table)
		}
	}
}

// TestPinned pins a small library: a few values to show what moved, and the
// SHA-256 of every table the generator writes. Changing them makes the
// reference outputs generated from synthetic libraries (golden-regen) stale:
// change them only together with those references. CI runs this test on
// amd64 and arm64, so it also checks that the content does not depend on
// the architecture.
func TestPinned(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Assets = 50
	cfg.Seed = 1
	_, db := generate(t, cfg)
	sc := schemas[DefaultSchema]
	runChecks(t, db, []check{
		{"asset UUID", "SELECT ZUUID FROM ZASSET WHERE Z_PK = 1", "244A8B9E-B813-4C7D-B278-48CE298511FC"},
		{"asset cloud GUID", "SELECT ZCLOUDASSETGUID FROM ZASSET WHERE Z_PK = 1", "FA14E6A4-2168-4B28-9ABF-5A0C5738F8D9"},
		{"fingerprint", "SELECT " + sc.fingerprint + " FROM ZADDITIONALASSETATTRIBUTES WHERE ZASSET = 1", "AZfGr/ANI6DgoLV7ipUcQiNl2k9u"},
		{"original name", "SELECT ZORIGINALFILENAME FROM ZADDITIONALASSETATTRIBUTES WHERE ZASSET = 1", "IMG_0001.HEIC"},
		{"library UUID", "SELECT Z_UUID FROM Z_METADATA", "1F5D18B3-40E5-4A53-BAEC-F92601239137"},
		// Floats are scanned by Go (strconv), not converted to text by SQLite.
		{"latitude", "SELECT ZLATITUDE FROM ZASSET WHERE ZLATITUDE != -180 ORDER BY Z_PK LIMIT 1", "53.387384"},
		{"longitude", "SELECT ZLONGITUDE FROM ZASSET WHERE ZLATITUDE != -180 ORDER BY Z_PK LIMIT 1", "-95.660543"},
		{"created", "SELECT ZDATECREATED FROM ZASSET WHERE Z_PK = 1", "561927700"},
		{"original size", "SELECT ZORIGINALFILESIZE FROM ZADDITIONALASSETATTRIBUTES WHERE ZASSET = 1", "3229764"},
		{"video size", "SELECT r.ZDATALENGTH FROM ZINTERNALRESOURCE r JOIN ZASSET a ON a.Z_PK = r.ZASSET " +
			"WHERE a.ZKIND = 1 AND r.ZDATASTORESUBTYPE = 1 AND r.ZDATALENGTH < 4294967296 ORDER BY a.Z_PK LIMIT 1", "54610772"},
		{"first membership", "SELECT " + sc.memberAlbum + " || '/' || " + sc.memberAsset + " FROM " + sc.members +
			" ORDER BY rowid LIMIT 1", "4/43"},
	})
	want := map[string]string{
		"ZADDITIONALASSETATTRIBUTES": "b96965c4ea1ad2b18c0d76904da1d12835fc6e5a9473c84f9a897b0b489cb108",
		"ZASSET":                     "1d595185e2418082a28ad8f1cdf9291f5d0e0bc7c1cb4b796b37b056ba6870d7",
		"ZCLOUDMASTER":               "05a84fe8fd631bc84a38abe316145329b5cdda150d79ce34b9fbcb46b6d0a707",
		"ZGENERICALBUM":              "c3af8e962975c5a78be41cce67a756291c1bf71fcae560a9715544e141d077d7",
		"ZINTERNALRESOURCE":          "64b1b0478830278dfbe96519f8380ac7c1e141179ef5c3e711cecbba1cae27df",
		"ZSHARE":                     "bffa367a357536c968883c89fa6fd254e74dba4fe5479e613d6942b1a7093073",
		"Z_METADATA":                 "1499d1544d311f49bf443419b78f525da0f1027e769fea8bce2ee62b9b4d0816",
		"Z_PRIMARYKEY":               "ecbe6291c3bf0769c89da854a7ba7acb21af1eca40ab8b2f85e642ee24676238",
		sc.members:                   "9cd153b1fd3fde06a3df61c12aa4d86779a101f33d1608bacd53196be0b28fca",
	}
	got := dump(t, db)
	for _, table := range slices.Sorted(maps.Keys(want)) {
		if got[table] != want[table] {
			t.Errorf("SHA-256 of %s:\n got %s\nwant %s", table, got[table], want[table])
		}
	}
}
