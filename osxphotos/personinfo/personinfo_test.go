package personinfo_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
)

const tolerance = 1e-9

func near(a, b float64) bool { return math.Abs(a-b) <= tolerance }

func checkArea(t *testing.T, got, want personinfo.Area) {
	t.Helper()
	if !near(got.X, want.X) || !near(got.Y, want.Y) || !near(got.H, want.H) || !near(got.W, want.W) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGeometry(t *testing.T) {
	// every face covers 800 pixels on a 4000x3000 photo; the center of b gives a
	// different, exactly representable point for every orientation.
	a := personinfo.FaceInfo{CenterX: 0.25, CenterY: 0.5, Size: 0.2}
	b := personinfo.FaceInfo{CenterX: 0.25, CenterY: 0.125, Size: 0.2}
	c := personinfo.FaceInfo{CenterX: -0.0001, CenterY: 0.3333, Size: 0.2}

	t.Run("center", func(t *testing.T) {
		tests := []struct {
			name        string
			face        personinfo.FaceInfo
			orientation int
			x, y        int
		}{
			{"a 1", a, 1, 1000, 1500},
			{"a 3", a, 3, 3000, 1500},
			{"a 6", a, 6, 1500, 3000},
			{"a 8", a, 8, 1500, 1000},
			{"b 0 is 1", b, 0, 1000, 2625},
			{"b 1", b, 1, 1000, 2625},
			{"b 2", b, 2, 3000, 2625},
			{"b 3", b, 3, 3000, 375},
			{"b 4", b, 4, 1000, 375},
			{"b 5", b, 5, 2625, 1000},
			{"b 6", b, 6, 2625, 3000},
			{"b 7", b, 7, 375, 3000},
			{"b 8", b, 8, 375, 1000},
			{"b 9 is left as stored", b, 9, 1000, 375},
			{"b -1 is left as stored", b, -1, 1000, 375},
			{"c truncates toward zero", c, 4, 0, 999},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				x, y := tt.face.Center(4000, 3000, tt.orientation)
				if x != tt.x || y != tt.y {
					t.Errorf("Center(4000, 3000, %d) = (%d, %d), want (%d, %d)", tt.orientation, x, y, tt.x, tt.y)
				}
			})
		}
	})

	t.Run("size and rect", func(t *testing.T) {
		if got := a.SizePixels(4000, 3000); !near(got, 800) {
			t.Errorf("SizePixels(4000, 3000) = %v, want 800", got)
		}
		if got := a.SizePixels(3000, 4000); !near(got, 800) {
			t.Errorf("SizePixels(3000, 4000) = %v, want 800", got)
		}
		tests := []struct {
			name           string
			face           personinfo.FaceInfo
			orientation    int
			x0, y0, x1, y1 float64
		}{
			{"a 1", a, 1, 600, 1100, 1400, 1900},
			{"a 6", a, 6, 1100, 2600, 1900, 3400},
			{"c truncates the center", c, 4, -400, 599, 400, 1399},
		}
		for _, tt := range tests {
			x0, y0, x1, y1 := tt.face.Rect(4000, 3000, tt.orientation)
			if !near(x0, tt.x0) || !near(y0, tt.y0) || !near(x1, tt.x1) || !near(y1, tt.y1) {
				t.Errorf("%s: Rect(4000, 3000, %d) = (%v, %v, %v, %v), want (%v, %v, %v, %v)",
					tt.name, tt.orientation, x0, y0, x1, y1, tt.x0, tt.y0, tt.x1, tt.y1)
			}
		}
	})

	wide := 800.0 / 4000
	tall := 800.0 / 3000

	t.Run("mwg area", func(t *testing.T) {
		checkArea(t, a.MWGArea(4000, 3000, 1), personinfo.Area{X: 0.25, Y: 0.5, H: tall, W: wide})
		checkArea(t, a.MWGArea(4000, 3000, 6), personinfo.Area{X: 0.5, Y: 0.75, H: wide, W: tall})
		checkArea(t, b.MWGArea(4000, 3000, 1), personinfo.Area{X: 0.25, Y: 0.875, H: tall, W: wide})
		checkArea(t, b.MWGArea(4000, 3000, 8), personinfo.Area{X: 0.125, Y: 0.25, H: wide, W: tall})
		checkArea(t, a.MWGArea(0, 3000, 1), personinfo.Area{})
		checkArea(t, a.MWGArea(4000, 0, 6), personinfo.Area{})
	})

	t.Run("mpri rect", func(t *testing.T) {
		upright := 0.25 - wide/2
		uprightY := 0.5 - tall/2
		checkArea(t, a.MPRIRect(4000, 3000, 1), personinfo.Area{X: upright, Y: uprightY, H: wide, W: tall})
		turnedX := 0.5 - tall/2
		turnedY := 0.75 - wide/2
		checkArea(t, a.MPRIRect(4000, 3000, 6), personinfo.Area{X: turnedX, Y: turnedY, H: tall, W: wide})
		checkArea(t, b.MPRIRect(4000, 3000, 1), personinfo.Area{X: 0.25 - wide/2, Y: 0.875 - tall/2, H: wide, W: tall})
		checkArea(t, a.MPRIRect(0, 3000, 1), personinfo.Area{})
		checkArea(t, a.MPRIRect(4000, 0, 6), personinfo.Area{})
	})

	t.Run("a photo of no size and any orientation", func(t *testing.T) {
		sizes := []struct{ width, height int }{{0, 0}, {0, 3000}, {4000, 0}}
		for orientation := -1; orientation <= 9; orientation++ {
			for _, s := range sizes {
				checkArea(t, a.MWGArea(s.width, s.height, orientation), personinfo.Area{})
				checkArea(t, a.MPRIRect(s.width, s.height, orientation), personinfo.Area{})
				x0, y0, x1, y1 := a.Rect(s.width, s.height, orientation)
				for _, v := range []float64{x0, y0, x1, y1} {
					if math.IsNaN(v) || math.IsInf(v, 0) {
						t.Errorf("Rect(%d, %d, %d) = (%v, %v, %v, %v), want finite", s.width, s.height, orientation, x0, y0, x1, y1)
						break
					}
				}
			}
		}
	})
}

// The cases of test_faceinfo.py:4619-4682: a detection with a zero face size,
// the center 0, 0 and the body region (0.4, 0.3, 0.6, 0.2).
func TestBodyRegion(t *testing.T) {
	body := func(detectionType int, size float64) personinfo.FaceInfo {
		return personinfo.FaceInfo{DetectionType: detectionType, Size: size, BodyCenterX: 0.4, BodyCenterY: 0.3, BodyWidth: 0.6, BodyHeight: 0.2}
	}

	t.Run("is pet", func(t *testing.T) {
		for detectionType, want := range map[int]bool{0: false, 1: false, 2: false, 3: true, 4: true, 5: false} {
			if got := (personinfo.FaceInfo{DetectionType: detectionType}).IsPet(); got != want {
				t.Errorf("IsPet of the detection type %d = %v, want %v", detectionType, got, want)
			}
		}
	})

	t.Run("orientation", func(t *testing.T) {
		for _, tt := range []struct {
			orientation int
			x, y, w, h  float64
		}{
			{1, 0.4, 0.7, 0.6, 0.2},
			{2, 0.6, 0.7, 0.6, 0.2},
			{3, 0.6, 0.3, 0.6, 0.2},
			{4, 0.4, 0.3, 0.6, 0.2},
			{5, 0.7, 0.4, 0.2, 0.6},
			{6, 0.7, 0.6, 0.2, 0.6},
			{7, 0.3, 0.6, 0.2, 0.6},
			{8, 0.3, 0.4, 0.2, 0.6},
		} {
			t.Run(strconv.Itoa(tt.orientation), func(t *testing.T) {
				checkArea(t, body(4, 0).MWGArea(4000, 3000, tt.orientation), personinfo.Area{X: tt.x, Y: tt.y, H: tt.h, W: tt.w})
			})
		}
	})

	// MPRI_Reg_Rect keeps the width in h and the height in w (personinfo.py:313).
	t.Run("dog and cat", func(t *testing.T) {
		for _, detectionType := range []int{3, 4} {
			pet := body(detectionType, 0)
			checkArea(t, pet.MWGArea(4000, 3000, 1), personinfo.Area{X: 0.4, Y: 0.7, H: 0.2, W: 0.6})
			checkArea(t, pet.MPRIRect(4000, 3000, 1), personinfo.Area{X: 0.1, Y: 0.6, H: 0.6, W: 0.2})
			checkArea(t, pet.MPRIRect(4000, 3000, 6), personinfo.Area{X: 0.6, Y: 0.3, H: 0.2, W: 0.6})
		}
	})

	t.Run("a person keeps the zero face region", func(t *testing.T) {
		checkArea(t, body(1, 0).MWGArea(4000, 3000, 1), personinfo.Area{X: 0, Y: 1})
	})

	t.Run("a pet without a body region", func(t *testing.T) {
		for _, size := range [][2]float64{{0, 0}, {0.6, 0}, {0, 0.2}} {
			pet := personinfo.FaceInfo{DetectionType: 4, BodyCenterX: 0.4, BodyCenterY: 0.3, BodyWidth: size[0], BodyHeight: size[1]}
			checkArea(t, pet.MWGArea(4000, 3000, 1), personinfo.Area{X: 0, Y: 1})
		}
	})

	t.Run("a pet with a face region", func(t *testing.T) {
		pet, person := body(3, 0.1), body(1, 0.1)
		checkArea(t, pet.MWGArea(4000, 3000, 6), person.MWGArea(4000, 3000, 6))
		checkArea(t, pet.MPRIRect(4000, 3000, 6), person.MPRIRect(4000, 3000, 6))
		if a := person.MWGArea(4000, 3000, 6); a.W == 0.2 || a.W == 0.6 {
			t.Errorf("the face region %+v has a size of the body region: the case cannot tell them apart", a)
		}
	})

	t.Run("a photo of no size", func(t *testing.T) {
		checkArea(t, body(4, 0).MWGArea(0, 3000, 1), personinfo.Area{})
		checkArea(t, body(4, 0).MPRIRect(4000, 0, 1), personinfo.Area{})
	})
}

func TestRotatePoint(t *testing.T) {
	tests := []struct {
		name                    string
		x, y, xmid, ymid, angle float64
		wantX, wantY            float64
	}{
		{"quarter turn about the origin", 1, 0, 0, 0, math.Pi / 2, 0, -1},
		{"quarter turn about a point", 3, 2, 2, 2, math.Pi / 2, 2, 1},
		{"quarter turn of a point below the center", 2, 3, 2, 2, math.Pi / 2, 3, 2},
		{"general turn", 3, 4, 1, 2, math.Pi / 6, 2 + math.Sqrt(3), 1 + math.Sqrt(3)},
		{"no turn", 5, 7, 1, 1, 0, 5, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y := personinfo.RotatePoint(tt.x, tt.y, tt.xmid, tt.ymid, tt.angle)
			if !near(x, tt.wantX) || !near(y, tt.wantY) {
				t.Errorf("RotatePoint = (%v, %v), want (%v, %v)", x, y, tt.wantX, tt.wantY)
			}
		})
	}
}
