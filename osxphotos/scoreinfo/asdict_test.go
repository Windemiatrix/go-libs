package scoreinfo_test

import (
	"maps"
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
	var filled scoreinfo.ScoreInfo
	fields := reflect.ValueOf(&filled).Elem()
	if fields.NumField() != len(keys) {
		t.Fatalf("ScoreInfo has %d fields, scoreinfo.py:14-40 %d", fields.NumField(), len(keys))
	}
	wantFilled, wantNone := map[string]any{}, map[string]any{}
	for i, k := range keys {
		v := float64(i + 1)
		fields.Field(i).Set(reflect.ValueOf(&v))
		wantFilled[k], wantNone[k] = v, nil
	}
	zero := 0.0
	wantZeroBehavioral := maps.Clone(wantNone)
	wantZeroBehavioral["behavioral"] = 0.0
	for _, tt := range []struct {
		name string
		s    scoreinfo.ScoreInfo
		want map[string]any
	}{
		{"filled", filled, wantFilled},
		{"every score None", scoreinfo.ScoreInfo{}, wantNone},
		{"0.0 is not None", scoreinfo.ScoreInfo{Behavioral: &zero}, wantZeroBehavioral},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.AsDict(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AsDict =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}
