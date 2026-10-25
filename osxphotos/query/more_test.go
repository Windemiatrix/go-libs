package query

import (
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func moreDay(d int) time.Time { return at("2020-01-01T00:00:00Z").AddDate(0, 0, d) }

type morePhotos = []*photoinfo.PhotoInfo

func TestMoreYear(t *testing.T) {
	t.Parallel()

	// The same moment is 2019 on the wall clock of -05 and 2020 on that of +03:
	// the year is read in the zone of Date, not in UTC.
	moment := at("2019-12-31T23:30:00Z")
	photos := morePhotos{
		{UUID: "A", IsPhoto: true, Date: moment},
		{UUID: "B", IsPhoto: true, Date: moment.In(time.FixedZone("+03", 3*3600))},
		{UUID: "C", IsPhoto: true, Date: at("2020-01-01T02:30:00Z").In(time.FixedZone("-05", -5*3600))},
	}
	applyRun(t, []applyCase{
		{"2019", Options{Year: []int{2019}}, photos, []string{"A", "C"}},
		{"2020", Options{Year: []int{2020}}, photos, []string{"B"}},
		{"2019 or 2020", Options{Year: []int{2020, 2019}}, photos, []string{"A", "B", "C"}},
		{"no photo of the year", Options{Year: []int{2021}}, photos, nil},
		{"empty list", Options{Year: []int{}}, photos, []string{"A", "B", "C"}},
	}, nil)
}

func TestMoreTimeOfDay(t *testing.T) {
	t.Parallel()

	// Z is 09:30 on its wall clock and 06:30 in UTC.
	photos := morePhotos{
		{UUID: "M", IsPhoto: true, Date: at("2020-06-01T00:00:00Z")},
		{UUID: "Z", IsPhoto: true, Date: at("2020-06-01T09:30:00+03:00")},
		{UUID: "N", IsPhoto: true, Date: at("2020-06-01T12:30:00Z")},
	}
	const half = 9*time.Hour + 30*time.Minute
	applyRun(t, []applyCase{
		{"from midnight is set and keeps all", Options{FromTime: ptr(time.Duration(0))}, photos, []string{"M", "Z", "N"}},
		{"to midnight is set and keeps none", Options{ToTime: ptr(time.Duration(0))}, photos, nil},
		{"from, equal is in", Options{FromTime: ptr(half)}, photos, []string{"Z", "N"}},
		{"from, after", Options{FromTime: ptr(half + time.Nanosecond)}, photos, []string{"N"}},
		{"from, wall clock of N", Options{FromTime: ptr(12*time.Hour + 30*time.Minute)}, photos, []string{"N"}},
		{"to, equal is out", Options{ToTime: ptr(half)}, photos, []string{"M"}},
		{"to, after", Options{ToTime: ptr(half + time.Nanosecond)}, photos, []string{"M", "Z"}},
		{"from and to", Options{FromTime: ptr(time.Hour), ToTime: ptr(10 * time.Hour)}, photos, []string{"Z"}},
		{"not set", Options{}, photos, []string{"M", "Z", "N"}},
	}, nil)
}

func TestMoreAdded(t *testing.T) {
	t.Parallel()

	// A0 has no DateAdded: it is 1970-01-01T00:00:00Z, as in Sort.
	photos := morePhotos{
		{UUID: "A1", IsPhoto: true, Date: moreDay(1), DateAdded: ptr(at("2021-01-01T00:00:00Z"))},
		{UUID: "A2", IsPhoto: true, Date: moreDay(2), DateAdded: ptr(at("2021-06-01T00:00:00Z"))},
		{UUID: "A0", IsPhoto: true, Date: moreDay(3)},
	}
	applyRun(t, []applyCase{
		{"after, equal is out", Options{AddedAfter: at("2021-01-01T00:00:00Z")}, photos, []string{"A2"}},
		{"after, equal in another zone is out", Options{AddedAfter: at("2021-01-01T03:00:00+03:00")}, photos, []string{"A2"}},
		{"after, a second earlier", Options{AddedAfter: at("2020-12-31T23:59:59Z")}, photos, []string{"A1", "A2"}},
		{"after the epoch keeps nil out", Options{AddedAfter: at("1970-01-01T00:00:00Z")}, photos, []string{"A1", "A2"}},
		{"after before the epoch keeps nil in", Options{AddedAfter: at("1969-12-31T23:59:59Z")}, photos, []string{"A1", "A2", "A0"}},
		{"before, equal is out", Options{AddedBefore: at("2021-06-01T00:00:00Z")}, photos, []string{"A1", "A0"}},
		{"before the epoch: nil is not before it", Options{AddedBefore: at("1970-01-01T00:00:00Z")}, photos, nil},
		{"after and before", Options{AddedAfter: at("2020-01-01T00:00:00Z"), AddedBefore: at("2021-03-01T00:00:00Z")}, photos, []string{"A1"}},
		{"not set", Options{}, photos, []string{"A1", "A2", "A0"}},
	}, nil)
}

func TestMoreAddedInLast(t *testing.T) {
	t.Parallel()

	// The windows are hours wide: the clock moves by seconds during the test.
	now := time.Now()
	photos := morePhotos{
		{UUID: "R", IsPhoto: true, Date: moreDay(1), DateAdded: ptr(now.Add(-time.Hour))},
		{UUID: "O", IsPhoto: true, Date: moreDay(2), DateAdded: ptr(now.Add(-48 * time.Hour))},
		{UUID: "N", IsPhoto: true, Date: moreDay(3)},
	}
	applyRun(t, []applyCase{
		{"a day", Options{AddedInLast: 24 * time.Hour}, photos, []string{"R"}},
		{"three days", Options{AddedInLast: 72 * time.Hour}, photos, []string{"R", "O"}},
		{"half an hour", Options{AddedInLast: 30 * time.Minute}, photos, nil},
		{"zero is not set", Options{}, photos, []string{"R", "O", "N"}},
	}, nil)
}

func TestMoreSize(t *testing.T) {
	t.Parallel()

	photos := morePhotos{
		{UUID: "S0", IsPhoto: true, Date: moreDay(1)},
		{UUID: "S1", IsPhoto: true, Date: moreDay(2), OriginalFilesize: 100},
		{UUID: "S2", IsPhoto: true, Date: moreDay(3), OriginalFilesize: 200},
		{UUID: "S3", IsPhoto: true, Date: moreDay(4), OriginalFilesize: 300},
	}
	applyRun(t, []applyCase{
		{"min, equal is in", Options{MinSize: 200}, photos, []string{"S2", "S3"}},
		{"min, above", Options{MinSize: 201}, photos, []string{"S3"}},
		{"max, equal is in", Options{MaxSize: 200}, photos, []string{"S0", "S1", "S2"}},
		{"max, below", Options{MaxSize: 199}, photos, []string{"S0", "S1"}},
		{"min and max", Options{MinSize: 100, MaxSize: 200}, photos, []string{"S1", "S2"}},
		{"zero is not set", Options{}, photos, []string{"S0", "S1", "S2", "S3"}},
	}, nil)
}

func TestMoreBurstPhotos(t *testing.T) {
	t.Parallel()

	frame := func(uuid, burst string, day, pick int) *photoinfo.PhotoInfo {
		return &photoinfo.PhotoInfo{UUID: uuid, IsPhoto: true, Burst: true, BurstUUID: burst, BurstPickType: pick, Date: moreDay(day)}
	}
	first, trashed, missing := frame("B1", "X", 5, 8), frame("B3", "X", 4, 8), frame("B4", "X", 1, 8)
	first.Favorite, trashed.InTrash, missing.IsMissing = true, true, true
	// B1 is the only favorite, B2 is an unselected frame, B3 is in the trash, B4
	// is missing, NB is not a frame of a burst although it carries its
	// BurstUUID, E1 and E2 are the frames of the burst whose BurstUUID is empty.
	catalog := morePhotos{
		first, frame("B2", "X", 3, 2), trashed, missing,
		{UUID: "NB", IsPhoto: true, BurstUUID: "X", Date: moreDay(2)},
		frame("Y1", "Y", 6, 8), frame("Y2", "Y", 7, 2),
		frame("E1", "", 8, 8), frame("E2", "", 9, 2),
	}
	reversed := slices.Clone(catalog)
	slices.Reverse(reversed)

	cases := []applyCase{
		{"off", Options{UUID: []string{"B1"}}, catalog, []string{"B1"}},
		{"the other frames, trash and unselected too", Options{UUID: []string{"B1"}, BurstPhotos: true}, catalog, []string{"B2", "B3", "B1"}},
		{"with missing frames", Options{UUID: []string{"B1"}, BurstPhotos: true, MissingBursts: true}, catalog, []string{"B4", "B2", "B3", "B1"}},
		{"missing frames without burst photos", Options{UUID: []string{"B1"}, MissingBursts: true}, catalog, []string{"B1"}},
		{"two frames of a burst, each once", Options{UUID: []string{"B1", "B3"}, Deleted: true, BurstPhotos: true}, catalog, []string{"B2", "B3", "B1"}},
		{"a missing frame in the result stays", Options{UUID: []string{"B4"}, Deleted: true, BurstPhotos: true}, catalog, []string{"B4", "B2", "B3", "B1"}},
		{"not a burst", Options{UUID: []string{"NB"}, BurstPhotos: true}, catalog, []string{"NB"}},
		{"empty BurstUUID is one burst", Options{UUID: []string{"E1"}, BurstPhotos: true}, catalog, []string{"E1", "E2"}},
		{"two bursts", Options{UUID: []string{"Y1", "B1"}, BurstPhotos: true}, catalog, []string{"B2", "B3", "B1", "Y1", "Y2"}},
		{"after the other filters", Options{Favorite: true, BurstPhotos: true}, catalog, []string{"B2", "B3", "B1"}},
	}
	var tests []applyCase
	for _, c := range cases {
		tests = append(tests, c, applyCase{c.name + ", catalog reversed", c.o, reversed, c.want})
	}
	applyRun(t, tests, nil)
}

func TestMoreFunction(t *testing.T) {
	t.Parallel()

	dropFirst := func(ps morePhotos) morePhotos { return ps[1:] }
	keepLast := func(ps morePhotos) morePhotos { return ps[len(ps)-1:] }
	repeatFirst := func(ps morePhotos) morePhotos { return append(ps, ps[0]) }
	nothing := func(morePhotos) morePhotos { return nil }

	photos := morePhotos{
		{UUID: "A", IsPhoto: true, Date: moreDay(1)},
		{UUID: "B", IsPhoto: true, Date: moreDay(2)},
		{UUID: "C", IsPhoto: true, Date: moreDay(3)},
	}
	burst := morePhotos{{UUID: "F", IsPhoto: true, Burst: true, BurstUUID: "X", BurstPickType: 8, Date: moreDay(1)}}
	fns := func(f ...func(morePhotos) morePhotos) []func(morePhotos) morePhotos { return f }
	applyRun(t, []applyCase{
		{"in order", Options{Function: fns(dropFirst, keepLast)}, photos, []string{"C"}},
		{"in the other order", Options{Function: fns(keepLast, dropFirst)}, photos, nil},
		{"after the filters", Options{UUID: []string{"A", "B"}, Function: fns(keepLast)}, photos, []string{"B"}},
		{"returns nil", Options{Function: fns(nothing)}, photos, nil},
		{"returns a photo twice, as upstream", Options{UUID: []string{"A"}, Function: fns(repeatFirst)}, photos, []string{"A", "A"}},
		{"returns a photo twice, burst photos remove it", Options{Function: fns(repeatFirst), BurstPhotos: true}, burst, []string{"F"}},
		{"none", Options{}, photos, []string{"A", "B", "C"}},
	}, nil)

	var got []string
	record := func(ps morePhotos) morePhotos {
		got = uuids(ps)
		return ps
	}
	Apply(Options{UUID: []string{"A", "C"}, Function: fns(record)}, photos, nil)
	if want := []string{"A", "C"}; !slices.Equal(got, want) {
		t.Errorf("Function received %q, want %q", got, want)
	}

	// The functions run before the frames of the bursts are added.
	bursts := morePhotos{
		{UUID: "B1", IsPhoto: true, Burst: true, BurstUUID: "X", BurstPickType: 8, Date: moreDay(2)},
		{UUID: "B2", IsPhoto: true, Burst: true, BurstUUID: "X", BurstPickType: 2, Date: moreDay(1)},
		{UUID: "Y1", IsPhoto: true, Burst: true, BurstUUID: "Y", BurstPickType: 8, Date: moreDay(3)},
	}
	o := Options{Burst: true, Function: fns(func(ps morePhotos) morePhotos { return ps[len(ps)-1:] }), BurstPhotos: true}
	if got, want := uuids(Apply(o, bursts, nil)), []string{"Y1"}; !slices.Equal(got, want) {
		t.Errorf("Apply = %q, want %q", got, want)
	}
	o.Function = fns(func(ps morePhotos) morePhotos { return ps[:1] })
	if got, want := uuids(Apply(o, bursts, nil)), []string{"B2", "B1"}; !slices.Equal(got, want) {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}
