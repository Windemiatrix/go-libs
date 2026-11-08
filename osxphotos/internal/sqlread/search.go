package sqlread

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// SearchTerm is one search term of an asset, as upstream reads it from
// psi.sqlite or leo.sqlite (_photosdb_process_searchinfo.py:201-247, 312-338):
// the UUID in upper case, the category of Photos 8, and the two strings without
// NUL and in NFC. RowID is ga.rowid for psi.sqlite and the number of the
// record, from 1, for leo.sqlite, so that it orders the terms as upstream does.
type SearchTerm struct {
	RowID               int64
	UUID                string
	Category            int
	Content, Normalized string
}

// The rowid of the tables is not listed: table_info does not show it.
var searchColumns = map[string][]string{
	"ga":     {"groupid", "assetid"},
	"groups": {"category", "content_string", "normalized_string"},
	"assets": {"uuid_0", "uuid_1"},
}

// Photos of macOS 27 keeps internal keys among the terms; they are no terms of
// the user (_photosdb_process_searchinfo.py:121-123). \n? stands for the $ of
// Python re, which also matches before a final newline.
var searchPlaceholder = regexp.MustCompile(`^PG[A-Za-z0-9]+SearchableText\n?$`)

// SearchTerms reads the terms of psi.sqlite in ga.rowid order
// (_photosdb_process_searchinfo.py:201-218) from a DB opened on that file, not
// on Photos.sqlite. A term of a NULL category, or whose content is a
// placeholder key (243-244), is left out; a NULL string is "", where upstream
// fails. A missing table or column is an error wrapping ErrMissingColumn.
func (d *DB) SearchTerms(ctx context.Context) ([]SearchTerm, error) {
	if err := d.CheckColumns(ctx, searchColumns); err != nil {
		return nil, err
	}
	const q = "SELECT ga.rowid, assets.uuid_0, assets.uuid_1, groups.category, groups.content_string, groups.normalized_string " +
		"FROM ga JOIN groups ON groups.rowid = ga.groupid JOIN assets ON ga.assetid = assets.rowid ORDER BY ga.rowid"
	var out []SearchTerm
	err := d.readRows(ctx, "search terms", q, func(rows *sql.Rows) error {
		var (
			t                   SearchTerm
			uuid0, uuid1        int64
			category            sql.Null[int]
			content, normalized sql.NullString
		)
		if err := rows.Scan(&t.RowID, &uuid0, &uuid1, &category, &content, &normalized); err != nil {
			return err
		}
		t.Content = searchString(content.String)
		if !category.Valid || searchPlaceholder.MatchString(t.Content) {
			return nil
		}
		t.UUID, t.Category, t.Normalized = uuidOf(uuid0, uuid1), category.V, searchString(normalized.String)
		out = append(out, t)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Photos appends a NUL to the strings of psi.sqlite
// (_photosdb_process_searchinfo.py:232).
func searchString(s string) string {
	return norm.NFC.String(strings.ReplaceAll(s, "\x00", ""))
}

// The two integers are the halves of the UUID as little-endian signed
// numbers (_photosdb_process_searchinfo.py:412-415).
func uuidOf(uuid0, uuid1 int64) string {
	b := binary.LittleEndian.AppendUint64(binary.LittleEndian.AppendUint64(nil, uint64(uuid0)), uint64(uuid1)) //nolint:gosec // G115: the bits of the signed integers are the bytes of the UUID
	return fmt.Sprintf("%X-%X-%X-%X-%X", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
