package photosdb

import (
	"database/sql"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/scoreinfo"
)

// A NULL column is nil, as upstream keeps None (photoinfo.py:1475-1503). A
// photo without a row has every score 0.0, the KeyError branch of upstream
// (photoinfo.py:1505-1535); each score is a pointer of its own, as the fields
// are exported.
func scoreInfo(r sqlread.Score, hasRow bool) scoreinfo.ScoreInfo {
	f := optionalFloat
	if !hasRow {
		f = func(sql.NullFloat64) *float64 { return new(float64) }
	}
	return scoreinfo.ScoreInfo{
		Overall:                 f(r.Overall),
		Curation:                f(r.Curation),
		Promotion:               f(r.Promotion),
		HighlightVisibility:     f(r.HighlightVisibility),
		Behavioral:              f(r.Behavioral),
		Failure:                 f(r.Failure),
		HarmoniousColor:         f(r.HarmoniousColor),
		Immersiveness:           f(r.Immersiveness),
		Interaction:             f(r.Interaction),
		InterestingSubject:      f(r.InterestingSubject),
		IntrusiveObjectPresence: f(r.IntrusiveObjectPresence),
		LivelyColor:             f(r.LivelyColor),
		LowLight:                f(r.LowLight),
		Noise:                   f(r.Noise),
		PleasantCameraTilt:      f(r.PleasantCameraTilt),
		PleasantComposition:     f(r.PleasantComposition),
		PleasantLighting:        f(r.PleasantLighting),
		PleasantPattern:         f(r.PleasantPattern),
		PleasantPerspective:     f(r.PleasantPerspective),
		PleasantPostProcessing:  f(r.PleasantPostProcessing),
		PleasantReflection:      f(r.PleasantReflection),
		PleasantSymmetry:        f(r.PleasantSymmetry),
		SharplyFocusedSubject:   f(r.SharplyFocusedSubject),
		TastefullyBlurred:       f(r.TastefullyBlurred),
		WellChosenSubject:       f(r.WellChosenSubject),
		WellFramedSubject:       f(r.WellFramedSubject),
		WellTimedShot:           f(r.WellTimedShot),
	}
}
