package gen

import (
	"embed"
	"fmt"
	"slices"
	"strings"
)

// ddlFS holds the DDL of each schema: a dump of sqlite_master of a real
// library without data, one statement per line.
//
//go:embed ddl/*.sql
var ddlFS embed.FS

type entity struct{ ent, super int } // Z_ENT and Z_SUPER in Z_PRIMARYKEY

// schema holds what the generator needs to know about a schema besides its
// DDL. Adding a schema is adding ddl/<name>.sql and one entry of schemas.
type schema struct {
	model       int               // PLModelVersion written to Z_METADATA
	fingerprint string            // ZADDITIONALASSETATTRIBUTES column, e.g. ZORIGINALSTABLEHASH
	adjustments string            // ZASSET column, e.g. ZADJUSTMENTSSTATE
	members     string            // album membership table, e.g. Z_33ASSETS
	memberAlbum string            // its album column, e.g. Z_33ALBUMS
	memberAsset string            // its asset column, e.g. Z_3ASSETS
	memberOrder string            // its order column, e.g. Z_FOK_3ASSETS
	entities    map[string]entity // by Z_NAME; rows are written in Z_ENT order, never by map iteration
}

var schemas = map[string]schema{
	"photos11_1": {
		model:       19607,
		fingerprint: "ZORIGINALSTABLEHASH",
		adjustments: "ZADJUSTMENTSSTATE",
		members:     "Z_33ASSETS",
		memberAlbum: "Z_33ALBUMS",
		memberAsset: "Z_3ASSETS",
		memberOrder: "Z_FOK_3ASSETS",
		// As in Z_PRIMARYKEY of the osxphotos test library Test-26.1 (the same
		// schema, model 19320): every entity number visible in the DDL of both
		// models is the same.
		entities: map[string]entity{
			"AdditionalAssetAttributes": {1, 0},
			"Asset":                     {3, 0},
			"CloudMaster":               {15, 0},
			"GenericAlbum":              {32, 0},
			"Album":                     {33, 32},
			"Folder":                    {41, 32},
			"InternalResource":          {51, 0},
			"Share":                     {66, 0},
			"CollectionShare":           {67, 66},
			"LibraryScope":              {68, 66},
		},
	},
}

// ddl is a parsed DDL file in the order the generator runs it.
type ddl struct {
	tables []string // CREATE TABLE and CREATE VIRTUAL TABLE, before the data
	late   []string // CREATE INDEX and CREATE UNIQUE INDEX, then CREATE VIEW, then CREATE TRIGGER, after the data
}

func loadDDL(name string) (ddl, error) {
	src, err := ddlFS.ReadFile("ddl/" + name + ".sql")
	if err != nil {
		return ddl{}, err
	}
	d, err := parseDDL(string(src))
	if err != nil {
		return ddl{}, fmt.Errorf("ddl/%s.sql: %w", name, err)
	}
	return d, nil
}

const (
	createTable   = "CREATE TABLE "
	createVirtual = "CREATE VIRTUAL TABLE "
	createIndex   = "CREATE INDEX "
	createUnique  = "CREATE UNIQUE INDEX "
	createView    = "CREATE VIEW "
	createTrigger = "CREATE TRIGGER "
)

// parseDDL splits a DDL file into statements, one per line. Blank lines and
// lines starting with "--" are skipped; any other line must start with one
// of the CREATE prefixes, end with ";" and hold no "--". A line other than a
// trigger holds exactly one ";"; a trigger line ends with "END;" and holds
// no other "END;" in any case. The CREATE TABLE of a shadow table of a
// virtual table in the same file is dropped: CREATE VIRTUAL TABLE creates it.
func parseDDL(src string) (ddl, error) {
	type stmt struct{ kind, sql string }
	var (
		stmts   []stmt
		virtual []string
	)
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		kinds := []string{createTable, createVirtual, createIndex, createUnique, createView, createTrigger}
		k := slices.IndexFunc(kinds, func(k string) bool { return strings.HasPrefix(line, k) })
		if k < 0 || !strings.HasSuffix(line, ";") {
			return ddl{}, fmt.Errorf("line %d: not a one-line CREATE TABLE, VIRTUAL TABLE, INDEX, UNIQUE INDEX, VIEW or TRIGGER statement ending with ';'", i+1)
		}
		if strings.Contains(line, "--") {
			return ddl{}, fmt.Errorf("line %d: a comment inside a statement", i+1)
		}
		if kinds[k] == createTrigger && (!strings.HasSuffix(line, "END;") || strings.Count(strings.ToUpper(line), "END;") != 1) {
			return ddl{}, fmt.Errorf("line %d: a CREATE TRIGGER statement does not end with the only 'END;' of the line", i+1)
		}
		if kinds[k] != createTrigger && strings.Count(line, ";") != 1 {
			return ddl{}, fmt.Errorf("line %d: more than one statement on the line", i+1)
		}
		if kinds[k] == createVirtual {
			virtual = append(virtual, objectName(line[len(createVirtual):]))
		}
		stmts = append(stmts, stmt{kinds[k], line})
	}

	var d ddl
	var indexes, views, triggers []string
	for _, s := range stmts {
		switch s.kind {
		case createTable:
			name := objectName(s.sql[len(createTable):])
			if slices.ContainsFunc(virtual, func(v string) bool { return strings.HasPrefix(name, v+"_") }) {
				continue
			}
			d.tables = append(d.tables, s.sql)
		case createVirtual:
			d.tables = append(d.tables, s.sql)
		case createIndex, createUnique:
			indexes = append(indexes, s.sql)
		case createView:
			views = append(views, s.sql)
		case createTrigger:
			triggers = append(triggers, s.sql)
		}
	}
	d.late = slices.Concat(indexes, views, triggers)
	return d, nil
}

// objectName returns the name at the start of s without double quotes.
func objectName(s string) string {
	end := strings.IndexAny(s, " (")
	if end < 0 {
		end = len(s)
	}
	return strings.Trim(s[:end], `"`)
}
