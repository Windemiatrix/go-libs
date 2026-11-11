package sqlread

import (
	"database/sql"
	"testing"
)

// ZCOMPACTUTI holds an integer code or a text "_" + UTI; both arrive as text
// (photosdb/photosdb_utils.py:280).
func TestResourcesCompactUTI(t *testing.T) {
	t.Parallel()

	path := syntheticDB(t)
	w := openWriter(t, path)
	first := count(t, w, "SELECT MIN(Z_PK) FROM ZINTERNALRESOURCE WHERE ZASSET IS NOT NULL")
	last := count(t, w, "SELECT MAX(Z_PK) FROM ZINTERNALRESOURCE WHERE ZASSET IS NOT NULL")
	exec(t, w,
		"UPDATE ZINTERNALRESOURCE SET ZCOMPACTUTI = NULL",
		"UPDATE ZINTERNALRESOURCE SET ZCOMPACTUTI = CAST(11 AS INTEGER) WHERE Z_PK = (SELECT MIN(Z_PK) FROM ZINTERNALRESOURCE WHERE ZASSET IS NOT NULL)",
		"UPDATE ZINTERNALRESOURCE SET ZCOMPACTUTI = '_org.webmproject.webp' WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZINTERNALRESOURCE WHERE ZASSET IS NOT NULL)",
	)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := openDB(t, path).Resources(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(res) < 3 {
		t.Fatalf("%d resources, want at least 3", len(res))
	}
	for _, r := range res {
		want := sql.NullString{}
		switch r.PK {
		case first:
			want = sql.NullString{String: "11", Valid: true}
		case last:
			want = sql.NullString{String: "_org.webmproject.webp", Valid: true}
		}
		if r.CompactUTI != want {
			t.Errorf("Z_PK %d: CompactUTI %+v, want %+v", r.PK, r.CompactUTI, want)
		}
	}
}
