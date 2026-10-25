package query

import (
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func flagsTwin(uuid string) *photoinfo.PhotoInfo {
	return &photoinfo.PhotoInfo{UUID: uuid, IsPhoto: true, OriginalFilesize: 2048, Date: at("2023-05-01T10:00:00Z"),
		Height: 3024, Width: 4032, UTI: "public.heic", AdjustmentType: ptr(0)}
}

func TestDuplicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(p *photoinfo.PhotoInfo)
		dup  bool
	}{
		{"same signature", func(*photoinfo.PhotoInfo) {}, true},
		{"original filesize", func(p *photoinfo.PhotoInfo) { p.OriginalFilesize++ }, false},
		{"date", func(p *photoinfo.PhotoInfo) { p.Date = p.Date.Add(time.Second) }, false},
		{"height", func(p *photoinfo.PhotoInfo) { p.Height++ }, false},
		{"width", func(p *photoinfo.PhotoInfo) { p.Width++ }, false},
		{"uti", func(p *photoinfo.PhotoInfo) { p.UTI = "public.jpeg" }, false},
		{"adjustment type nil against 0", func(p *photoinfo.PhotoInfo) { p.AdjustmentType = nil }, false},
		{"adjustment type 2 against 0", func(p *photoinfo.PhotoInfo) { p.AdjustmentType = ptr(2) }, false},
		// photosdb.py:3419 takes the raw column, not hasadjustments.
		{"has adjustments", func(p *photoinfo.PhotoInfo) { p.HasAdjustments = true }, true},
		{"same moment in another zone", func(p *photoinfo.PhotoInfo) { p.Date = p.Date.In(time.FixedZone("+03", 3*3600)) }, true},
		{"other fields", func(p *photoinfo.PhotoInfo) {
			p.OriginalFilename, p.InTrash, p.IsPhoto, p.IsMovie = "IMG_0002.HEIC", true, false, true
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, b, x, c := flagsTwin("A"), flagsTwin("B"), flagsTwin("X"), flagsTwin("C")
			tt.edit(x)
			want := []string{"B", "C"}
			if tt.dup {
				want = []string{"B", "X", "C"}
			}
			if got := uuids(Duplicates(a, []*photoinfo.PhotoInfo{a, b, x, c})); !slices.Equal(got, want) {
				t.Errorf("Duplicates = %q, want %q", got, want)
			}
		})
	}

	a, b := flagsTwin("A"), flagsTwin("B")
	a.AdjustmentType, b.AdjustmentType = nil, nil
	if got := uuids(Duplicates(a, []*photoinfo.PhotoInfo{a, b})); !slices.Equal(got, []string{"B"}) {
		t.Errorf("Duplicates with AdjustmentType nil on both = %q, want [B]", got)
	}
	if got := Duplicates(a, []*photoinfo.PhotoInfo{a, flagsTwin("A")}); got != nil {
		t.Errorf("Duplicates of a photo with no other = %q, want nil", uuids(got))
	}
}

func TestApplyDuplicate(t *testing.T) {
	t.Parallel()

	twin := func(uuid, date string, edit func(*photoinfo.PhotoInfo)) *photoinfo.PhotoInfo {
		p := flagsTwin(uuid)
		p.Date = at(date)
		if edit != nil {
			edit(p)
		}
		return p
	}
	trash := func(p *photoinfo.PhotoInfo) { p.InTrash = true }
	located := func(p *photoinfo.PhotoInfo) { p.Latitude = ptr(1.5) }
	unselected := func(p *photoinfo.PhotoInfo) { p.Burst, p.BurstPickType = true, 2 }
	noKind := func(p *photoinfo.PhotoInfo) { p.IsPhoto = false }

	trashed := []*photoinfo.PhotoInfo{
		twin("P", "2020-01-01T00:00:00Z", nil),
		twin("PT", "2020-01-01T00:00:00Z", trash),
		twin("S", "2021-01-01T00:00:00Z", nil),
	}
	// photos() leaves out K's duplicates; get_photo of PhotoInfo.duplicates does not
	// (photoinfo.py:1435).
	hidden := []*photoinfo.PhotoInfo{
		twin("KU", "2020-01-01T00:00:00Z", unselected),
		twin("K", "2020-01-01T00:00:00Z", nil),
		twin("KN", "2020-01-01T00:00:00Z", noKind),
	}
	locations := []*photoinfo.PhotoInfo{
		twin("L1", "2020-01-01T00:00:00Z", located),
		twin("L2", "2020-01-01T00:00:00Z", nil),
		twin("L3", "2021-01-01T00:00:00Z", nil),
		twin("L4", "2021-01-01T00:00:00Z", located),
	}
	applyRun(t, []applyCase{
		{"without duplicates", Options{Duplicate: true}, trashed[2:], nil},
		{"duplicate in the trash left out", Options{Duplicate: true}, trashed, []string{"P"}},
		{"duplicate in the trash with deleted", Options{Duplicate: true, Deleted: true}, trashed, []string{"P", "PT"}},
		{"duplicate in the trash with deleted only", Options{Duplicate: true, DeletedOnly: true}, trashed, []string{"PT"}},
		{"duplicate in the trash with both", Options{Duplicate: true, Deleted: true, DeletedOnly: true}, trashed, []string{"PT"}},
		{"unselected frame and object without kind", Options{Duplicate: true}, hidden, []string{"K", "KN", "KU"}},
		{"uuid does not limit the duplicates", Options{Duplicate: true, UUID: []string{"K"}}, hidden, []string{"K", "KN", "KU"}},
		{"location after the step", Options{Duplicate: true, Location: true}, locations, []string{"L1", "L4"}},
	}, nil)

	// The groups differ by Date. Favorite runs before the step and selects g1b,
	// g2c, g3a and g3b; g2b comes before g2a in the catalog.
	groups := []*photoinfo.PhotoInfo{
		twin("g1a", "2020-01-01T00:00:00Z", nil),
		twin("g1b", "2020-01-01T00:00:00Z", nil),
		twin("g2b", "2019-01-01T00:00:00Z", nil),
		twin("g2a", "2019-01-01T00:00:00Z", nil),
		twin("g2c", "2019-01-01T00:00:00Z", nil),
		twin("g3b", "2021-01-01T00:00:00Z", nil),
		twin("g3a", "2021-01-01T00:00:00Z", nil),
	}
	for _, p := range groups {
		p.Favorite = slices.Contains([]string{"g1b", "g2c", "g3a", "g3b"}, p.UUID)
	}
	var stepOrder []string
	o := Options{Duplicate: true, Favorite: true, Function: []func([]*photoinfo.PhotoInfo) []*photoinfo.PhotoInfo{
		func(in []*photoinfo.PhotoInfo) []*photoinfo.PhotoInfo {
			stepOrder = uuids(in)
			return in
		},
	}}
	got := uuids(Apply(o, groups, nil))
	if want := []string{"g2c", "g2a", "g2b", "g1b", "g1a", "g3a", "g3b"}; !slices.Equal(stepOrder, want) {
		t.Errorf("order of the duplicate step = %q, want %q", stepOrder, want)
	}
	if want := []string{"g2a", "g2b", "g2c", "g1a", "g1b", "g3a", "g3b"}; !slices.Equal(got, want) {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}
