package gen

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	_ "modernc.org/sqlite" // the "sqlite" database/sql driver
)

const (
	// Z_OPT: a change counter in Photos (as in the osxphotos test library
	// Test-26.1); 1, a row written once, is synthetic.
	zOpt            = 1
	masterPKOffset  = 10   // ZCLOUDMASTER.Z_PK starts at 11, so it never equals ZASSET.Z_PK of the asset
	orderStep       = 1024 // step of the membership order, as in the osxphotos test library Test-26.1
	cloudInCloud    = 3    // ZCLOUDMASTER.ZCLOUDLOCALSTATE of a master in iCloud
	libraryScopePK  = 1    // Z_PK of the one ZSHARE row
	ctxCheckEvery   = 1000 // assets between checks of the context
	statementPrefix = 80   // bytes of a statement quoted in an error
)

// write creates the tables of the DDL, inserts the plan in one transaction,
// then creates the indexes, views and triggers. The triggers call Core Data
// functions that exist only inside the Photos app; created after the data,
// they never fire here, and unique indexes created after the data check it.
func write(ctx context.Context, path string, sc schema, d ddl, p *plan) (err error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	db.SetMaxOpenConns(1)
	// A fresh file that is removed on failure needs no syncs.
	if _, err := db.ExecContext(ctx, "PRAGMA synchronous = OFF"); err != nil {
		return err
	}
	if err := execAll(ctx, db, d.tables); err != nil {
		return err
	}
	if err := insert(ctx, db, sc, p); err != nil {
		return err
	}
	return execAll(ctx, db, d.late)
}

func execAll(ctx context.Context, db *sql.DB, stmts []string) error {
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("%s: %w", s[:min(len(s), statementPrefix)], err)
		}
	}
	return nil
}

// prepare returns an INSERT of the columns into table.
func prepare(ctx context.Context, tx *sql.Tx, table string, cols ...string) (*sql.Stmt, error) {
	q := "INSERT INTO " + table + " (" + strings.Join(cols, ", ") + ") VALUES (" +
		strings.Repeat("?, ", len(cols)-1) + "?)"
	return tx.PrepareContext(ctx, q) //nolint:gosec // G202: table and column names come from the schema table; values are bound
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// insert writes every row of the plan in one transaction.
func insert(ctx context.Context, db *sql.DB, sc schema, p *plan) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	for _, step := range []func(context.Context, *sql.Tx, schema, *plan) error{
		insertAssets, insertAttributes, insertMasters, insertResources, insertAlbums, insertMembers, insertMetadata,
	} {
		if err := step(ctx, tx, sc, p); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertAssets(ctx context.Context, tx *sql.Tx, sc schema, p *plan) error {
	stmt, err := prepare(ctx, tx, "ZASSET", "Z_PK", "Z_ENT", "Z_OPT", "ZUUID", "ZKIND", "ZKINDSUBTYPE",
		"ZUNIFORMTYPEIDENTIFIER", "ZFILENAME", "ZDIRECTORY", "ZDATECREATED", "ZADDEDDATE", "ZMODIFICATIONDATE",
		"ZHIDDEN", "ZFAVORITE", "ZTRASHEDSTATE", "ZTRASHEDDATE", "ZVISIBILITYSTATE", sc.adjustments,
		"ZADJUSTMENTTIMESTAMP", "ZWIDTH", "ZHEIGHT", "ZORIENTATION", "ZLATITUDE", "ZLONGITUDE", "ZHDRTYPE",
		"ZDEPTHTYPE", "ZSPATIALTYPE", "ZSYNDICATIONSTATE", "ZSAVEDASSETTYPE", "ZCLOUDASSETGUID", "ZMASTER",
		"ZADDITIONALATTRIBUTES", "ZLIBRARYSCOPE", "ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE", "ZLIBRARYSCOPESHARESTATE",
		"ZDERIVEDCAMERACAPTUREDEVICE", "ZAVALANCHEUUID", "ZAVALANCHEPICKTYPE")
	if err != nil {
		return err
	}
	n := len(p.assets)
	for i, a := range p.assets {
		if i%ctxCheckEvery == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		var trashedAt, adjustedAt, scope, burst any
		if a.trashed {
			trashedAt = a.trashedAt
		}
		if a.adjustments > 0 {
			adjustedAt = a.adjustedAt
		}
		// In the library scope: the share row, participation and share state 1.
		// synthetic: the bit flags of the share state are not reproduced.
		inScope := b2i(a.shared)
		if a.shared {
			scope = libraryScopePK
		}
		if a.burst > 0 {
			burst = p.bursts[a.burst-1]
		}
		// ZFILENAME is <ZUUID>.<extension>, ZDIRECTORY its first character,
		// as in the osxphotos test library Test-26.1.
		if _, err := stmt.ExecContext(ctx, i+1, sc.entities["Asset"].ent, zOpt, a.uuid, a.kind, a.subtype,
			a.f.uti, a.uuid+"."+a.f.fileExt, a.uuid[:1], a.created, a.added, a.modified,
			b2i(a.hidden), b2i(a.favorite), b2i(a.trashed), trashedAt, a.visibility, a.adjustments,
			adjustedAt, a.width, a.height, 1, a.lat, a.lon, 0,
			0, 0, 0, a.savedType, a.cloudGUID, i+1+masterPKOffset,
			n-i, scope, inScope, inScope,
			0, burst, a.pick); err != nil {
			return fmt.Errorf("ZASSET: %w", err)
		}
	}
	return nil
}

// insertAttributes writes the rows in the reverse order of the assets, so
// that their Z_PK differs from the one of the asset.
func insertAttributes(ctx context.Context, tx *sql.Tx, sc schema, p *plan) error {
	stmt, err := prepare(ctx, tx, "ZADDITIONALASSETATTRIBUTES", "Z_PK", "Z_ENT", "Z_OPT", "ZASSET",
		"ZORIGINALFILENAME", sc.fingerprint, "ZORIGINALFILESIZE", "ZORIGINALWIDTH", "ZORIGINALHEIGHT",
		"ZORIGINALORIENTATION", "ZTIMEZONEOFFSET", "ZINFERREDTIMEZONEOFFSET", "ZTIMEZONENAME",
		"ZCAMERACAPTUREDEVICE", "ZORIGINALRESOURCECHOICE", "ZSYNDICATIONHISTORY")
	if err != nil {
		return err
	}
	n := len(p.assets)
	for k := range n {
		i := n - 1 - k
		a := p.assets[i]
		var offset, name any
		if tz := timeZones[i%len(timeZones)]; tz.name != "" {
			offset, name = tz.offset, tz.name
		}
		original := a.resources[0]
		if _, err := stmt.ExecContext(ctx, k+1, sc.entities["AdditionalAssetAttributes"].ent, zOpt, i+1,
			a.original, original.fingerprint, original.length, a.width, a.height,
			1, offset, offset, name,
			0, 0, 0); err != nil {
			return fmt.Errorf("ZADDITIONALASSETATTRIBUTES: %w", err)
		}
	}
	return nil
}

func insertMasters(ctx context.Context, tx *sql.Tx, sc schema, p *plan) error {
	stmt, err := prepare(ctx, tx, "ZCLOUDMASTER", "Z_PK", "Z_ENT", "Z_OPT", "ZCLOUDMASTERGUID", "ZCLOUDLOCALSTATE",
		"ZORIGINALFILENAME", "ZUNIFORMTYPEIDENTIFIER", "ZCREATIONDATE", "ZIMPORTDATE")
	if err != nil {
		return err
	}
	for i, a := range p.assets {
		// synthetic: not observed in a test library (the test libraries have no iCloud)
		if _, err := stmt.ExecContext(ctx, i+1+masterPKOffset, sc.entities["CloudMaster"].ent, zOpt, a.master,
			cloudInCloud, a.original, a.f.uti, a.created, a.added); err != nil {
			return fmt.Errorf("ZCLOUDMASTER: %w", err)
		}
	}
	return nil
}

func insertResources(ctx context.Context, tx *sql.Tx, sc schema, p *plan) error {
	stmt, err := prepare(ctx, tx, "ZINTERNALRESOURCE", "Z_PK", "Z_ENT", "Z_OPT", "ZASSET", "ZDATASTORESUBTYPE",
		"ZRESOURCETYPE", "ZVERSION", "ZRECIPEID", "ZCOMPACTUTI", "ZDATALENGTH", "ZFINGERPRINT",
		"ZLOCALAVAILABILITY", "ZREMOTEAVAILABILITY", "ZTRASHEDSTATE")
	if err != nil {
		return err
	}
	pk := 0
	for i, a := range p.assets {
		local := 1
		if a.cloudOnly {
			local = -1 // only in iCloud
		}
		for _, r := range a.resources {
			pk++
			// ZVERSION and ZRECIPEID 0 as for originals in the osxphotos test library Test-26.1.
			if _, err := stmt.ExecContext(ctx, pk, sc.entities["InternalResource"].ent, zOpt, i+1, r.subtype,
				r.f.resource, 0, 0, r.f.compact, r.length, r.fingerprint,
				local, 1, 0); err != nil {
				return fmt.Errorf("ZINTERNALRESOURCE: %w", err)
			}
		}
	}
	return nil
}

func insertAlbums(ctx context.Context, tx *sql.Tx, sc schema, p *plan) error {
	stmt, err := prepare(ctx, tx, "ZGENERICALBUM", "Z_PK", "Z_ENT", "Z_OPT", "ZUUID", "ZCLOUDGUID", "ZTITLE",
		"ZKIND", "ZPARENTFOLDER", "ZTRASHEDSTATE", "ZCREATIONDATE", "ZSTARTDATE", "ZENDDATE",
		"ZCUSTOMSORTKEY", "ZCUSTOMSORTASCENDING", "ZCLOUDLOCALSTATE")
	if err != nil {
		return err
	}
	for j, g := range p.albums {
		// Z_ENT, sort key, sort order and cloud local state as for the albums
		// and folders of the osxphotos test library Test-26.1.
		ent, sortKey := sc.entities["Folder"].ent, 0
		if g.kind == 2 {
			ent, sortKey = sc.entities["Album"].ent, 1
		}
		var title, parent, start, end any
		if g.title != "" {
			title = g.title
		}
		if g.parent >= 0 {
			parent = g.parent + 1
		}
		if len(g.members) > 0 {
			lo, hi := p.assets[g.members[0]].created, p.assets[g.members[0]].created
			for _, i := range g.members {
				lo, hi = min(lo, p.assets[i].created), max(hi, p.assets[i].created)
			}
			start, end = lo, hi
		}
		if _, err := stmt.ExecContext(ctx, j+1, ent, zOpt, g.uuid, g.cloudGUID, title,
			g.kind, parent, 0, g.created, start, end,
			sortKey, 1, 0); err != nil {
			return fmt.Errorf("ZGENERICALBUM: %w", err)
		}
	}
	return nil
}

func insertMembers(ctx context.Context, tx *sql.Tx, sc schema, p *plan) error {
	stmt, err := prepare(ctx, tx, sc.members, sc.memberAlbum, sc.memberAsset, sc.memberOrder)
	if err != nil {
		return err
	}
	for j, g := range p.albums {
		for k, i := range g.members {
			if _, err := stmt.ExecContext(ctx, j+1, i+1, (k+1)*orderStep); err != nil {
				return fmt.Errorf("%s: %w", sc.members, err)
			}
		}
	}
	return nil
}

// insertMetadata writes the library scope, Z_METADATA and Z_PRIMARYKEY.
func insertMetadata(ctx context.Context, tx *sql.Tx, sc schema, p *plan) error {
	shared := p.shared()
	if shared {
		stmt, err := prepare(ctx, tx, "ZSHARE", "Z_PK", "Z_ENT", "Z_OPT", "ZUUID", "ZSCOPEIDENTIFIER", "ZTRASHEDSTATE")
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, libraryScopePK, sc.entities["LibraryScope"].ent, zOpt,
			p.scopeUUID, p.scopeID, 0); err != nil {
			return fmt.Errorf("ZSHARE: %w", err)
		}
	}

	stmt, err := prepare(ctx, tx, "Z_METADATA", "Z_VERSION", "Z_UUID", "Z_PLIST")
	if err != nil {
		return err
	}
	// Bound as []byte: the column holds a BLOB as in a real library; a TEXT
	// value would make upstream fail to read the plist and guess the schema.
	if _, err := stmt.ExecContext(ctx, 1, p.libraryUUID, modelPlist(sc.model)); err != nil {
		return fmt.Errorf("Z_METADATA: %w", err)
	}

	resources, masters := 0, 0
	for _, a := range p.assets {
		resources += len(a.resources)
	}
	if len(p.assets) > 0 {
		masters = len(p.assets) + masterPKOffset
	}
	// Z_MAX of a root entity is MAX(Z_PK) of its table; of a subentity it is
	// 0, as in the osxphotos test library Test-26.1.
	maxPK := map[string]int{
		"AdditionalAssetAttributes": len(p.assets),
		"Asset":                     len(p.assets),
		"CloudMaster":               masters,
		"GenericAlbum":              len(p.albums),
		"InternalResource":          resources,
		"Share":                     b2i(shared) * libraryScopePK,
	}
	stmt, err = prepare(ctx, tx, "Z_PRIMARYKEY", "Z_ENT", "Z_NAME", "Z_SUPER", "Z_MAX")
	if err != nil {
		return err
	}
	names := slices.SortedFunc(maps.Keys(sc.entities), func(a, b string) int {
		return sc.entities[a].ent - sc.entities[b].ent
	})
	for _, name := range names {
		e, zMax := sc.entities[name], 0
		if e.super == 0 {
			zMax = maxPK[name]
		}
		if _, err := stmt.ExecContext(ctx, e.ent, name, e.super, zMax); err != nil {
			return fmt.Errorf("Z_PRIMARYKEY: %w", err)
		}
	}
	return nil
}

// modelPlist is the XML plist of Z_METADATA with the model number only.
func modelPlist(model int) []byte {
	return fmt.Appendf(nil, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PLModelVersion</key>
	<integer>%d</integer>
</dict>
</plist>
`, model)
}
