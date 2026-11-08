package sqlread

import (
	"context"
	"database/sql"
	"encoding/binary"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var leoColumns = map[string][]string{
	"lexicon": {"lexeme_id", "type", "category", "content"},
	"items":   {"identifier", "lexeme_ids", "type"},
}

// LEO_CATEGORY_TO_PHOTOS8 (_photosdb_process_searchinfo.py:40-119); 2030 holds only placeholder keys, so upstream leaves it out (43-44).
var leoCategories = map[int]int{
	1010: 1100, 1020: 1101, 1030: 1103, 1040: 1104, 1050: 1106, 1060: 1107, 1070: 1070,
	2010: 1000, 2050: 2, 2060: 1, 2070: 3, 2080: 4, 2090: 5, 2100: 6, 2110: 7, 2120: 8, 2130: 9, 2140: 10, 2150: 11, 2160: 12, 2170: 13,
	2180: 1008, 2190: 1007, 2210: 14, 2220: 1700, 2230: 1701, 2240: 1800, 2250: 1801, 2260: 1802,
	3001: 1300, 3010: 1330, 3030: 3030,
	4000: 1500, 4010: 1510, 4020: 1520, 4050: 2500, 4060: 2600, 4080: 2800, 4090: 1600, 4100: 1610, 4110: 1205, 4120: 1203, 4130: 4130,
	5000: 1900, 5010: 1901, 5020: 1902, 5030: 1903, 5050: 1905, 5060: 1906, 5070: 1907, 5080: 1908, 5090: 1909, 5100: 1916,
	5110: 1911, 5120: 1912, 5130: 1913, 5140: 1914, 5150: 1915, 5180: 1918, 5190: 1919, 5200: 5200,
	6000: 2300, 7000: 1201, 7010: 1400, 8000: 2000, 8050: 2100, 8051: 8051, 8060: 2200, 8070: 1200, 8080: 1202, 8170: 8170,
	11000: 11000, 11010: 11010,
}

// LeoTerms reads the terms of leo.sqlite of macOS 27 from a DB opened on that
// file (_photosdb_process_searchinfo.py:292-338), in items.rowid order. RowID
// counts the returned terms from 1; UUID is the identifier in upper case. A
// lexeme of an unmapped category, without text content or with a placeholder
// key, and an item without an identifier give no term. A missing table or
// column is an error wrapping ErrMissingColumn.
func (d *DB) LeoTerms(ctx context.Context) ([]SearchTerm, error) {
	if err := d.CheckColumns(ctx, leoColumns); err != nil {
		return nil, err
	}
	categories, contents := map[int64]int{}, map[int64]string{}
	err := d.readRows(ctx, "leo lexicon", "SELECT lexeme_id, type, category, content FROM lexicon", func(rows *sql.Rows) error {
		var (
			id                      sql.Null[int64]
			lexemeType, leoCategory sql.Null[int]
			content                 sql.NullString
		)
		if err := rows.Scan(&id, &lexemeType, &leoCategory, &content); err != nil {
			return err
		}
		category, ok := leoCategories[leoCategory.V]
		if !ok || !id.Valid {
			return nil
		}
		categories[id.V] = category
		if s := searchString(content.String); lexemeType.V == 1 && !searchPlaceholder.MatchString(s) {
			contents[id.V] = s
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var out []SearchTerm
	err = d.readRows(ctx, "leo items", "SELECT identifier, lexeme_ids FROM items WHERE type = 1 ORDER BY rowid", func(rows *sql.Rows) error {
		var (
			identifier sql.NullString
			blob       []byte
		)
		if err := rows.Scan(&identifier, &blob); err != nil {
			return err
		}
		if identifier.String == "" {
			return nil
		}
		for _, id := range lexemeIDs(blob) {
			content := contents[int64(id)]
			if content == "" {
				continue
			}
			out = append(out, SearchTerm{
				RowID: int64(len(out)) + 1, UUID: strings.ToUpper(identifier.String), Category: categories[int64(id)],
				Content: content, Normalized: norm.NFC.String(strings.ToLower(content)),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// decode_leo_lexeme_ids (_photosdb_process_searchinfo.py:418-423).
func lexemeIDs(blob []byte) []uint32 {
	ids := make([]uint32, len(blob)/4)
	for i := range ids {
		ids[i] = binary.LittleEndian.Uint32(blob[4*i:])
	}
	return ids
}
