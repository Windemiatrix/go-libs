package personinfo_test

import (
	"reflect"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
)

func TestPersonInfoAsDict(t *testing.T) {
	tests := []struct {
		name string
		p    personinfo.PersonInfo
		want map[string]any
	}{
		{
			name: "filled",
			p: personinfo.PersonInfo{
				UUID: "P-1", Name: "Pat Example", DisplayName: "Pat", KeyFace: 7, FaceCount: 3,
				KeyPhotoUUID: "A-1", Favorite: true, SortOrder: 2,
			},
			want: map[string]any{
				"uuid": "P-1", "name": "Pat Example", "displayname": "Pat", "keyface": int64(7), "facecount": 3,
				"keyphoto": "A-1", "favorite": true, "sort_order": 2, "feature_less": false,
			},
		},
		{
			name: "feature less, not a favorite",
			p:    personinfo.PersonInfo{FeatureLess: true},
			want: map[string]any{
				"uuid": nil, "name": "", "displayname": nil, "keyface": int64(0), "facecount": 0,
				"keyphoto": nil, "favorite": false, "sort_order": 0, "feature_less": true,
			},
		},
		{
			name: "zero",
			want: map[string]any{
				"uuid": nil, "name": "", "displayname": nil, "keyface": int64(0), "facecount": 0,
				"keyphoto": nil, "favorite": false, "sort_order": 0, "feature_less": false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.AsDict(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AsDict =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

func TestFaceInfoAsDict(t *testing.T) {
	area := func(a personinfo.Area) map[string]any { return map[string]any{"x": a.X, "y": a.Y, "h": a.H, "w": a.W} }
	zeroArea := map[string]any{"x": 0, "y": 0, "h": 0, "w": 0}

	t.Run("filled", func(t *testing.T) {
		f := personinfo.FaceInfo{
			PK: 7, UUID: "F-7", Name: "Pat Example", AssetUUID: "A-1", PersonPK: 2, PersonUUID: "P-1",
			CenterX: 0.25, CenterY: 0.125, Size: 0.2, Quality: 0.75, SourceWidth: 4000, SourceHeight: 3000,
			HasSmile: true, InTrash: true,
			AgeType: 1, EyeMakeupType: 2, EyeState: 3, FacialHairType: 4, GenderType: 5, GlassesType: 6,
			HairColorType: 7, LipMakeupType: 8, SmileType: 9, Roll: 0.5,
		}
		// Orientation 6 swaps the axes, so that width and height in the wrong order give other values.
		x0, y0, x1, y1 := f.Rect(4000, 3000, 6)
		want := map[string]any{
			"_pk": int64(7), "uuid": "F-7", "name": "Pat Example", "asset_uuid": "A-1", "_person_pk": int64(2),
			"center_x": 0.25, "center_y": 0.125, "center": []any{2625, 3000}, "size": 0.2, "face_rect": []any{[]any{x0, y0}, []any{x1, y1}},
			"mpri_reg_rect": area(f.MPRIRect(4000, 3000, 6)), "mwg_rs_area": area(f.MWGArea(4000, 3000, 6)),
			"roll": 0.5, "pitch": 0, "yaw": 0, "quality": 0.75, "source_width": 4000, "source_height": 3000,
			"has_smile": 1, "manual": 0, "face_type": nil, "age_type": 1, "eye_makeup_type": 2, "eye_state": 3,
			"facial_hair_type": 4, "gender_type": 5, "glasses_type": 6, "hair_color_type": 7, "intrash": 1,
			"lip_makeup_type": 8, "smile_type": 9,
		}
		if got := f.AsDict(4000, 3000, 6); !reflect.DeepEqual(got, want) {
			t.Errorf("AsDict =\n%#v\nwant\n%#v", got, want)
		}
		if x0 == y0 || reflect.DeepEqual(f.MPRIRect(4000, 3000, 6), f.MWGArea(4000, 3000, 6)) {
			t.Error("the corners or the two areas are equal: the case cannot tell them apart")
		}
	})

	// With the filled case, each flag has its own pattern across the cases, so that two swapped keys fail.
	t.Run("flags", func(t *testing.T) {
		for _, tt := range []struct {
			f    personinfo.FaceInfo
			want [3]any
		}{
			{personinfo.FaceInfo{Manual: true, InTrash: true}, [3]any{0, 1, 1}},
			{personinfo.FaceInfo{HasSmile: true}, [3]any{1, 0, 0}},
		} {
			d := tt.f.AsDict(4000, 3000, 1)
			if got := [3]any{d["has_smile"], d["manual"], d["intrash"]}; got != tt.want {
				t.Errorf("%+v: has_smile, manual, intrash = %v, want %v", tt.f, got, tt.want)
			}
		}
	})

	t.Run("photo without a width or a height", func(t *testing.T) {
		f := personinfo.FaceInfo{CenterX: 0.25, CenterY: 0.125, Size: 0.2}
		for _, size := range [][2]int{{0, 3000}, {4000, 0}} {
			d := f.AsDict(size[0], size[1], 1)
			if !reflect.DeepEqual(d["mpri_reg_rect"], zeroArea) || !reflect.DeepEqual(d["mwg_rs_area"], zeroArea) {
				t.Errorf("AsDict(%d, %d, 1): mpri_reg_rect = %v, mwg_rs_area = %v, want %v", size[0], size[1], d["mpri_reg_rect"], d["mwg_rs_area"], zeroArea)
			}
		}
	})

	// mwg_rs_area and mpri_reg_rect of a photo without a size are integer zeros (personinfo.py:247-248, 278-279).
	t.Run("zero", func(t *testing.T) {
		want := map[string]any{
			"_pk": int64(0), "uuid": nil, "name": nil, "asset_uuid": nil, "_person_pk": int64(0),
			"center_x": 0.0, "center_y": 0.0, "center": []any{0, 0}, "size": 0.0, "face_rect": []any{[]any{0.0, 0.0}, []any{0.0, 0.0}},
			"mpri_reg_rect": zeroArea, "mwg_rs_area": zeroArea,
			"roll": 0.0, "pitch": 0, "yaw": 0, "quality": 0.0, "source_width": 0, "source_height": 0,
			"has_smile": 0, "manual": 0, "face_type": nil, "age_type": 0, "eye_makeup_type": 0, "eye_state": 0,
			"facial_hair_type": 0, "gender_type": 0, "glasses_type": 0, "hair_color_type": 0, "intrash": 0,
			"lip_makeup_type": 0, "smile_type": 0,
		}
		if got := (personinfo.FaceInfo{}).AsDict(0, 0, 0); !reflect.DeepEqual(got, want) {
			t.Errorf("AsDict =\n%#v\nwant\n%#v", got, want)
		}
	})
}
