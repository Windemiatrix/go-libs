package gen

import (
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// keywordColumns is the keyword column of the keyword join table Z_1KEYWORDS
// by schema; the generator does not write keywords, so it is not in the
// schema table.
var keywordColumns = map[string]string{"photos11_1": "Z_52KEYWORDS"}

// readerColumns lists the tables and columns that the v1 queries of upstream
// osxphotos read (photosdb.py, _photosdb_process_shared_library.py,
// _photosdb_process_syndicationinfo.py); names that depend on the schema
// come from the schema table.
func readerColumns(sc schema, keywords string) map[string][]string {
	return map[string][]string{
		"ZASSET": append(strings.Fields(`Z_PK ZUUID ZMODIFICATIONDATE ZDATECREATED ZHIDDEN ZFAVORITE ZDIRECTORY ZFILENAME
			ZLATITUDE ZLONGITUDE ZCLOUDBATCHPUBLISHDATE ZKIND ZUNIFORMTYPEIDENTIFIER ZAVALANCHEUUID
			ZAVALANCHEPICKTYPE ZKINDSUBTYPE ZHDRTYPE ZCLOUDASSETGUID ZMOMENT ZTRASHEDSTATE ZHEIGHT
			ZWIDTH ZORIENTATION ZDEPTHTYPE ZADJUSTMENTTIMESTAMP ZVISIBILITYSTATE ZTRASHEDDATE
			ZSAVEDASSETTYPE ZADDEDDATE ZCLOUDOWNERHASHEDPERSONID ZMOMENTSHARE ZSPATIALTYPE ZMASTER
			ZADDITIONALATTRIBUTES ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE ZLIBRARYSCOPESHARESTATE
			ZLIBRARYSCOPE ZSYNDICATIONSTATE`), sc.adjustments),
		"ZADDITIONALASSETATTRIBUTES": append(strings.Fields(`Z_PK ZASSET ZTITLE ZORIGINALFILENAME ZTIMEZONEOFFSET
			ZINFERREDTIMEZONEOFFSET ZTIMEZONENAME ZCAMERACAPTUREDEVICE ZREVERSELOCATIONDATA
			ZORIGINALRESOURCECHOICE ZORIGINALHEIGHT ZORIGINALWIDTH ZORIGINALORIENTATION
			ZORIGINALFILESIZE ZIMPORTEDBYDISPLAYNAME ZIMPORTEDBYBUNDLEIDENTIFIER ZASSETDESCRIPTION
			ZUNMANAGEDADJUSTMENT ZSYNDICATIONHISTORY ZSYNDICATIONIDENTIFIER`), sc.fingerprint),
		"ZINTERNALRESOURCE": strings.Fields(`ZASSET ZLOCALAVAILABILITY ZREMOTEAVAILABILITY ZDATASTORESUBTYPE
			ZCOMPACTUTI ZFINGERPRINT ZDATALENGTH ZRESOURCETYPE ZFILESYSTEMBOOKMARK`),
		"ZCLOUDMASTER": strings.Fields("Z_PK ZCLOUDLOCALSTATE ZCLOUDMASTERGUID"),
		"ZGENERICALBUM": strings.Fields(`Z_PK ZUUID ZTITLE ZCLOUDLOCALSTATE ZCLOUDOWNERFIRSTNAME ZCLOUDOWNERLASTNAME
			ZCLOUDOWNERHASHEDPERSONID ZKIND ZPARENTFOLDER ZTRASHEDSTATE ZCREATIONDATE ZSTARTDATE
			ZENDDATE ZCUSTOMSORTASCENDING ZCUSTOMSORTKEY ZCLOUDGUID`),
		sc.members:             {sc.memberAlbum, sc.memberAsset, sc.memberOrder},
		"ZKEYWORD":             {"Z_PK", "ZTITLE"},
		"Z_1KEYWORDS":          {"Z_1ASSETATTRIBUTES", keywords},
		"ZASSETDESCRIPTION":    {"Z_PK", "ZLONGDESCRIPTION"},
		"ZUNMANAGEDADJUSTMENT": {"Z_PK", "ZADJUSTMENTFORMATIDENTIFIER"},
		"ZPERSON":              strings.Fields("Z_PK ZPERSONUUID ZFULLNAME ZFACECOUNT ZKEYFACE ZDISPLAYNAME ZTYPE ZMANUALORDER"),
		"ZDETECTEDFACE":        strings.Fields("Z_PK ZUUID ZASSETFORFACE ZPERSONFORFACE"),
		"ZFILESYSTEMVOLUME":    {"ZUUID", "ZNAME"},
		"ZSHARE":               strings.Fields("Z_PK Z_ENT ZUUID ZSCOPEIDENTIFIER"),
		"Z_METADATA":           strings.Fields("Z_VERSION Z_UUID Z_PLIST"),
		"Z_PRIMARYKEY":         strings.Fields("Z_ENT Z_NAME Z_SUPER Z_MAX"),
	}
}

// TestReaderColumns generates an empty library of every schema and checks
// that it has every column the upstream readers use. It stands in for running
// the sqlread queries on an empty database of each DDL until sqlread exists.
func TestReaderColumns(t *testing.T) {
	for _, name := range slices.Sorted(maps.Keys(schemas)) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sc := schemas[name]
			keywords, ok := keywordColumns[name]
			if !ok {
				t.Fatalf("no keyword column for schema %s in keywordColumns", name)
			}
			cfg := DefaultConfig()
			cfg.Schema, cfg.Assets = name, 0
			_, db := generate(t, cfg)
			cols := readerColumns(sc, keywords)
			for _, table := range slices.Sorted(maps.Keys(cols)) {
				have := rowStrings(t, db, "SELECT name FROM pragma_table_info('"+table+"')")
				if len(have) == 0 {
					t.Errorf("%s: no table %s", name, table)
					continue
				}
				for _, col := range cols[table] {
					if !slices.Contains(have, col) {
						t.Errorf("%s: no column %s.%s", name, table, col)
					}
				}
			}
		})
	}
}

// insertInto matches a statement that writes data ("AFTER INSERT ON" of a
// trigger does not).
var insertInto = regexp.MustCompile(`(?i)\b(insert|replace)\s+(or\s+\w+\s+)?into\b`)

// TestDDLFiles checks that every file in ddl/ has an entry in the schema
// table and back, and that each holds CREATE statements only.
func TestDDLFiles(t *testing.T) {
	files, err := fs.Glob(ddlFS, "ddl/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(strings.TrimPrefix(f, "ddl/"), ".sql"))
	}
	if want := slices.Sorted(maps.Keys(schemas)); !slices.Equal(names, want) {
		t.Errorf("ddl files %q, schema table entries %q", names, want)
	}
	for _, f := range files {
		src, err := fs.ReadFile(ddlFS, f)
		if err != nil {
			t.Fatal(err)
		}
		if insertInto.Match(src) {
			t.Errorf("%s: INSERT is not allowed in a DDL file", f)
		}
		d, err := parseDDL(string(src))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if len(d.tables) == 0 {
			t.Errorf("%s: no tables", f)
		}
	}
	if _, ok := schemas[DefaultSchema]; !ok {
		t.Errorf("DefaultSchema %q is not in the schema table", DefaultSchema)
	}
}

// TestSchemaTable checks that every schema table entry has the entities the
// generator writes; its column names are checked against the DDL by
// TestReaderColumns.
func TestSchemaTable(t *testing.T) {
	for _, name := range slices.Sorted(maps.Keys(schemas)) {
		sc := schemas[name]
		for _, e := range []string{"AdditionalAssetAttributes", "Asset", "CloudMaster", "InternalResource",
			"GenericAlbum", "Album", "Folder", "Share", "LibraryScope", "CollectionShare"} {
			if _, ok := sc.entities[e]; !ok {
				t.Errorf("%s: no entity %s", name, e)
			}
		}
		if sc.model <= 0 {
			t.Errorf("%s: model %d", name, sc.model)
		}
	}
}

func TestParseDDL(t *testing.T) {
	src := "-- header\n" +
		"\n" +
		"CREATE TABLE A ( Z_PK INTEGER PRIMARY KEY );\n" +
		"CREATE TRIGGER T AFTER INSERT ON A BEGIN SELECT 1; END;\n" +
		"CREATE INDEX I ON A (Z_PK);\n" +
		"CREATE VIEW V AS SELECT * FROM A;\n" +
		"CREATE VIRTUAL TABLE R USING RTREE (id, x0, x1);\n" +
		"CREATE TABLE \"R_node\"(nodeno INTEGER PRIMARY KEY,data);\n" +
		"CREATE TABLE R_parent(nodeno INTEGER PRIMARY KEY,parentnode);\n" +
		"CREATE TABLE RX (id);\n" +
		"CREATE UNIQUE INDEX U ON A (Z_PK);\n"
	d, err := parseDDL(src)
	if err != nil {
		t.Fatal(err)
	}
	wantTables := []string{
		"CREATE TABLE A ( Z_PK INTEGER PRIMARY KEY );",
		"CREATE VIRTUAL TABLE R USING RTREE (id, x0, x1);",
		"CREATE TABLE RX (id);",
	}
	if !slices.Equal(d.tables, wantTables) {
		t.Errorf("tables:\n got %q\nwant %q", d.tables, wantTables)
	}
	wantLate := []string{
		"CREATE INDEX I ON A (Z_PK);",
		"CREATE UNIQUE INDEX U ON A (Z_PK);",
		"CREATE VIEW V AS SELECT * FROM A;",
		"CREATE TRIGGER T AFTER INSERT ON A BEGIN SELECT 1; END;",
	}
	if !slices.Equal(d.late, wantLate) {
		t.Errorf("late:\n got %q\nwant %q", d.late, wantLate)
	}

	for _, bad := range []struct{ src, line string }{
		{"CREATE TABLE A (x);\nINSERT INTO A VALUES (1);\n", "line 2"},
		{"-- c\nCREATE TABLE A (x)\n", "line 2"},
		{"CREATE TABLE A (\nx);\n", "line 1"},
		{"\n\nDROP TABLE A;\n", "line 3"},
		{"CREATE TABLE A (x); ATTACH DATABASE 'x' AS p;\n", "line 1"},
		{"CREATE INDEX I ON A (x); PRAGMA writable_schema = ON;\n", "line 1"},
		{"-- c\nCREATE TRIGGER T AFTER INSERT ON A BEGIN SELECT 1; END; PRAGMA writable_schema = ON;\n", "line 2"},
		{"CREATE TRIGGER T AFTER INSERT ON A BEGIN SELECT 1; END; ATTACH DATABASE 'x' AS p; " +
			"CREATE TRIGGER U AFTER INSERT ON A BEGIN SELECT 1; END;\n", "line 1"},
		{"CREATE TRIGGER T AFTER INSERT ON A BEGIN SELECT 1; end; ATTACH DATABASE 'x' AS p; " +
			"CREATE TRIGGER U AFTER INSERT ON A BEGIN SELECT 1; END;\n", "line 1"},
		{"CREATE INDEX I ON A (x) -- x;\n", "line 1"},
	} {
		if _, err := parseDDL(bad.src); err == nil || !strings.Contains(err.Error(), bad.line) {
			t.Errorf("parseDDL(%q): got error %v, want one about %s", bad.src, err, bad.line)
		}
	}
}
