package photosdb

import (
	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/scoreinfo"
)

// A NULL column is 0, where upstream keeps None (photoinfo.py:1477-1505). A
// photo without a row gets the zero value of r, which is the KeyError branch of
// upstream (photoinfo.py:1507-1537).
func scoreInfo(r sqlread.Score) scoreinfo.ScoreInfo {
	return scoreinfo.ScoreInfo{
		Overall:                 r.Overall.Float64,
		Curation:                r.Curation.Float64,
		Promotion:               r.Promotion.Float64,
		HighlightVisibility:     r.HighlightVisibility.Float64,
		Behavioral:              r.Behavioral.Float64,
		Failure:                 r.Failure.Float64,
		HarmoniousColor:         r.HarmoniousColor.Float64,
		Immersiveness:           r.Immersiveness.Float64,
		Interaction:             r.Interaction.Float64,
		InterestingSubject:      r.InterestingSubject.Float64,
		IntrusiveObjectPresence: r.IntrusiveObjectPresence.Float64,
		LivelyColor:             r.LivelyColor.Float64,
		LowLight:                r.LowLight.Float64,
		Noise:                   r.Noise.Float64,
		PleasantCameraTilt:      r.PleasantCameraTilt.Float64,
		PleasantComposition:     r.PleasantComposition.Float64,
		PleasantLighting:        r.PleasantLighting.Float64,
		PleasantPattern:         r.PleasantPattern.Float64,
		PleasantPerspective:     r.PleasantPerspective.Float64,
		PleasantPostProcessing:  r.PleasantPostProcessing.Float64,
		PleasantReflection:      r.PleasantReflection.Float64,
		PleasantSymmetry:        r.PleasantSymmetry.Float64,
		SharplyFocusedSubject:   r.SharplyFocusedSubject.Float64,
		TastefullyBlurred:       r.TastefullyBlurred.Float64,
		WellChosenSubject:       r.WellChosenSubject.Float64,
		WellFramedSubject:       r.WellFramedSubject.Float64,
		WellTimedShot:           r.WellTimedShot.Float64,
	}
}
