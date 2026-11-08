package golden

import (
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
)

// Search terms are the content of the photos: the details keep the keys and
// the lengths of the lists, not the terms.
func TestWriteDetailsSearchInfo(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	r := searchRecords()
	r[0].Content, r[0].Normalized = "SECRET-LABEL", "secret-label"
	lib.SearchInfo[selectedFrame] = searchinfo.New(r)
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "labels", Want: []byte(`["SECRET-STALE",""]`), Reason: "r", Upstream: "a.py:1"}
	unparsedList := Divergence{Library: "lib", UUID: selectedFrame, Field: "labels_normalized", Want: []byte(`{"k":"SECRET-OBJECT"}`), Reason: "r", Upstream: "a.py:1"}
	unparsedObject := Divergence{Library: "lib", UUID: selectedFrame, Field: "search_info", Want: []byte(`["SECRET-LIST"]`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, stale, unparsedList, unparsedObject)
	if got, want := diffKeys(rep), []string{"labels", "labels_normalized", "search_info", "search_info_normalized"}; !slices.Equal(got, want) {
		t.Fatalf("differences in %v, want %v", got, want)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SECRET", "secret", "Beach", "beach", "Dog", "dog", "Townsville", "New Year"} {
		if strings.Contains(b.String(), leak) {
			t.Errorf("WriteDetails writes %q", leak)
		}
	}
	for _, line := range []string{
		"labels " + selectedFrame + ` want=["<value>","<value>"] got=["<value>","<value>"]` + "\n",
		"labels_normalized " + selectedFrame + ` want=["<value>","<value>"] got=["<value>","<value>"]` + "\n",
		`"city":"<value>"`,
		`"holidays":["<value>"]`,
		`"labels":["<value>","<value>"]`,
		`"activities":[]`,
		`"year":""`,
		"stale labels " + emptyPhoto + ` want=["<value>",""] got=*` + "\n",
		"stale labels_normalized " + selectedFrame + " want=<unparsed> got=*\n",
		"stale search_info " + selectedFrame + " want=<unparsed> got=*\n",
	} {
		if !strings.Contains(b.String(), line) {
			t.Errorf("WriteDetails lacks %q:\n%s", line, b.String())
		}
	}
}
