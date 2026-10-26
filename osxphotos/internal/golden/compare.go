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
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/query"
)

// Library is the Go side of a comparison, as plain values.
type Library struct {
	Photos []*photoinfo.PhotoInfo // every photo of the database: photosdb.DB.Photos()
	Albums []*albuminfo.AlbumInfo // every user album, trashed ones included; may be nil without AlbumKeys

	// BurstAlbums holds, by photo UUID, the album UUIDs of osxphotos
	// burst_album_info (photosdb.DB.BurstAlbumInfoOf); each must be in Albums,
	// and Compare ignores albums in the trash, as in AlbumUUIDs. A photo without
	// an entry has none; may be nil without AlbumKeys.
	BurstAlbums map[string][]string

	Imports  []*albuminfo.ImportInfo // every import session, trashed ones included: the sessions of PhotoInfo.ImportUUID; may be nil without AlbumKeys
	Projects []*albuminfo.AlbumInfo  // every project, trashed ones included: the projects of PhotoInfo.ProjectUUIDs (photosdb.DB.Project); may be nil without AlbumKeys
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

type env struct {
	albums        map[string]*albuminfo.AlbumInfo
	projects      map[string]*albuminfo.AlbumInfo
	goAlbumElems  map[string]map[string]any // albums and projects
	goImportElems map[string]map[string]any
	bursts        map[string][]string
	burstAlbums   map[string][]string
}

var (
	errAlbumMissing   = errors.New("an album of a photo is missing from Library.Albums")
	errImportMissing  = errors.New("an import session of a photo is missing from Library.Imports")
	errProjectMissing = errors.New("a project of a photo is missing from Library.Projects")
)

func newEnv(lib Library) (*env, error) {
	e := &env{
		albums:        map[string]*albuminfo.AlbumInfo{},
		projects:      map[string]*albuminfo.AlbumInfo{},
		goAlbumElems:  map[string]map[string]any{},
		goImportElems: map[string]map[string]any{},
		bursts:        map[string][]string{},
		burstAlbums:   lib.BurstAlbums,
	}
	for _, set := range []struct {
		albums []*albuminfo.AlbumInfo
		index  map[string]*albuminfo.AlbumInfo
		parent bool // project_info does not compare it: projectInfoRule
	}{{lib.Albums, e.albums, true}, {lib.Projects, e.projects, false}} {
		for _, a := range set.albums {
			if _, dup := e.goAlbumElems[a.UUID]; dup {
				return nil, errors.New("golden: an album UUID is repeated in Library.Albums or Library.Projects")
			}
			set.index[a.UUID] = a
			elem := withDates(map[string]any{
				"uuid":         a.UUID,
				"title":        a.Title,
				"folder_names": strs(a.FolderNames),
			}, a.CreationDate, a.StartDate, a.EndDate)
			if set.parent {
				elem["parent"] = a.ParentUUID
			}
			e.goAlbumElems[a.UUID] = elem
		}
	}
	for _, s := range lib.Imports {
		if _, dup := e.goImportElems[s.UUID]; dup {
			return nil, errors.New("golden: an import session UUID is repeated in Library.Imports")
		}
		e.goImportElems[s.UUID] = withDates(map[string]any{"uuid": s.UUID, "title": s.Title}, s.CreationDate, s.StartDate, s.EndDate)
	}
	for _, p := range lib.Photos {
		if p.Burst {
			e.bursts[p.BurstUUID] = append(e.bursts[p.BurstUUID], p.UUID)
		}
	}
	for _, ids := range e.bursts {
		slices.Sort(ids)
	}
	return e, nil
}

// Albums and import sessions share their dates (AlbumInfoBaseClass,
// albuminfo.py:56-107).
func withDates(elem map[string]any, created, start, end *time.Time) map[string]any {
	elem["creation_date"] = goMoment(created, defaultCreationDate)
	elem["start_date"] = goMoment(start, time.Time{})
	elem["end_date"] = goMoment(end, time.Time{})
	return elem
}

func members(index map[string]*albuminfo.AlbumInfo, missing error, uuids []string) ([]*albuminfo.AlbumInfo, error) {
	var out []*albuminfo.AlbumInfo
	seen := map[string]bool{}
	for _, id := range uuids {
		a, ok := index[id]
		if !ok {
			return nil, missing
		}
		if a.InTrash || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, a)
	}
	return out, nil
}

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

// Compare compares lib with ref on keys (each in PhotoKeys or AlbumKeys).
// The sets of objects (key uuid) are compared always, whether or not keys
// lists uuid; the Go set is what upstream photos() returns
// (photosdb/photosdb.py:3346-3360). An error is a problem of the input, not
// a difference: an unknown key, a reference value of the wrong shape, an
// album UUID missing from lib.Albums, a project missing from lib.Projects or
// an import session missing from lib.Imports, a UUID repeated in ref (Query
// and Deleted together), in lib.Photos, in lib.Albums and lib.Projects
// together or in lib.Imports. Errors carry no UUID and no value.
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
	e, err := newEnv(lib)
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
			d, err := compareKey(e, r, ro, p, ref.Library)
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

func compareKey(e *env, r rule, ro refObject, p *photoinfo.PhotoInfo, library string) (*Diff, error) {
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
	if p == nil {
		return nil, nil
	}
	got, err := r.got(e, p, want)
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
// UUIDs and values; a value of persons is written as its number of names.
func (r *Report) WriteDetails(w io.Writer) error {
	for _, d := range r.Diffs {
		if _, err := fmt.Fprintf(w, "%s %s want=%s got=%s\n", d.Field, d.UUID, redact(d.Field, d.Want), redact(d.Field, d.Got)); err != nil {
			return err
		}
	}
	for _, d := range r.Stale {
		if _, err := fmt.Fprintf(w, "stale %s %s want=%s got=%s\n", d.Field, d.UUID,
			redact(d.Field, orAny(d.Want)), redact(d.Field, orAny(d.Got))); err != nil {
			return err
		}
	}
	return nil
}

// The names of persons are personal data, and the details go to the test log.
func redact(field, value string) string {
	var names []string
	if field != "persons" || json.Unmarshal([]byte(value), &names) != nil {
		return value
	}
	return fmt.Sprintf("<%d names>", len(names))
}

func orAny(raw []byte) string {
	if len(raw) == 0 {
		return "*"
	}
	return string(raw)
}
