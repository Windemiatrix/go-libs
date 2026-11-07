package golden

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func goCloud(m map[string]any) (string, error) {
	v, err := rules[cloudKey].got(&env{cloudMeta: map[string]map[string]any{"U": m}}, &photoinfo.PhotoInfo{UUID: "U"}, nil)
	if err != nil {
		return "", err
	}
	return canonical(v)
}

// The expectations are literals: the reference is what json.dumps of upstream
// writes for the dictionary plistlib.loads gives, the Go side what
// howett.net/plist gives.
func TestCloudMetadataRule(t *testing.T) {
	t.Parallel()

	whole := func() map[string]any {
		return map[string]any{
			"text":   "v",
			"count":  uint64(7),
			"neg":    int64(-3),
			"ratio":  0.5,
			"flag":   false,
			"when":   time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
			"nested": map[string]any{"list": []any{uint64(1), map[string]any{"k": "v"}}},
			"empty":  map[string]any{},
			"none":   []any{},
		}
	}
	const wholeRef = `{"nested": {"list": [1, {"k": "v"}]}, "text": "v", "count": 7, "neg": -3, "ratio": 0.5, "flag": false, ` +
		`"when": "2020-01-02T03:04:05", "none": [], "empty": {}}`
	with := func(key string, v any) map[string]any {
		m := whole()
		m[key] = v
		return m
	}

	tests := []struct {
		name  string
		ref   string
		got   map[string]any
		equal bool
	}{
		{"every kind of value, the keys in another order", wholeRef, whole(), true},
		{"a text", wholeRef, with("text", "w"), false},
		{"a key more", wholeRef, with("extra", true), false},
		{"a nil value", wholeRef, with("flag", nil), false},
		{"a boolean", wholeRef, with("flag", true), false},
		{"an integer", wholeRef, with("count", uint64(8)), false},
		{"an integer of another type", wholeRef, with("count", int64(7)), true},
		{"a negative integer as unsigned", wholeRef, with("neg", uint64(3)), false},
		{"a real of four bytes is a double", `{"r": 0.10000000149011612}`, map[string]any{"r": float32(0.1)}, true},
		{"a real of four bytes is not the nearest double", `{"r": 0.1}`, map[string]any{"r": float32(0.1)}, false},
		{"a real by one ulp", `{"r": 0.5}`, map[string]any{"r": 0.5000000000000001}, false},
		{"a real written as an exponent", `{"r": 1e-05}`, map[string]any{"r": 0.00001}, true},
		{"a whole real is the integer of its value", `{"r": 3.0}`, map[string]any{"r": uint64(3)}, true},
		{"the integers above 2^53 and above int64", `{"a": 9007199254740993, "b": 18446744073709551615, "c": -9223372036854775808}`,
			map[string]any{"a": int64(9007199254740993), "b": uint64(18446744073709551615), "c": int64(-9223372036854775808)}, true},
		{"an integer above 2^53 by one", `{"a": 9007199254740993}`, map[string]any{"a": int64(9007199254740992)}, false},
		{"a date with microseconds", `{"d": "2020-01-02T03:04:05.000006"}`, map[string]any{"d": time.Date(2020, 1, 2, 3, 4, 5, 6000, time.UTC)}, true},
		{"a date with a microsecond more", `{"d": "2020-01-02T03:04:05.000006"}`, map[string]any{"d": time.Date(2020, 1, 2, 3, 4, 5, 7000, time.UTC)}, false},
		{"a date in another zone, the same moment", `{"d": "2020-01-02T03:04:05"}`,
			map[string]any{"d": time.Date(2020, 1, 2, 8, 34, 5, 0, time.FixedZone("X", 19800))}, true},
		{"a date in the list", `{"l": ["2020-01-02T03:04:05"]}`, map[string]any{"l": []any{time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)}}, true},
		{"a date with an offset in the reference", `{"d": "2020-01-02T03:04:05+00:00"}`, map[string]any{"d": time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)}, false},
		{"the order of a list", `{"l": [1, 2]}`, map[string]any{"l": []any{uint64(2), uint64(1)}}, false},
		{"a list that is shorter", `{"l": [1, 2]}`, map[string]any{"l": []any{uint64(1)}}, false},
		{"a nested dictionary", `{"o": {"p": {"q": 1}}}`, map[string]any{"o": map[string]any{"p": map[string]any{"q": uint64(1)}}}, true},
		{"a nested dictionary that differs", `{"o": {"p": {"q": 1}}}`, map[string]any{"o": map[string]any{"p": map[string]any{"q": uint64(2)}}}, false},
		{"nothing against {}", `{}`, nil, true},
		{"an empty dictionary against {}", `{}`, map[string]any{}, true},
		{"nothing against a value", `{"a": 1}`, nil, false},
		{"a value against {}", `{}`, map[string]any{"a": uint64(1)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			want, err := refCanon(cloudKey, tt.ref)
			if err != nil {
				t.Fatal(err)
			}
			got, err := goCloud(tt.got)
			if err != nil {
				t.Fatal(err)
			}
			if (want == got) != tt.equal {
				t.Errorf("reference %s, Go %s; equal = %v, want %v", want, got, want == got, tt.equal)
			}
		})
	}

	t.Run("the Go side is not changed", func(t *testing.T) {
		t.Parallel()
		m := whole()
		if _, err := goCloud(m); err != nil {
			t.Fatal(err)
		}
		if got, want := m["when"], any(time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)); got != want {
			t.Errorf("when = %v, want the time.Time it was", got)
		}
	})
}

func TestCloudMetadataRuleForm(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"null": `null`, "a number": `5`, "a list": `[]`, "a list of objects": `[{}]`, "a string": `"SECRET-V"`, "a boolean": `true`,
	} {
		t.Run("reference "+name, func(t *testing.T) {
			t.Parallel()
			_, err := refCanon(cloudKey, raw)
			var se *shapeError
			if !errors.As(err, &se) || !strings.Contains(err.Error(), "cloud_metadata: want an object") {
				t.Fatalf("error = %v, want a shape error about the object", err)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Errorf("the error repeats the value: %v", err)
			}
		})
	}
	t.Run("reference number out of range", func(t *testing.T) {
		t.Parallel()
		var se *shapeError
		if _, err := refCanon(cloudKey, `{"a": 1e999}`); !errors.As(err, &se) {
			t.Errorf("error = %v, want a shape error", err)
		}
	})
	t.Run("reference without the key", func(t *testing.T) {
		t.Parallel()
		_, err := rules[cloudKey].ref(Object{})
		if err == nil || !strings.Contains(err.Error(), "cloud_metadata: want a value (the key is missing)") {
			t.Errorf("error = %v, want the missing key", err)
		}
	})
	for name, m := range map[string]map[string]any{
		"bytes":           {"k": []byte("SECRET-B")},
		"bytes in a list": {"k": []any{"a", []byte("SECRET-B")}},
		"bytes in a dict": {"k": map[string]any{"j": []byte("SECRET-B")}},
	} {
		t.Run("Go "+name, func(t *testing.T) {
			t.Parallel()
			_, err := goCloud(m)
			var se *shapeError
			if !errors.As(err, &se) {
				t.Fatalf("error = %v, want a shape error: json.dumps of upstream cannot write bytes", err)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Errorf("the error repeats the value: %v", err)
			}
		})
	}
}

func TestRedactCloudMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, in, want string }{
		{"values", `{"a":"x","b":[1,"",true],"c":{"d":2.5},"e":[],"f":{}}`,
			`{"a":"<value>","b":["<value>","","<value>"],"c":{"d":"<value>"},"e":[],"f":{}}`},
		{"nothing", `{}`, `{}`},
		{"not an object", `[1]`, `<unparsed>`},
	}
	for _, tt := range tests {
		if got := redact(cloudKey, tt.in); got != tt.want {
			t.Errorf("%s: redact = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestWriteDetailsCloudMetadata(t *testing.T) {
	t.Parallel()
	lib := fixtureLib()
	lib.CloudMetadata[selectedFrame]["mediaType"] = "SECRET-MEDIA"
	stale := Divergence{Library: "lib", UUID: emptyPhoto, Field: "cloud_metadata", Want: []byte(`{"mediaType":"SECRET-STALE","flag":false}`), Reason: "r", Upstream: "a.py:1"}
	unparsed := Divergence{Library: "lib", UUID: selectedFrame, Field: "cloud_metadata", Want: []byte(`[1]`), Reason: "r", Upstream: "a.py:1"}
	rep := compare(t, fixtureRef(), lib, stale, unparsed)
	if got := diffKeys(rep); !slices.Equal(got, []string{"cloud_metadata"}) {
		t.Fatalf("differences in %v, want cloud_metadata", got)
	}
	var b strings.Builder
	if err := rep.WriteDetails(&b); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SECRET", "image", "9007199254740993", "0.5", "2020-01-02"} {
		if strings.Contains(b.String(), leak) {
			t.Errorf("WriteDetails writes %q", leak)
		}
	}
	for _, line := range []string{
		`"big":"<value>"`,
		`"mediaType":"<value>"`,
		`"nested":{"list":["<value>","<value>"]}`,
		`stale cloud_metadata ` + emptyPhoto + ` want={"flag":"<value>","mediaType":"<value>"} got=*` + "\n",
		"stale cloud_metadata " + selectedFrame + " want=<unparsed> got=*\n",
	} {
		if !strings.Contains(b.String(), line) {
			t.Errorf("WriteDetails lacks %q:\n%s", line, b.String())
		}
	}
}
