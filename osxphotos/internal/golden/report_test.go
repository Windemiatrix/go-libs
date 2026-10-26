package golden

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func secretReport(t *testing.T) *Report {
	t.Helper()
	lib := fixtureLib()
	lib.Photos[0].Title = "SECRET-TITLE"
	lib.Photos[0].Width++
	lib.Photos[2].Description = "x"
	lib.Photos = append(lib.Photos, &photoinfo.PhotoInfo{UUID: "00000000-0000-0000-0000-0000000000e1", IsPhoto: true})
	divs := []Divergence{
		{Library: "lib", UUID: emptyPhoto, Field: "description", Reason: "r", Upstream: "a.py:1"},
		{Library: "lib", UUID: selectedFrame, Field: "height", Want: []byte(`"SECRET-STALE"`), Reason: "r", Upstream: "a.py:1"},
	}
	return compare(t, fixtureRef(), lib, divs...)
}

// The summary is what diff-local prints to a terminal for the library of its
// owner, so it has numbers and key names only.
func TestSummaryKeepsValuesOut(t *testing.T) {
	t.Parallel()
	rep := secretReport(t)
	sum := rep.Summary()
	for _, leak := range []string{"SECRET", selectedFrame, emptyPhoto, "00000000-0000", "Title A", "Desc"} {
		if strings.Contains(sum, leak) {
			t.Errorf("Summary has %q:\n%s", leak, sum)
		}
	}
	want := "library lib: 2 objects, 72 keys compared\n" +
		"differences: 3 unexplained, 1 explained; stale divergences: 1\n" +
		"  description: 0 unexplained, 1 explained\n" +
		"  title: 1 unexplained, 0 explained\n" +
		"  uuid: 1 unexplained, 0 explained\n" +
		"  width: 1 unexplained, 0 explained\n"
	if sum != want {
		t.Errorf("Summary:\n%s\nwant:\n%s", sum, want)
	}

	var details strings.Builder
	if err := rep.WriteDetails(&details); err != nil {
		t.Fatal(err)
	}
	for _, need := range []string{"SECRET-TITLE", selectedFrame, "SECRET-STALE", "stale height", "00000000-0000-0000-0000-0000000000e1"} {
		if !strings.Contains(details.String(), need) {
			t.Errorf("WriteDetails lacks %q:\n%s", need, details.String())
		}
	}
	lines := strings.Split(strings.TrimSuffix(details.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("WriteDetails has %d lines, want 3 differences and 1 stale record:\n%s", len(lines), details.String())
	}
	order := []string{"title ", "uuid ", "width ", "stale height "}
	last := -1
	for _, l := range lines {
		found := -1
		for i, p := range order {
			if strings.HasPrefix(l, p) {
				found = i
			}
		}
		if found < last {
			t.Errorf("lines are not in the order Field, UUID, stale: %q", l)
		}
		last = found
	}
	if !strings.HasPrefix(lines[0], "title "+selectedFrame+` want="Title A" got="SECRET-TITLE"`) {
		t.Errorf("the first line is %q", lines[0])
	}
}

func TestReportDeterministic(t *testing.T) {
	t.Parallel()
	rep := secretReport(t)
	first := rep.Summary()
	var firstDetails strings.Builder
	if err := rep.WriteDetails(&firstDetails); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		var b strings.Builder
		if err := rep.WriteDetails(&b); err != nil {
			t.Fatal(err)
		}
		if rep.Summary() != first || b.String() != firstDetails.String() {
			t.Fatalf("call %d gave other bytes", i)
		}
	}
	again := secretReport(t)
	var b strings.Builder
	_ = again.WriteDetails(&b)
	if again.Summary() != first || b.String() != firstDetails.String() {
		t.Error("two Compare calls on the same input give different text")
	}
}

type failingWriter struct{ after int }

var errWrite = errors.New("write failed")

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.after == 0 {
		return 0, errWrite
	}
	w.after--
	return len(p), nil
}

func TestWriteDetailsFailure(t *testing.T) {
	t.Parallel()
	rep := secretReport(t)
	for _, after := range []int{0, 2, 3} {
		if err := rep.WriteDetails(&failingWriter{after: after}); !errors.Is(err, errWrite) {
			t.Errorf("after %d lines: error = %v, want the error of the writer", after, err)
		}
	}
	if err := rep.WriteDetails(&failingWriter{after: 4}); err != nil {
		t.Errorf("a writer that takes all lines: %v", err)
	}
}

func TestReportOK(t *testing.T) {
	t.Parallel()
	if !(&Report{}).OK() {
		t.Error("an empty report is not OK")
	}
	if (&Report{Diffs: []Diff{{}}}).OK() || (&Report{Stale: []Divergence{{}}}).OK() {
		t.Error("a report with a difference or a stale record is OK")
	}
	empty := compare(t, fixtureRef(), fixtureLib())
	want := "library lib: 2 objects, 72 keys compared\ndifferences: 0 unexplained, 0 explained; stale divergences: 0\n"
	if got := empty.Summary(); got != want {
		t.Errorf("Summary = %q, want %q", got, want)
	}
	var b strings.Builder
	if err := empty.WriteDetails(&b); err != nil || b.Len() != 0 {
		t.Errorf("WriteDetails = %q, %v; want nothing", b.String(), err)
	}
}

func FuzzLoad(f *testing.F) {
	obj := `{"title": "t", "uuid": "00000000-0000-0000-0000-000000000001"}`
	for _, s := range []string{"[" + obj + "]", "[]", `[{"SECRET-SEED": 1}]`, `["SECRET-SEED"]`, `{"uuid": "SECRET-SEED"}`, `[{"uuid": 7, "title": "SECRET-SEED"}]`, "[" + obj + "," + obj + "]", `[`, ``, `null`} {
		f.Add(s)
	}
	dir := f.TempDir()
	deleted := filepath.Join(dir, "deleted.json")
	if err := os.WriteFile(deleted, []byte("[]"), 0o600); err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, in string) {
		query := filepath.Join(t.TempDir(), "query.json")
		if err := os.WriteFile(query, []byte(in), 0o600); err != nil {
			t.Fatal(err)
		}
		ref, err := LoadFiles("fuzz", query, deleted, []string{"title"})
		if err != nil {
			if strings.Contains(in, "SECRET-") && strings.Contains(err.Error(), "SECRET-") {
				t.Errorf("the error repeats the input: %v", err)
			}
			return
		}
		if err := ref.CheckOrder(); err != nil && strings.Contains(err.Error(), "SECRET-") {
			t.Errorf("CheckOrder repeats the input: %v", err)
		}
	})
}

func BenchmarkCompare20k(b *testing.B) {
	const n, perBurst = 20000, 5
	lib := Library{BurstAlbums: map[string][]string{}}
	for i := 0; i < 30; i++ {
		lib.Albums = append(lib.Albums, &albuminfo.AlbumInfo{UUID: fmt.Sprintf("00000000-0000-0000-0001-%012x", i+1), Title: fmt.Sprintf("Album %d", i)})
	}
	var objs []Object
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("00000000-0000-0000-0000-%012x", i+1)
		burst := fmt.Sprintf("BURST-%d", i/perBurst)
		p := &photoinfo.PhotoInfo{
			UUID: id, Filename: "IMG_0003.JPG", OriginalFilename: "IMG_0003.JPG", Date: time.Unix(0, 0), UTI: "public.jpeg", UTIOriginal: "public.jpeg",
			IsPhoto: true, Burst: true, BurstUUID: burst,
			AlbumUUIDs: []string{lib.Albums[i%30].UUID, lib.Albums[(i+1)%30].UUID, lib.Albums[(i+2)%30].UUID},
		}
		lib.BurstAlbums[id] = p.AlbumUUIDs
		lib.Photos = append(lib.Photos, p)

		var others []string
		for j := i / perBurst * perBurst; j < (i/perBurst+1)*perBurst; j++ {
			if j != i {
				others = append(others, fmt.Sprintf("00000000-0000-0000-0000-%012x", j+1))
			}
		}
		var info, titles []string
		folders := map[string]string{}
		for k := 0; k < 3; k++ {
			a := lib.Albums[(i+k)%30]
			info = append(info, albumJSON(a.UUID, jsonString(a.Title), "null", "[]", `"2001-01-01T00:00:00+00:00"`, "null", "null", "null"))
			titles = append(titles, jsonString(a.Title))
			folders[a.Title] = "[]"
		}
		var fparts []string
		for t, v := range folders {
			fparts = append(fparts, jsonString(t)+": "+v)
		}
		objs = append(objs, refObj(refEmptyPhoto(), map[string]string{
			"uuid": jsonString(id), "burst": "true", "burst_photos": jsonStrings(others),
			"album_info": "[" + strings.Join(info, ", ") + "]", "burst_album_info": "[" + strings.Join(info, ", ") + "]",
			"albums": "[" + strings.Join(titles, ", ") + "]", "burst_albums": "[" + strings.Join(titles, ", ") + "]",
			"folders": "{" + strings.Join(fparts, ", ") + "}",
		}))
	}
	// albums and burst_albums are sorted lists: sort the titles in the reference.
	for _, o := range objs {
		for _, k := range []string{"albums", "burst_albums"} {
			titles, _ := refStrings(o, k)
			slices.Sort(titles)
			o[k] = []byte(jsonStrings(titles))
		}
	}
	ref := &Reference{Library: "bench", Query: objs}
	keys := everyKey()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rep, err := Compare(ref, lib, keys, nil)
		if err != nil {
			b.Fatal(err)
		}
		if !rep.OK() {
			b.Fatalf("%d differences, first %+v", len(rep.Diffs), rep.Diffs[:min(len(rep.Diffs), 5)])
		}
	}
}
