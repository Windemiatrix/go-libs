package photosdb

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/adjustmentsinfo"
)

var adjDataValue = map[string]any{
	"adjustments":   []any{map[string]any{"identifier": "Crop"}},
	"metadata":      map[string]any{"orientation": 6},
	"formatVersion": "1.4",
	"versionInfo":   map[string]any{"appVersion": "1.0"},
}

const adjJSON = `{"adjustments":[{"identifier":"Crop"}],"metadata":{"orientation":6},"formatVersion":"1.4","versionInfo":{"appVersion":"1.0"}}`

// Upstream adj_orientation is 0 when adjustmentData does not decode
// (adjustmentsinfo.py:185-192).
var adjZero = adjustmentsinfo.AdjustmentsInfo{Orientation: 0}

var adjStamp = time.Date(2024, 5, 1, 10, 30, 0, 0, time.FixedZone("UTC+3", 3*3600))

var adjFormats = []struct {
	name   string
	format int
}{{"binary", plist.BinaryFormat}, {"XML", plist.XMLFormat}}

func adjCompress(t testing.TB, data []byte, zlibHeader bool) []byte {
	t.Helper()
	var (
		buf bytes.Buffer
		w   io.WriteCloser
		err error
	)
	if zlibHeader {
		w = zlib.NewWriter(&buf)
	} else if w, err = flate.NewWriter(&buf, flate.BestCompression); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func adjPaddedJSON(size int) []byte {
	head := adjJSON[:len(adjJSON)-1] + `,"pad":"`
	return []byte(head + strings.Repeat("x", size-len(head)-2) + `"}`)
}

func adjPaddedFile(size int) []byte {
	const editor = `<dict><key>adjustmentEditorBundleID</key><string>com.example.editor</string><key>pad</key><string>`
	head := xmlPlist(editor + `</string></dict>`)
	return xmlPlist(editor + strings.Repeat("x", size-len(head)) + `</string></dict>`)
}

func assertAdjDecoded(t *testing.T, a *adjustmentsinfo.AdjustmentsInfo) {
	t.Helper()
	wantAdjustments := []any{map[string]any{"identifier": "Crop"}}
	wantVersionInfo := map[string]any{"appVersion": "1.0"}
	if !reflect.DeepEqual(a.Adjustments, wantAdjustments) || a.Metadata == nil || fmt.Sprint(a.Orientation) != "6" ||
		a.AdjustmentFormatVersion != "1.4" || !reflect.DeepEqual(a.VersionInfo, wantVersionInfo) {
		t.Errorf("Adjustments %v, Metadata %v, Orientation %v, AdjustmentFormatVersion %v, VersionInfo %v; want %v, a dictionary, 6, 1.4, %v",
			a.Adjustments, a.Metadata, a.Orientation, a.AdjustmentFormatVersion, a.VersionInfo, wantAdjustments, wantVersionInfo)
	}
}

func assertAdjNotDecoded(t *testing.T, a *adjustmentsinfo.AdjustmentsInfo) {
	t.Helper()
	if a.Adjustments != nil || a.Metadata != nil || a.Orientation != 0 || a.AdjustmentFormatVersion != nil || a.VersionInfo != nil {
		t.Errorf("Adjustments %v, Metadata %v, Orientation %v, AdjustmentFormatVersion %v, VersionInfo %v; want nil, nil, 0, nil, nil",
			a.Adjustments, a.Metadata, a.Orientation, a.AdjustmentFormatVersion, a.VersionInfo)
	}
}

type adjDataCase struct {
	name    string
	data    any // adjustmentData; nil leaves the key out
	decoded bool
}

// The last two are expensive for a parser and must not take long; go test
// -timeout stops a run that does. 40000 arrays keep the XML file under 1 MiB.
func adjDataCases(t testing.TB) []adjDataCase {
	t.Helper()
	binData := marshal(t, adjDataValue, plist.BinaryFormat)
	return []adjDataCase{
		{"binary property list", binData, true},
		{"XML property list", marshal(t, adjDataValue, plist.XMLFormat), true},
		{"JSON", []byte(adjJSON), true},
		{"JSON in a string", adjJSON, true},
		{"raw deflate of JSON", adjCompress(t, []byte(adjJSON), false), true},
		{"zlib of a property list", adjCompress(t, binData, true), true},
		{"inflated to the limit", adjCompress(t, adjPaddedJSON(maxPlistSize), true), true},
		{"inflated over the limit", adjCompress(t, adjPaddedJSON(maxPlistSize+1), true), false},
		{"JSON null", []byte("null"), false},
		{"not decodable", []byte("not adjustment data"), false},
		{"a number", 5, false},
		{"missing", nil, false},
		{"a chain of shared arrays", binaryPlist(binaryArrayChain(0, 40, 2, 1), 1), false},
		{"40000 nested arrays", nestedArrays(40_000), false},
	}
}

func adjFile(t testing.TB, data any, format int) []byte {
	t.Helper()
	file := map[string]any{
		"adjustmentEditorBundleID":   "com.example.editor",
		"adjustmentFormatIdentifier": "com.example.format",
		"adjustmentBaseVersion":      0,
		"adjustmentFormatVersion":    "1.1",
		"adjustmentTimestamp":        adjStamp,
	}
	if data != nil {
		file["adjustmentData"] = data
	}
	return marshal(t, file, format)
}

func TestDecodeAdjustmentsData(t *testing.T) {
	t.Parallel()
	for _, tt := range adjDataCases(t) {
		for _, f := range adjFormats {
			t.Run(fmt.Sprintf("%s, %s file", tt.name, f.name), func(t *testing.T) {
				t.Parallel()
				wantKeys := 5
				var wantData []byte
				switch d := tt.data.(type) {
				case []byte:
					wantData = d
				case string:
					wantData = []byte(d)
				}
				if tt.data != nil {
					wantKeys++
				}
				a := decodeAdjustments(adjFile(t, tt.data, f.format))
				if len(a.Plist) != wantKeys || a.Editor != "com.example.editor" || a.FormatID != "com.example.format" ||
					fmt.Sprint(a.BaseVersion) != "0" || a.FormatVersion != "1.1" {
					t.Errorf("Plist of %d keys, Editor %q, FormatID %q, BaseVersion %v, FormatVersion %v; want %d keys, com.example.editor, com.example.format, 0, 1.1",
						len(a.Plist), a.Editor, a.FormatID, a.BaseVersion, a.FormatVersion, wantKeys)
				}
				if a.Timestamp == nil || !a.Timestamp.Equal(adjStamp) || a.Timestamp.Location() != time.UTC {
					t.Errorf("Timestamp = %v, want %v in UTC", a.Timestamp, adjStamp)
				}
				if !bytes.Equal(a.Data, wantData) || (a.Data == nil) != (wantData == nil) {
					t.Errorf("Data of %d bytes, want %d", len(a.Data), len(wantData))
				}
				if tt.decoded {
					assertAdjDecoded(t, a)
				} else {
					assertAdjNotDecoded(t, a)
				}
			})
		}
	}
}

func FuzzDecodeAdjustments(f *testing.F) {
	for _, c := range adjDataCases(f) {
		for _, format := range adjFormats {
			if file := adjFile(f, c.data, format.format); len(file) <= 8<<10 { // the large cases slow the fuzzer down
				f.Add(file)
			}
		}
	}
	f.Add(modelChain(12, 2, 1))
	f.Fuzz(func(t *testing.T, file []byte) {
		a := decodeAdjustments(file)
		if a.Plist == nil {
			if !reflect.DeepEqual(*a, adjZero) {
				t.Fatalf("decodeAdjustments without a Plist = %+v, want all zero", *a)
			}
		} else if n := countValues(a.Plist); n > len(file) {
			t.Fatalf("decodeAdjustments gave %d values from %d bytes", n, len(file))
		}
		if a.Metadata == nil && a.Orientation != 0 {
			t.Fatalf("decodeAdjustments without Metadata has Orientation %v, want 0", a.Orientation)
		}
	})
}

func TestDecodeAdjustmentsFile(t *testing.T) {
	t.Parallel()

	valid := marshal(t, map[string]any{"adjustmentEditorBundleID": "com.example.editor"}, plist.BinaryFormat)
	// The last four are expensive for a parser; go test -timeout stops a run
	// that takes long.
	zero := map[string][]byte{
		"not a dictionary":            marshal(t, []any{"com.example.editor"}, plist.BinaryFormat),
		"not a property list":         []byte("not a property list"),
		"truncated":                   valid[:len(valid)/2],
		"empty":                       nil,
		"over the limit":              adjPaddedFile(maxPlistSize + 1),
		"a chain of shared arrays":    binaryPlist(binaryArrayChain(0, 40, 2, 1), 1),
		"deepest binary arrays":       binaryPlist(binaryArrayChain(0, maxBinaryObjects-1, 1, 2), 2),
		"60000 nested arrays":         nestedArrays(60_000),
		"an array of 4095 references": flatArray(maxBinaryObjects),
	}
	for name, file := range zero {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if a := decodeAdjustments(file); !reflect.DeepEqual(*a, adjZero) {
				t.Errorf("decodeAdjustments = %+v, want all zero", *a)
			}
		})
	}

	t.Run("at the limit", func(t *testing.T) {
		t.Parallel()
		if a := decodeAdjustments(adjPaddedFile(maxPlistSize)); a.Editor != "com.example.editor" {
			t.Errorf("Editor = %q, want com.example.editor", a.Editor)
		}
	})

	t.Run("values of other types", func(t *testing.T) {
		t.Parallel()
		a := decodeAdjustments(marshal(t, map[string]any{
			"adjustmentEditorBundleID":   1,
			"adjustmentFormatIdentifier": []any{"com.example.format"},
			"adjustmentTimestamp":        "2024-05-01T10:30:00Z",
		}, plist.XMLFormat))
		if a.Editor != "" || a.FormatID != "" || a.Timestamp != nil || len(a.Plist) != 3 {
			t.Errorf("Editor %q, FormatID %q, Timestamp %v, Plist of %d keys; want \"\", \"\", nil, 3 keys", a.Editor, a.FormatID, a.Timestamp, len(a.Plist))
		}
	})

	t.Run("XML date with an offset", func(t *testing.T) {
		t.Parallel()
		a := decodeAdjustments(xmlPlist("<dict><key>adjustmentTimestamp</key><date>2024-05-01T10:30:00+03:00</date></dict>"))
		want := time.Date(2024, 5, 1, 7, 30, 0, 0, time.UTC)
		if a.Timestamp == nil || !a.Timestamp.Equal(want) || a.Timestamp.Location() != time.UTC {
			t.Errorf("Timestamp = %v, want %v in UTC", a.Timestamp, want)
		}
	})

	for _, tt := range []struct{ name, data, want string }{
		{"metadata without orientation", `{"metadata":{"masterWidth":100}}`, "0"},
		{"orientation not an integer", `{"metadata":{"orientation":6.5}}`, "6.5"},
		{"metadata not a dictionary", `{"metadata":[{"orientation":6}]}`, "0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := decodeAdjustments(marshal(t, map[string]any{"adjustmentData": []byte(tt.data)}, plist.BinaryFormat))
			if fmt.Sprint(a.Orientation) != tt.want || a.Adjustments != nil {
				t.Errorf("Orientation %v, Adjustments %v; want %s, nil", a.Orientation, a.Adjustments, tt.want)
			}
		})
	}
}

type countingZeros struct{ left, n int }

func (r *countingZeros) Read(p []byte) (int, error) {
	if r.left == 0 {
		return 0, io.EOF
	}
	k := min(len(p), r.left)
	clear(p[:k])
	r.left -= k
	r.n += k
	return k, nil
}

func TestInflateEndless(t *testing.T) {
	t.Parallel()
	r := &countingZeros{left: 2*maxPlistSize + 8}
	if b := inflate(r); b != nil {
		t.Errorf("inflate of %d zero bytes = %d bytes, want nil", 2*maxPlistSize+8, len(b))
	}
	if r.n > maxPlistSize+1 {
		t.Errorf("inflate read %d bytes, want at most %d", r.n, maxPlistSize+1)
	}
}

func adjWrite(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func adjEditor(t *testing.T, editor string) []byte {
	t.Helper()
	return marshal(t, map[string]any{"adjustmentEditorBundleID": editor}, plist.BinaryFormat)
}

func adjLibrary(t *testing.T) (lib, uuid, edited, original string) {
	t.Helper()
	lib, uuid = editedLibrary(t, "ZKIND = 0, ZKINDSUBTYPE = 2, ZHASADJUSTMENTS = 1, ZUNIFORMTYPEIDENTIFIER = 'public.jpeg'", "")
	x := uuid[:1]
	return lib, uuid, filepath.Join(lib, "resources", "renders", x, uuid+".plist"), filepath.Join(lib, "originals", x, uuid+"_5.aae")
}

func adjOpen(t *testing.T, lib, uuid string) (*DB, *photoInfo) {
	t.Helper()
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	p, ok := db.Photo(uuid)
	if !ok {
		t.Fatalf("Photo(%s): not found", uuid)
	}
	return db, p
}

func TestDBAdjustments(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()
		lib, uuid, edited, original := adjLibrary(t)
		adjWrite(t, edited, adjEditor(t, "com.example.edited"))
		adjWrite(t, original, adjEditor(t, "com.example.original"))
		db, p := adjOpen(t, lib, uuid)
		a, o := db.Adjustments(p), db.OriginalAdjustments(p)
		if a == nil || o == nil || a.Editor != "com.example.edited" || o.Editor != "com.example.original" {
			t.Errorf("Adjustments %+v, OriginalAdjustments %+v; want the editors com.example.edited and com.example.original", a, o)
		}
	})

	t.Run("no files", func(t *testing.T) {
		t.Parallel()
		lib, uuid, _, _ := adjLibrary(t)
		db, p := adjOpen(t, lib, uuid)
		if a, o := db.Adjustments(p), db.OriginalAdjustments(p); a != nil || o != nil {
			t.Errorf("Adjustments %+v, OriginalAdjustments %+v; want nil, nil", a, o)
		}
	})

	// The files are checked again when they are read, as the paths are by Open.
	t.Run("not files after Open", func(t *testing.T) {
		t.Parallel()
		lib, uuid, edited, original := adjLibrary(t)
		adjWrite(t, edited, adjEditor(t, "com.example.edited"))
		adjWrite(t, original, adjEditor(t, "com.example.original"))
		db, p := adjOpen(t, lib, uuid)
		if p.AdjustmentsPath == "" || p.OriginalAdjustmentsPath == "" {
			t.Fatalf("AdjustmentsPath %q, OriginalAdjustmentsPath %q; want both", p.AdjustmentsPath, p.OriginalAdjustmentsPath)
		}
		for _, name := range []string{edited, original} {
			if err := os.Remove(name); err != nil {
				t.Fatal(err)
			}
		}
		writeFile(t, filepath.Join(filepath.Dir(edited), "regular"))
		editedSymlink(t, "regular", edited)
		editedMkdir(t, original)
		if a, o := db.Adjustments(p), db.OriginalAdjustments(p); a != nil || o != nil {
			t.Errorf("Adjustments %+v, OriginalAdjustments %+v; want nil, nil", a, o)
		}
	})

	t.Run("size limit", func(t *testing.T) {
		t.Parallel()
		lib, uuid, edited, original := adjLibrary(t)
		adjWrite(t, edited, adjPaddedFile(maxPlistSize))
		// A valid list one byte over the limit: cut at the limit it would still decode.
		adjWrite(t, original, append(adjPaddedFile(maxPlistSize), '\n'))
		db, p := adjOpen(t, lib, uuid)
		if a := db.Adjustments(p); a == nil || a.Editor != "com.example.editor" {
			t.Errorf("Adjustments of a file of 1 MiB: nil %v, want the editor com.example.editor", a == nil)
		}
		if o := db.OriginalAdjustments(p); o == nil || o.Plist != nil {
			t.Errorf("OriginalAdjustments of a file over 1 MiB: nil %v, want a non-nil value with a nil Plist", o == nil)
		}
	})
}

func TestReadAdjustmentsOutside(t *testing.T) {
	t.Parallel()
	lib, outside := t.TempDir(), t.TempDir()
	db := &DB{info: Info{LibraryPath: lib}}
	inside := filepath.Join(lib, "resources", "renders", "a.plist")
	writeFile(t, inside)
	writeFile(t, filepath.Join(outside, "a.plist"))
	editedSymlink(t, outside, filepath.Join(lib, "resources", "linked"))
	if db.readAdjustments(inside) == nil {
		t.Fatalf("readAdjustments(%q) = nil, want the file", inside)
	}
	up, err := filepath.Rel(lib, filepath.Join(outside, "a.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(outside, "a.plist"),
		lib + string(filepath.Separator) + up,
		filepath.Join(lib, "resources", "linked", "a.plist"),
	} {
		if a := db.readAdjustments(path); a != nil {
			t.Errorf("readAdjustments(%q) = %+v, want nil", path, *a)
		}
	}
}
