package sqlread

import (
	"context"
	"database/sql"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// Score is one ZCOMPUTEDASSETATTRIBUTES row with the four scores of its
// ZASSET row (_photosdb_process_scoreinfo.py:53-98). Behavioral and
// Interaction are read for Photos 10 and earlier only and are not valid
// otherwise, where upstream selects null (62-76).
type Score struct {
	PK, AssetPK int64

	Overall, Curation, Promotion, HighlightVisibility sql.NullFloat64

	Behavioral, Interaction sql.NullFloat64

	Failure, HarmoniousColor, Immersiveness, InterestingSubject, IntrusiveObjectPresence, LivelyColor, LowLight, Noise sql.NullFloat64

	PleasantCameraTilt, PleasantComposition, PleasantLighting, PleasantPattern, PleasantPerspective sql.NullFloat64

	PleasantPostProcessing, PleasantReflection, PleasantSymmetry sql.NullFloat64

	SharplyFocusedSubject, TastefullyBlurred, WellChosenSubject, WellFramedSubject, WellTimedShot sql.NullFloat64
}

// The columns and their destinations are listed together so that the Scan
// keeps their order.
func scoreColumns(s schema.Schema, r *Score) ([]column, []any) {
	a := func(name string) column { return column{"a", "ZASSET", name} }
	c := func(name string) column { return column{"c", "ZCOMPUTEDASSETATTRIBUTES", name} }
	type field struct {
		col column
		to  *sql.NullFloat64
	}
	fields := []field{
		{a("ZOVERALLAESTHETICSCORE"), &r.Overall}, {a("ZCURATIONSCORE"), &r.Curation},
		{a("ZPROMOTIONSCORE"), &r.Promotion}, {a("ZHIGHLIGHTVISIBILITYSCORE"), &r.HighlightVisibility},
		{c("ZFAILURESCORE"), &r.Failure}, {c("ZHARMONIOUSCOLORSCORE"), &r.HarmoniousColor},
		{c("ZIMMERSIVENESSSCORE"), &r.Immersiveness}, {c("ZINTERESTINGSUBJECTSCORE"), &r.InterestingSubject},
		{c("ZINTRUSIVEOBJECTPRESENCESCORE"), &r.IntrusiveObjectPresence}, {c("ZLIVELYCOLORSCORE"), &r.LivelyColor},
		{c("ZLOWLIGHT"), &r.LowLight}, {c("ZNOISESCORE"), &r.Noise},
		{c("ZPLEASANTCAMERATILTSCORE"), &r.PleasantCameraTilt}, {c("ZPLEASANTCOMPOSITIONSCORE"), &r.PleasantComposition},
		{c("ZPLEASANTLIGHTINGSCORE"), &r.PleasantLighting}, {c("ZPLEASANTPATTERNSCORE"), &r.PleasantPattern},
		{c("ZPLEASANTPERSPECTIVESCORE"), &r.PleasantPerspective}, {c("ZPLEASANTPOSTPROCESSINGSCORE"), &r.PleasantPostProcessing},
		{c("ZPLEASANTREFLECTIONSSCORE"), &r.PleasantReflection}, {c("ZPLEASANTSYMMETRYSCORE"), &r.PleasantSymmetry},
		{c("ZSHARPLYFOCUSEDSUBJECTSCORE"), &r.SharplyFocusedSubject}, {c("ZTASTEFULLYBLURREDSCORE"), &r.TastefullyBlurred},
		{c("ZWELLCHOSENSUBJECTSCORE"), &r.WellChosenSubject}, {c("ZWELLFRAMEDSUBJECTSCORE"), &r.WellFramedSubject},
		{c("ZWELLTIMEDSHOTSCORE"), &r.WellTimedShot},
	}
	if s.PhotosVersion <= 10 {
		fields = append(fields, field{c("ZBEHAVIORALSCORE"), &r.Behavioral}, field{c("ZINTERACTIONSCORE"), &r.Interaction})
	}
	cols := []column{c("Z_PK"), c("ZASSET")}
	dst := []any{&r.PK, &r.AssetPK}
	for _, f := range fields {
		cols = append(cols, f.col)
		dst = append(dst, f.to)
	}
	return cols, dst
}

func scoreSelect(s schema.Schema) []column {
	cols, _ := scoreColumns(s, &Score{})
	return cols
}

// Scores returns the rows whose ZASSET is an asset, ordered by ZASSET, then
// Z_PK, with the columns of s. Upstream joins ZASSET the same way, so a row
// without an asset is dropped (_photosdb_process_scoreinfo.py:97-98).
func (d *DB) Scores(ctx context.Context, s schema.Schema) ([]Score, error) {
	var r Score
	cols, dst := scoreColumns(s, &r)
	q := "SELECT " + selectList(cols) +
		" FROM ZCOMPUTEDASSETATTRIBUTES c JOIN ZASSET a ON a.Z_PK = c.ZASSET ORDER BY c.ZASSET, c.Z_PK"
	var out []Score
	err := d.readRows(ctx, "scores", q, func(rows *sql.Rows) error {
		if err := rows.Scan(dst...); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
