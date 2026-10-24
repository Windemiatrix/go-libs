package golden

import (
	"errors"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// TestMissingKey: a rule asks for its key, and a reference without it is a
// mistake of the input for every rule, not a null.
func TestMissingKey(t *testing.T) {
	t.Parallel()
	for _, key := range allKeys() {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			_, err := rules[key].ref(Object{})
			var se *shapeError
			if !errors.As(err, &se) || !strings.Contains(err.Error(), "the key is missing") {
				t.Errorf("error = %v, want a shape error about the missing key", err)
			}
		})
	}
	t.Run("an album list without its albums key", func(t *testing.T) {
		t.Parallel()
		// albums needs album_info for the titles of shared albums.
		_, err := rules["albums"].ref(Object{"albums": []byte(`[]`)})
		if err == nil || !strings.Contains(err.Error(), "album_info") {
			t.Errorf("error = %v, want one about album_info", err)
		}
		_, err = rules["albums"].ref(Object{"album_info": []byte(`[]`)})
		if err == nil || !strings.Contains(err.Error(), "albums") {
			t.Errorf("error = %v, want one about albums", err)
		}
		_, err = rules["folders"].ref(Object{"album_info": []byte(`[]`)})
		if err == nil || !strings.Contains(err.Error(), "folders") {
			t.Errorf("error = %v, want one about folders", err)
		}
		_, err = rules["burst_albums"].ref(Object{"burst_albums": []byte(`[]`)})
		if err == nil || !strings.Contains(err.Error(), "burst_album_info") {
			t.Errorf("error = %v, want one about burst_album_info", err)
		}
	})
}

// TestAlbumShape: an element of album_info that has not the form osxphotos writes.
func TestAlbumShape(t *testing.T) {
	t.Parallel()
	const good = `{"creation_date": "2020-01-01T00:00:00+00:00", "end_date": null, "folder_list": [], "folder_names": [], "owner": null, "parent": null, "sort_order": null, "start_date": null, "title": "T", "uuid": "U"}`
	replace := func(key, value string) string {
		o, _ := decodeObject([]byte(good))
		o[key] = []byte(value)
		var parts []string
		for k, v := range o {
			parts = append(parts, `"`+k+`": `+string(v))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	without := func(key string) string {
		o, _ := decodeObject([]byte(good))
		delete(o, key)
		var parts []string
		for k, v := range o {
			parts = append(parts, `"`+k+`": `+string(v))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}

	if _, err := rules["album_info"].ref(Object{"album_info": []byte("[" + good + "]")}); err != nil {
		t.Fatalf("the good element: %v", err)
	}
	cases := []struct{ name, elem, want string }{
		{"no uuid", without("uuid"), "album_info[0].uuid: want a value"},
		{"uuid number", replace("uuid", "5"), "album_info[0].uuid: want a string"},
		{"title number", replace("title", "5"), "album_info[0].title: want a string"},
		{"no title", without("title"), "album_info[0].title: want a value"},
		{"parent number", replace("parent", "5"), "album_info[0].parent: want a string or null"},
		{"no parent", without("parent"), "album_info[0].parent: want a value"},
		{"folder_names null", replace("folder_names", "null"), "album_info[0].folder_names: want a list of strings"},
		{"folder_names with a number", replace("folder_names", "[1]"), "album_info[0].folder_names[0]: want a string or null"},
		{"no folder_names", without("folder_names"), "album_info[0].folder_names: want a value"},
		{"creation_date number", replace("creation_date", "5"), "album_info[0].creation_date: want an ISO 8601 string"},
		{"creation_date text", replace("creation_date", `"SECRET-DATE"`), "album_info[0].creation_date: want an ISO 8601 string"},
		{"no creation_date", without("creation_date"), "album_info[0].creation_date: want a value"},
		{"start_date text", replace("start_date", `"x"`), "album_info[0].start_date: want an ISO 8601 string"},
		{"end_date text", replace("end_date", `"x"`), "album_info[0].end_date: want an ISO 8601 string"},
		{"no owner", without("owner"), "album_info[0].owner: want a value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, key := range []string{"album_info", "burst_album_info"} {
				_, err := rules[key].ref(Object{key: []byte("[" + c.elem + "]")})
				want := strings.ReplaceAll(c.want, "album_info", key)
				var se *shapeError
				if !errors.As(err, &se) || !strings.Contains(err.Error(), want) {
					t.Errorf("%s: error = %v, want %q", key, err, want)
				}
				if err != nil && strings.Contains(err.Error(), "SECRET") {
					t.Errorf("the error repeats the value: %v", err)
				}
			}
		})
	}

	t.Run("folders value is not a list", func(t *testing.T) {
		t.Parallel()
		o := Object{"album_info": []byte("[]"), "folders": []byte(`{"A": 5}`)}
		if _, err := rules["folders"].ref(o); err == nil || !strings.Contains(err.Error(), "folders.value: want a list of strings") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("albums is not a list", func(t *testing.T) {
		t.Parallel()
		o := Object{"album_info": []byte("[]"), "albums": []byte(`{}`)}
		if _, err := rules["albums"].ref(o); err == nil || !strings.Contains(err.Error(), "albums: want a list of strings") {
			t.Errorf("error = %v", err)
		}
	})
}

// TestAlbumsMissingOnTheGoSide: an album that a photo lists and Library does
// not have is an error for each of the keys that read the albums.
func TestAlbumsMissingOnTheGoSide(t *testing.T) {
	t.Parallel()
	for _, key := range AlbumKeys() {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			lib := Library{
				Photos: []*photoinfo.PhotoInfo{{UUID: p1, IsPhoto: true, AlbumUUIDs: []string{albumA1}, BurstAlbumUUIDs: []string{albumA1}}},
				Albums: []*albuminfo.AlbumInfo{{UUID: albumA9}},
			}
			ref := refOf(refObj(refP1()))
			_, err := Compare(ref, lib, []string{key}, nil)
			if err == nil || !strings.Contains(err.Error(), "missing from Library.Albums") || !strings.Contains(err.Error(), "key "+key) {
				t.Errorf("error = %v, want one about a missing album for %s", err, key)
			}
			if err != nil && strings.Contains(err.Error(), albumA1) {
				t.Errorf("the error names the album: %v", err)
			}
		})
	}
}

func TestOrderKeyErrors(t *testing.T) {
	t.Parallel()
	good := object("2020-01-01T00:00:00+00:00", "", p1)
	t.Run("uuid is not a string", func(t *testing.T) {
		t.Parallel()
		o := object("2020-01-01T00:00:00+00:00", "", p1)
		o["uuid"] = []byte("5")
		if err := refOf(good, o).CheckOrder(); err == nil || !strings.Contains(err.Error(), "element 2: uuid: want a string") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("date is a number", func(t *testing.T) {
		t.Parallel()
		o := object("2020-01-01T00:00:00+00:00", "", p1)
		o["date"] = []byte("5")
		if err := refOf(o).CheckOrder(); err == nil || !strings.Contains(err.Error(), "date: want an ISO 8601 string") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("date_added is a number", func(t *testing.T) {
		t.Parallel()
		o := object("2020-01-01T00:00:00+00:00", "", p1)
		o["date_added"] = []byte("5")
		if err := refOf(o).CheckOrder(); err == nil || !strings.Contains(err.Error(), "date_added: want an ISO 8601 string") {
			t.Errorf("error = %v", err)
		}
	})
}

func TestIsCanonicalOnJunk(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"{", "", "1e999", "[1,"} {
		if isCanonical([]byte(raw)) {
			t.Errorf("isCanonical(%q) = true", raw)
		}
	}
	for _, raw := range []string{"-0", "0", "-1", `"a"`, "[]", "{}", `{"a":[1,2.5,null]}`, "9223372036854775807", "1e+21", "1.5e-7", "0.00001"} {
		if !isCanonical([]byte(raw)) {
			t.Errorf("isCanonical(%q) = false", raw)
		}
	}
}

func TestFirstOfEmpty(t *testing.T) {
	t.Parallel()
	if first(nil) != 0 || first([]byte("  ")) != 0 || first([]byte(" [")) != '[' {
		t.Error("first does not skip spaces or does not tell an empty value")
	}
	if _, err := refInt(Object{}, "width", true); err == nil {
		t.Error("refInt: no error for a missing key")
	}
	if _, ok := decodeFloat([]byte("")); ok {
		t.Error("decodeFloat accepts an empty value")
	}
}
