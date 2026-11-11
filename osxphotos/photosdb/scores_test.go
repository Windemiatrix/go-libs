package photosdb

import (
	"database/sql"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/scoreinfo"
)

// Every column has its own value, so that two swapped columns show.
var fullScoreRow = sqlread.Score{
	Overall: nullFloat(0.01), Curation: nullFloat(0.02), Promotion: nullFloat(0.03), HighlightVisibility: nullFloat(0.04),
	Behavioral: nullFloat(0.05), Failure: nullFloat(0.06), HarmoniousColor: nullFloat(0.07), Immersiveness: nullFloat(0.08),
	Interaction: nullFloat(0.09), InterestingSubject: nullFloat(0.1), IntrusiveObjectPresence: nullFloat(0.11),
	LivelyColor: nullFloat(0.12), LowLight: nullFloat(0.13), Noise: nullFloat(0.14), PleasantCameraTilt: nullFloat(0.15),
	PleasantComposition: nullFloat(0.16), PleasantLighting: nullFloat(0.17), PleasantPattern: nullFloat(0.18),
	PleasantPerspective: nullFloat(0.19), PleasantPostProcessing: nullFloat(0.2), PleasantReflection: nullFloat(0.21),
	PleasantSymmetry: nullFloat(0.22), SharplyFocusedSubject: nullFloat(0.23), TastefullyBlurred: nullFloat(0.24),
	WellChosenSubject: nullFloat(0.25), WellFramedSubject: nullFloat(0.26), WellTimedShot: nullFloat(0.27),
}

func ptr[T any](v T) *T { return &v }

// A new value on every call: a case may change it.
func fullScoreInfo() scoreinfo.ScoreInfo {
	return scoreinfo.ScoreInfo{
		Overall: ptr(0.01), Curation: ptr(0.02), Promotion: ptr(0.03), HighlightVisibility: ptr(0.04),
		Behavioral: ptr(0.05), Failure: ptr(0.06), HarmoniousColor: ptr(0.07), Immersiveness: ptr(0.08),
		Interaction: ptr(0.09), InterestingSubject: ptr(0.1), IntrusiveObjectPresence: ptr(0.11),
		LivelyColor: ptr(0.12), LowLight: ptr(0.13), Noise: ptr(0.14), PleasantCameraTilt: ptr(0.15),
		PleasantComposition: ptr(0.16), PleasantLighting: ptr(0.17), PleasantPattern: ptr(0.18),
		PleasantPerspective: ptr(0.19), PleasantPostProcessing: ptr(0.2), PleasantReflection: ptr(0.21),
		PleasantSymmetry: ptr(0.22), SharplyFocusedSubject: ptr(0.23), TastefullyBlurred: ptr(0.24),
		WellChosenSubject: ptr(0.25), WellFramedSubject: ptr(0.26), WellTimedShot: ptr(0.27),
	}
}

// A photo without a row: every score 0.0, not None (photoinfo.py:1505-1535).
func noScoreRow() scoreinfo.ScoreInfo {
	var s scoreinfo.ScoreInfo
	fields := reflect.ValueOf(&s).Elem()
	for i := range fields.NumField() {
		fields.Field(i).Set(reflect.ValueOf(new(float64)))
	}
	return s
}

// The expected values are literals: ScoreInfo of a photo from the rows of
// ZCOMPUTEDASSETATTRIBUTES (photoinfo.py:1459-1535,
// _photosdb_process_scoreinfo.py:132-162). The Z_PK of a row is never the ZASSET
// of its photo.
func TestLoadScores(t *testing.T) {
	t.Parallel()

	row := func(pk, asset int64, edit func(*sqlread.Score)) sqlread.Score {
		r := fullScoreRow
		r.PK, r.AssetPK = pk, asset
		if edit != nil {
			edit(&r)
		}
		return r
	}
	failure := func(v float64) func(*sqlread.Score) { return func(r *sqlread.Score) { r.Failure = nullFloat(v) } }
	withFailure := func(v *float64) scoreinfo.ScoreInfo {
		s := fullScoreInfo()
		s.Failure = v
		return s
	}
	half, sevenTenths := 0.5, 0.7

	tests := []struct {
		name string
		rows []sqlread.Score
		want scoreinfo.ScoreInfo
	}{
		{"every column", []sqlread.Score{row(11, 1, nil)}, fullScoreInfo()},
		{"a NULL column is None", []sqlread.Score{row(11, 1, func(r *sqlread.Score) { r.Failure = sql.NullFloat64{} })}, withFailure(nil)},
		{"every column NULL is None", []sqlread.Score{{PK: 11, AssetPK: 1}}, scoreinfo.ScoreInfo{}},
		{"no row: 0.0", nil, noScoreRow()},
		{"the row of another asset", []sqlread.Score{row(11, 99, nil)}, noScoreRow()},
		{"the highest Z_PK decides, rows in its order", []sqlread.Score{row(15, 1, failure(0.5)), row(17, 1, failure(0.7))}, withFailure(&sevenTenths)},
		{"the highest Z_PK decides, rows in the other order", []sqlread.Score{row(17, 1, failure(0.7)), row(15, 1, failure(0.5))}, withFailure(&sevenTenths)},
		{"the highest Z_PK is a row of NULLs", []sqlread.Score{row(15, 1, nil), {PK: 17, AssetPK: 1}}, scoreinfo.ScoreInfo{}},
	}
	for _, tt := range tests {
		c, err := load(tables{assets: []sqlread.Asset{baseAsset(1, "A-1")}, scores: tt.rows})
		if err != nil {
			t.Fatalf("%s: load: %v", tt.name, err)
		}
		if got := c.byUUID["A-1"].Score; !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: Score %v, want %v", tt.name, got.AsDict(), tt.want.AsDict())
		}
	}

	c, err := load(tables{
		assets: []sqlread.Asset{baseAsset(1, "A-1"), baseAsset(2, "A-2"), baseAsset(3, "A-3"), baseAsset(4, "A-4")},
		scores: []sqlread.Score{row(21, 1, nil), row(22, 2, failure(0.5))},
	})
	if err != nil {
		t.Fatal(err)
	}
	for uuid, want := range map[string]scoreinfo.ScoreInfo{"A-1": fullScoreInfo(), "A-2": withFailure(&half), "A-3": noScoreRow(), "A-4": noScoreRow()} {
		if got := c.byUUID[uuid].Score; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: Score %v, want %v: each photo takes the row of its own ZASSET", uuid, got.AsDict(), want.AsDict())
		}
	}
	*c.byUUID["A-3"].Score.Overall = 1
	if s := c.byUUID["A-3"].Score; *s.Curation != 0 || *c.byUUID["A-4"].Score.Overall != 0 {
		t.Error("the scores of a photo without a row share a value: a change of one changed another")
	}
}

// The rows are written into the copy of the synthetic library, so that the
// case does not depend on the rows it has; they are picked by integer keys, so
// that no identifier of a library is in the test.
func TestLoadScoresDamaged(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	before, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	photos := before.Photos()
	if len(photos) < 4 {
		t.Fatalf("the synthetic library has %d photos, want 4 or more: the case is vacuous", len(photos))
	}
	pk := func(i int) string { return strconv.FormatInt(photos[i].PK, 10) }
	const insert = "INSERT INTO ZCOMPUTEDASSETATTRIBUTES (Z_PK, ZASSET, ZFAILURESCORE, ZNOISESCORE) VALUES "
	update := func(i int, overall, curation, promotion, highlight string) string {
		return "UPDATE ZASSET SET ZOVERALLAESTHETICSCORE = " + overall + ", ZCURATIONSCORE = " + curation +
			", ZPROMOTIONSCORE = " + promotion + ", ZHIGHLIGHTVISIBILITYSCORE = " + highlight + " WHERE Z_PK = " + pk(i)
	}
	change(t, lib,
		"DELETE FROM ZCOMPUTEDASSETATTRIBUTES WHERE ZASSET IN ("+pk(0)+", "+pk(1)+", "+pk(2)+")",
		insert+"(900001, "+pk(0)+", NULL, 0.125)",
		insert+"(900002, "+pk(2)+", 0.1, NULL)",
		insert+"(900003, "+pk(2)+", 0.9, NULL)",
		update(0, "0.5", "0.25", "0.125", "0.0625"),
		update(1, "0.75", "0.5", "0.25", "0.125"),
		update(2, "0.375", "0.3125", "0.1875", "0.09375"),
	)

	after, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	score := func(i int) scoreinfo.ScoreInfo {
		p, ok := after.Photo(photos[i].UUID)
		if !ok {
			t.Fatalf("photo %d is gone", i)
		}
		return p.Score
	}
	// The inserted rows set five columns, the rest are NULL: the expected value is nil, not 0.
	for _, c := range []struct {
		name string
		got  scoreinfo.ScoreInfo
		want scoreinfo.ScoreInfo
	}{
		{"a NULL column", score(0), scoreinfo.ScoreInfo{Overall: ptr(0.5), Curation: ptr(0.25), Promotion: ptr(0.125), HighlightVisibility: ptr(0.0625), Noise: ptr(0.125)}},
		{"no row, scores in ZASSET", score(1), noScoreRow()},
		{"two rows", score(2), scoreinfo.ScoreInfo{Overall: ptr(0.375), Curation: ptr(0.3125), Promotion: ptr(0.1875), HighlightVisibility: ptr(0.09375), Failure: ptr(0.9)}},
	} {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s: Score %v, want %v", c.name, c.got.AsDict(), c.want.AsDict())
		}
	}

	var changed int
	for i, p := range photos[3:] {
		if !reflect.DeepEqual(score(i+3), p.Score) {
			changed++
		}
	}
	if changed != 0 {
		t.Errorf("%d of the %d other photos changed their score", changed, len(photos)-3)
	}
}

// Upstream keeps a value of any type (_photosdb_process_scoreinfo.py:132-162);
// the difference is recorded in UPSTREAM.md.
func TestOpenScoreWrongType(t *testing.T) {
	t.Parallel()

	lib := testlib.OpenPristine(t, "synthetic")
	before, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Photos()) == 0 {
		t.Fatal("the synthetic library has no photos: the case is vacuous")
	}
	pk := strconv.FormatInt(before.Photos()[0].PK, 10)
	change(t, lib,
		"DELETE FROM ZCOMPUTEDASSETATTRIBUTES WHERE ZASSET = "+pk,
		"INSERT INTO ZCOMPUTEDASSETATTRIBUTES (Z_PK, ZASSET, ZFAILURESCORE) VALUES (900001, "+pk+", 'SECRET-S')",
	)
	_, err = Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "scores: row ") || !strings.Contains(err.Error(), "unexpected value type") {
		t.Fatalf("Open error %v, want the scores reader to refuse the text in ZFAILURESCORE", err)
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Errorf("Open error %q holds the value", err)
	}
}

// The oracle is the database: the counts of NULLs and of non-zero values and
// the sums of the same rows by SQL, weighted by the Z_PK of the photo, so that
// a score given to another photo shows. The row of a photo is the one with the
// highest Z_PK. The messages hold counts, not the scores of a library.
func TestLoadScoresLibraries(t *testing.T) {
	t.Parallel()

	const rows = " FROM ZASSET a JOIN ZCOMPUTEDASSETATTRIBUTES c ON c.ZASSET = a.Z_PK" +
		" WHERE EXISTS (SELECT 1 FROM ZADDITIONALASSETATTRIBUTES x WHERE x.ZASSET = a.Z_PK)" +
		" AND c.Z_PK = (SELECT MAX(c2.Z_PK) FROM ZCOMPUTEDASSETATTRIBUTES c2 WHERE c2.ZASSET = a.Z_PK)"
	type column struct {
		name, expr string
		get        func(scoreinfo.ScoreInfo) *float64
	}
	columns := []column{
		{"overall", "a.ZOVERALLAESTHETICSCORE", func(s scoreinfo.ScoreInfo) *float64 { return s.Overall }},
		{"failure", "c.ZFAILURESCORE", func(s scoreinfo.ScoreInfo) *float64 { return s.Failure }},
		{"low light", "c.ZLOWLIGHT", func(s scoreinfo.ScoreInfo) *float64 { return s.LowLight }},
		{"well timed shot", "c.ZWELLTIMEDSHOTSCORE", func(s scoreinfo.ScoreInfo) *float64 { return s.WellTimedShot }},
	}

	var overall, failure, noBehavioral atomic.Int64
	t.Run("libraries", func(t *testing.T) {
		for _, lib := range goldenLibraries(t) {
			t.Run(lib.name, func(t *testing.T) {
				t.Parallel()
				db, err := Open(t.Context(), lib.path, Options{TempDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				ro := roDB(t, lib.path)
				checks := slices.Clone(columns)
				if db.info.PhotosVersion <= 10 {
					checks = append(checks,
						column{"behavioral", "c.ZBEHAVIORALSCORE", func(s scoreinfo.ScoreInfo) *float64 { return s.Behavioral }},
						column{"interaction", "c.ZINTERACTIONSCORE", func(s scoreinfo.ScoreInfo) *float64 { return s.Interaction }})
				}

				hasRow := map[string]bool{}
				for _, id := range testlib.Rows(t, ro, "SELECT a.Z_PK"+rows) {
					hasRow[id] = true
				}
				var withRow, behavioral int
				for i, p := range db.Photos() {
					if !hasRow[strconv.FormatInt(p.PK, 10)] {
						if !reflect.DeepEqual(p.Score, noScoreRow()) {
							t.Errorf("photo %d: without a row in the database, a score is not 0.0", i)
						}
						continue
					}
					withRow++
					if p.Score.Behavioral != nil || p.Score.Interaction != nil {
						behavioral++
					}
				}
				if db.info.PhotosVersion > 10 {
					if behavioral != 0 {
						t.Errorf("%d photos with a row have Behavioral or Interaction, want None from Photos 11 on", behavioral)
					}
					noBehavioral.Add(int64(withRow))
				}

				for _, c := range checks {
					var wantN int
					var wantSum float64
					query := "SELECT COUNT(*), COALESCE(SUM(" + c.expr + " * a.Z_PK), 0)" + rows + " AND COALESCE(" + c.expr + ", 0) != 0"
					if err := ro.QueryRowContext(t.Context(), query).Scan(&wantN, &wantSum); err != nil {
						t.Fatalf("%s: %v", c.name, err)
					}
					wantNulls := testlib.Count(t, ro, "SELECT COUNT(*)"+rows+" AND "+c.expr+" IS NULL")
					var n, nulls int
					var sum float64
					for _, p := range db.Photos() {
						switch v := c.get(p.Score); {
						case v == nil:
							nulls++
						case *v != 0:
							n++
							sum += *v * float64(p.PK)
						}
					}
					if n != wantN || math.Abs(sum-wantSum) > 1e-6*(1+math.Abs(wantSum)) {
						t.Errorf("%s: %d photos with a score, the weighted sum differs by %g; the database has %d", c.name, n, sum-wantSum, wantN)
					}
					if nulls != wantNulls {
						t.Errorf("%s: %d photos with None, the database has %d NULLs", c.name, nulls, wantNulls)
					}
					t.Logf("Photos %v: %s: %d photos with a non-zero value, %d with None", db.info.PhotosVersion, c.name, n, nulls)
					switch c.name {
					case "overall":
						overall.Add(int64(n))
					case "failure":
						failure.Add(int64(n))
					}
				}
				t.Logf("Photos %v: %d photos, %d with a row, %d of them with Behavioral or Interaction",
					db.info.PhotosVersion, len(db.Photos()), withRow, behavioral)
			})
		}
	})
	if overall.Load() == 0 || failure.Load() == 0 {
		t.Error("no photo has Overall, or none has Failure, in any library: the test is vacuous")
	}
	if noBehavioral.Load() == 0 {
		t.Error("no photo of Photos 11 or later has a row in any library: None of Behavioral is not tested")
	}
}
