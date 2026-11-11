package golden

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

// Library is the Go side of a comparison.
type Library struct {
	Photos []*photoinfo.PhotoInfo    // every photo of the database: photosdb.DB.Photos()
	Dicts  map[string]map[string]any // by photo UUID: photosdb.DB.AsDict(p, false), the values compared
}

// Diff is one difference between the reference and Go.
type Diff struct {
	UUID  string
	Field string // a key of fields.txt
	Want  string // normalized upstream value, compact JSON
	Got   string // normalized Go value, compact JSON
}

// Report is the result of Compare for one library.
type Report struct {
	Library   string
	Objects   int            // objects of the reference (query.json + deleted.json)
	Keys      []string       // the compared keys, sorted, without repeats
	Diffs     []Diff         // not explained by a divergence, sorted by Field, then UUID
	Explained map[string]int // per Diff.Field, the number of differences a divergence explained
	Stale     []Divergence   // records of this library and of Keys that explained nothing
}

// OK reports no unexplained differences and no stale records.
func (r *Report) OK() bool { return len(r.Diffs) == 0 && len(r.Stale) == 0 }

var errKeyMissing = errors.New("the key is missing from the dictionary of the photo in Library.Dicts")

type refObject struct {
	obj  Object
	uuid string
	file string
	n    int
}

func (o refObject) where(library string) string {
	return fmt.Sprintf("golden: %s: %s element %d", library, o.file, o.n)
}

func indexRef(ref *Reference) ([]refObject, error) {
	var out []refObject
	first := map[string]refObject{}
	for _, part := range []struct {
		file    string
		objects []Object
	}{{queryFile, ref.Query}, {deletedFile, ref.Deleted}} {
		for i, obj := range part.objects {
			ro := refObject{obj: obj, file: part.file, n: i + 1}
			id, err := decode[string](obj, "uuid", false, "a non-empty string")
			if err == nil && id == "" {
				err = shape("uuid", "a non-empty string")
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", ro.where(ref.Library), err)
			}
			if prev, dup := first[id]; dup {
				return nil, fmt.Errorf("%s: the uuid repeats that of %s element %d", ro.where(ref.Library), prev.file, prev.n)
			}
			ro.uuid = id
			first[id] = ro
			out = append(out, ro)
		}
	}
	return out, nil
}

func selectRules(keys []string) (selected []rule, names []string, err error) {
	names = slices.Clone(keys)
	slices.Sort(names)
	names = slices.Compact(names)
	for _, k := range names {
		r, ok := rules[k]
		if !ok {
			return nil, nil, fmt.Errorf("golden: unknown key %q", k)
		}
		selected = append(selected, r)
	}
	return selected, names, nil
}

// Compare compares lib with ref on keys (each in Keys). The sets of objects
// (key uuid) are compared always, whether or not keys lists uuid; the Go set
// is what upstream photos() returns (photosdb/photosdb.py:3391-3405). The Go
// value of a key is that of lib.Dicts. An error is a problem of the input,
// not a difference: an unknown key, a reference value of the wrong shape, a
// key compared that the dictionary of a photo of the Go set lacks, a UUID
// repeated in ref (Query and Deleted together) or in lib.Photos. Errors carry
// no UUID and no value.
func Compare(ref *Reference, lib Library, keys []string, divs []Divergence) (*Report, error) {
	selected, names, err := selectRules(keys)
	if err != nil {
		return nil, err
	}
	objs, err := indexRef(ref)
	if err != nil {
		return nil, err
	}
	goSet, err := indexGo(lib)
	if err != nil {
		return nil, err
	}

	var diffs []Diff
	inRef := make(map[string]bool, len(objs))
	for _, ro := range objs {
		inRef[ro.uuid] = true
		p := goSet[ro.uuid]
		for _, r := range selected {
			if r.got == nil { // uuid: the sets are compared below
				continue
			}
			d, err := compareKey(r, ro, p != nil, lib.Dicts[ro.uuid], ref.Library)
			if err != nil {
				return nil, err
			}
			if d != nil {
				diffs = append(diffs, *d)
			}
		}
		if p == nil {
			diffs = append(diffs, Diff{UUID: ro.uuid, Field: "uuid", Want: "true", Got: "false"})
		}
	}
	for id := range goSet {
		if !inRef[id] {
			diffs = append(diffs, Diff{UUID: id, Field: "uuid", Want: "false", Got: "true"})
		}
	}

	rep := &Report{Library: ref.Library, Objects: len(objs), Keys: names, Explained: map[string]int{}}
	rep.Diffs, rep.Stale = explain(rep, diffs, divs, names)
	return rep, nil
}

func indexGo(lib Library) (map[string]*photoinfo.PhotoInfo, error) {
	all := make(map[string]bool, len(lib.Photos))
	for _, p := range lib.Photos {
		if all[p.UUID] {
			return nil, errors.New("golden: a UUID is repeated in Library.Photos")
		}
		all[p.UUID] = true
	}
	set := map[string]*photoinfo.PhotoInfo{}
	// Deleted: the reference holds the trash too (deleted.json).
	for _, p := range query.Apply(query.Options{Deleted: true}, lib.Photos, nil) {
		set[p.UUID] = p
	}
	return set, nil
}

func compareKey(r rule, ro refObject, inGo bool, d map[string]any, library string) (*Diff, error) {
	fail := func(err error) error {
		return fmt.Errorf("%s: key %s: %w", ro.where(library), r.key, err)
	}
	want, err := r.ref(ro.obj)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ro.where(library), err)
	}
	if r.skip != nil {
		skip, err := r.skip(ro.obj)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ro.where(library), err)
		}
		if skip {
			return nil, nil
		}
	}
	if !inGo {
		return nil, nil
	}
	if _, ok := d[r.key]; !ok {
		return nil, fail(errKeyMissing)
	}
	got, err := r.got(d, want)
	if err != nil {
		return nil, fail(err)
	}
	ws, wantErr := canonical(want)
	gs, gotErr := canonical(got)
	if err := errors.Join(wantErr, gotErr); err != nil { // NaN or infinity: only the Go side can have it
		return nil, fail(err)
	}
	if ws == gs {
		return nil, nil
	}
	return &Diff{UUID: ro.uuid, Field: r.key, Want: ws, Got: gs}, nil
}

func explain(rep *Report, diffs []Diff, divs []Divergence, keys []string) (open []Diff, stale []Divergence) {
	used := make([]bool, len(divs))
	for _, diff := range diffs {
		matched := false
		for i, d := range divs {
			if d.explains(rep.Library, diff) {
				used[i], matched = true, true
			}
		}
		if matched {
			rep.Explained[diff.Field]++
		} else {
			open = append(open, diff)
		}
	}
	slices.SortFunc(open, func(a, b Diff) int {
		return cmp.Or(strings.Compare(a.Field, b.Field), strings.Compare(a.UUID, b.UUID))
	})
	for i, d := range divs {
		if !used[i] && d.Library == rep.Library && (d.Field == "uuid" || slices.Contains(keys, d.Field)) {
			stale = append(stale, d)
		}
	}
	slices.SortFunc(stale, func(a, b Divergence) int {
		return cmp.Or(strings.Compare(a.Field, b.Field), strings.Compare(a.UUID, b.UUID),
			bytes.Compare(a.Want, b.Want), bytes.Compare(a.Got, b.Got), strings.Compare(a.Reason, b.Reason))
	})
	return open, stale
}

func (d Divergence) explains(library string, diff Diff) bool {
	return d.Library == library && d.Field == diff.Field &&
		(d.UUID == "*" || d.UUID == diff.UUID) &&
		(len(d.Want) == 0 || string(d.Want) == diff.Want) &&
		(len(d.Got) == 0 || string(d.Got) == diff.Got)
}

// Summary returns numbers and key names only, never a UUID or a value.
func (r *Report) Summary() string {
	open := map[string]int{}
	for _, d := range r.Diffs {
		open[d.Field]++
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "library %s: %d objects, %d keys compared\n", r.Library, r.Objects, len(r.Keys))
	explained := 0
	for _, n := range r.Explained {
		explained += n
	}
	fmt.Fprintf(&b, "differences: %d unexplained, %d explained; stale divergences: %d\n", len(r.Diffs), explained, len(r.Stale))
	fields := map[string]bool{}
	for k := range open {
		fields[k] = true
	}
	for k := range r.Explained {
		fields[k] = true
	}
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		fmt.Fprintf(&b, "  %s: %d unexplained, %d explained\n", k, open[k], r.Explained[k])
	}
	return b.String()
}

// WriteDetails writes one line per difference and per stale record, with
// UUIDs and values; a value of persons is written as its number of names; the
// owner of a photo or of an album, the user of a comment or a like, the name
// and the display name of a person of person_info and the name of a face of
// face_info as "<name>"; the text of a comment as "<text>"; a value of
// exif_info, place, cloud_metadata, search_info, search_info_normalized,
// adjustments, original_adjustments, labels, labels_normalized or
// path_derivatives that is not null or "" as "<value>", the keys and the
// lengths of the lists kept; and a value of album_info, burst_album_info,
// project_info, comments, likes, person_info, face_info, labels,
// labels_normalized or path_derivatives that is not a list, or of exif_info,
// place, cloud_metadata, search_info, search_info_normalized, adjustments or
// original_adjustments that is not an object, as "<unparsed>".
func (r *Report) WriteDetails(w io.Writer) error {
	for _, d := range r.Diffs {
		if _, err := fmt.Fprintf(w, "%s %s want=%s got=%s\n", d.Field, d.UUID, redact(d.Field, d.Want), redact(d.Field, d.Got)); err != nil {
			return err
		}
	}
	for _, d := range r.Stale {
		if _, err := fmt.Fprintf(w, "stale %s %s want=%s got=%s\n", d.Field, d.UUID,
			redactOrAny(d.Field, d.Want), redactOrAny(d.Field, d.Got)); err != nil {
			return err
		}
	}
	return nil
}

// The names of persons, owners and authors and the text of comments are
// personal data, and the details go to the test log.
var redacted = map[string]map[string]string{
	"album_info": {"owner": "<name>"}, "burst_album_info": {"owner": "<name>"}, "project_info": {"owner": "<name>"},
	"comments": {"user": "<name>", "text": "<text>"}, "likes": {"user": "<name>"}, "person_info": {"name": "<name>", "displayname": "<name>"},
	"face_info": {"name": "<name>"},
}

func redact(field, value string) string {
	var names []string
	if field == "persons" && json.Unmarshal([]byte(value), &names) == nil {
		return fmt.Sprintf("<%d names>", len(names))
	}
	if field == "owner" && value != `""` {
		return "<name>"
	}
	switch field {
	case "exif_info", "place", "cloud_metadata", "search_info", "search_info_normalized", "adjustments", "original_adjustments":
		return redactAs[map[string]any](value)
	case "labels", "labels_normalized", "path_derivatives":
		return redactAs[[]any](value)
	}
	placeholders, ok := redacted[field]
	if !ok {
		return value
	}
	var items []map[string]any
	if json.Unmarshal([]byte(value), &items) != nil {
		return "<unparsed>"
	}
	for _, it := range items {
		for k, placeholder := range placeholders {
			if s, _ := it[k].(string); s != "" {
				it[k] = placeholder
			}
		}
	}
	s, _ := canonical(items) // decoded JSON holds no NaN
	return s
}

// The coordinates and the camera of a photo, the names and the address of its
// place, its search terms, its edits and the paths of the machine are personal
// data too: the keys and the lengths of the lists stay, so that a null or ""
// can be told from a value and a difference located.
func redactAs[T map[string]any | []any](value string) string {
	var v T
	if json.Unmarshal([]byte(value), &v) != nil {
		return "<unparsed>"
	}
	s, _ := canonical(redactLeaves(v)) // decoded JSON holds no NaN
	return s
}

func redactLeaves(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = redactLeaves(e)
		}
	case []any:
		for i, e := range x {
			x[i] = redactLeaves(e)
		}
	default:
		if v != nil && v != "" {
			return "<value>"
		}
	}
	return v
}

func redactOrAny(field string, raw []byte) string {
	if len(raw) == 0 {
		return "*"
	}
	return redact(field, string(raw))
}
