package photosdb

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func TestShareInfoFields(t *testing.T) {
	t.Parallel()

	var r sqlread.Share
	rv := reflect.ValueOf(&r).Elem()
	for i := range rv.NumField() {
		switch f := rv.Field(i); f.Interface().(type) {
		case int64:
			f.SetInt(1)
		case sql.NullInt64:
			f.Set(reflect.ValueOf(nullInt(int64(100 + i))))
		case sql.NullFloat64:
			f.Set(reflect.ValueOf(nullFloat(float64(86400 * i))))
		case sql.NullString:
			f.Set(reflect.ValueOf(nullString(fmt.Sprintf("TEST-%02d", i))))
		case []byte:
			f.SetBytes([]byte{byte(i)})
		default:
			t.Fatalf("sqlread.Share.%s: type %s", rv.Type().Field(i).Name, f.Type())
		}
	}
	// Upstream does not normalize the title (shareinfo.py:146-147).
	r.Title = nullString("Cafe\U00000301")
	got := reflect.ValueOf(shareInfo(r)).Elem()
	if got.NumField() != rv.NumField() {
		t.Fatalf("ShareInfo has %d fields, sqlread.Share %d", got.NumField(), rv.NumField())
	}
	for i := range rv.NumField() {
		name := rv.Type().Field(i).Name
		if g := got.FieldByName(name); !g.IsValid() || !converted(g.Interface(), rv.Field(i).Interface()) {
			t.Errorf("ShareInfo.%s is not the value of sqlread.Share.%s", name, name)
		}
	}

	if got := shareInfo(sqlread.Share{PK: 2}); !reflect.DeepEqual(got, &photoinfo.ShareInfo{PK: 2}) {
		t.Errorf("a row of NULLs: %+v", *got)
	}
}

func converted(got, row any) bool {
	switch r := row.(type) {
	case int64:
		return got == any(r)
	case sql.NullInt64:
		p, ok := got.(*int)
		return ok && p != nil && int64(*p) == r.Int64
	case sql.NullFloat64:
		d, ok := got.(*time.Time)
		return ok && d != nil && d.Equal(time.Unix(coreDataOffset+int64(r.Float64), 0)) && d.Location() == time.Local
	case sql.NullString:
		return got == any(r.String)
	case []byte:
		b, ok := got.([]byte)
		return ok && bytes.Equal(b, r)
	}
	return false
}

func TestLocalTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    sql.NullFloat64
		want time.Time // the zero time for nil
	}{
		{"NULL", sql.NullFloat64{}, time.Time{}},
		// Upstream leaves 0 unconverted (shareinfo.py:74-75).
		{"0", nullFloat(0), time.Time{}},
		{"-0", nullFloat(math.Copysign(0, -1)), time.Time{}},
		{"NaN", nullFloat(math.NaN()), time.Time{}},
		{"after the year 9999", nullFloat(1e12), time.Time{}},
		{"a second", nullFloat(1), time.Date(2001, 1, 1, 0, 0, 1, 0, time.UTC)},
		{"2023-01-01", nullFloat(694224000), time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		got := localTime(tt.v)
		if tt.want.IsZero() {
			if got != nil {
				t.Errorf("%s: %v, want nil", tt.name, got)
			}
			continue
		}
		if got == nil || !got.Equal(tt.want) || got.Location() != time.Local {
			t.Errorf("%s: %v, want %v in time.Local", tt.name, got, tt.want)
		}
	}
}

func TestLoadShares(t *testing.T) {
	t.Parallel()

	asset := func(pk int64, uuid string, moment, scope sql.NullInt64) sqlread.Asset {
		a := baseAsset(pk, uuid)
		a.MomentShare, a.Scope = moment, scope
		return a
	}
	skipped := asset(7, "A-none", nullInt(1), nullInt(2))
	skipped.HasAttributes = false
	moment, library := loadShares([]sqlread.Share{{PK: 0}, {PK: 1}, {PK: 2}}, []sqlread.Asset{
		asset(1, "A-moment", nullInt(1), sql.NullInt64{}),
		asset(2, "A-library", sql.NullInt64{}, nullInt(2)),
		asset(3, "A-both", nullInt(1), nullInt(2)),
		asset(4, "A-none", sql.NullInt64{}, sql.NullInt64{}),
		asset(5, "A-missing", nullInt(99), nullInt(98)),
		asset(6, "A-zero", nullInt(0), nullInt(0)),
		skipped,
	})
	db := &DB{momentShares: moment, libraryShares: library}
	pk := func(s *photoinfo.ShareInfo) int64 {
		if s == nil {
			return -1
		}
		return s.PK
	}
	tests := []struct {
		uuid            string
		moment, library int64 // Z_PK of the share; -1 for nil
	}{
		{"A-moment", 1, -1},
		{"A-library", -1, 2},
		{"A-both", 1, 2},
		{"A-none", -1, -1},
		{"A-missing", -1, -1},
		// The join of upstream finds a row with Z_PK 0 too (shareinfo.py:140-146).
		{"A-zero", 0, 0},
		{"A-unknown", -1, -1},
	}
	for _, tt := range tests {
		p := &photoInfo{UUID: tt.uuid}
		if m, l := pk(db.SharedMomentInfo(p)), pk(db.ShareInfo(p)); m != tt.moment || l != tt.library {
			t.Errorf("%s: SharedMomentInfo %d, ShareInfo %d; want %d, %d", tt.uuid, m, l, tt.moment, tt.library)
		}
	}
}

func TestShareInfoLibraries(t *testing.T) {
	t.Parallel()

	// A share through ZLIBRARYSCOPE is checked whatever the test libraries hold.
	const scoped = "synthetic with a photo in a shared library"
	libs := map[string]string{scoped: testlib.OpenPristine(t, "synthetic")}
	change(t, libs[scoped], "UPDATE ZASSET SET ZLIBRARYSCOPE = (SELECT MIN(Z_PK) FROM ZSHARE) "+
		"WHERE Z_PK = (SELECT MIN(ZASSET) FROM ZADDITIONALASSETATTRIBUTES)")
	for _, name := range testlib.Libraries(t) {
		libs[name] = testlib.OpenPristine(t, name)
	}
	for name, path := range libs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, err := Open(t.Context(), path, Options{TempDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			src := roDB(t, path)
			hasTable := count(t, src, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'ZSHARE'") == 1
			for _, k := range []struct {
				column string
				get    func(*photoInfo) *photoinfo.ShareInfo
			}{{"ZMOMENTSHARE", db.SharedMomentInfo}, {"ZLIBRARYSCOPE", db.ShareInfo}} {
				linked := 0
				for i, p := range db.Photos() {
					var want *photoinfo.ShareInfo
					if hasTable {
						want = joinedShare(t, src, "SELECT s.Z_PK, s.ZTITLE, s.ZUUID FROM ZSHARE s JOIN ZASSET a ON a."+k.column+
							" = s.Z_PK WHERE a.ZUUID = ?", p.UUID)
					}
					got := k.get(p)
					if !sameShare(got, want) {
						t.Errorf("photo %d: the share differs from the ZSHARE row of ZASSET.%s", i, k.column)
					}
					if got != nil {
						linked++
					}
				}
				t.Logf("%d photos with a share through ZASSET.%s", linked, k.column)
				if name == scoped && linked == 0 {
					t.Errorf("no photo with a share through ZASSET.%s: the case is vacuous", k.column)
				}
			}
		})
	}
}

func sameShare(got, want *photoinfo.ShareInfo) bool {
	if got == nil || want == nil {
		return got == want
	}
	return got.PK == want.PK && got.Title == want.Title && got.UUID == want.UUID
}

func joinedShare(t *testing.T, src *sql.DB, query, photoUUID string) *photoinfo.ShareInfo {
	t.Helper()
	var (
		s         photoinfo.ShareInfo
		title, id sql.NullString
	)
	err := src.QueryRowContext(t.Context(), query, photoUUID).Scan(&s.PK, &title, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	s.Title, s.UUID = title.String, id.String
	return &s
}
