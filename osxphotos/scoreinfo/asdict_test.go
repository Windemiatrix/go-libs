package scoreinfo_test

import (
	"reflect"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/scoreinfo"
)

// The keys of scoreinfo.py:14-40 in file order; the filled score gives the
// i-th key the value i+1, so a key read from another field is seen.
var keys = []string{
	"overall", "curation", "promotion", "highlight_visibility", "behavioral", "failure", "harmonious_color",
	"immersiveness", "interaction", "interesting_subject", "intrusive_object_presence", "lively_color", "low_light",
	"noise", "pleasant_camera_tilt", "pleasant_composition", "pleasant_lighting", "pleasant_pattern",
	"pleasant_perspective", "pleasant_post_processing", "pleasant_reflection", "pleasant_symmetry",
	"sharply_focused_subject", "tastefully_blurred", "well_chosen_subject", "well_framed_subject", "well_timed_shot",
}

func TestAsDict(t *testing.T) {
	filled := scoreinfo.ScoreInfo{
		Overall: 1, Curation: 2, Promotion: 3, HighlightVisibility: 4, Behavioral: 5, Failure: 6, HarmoniousColor: 7,
		Immersiveness: 8, Interaction: 9, InterestingSubject: 10, IntrusiveObjectPresence: 11, LivelyColor: 12, LowLight: 13,
		Noise: 14, PleasantCameraTilt: 15, PleasantComposition: 16, PleasantLighting: 17, PleasantPattern: 18,
		PleasantPerspective: 19, PleasantPostProcessing: 20, PleasantReflection: 21, PleasantSymmetry: 22,
		SharplyFocusedSubject: 23, TastefullyBlurred: 24, WellChosenSubject: 25, WellFramedSubject: 26, WellTimedShot: 27,
	}
	wantFilled, wantZero := map[string]any{}, map[string]any{}
	for i, k := range keys {
		wantFilled[k], wantZero[k] = float64(i+1), 0.0
	}
	for _, tt := range []struct {
		name string
		s    scoreinfo.ScoreInfo
		want map[string]any
	}{
		{"filled", filled, wantFilled},
		{"no row: 0.0, as upstream", scoreinfo.ScoreInfo{}, wantZero},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.AsDict(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AsDict =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}
