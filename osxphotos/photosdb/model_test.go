package photosdb

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

const xmlHead = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">`

func xmlPlist(body string) []byte {
	return []byte(xmlHead + body + "</plist>\n")
}

func fillToLimit(head, unit string) []byte {
	return []byte(head + strings.Repeat(unit, (maxPlistSize-len(head))/len(unit)))
}

func modelPlist(v string) []byte {
	return xmlPlist("<dict><key>PLModelVersion</key>" + v + "</dict>")
}

func paddedPlist(size int) []byte {
	head := xmlPlist("<dict><key>PLModelVersion</key><integer>19607</integer><key>Pad</key><string></string></dict>")
	pad := size - len(head)
	return xmlPlist("<dict><key>PLModelVersion</key><integer>19607</integer><key>Pad</key><string>" +
		strings.Repeat("x", pad) + "</string></dict>")
}

func nestedArrays(depth int) []byte {
	return xmlPlist(strings.Repeat("<array>", depth) + strings.Repeat("</array>", depth))
}

func cutAfter(data []byte, marker string) []byte {
	i := bytes.Index(data, []byte(marker))
	if i < 0 {
		panic("no " + marker)
	}
	return data[:i+len(marker)]
}

func binaryPlist(objects [][]byte, size int) []byte {
	out := []byte("bplist00")
	offsets := make([]uint64, len(objects))
	for i, o := range objects {
		offsets[i] = uint64(len(out))
		out = append(out, o...)
	}
	return withOffsetTable(out, offsets, size)
}

// The format allows several objects at one offset.
func withOffsetTable(out []byte, offsets []uint64, size int) []byte {
	table := uint64(len(out))
	for _, off := range offsets {
		out = appendSized(out, off, size)
	}
	// Trailer: 5 unused bytes, sort version, offset size, reference size,
	// number of objects, root object, offset of the offset table.
	out = append(out, 0, 0, 0, 0, 0, 0, byte(size), byte(size))
	out = binary.BigEndian.AppendUint64(out, uint64(len(offsets)))
	out = binary.BigEndian.AppendUint64(out, 0)
	return binary.BigEndian.AppendUint64(out, table)
}

func appendSized(b []byte, v uint64, size int) []byte {
	for i := size - 1; i >= 0; i-- {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}

// The count is the low nibble of the marker, so it is under 15; a dictionary
// takes its key references first.
func bpRefs(marker byte, count, size int, refs ...int) []byte {
	o := []byte{marker<<4 | byte(count)}
	for _, r := range refs {
		o = appendSized(o, uint64(r), size)
	}
	return o
}

// With width 2 every array is reachable along 2^k paths.
func binaryArrayChain(first, depth, width, size int) [][]byte {
	objects := make([][]byte, 0, depth+1)
	for k := range depth {
		refs := make([]int, width)
		for i := range refs {
			refs[i] = first + k + 1
		}
		objects = append(objects, bpRefs(0xA, width, size, refs...))
	}
	return append(objects, []byte{0x10, 0x00})
}

func modelChain(depth, width, size int) []byte {
	key := append([]byte{0x50 | byte(len(modelKey))}, modelKey...)
	objects := [][]byte{bpRefs(0xD, 1, size, 1, 2), key}
	return binaryPlist(append(objects, binaryArrayChain(2, depth, width, size)...), size)
}

// The count is a two-byte integer after the marker, so it may be 15 or more;
// the references are two bytes too.
func bpLongRefs(marker byte, refs ...int) []byte {
	o := binary.BigEndian.AppendUint16([]byte{marker<<4 | 0xF, 0x11}, uint16(len(refs)))
	for _, r := range refs {
		o = appendSized(o, uint64(r), 2)
	}
	return o
}

// n ≥ 2 and few enough for two-byte offsets.
func flatArray(n int) []byte {
	var refs []int
	objects := [][]byte{nil}
	for i := 1; i < n; i++ {
		refs = append(refs, i)
		objects = append(objects, []byte{0x10, byte(i)})
	}
	objects[0] = bpLongRefs(0xA, refs...)
	return binaryPlist(objects, 2)
}

func repeatRef(ref, n int) []int {
	refs := make([]int, n)
	for i := range refs {
		refs[i] = ref
	}
	return refs
}

// The m indices after the root all have the offset of one array of k
// references, so no index repeats, yet any would get m*k values.
func aliasedArrays(m, k int) []byte {
	out := append([]byte("bplist00"), bpLongRefs(0xA, repeatRef(m+1, k)...)...)
	offsets := make([]uint64, m+2)
	offsets[0] = uint64(len(out))
	root := make([]int, m)
	for i := range root {
		root[i] = i + 1
		offsets[i+1] = bplistHeaderSize
	}
	out = append(out, bpLongRefs(0xA, root...)...)
	offsets[m+1] = uint64(len(out))
	return withOffsetTable(append(out, 0x51, 'x'), offsets, 2)
}

func emptyDictBinary() []byte {
	return binaryPlist([][]byte{bpRefs(0xD, 0, 1)}, 1)
}

const (
	trailerOffsetSize = 6
	trailerRefSize    = 7
	trailerObjects    = 8
	trailerTop        = 16
	trailerTable      = 24
)

func setTrailer(data []byte, field, width int, v uint64) []byte {
	out := bytes.Clone(data)
	copy(out[len(out)-bplistTrailerSize+field:], appendSized(nil, v, width))
	return out
}

// The valid inputs of the other tests, the reference cases among them, must
// pass the check: otherwise they would no longer test the parser.
func TestCheckBinaryTrailer(t *testing.T) {
	t.Parallel()
	base := emptyDictBinary()
	limit := uint64(len(base) - bplistTrailerSize)
	refused := []struct {
		name string
		data []byte
		want error
	}{
		{"truncated binary", []byte("bplist00\xd1\x01\x02"), errBinaryTrailer},
		{"39 bytes", append([]byte("bplist00"), make([]byte, bplistTrailerSize-1)...), errBinaryTrailer},
		{"2^40 objects", setTrailer(base, trailerObjects, 8, 1<<40), errBinaryTrailer},
		{"no objects", setTrailer(base, trailerObjects, 8, 0), errBinaryTrailer},
		{"more objects than bytes", setTrailer(base, trailerObjects, 8, limit+1), errBinaryTrailer},
		{"offset size 0", setTrailer(base, trailerOffsetSize, 1, 0), errBinaryTrailer},
		{"offset size 9", setTrailer(base, trailerOffsetSize, 1, 9), errBinaryTrailer},
		{"reference size 0", setTrailer(base, trailerRefSize, 1, 0), errBinaryTrailer},
		{"reference size 9", setTrailer(base, trailerRefSize, 1, 9), errBinaryTrailer},
		{"root object out of range", setTrailer(base, trailerTop, 8, 1), errBinaryTrailer},
		{"root object MaxUint64", setTrailer(base, trailerTop, 8, math.MaxUint64), errBinaryTrailer},
		{"offset table in the header", setTrailer(base, trailerTable, 8, bplistHeaderSize-1), errBinaryTrailer},
		{"offset table after the trailer start", setTrailer(base, trailerTable, 8, limit+1), errBinaryTrailer},
		{"offset table at MaxUint64", setTrailer(base, trailerTable, 8, math.MaxUint64), errBinaryTrailer},
		{"offset table runs into the trailer", setTrailer(base, trailerObjects, 8, 2), errBinaryTrailer},
		{"wide offsets run into the trailer", setTrailer(base, trailerOffsetSize, 1, 2), errBinaryTrailer},
		{"one object over the limit", flatArray(maxBinaryObjects + 1), errBinaryObjects},
		// 900 KB, under the size limit; the parser took 2.5 s on them.
		{"100000 nested arrays at the root", binaryPlist(binaryArrayChain(0, 100_000, 1, 4), 4), errBinaryObjects},
		{"100000 nested arrays in PLModelVersion", modelChain(100_000, 1, 4), errBinaryObjects},
	}
	for _, tt := range refused {
		if err := checkBinaryTrailer(tt.data); !errors.Is(err, tt.want) {
			t.Errorf("%s: checkBinaryTrailer = %v, want %v", tt.name, err, tt.want)
		}
		_, _, err := detectSchema(tt.data)
		if !errors.Is(err, ErrUnsupportedSchema) || !errors.Is(err, tt.want) || !strings.Contains(err.Error(), msgNotPlist) {
			t.Errorf("%s: detectSchema error = %v, want %q wrapping ErrUnsupportedSchema and %v", tt.name, err, msgNotPlist, tt.want)
		}
	}

	accepted := []struct {
		name string
		data []byte
	}{
		{"empty dictionary", base},
		{"offset table right after the header", setTrailer(base, trailerTable, 8, bplistHeaderSize)},
		{"8-byte offsets and references", binaryPlist([][]byte{bpRefs(0xD, 0, 8)}, 8)},
		{"objects at the limit", flatArray(maxBinaryObjects)},
		{"marshaled", marshal(t, map[string]any{modelKey: 19607}, plist.BinaryFormat)},
		{"XML", modelPlist("<integer>19607</integer>")},
		{"garbage", []byte("garbage")},
		{"bplis", []byte("bplis")},
	}
	for _, c := range referenceCases() {
		accepted = append(accepted, struct {
			name string
			data []byte
		}{c.name, c.plist})
	}
	for _, c := range binaryCases(t) {
		accepted = append(accepted, struct {
			name string
			data []byte
		}{c.name, c.plist})
	}
	for _, tt := range accepted {
		if err := checkBinaryTrailer(tt.data); err != nil {
			t.Errorf("%s: checkBinaryTrailer = %v, want nil", tt.name, err)
		}
	}
}

// Without the check the parser would read an input that is neither binary
// nor XML as a text property list.
func TestDecodePlistFormat(t *testing.T) {
	t.Parallel()
	const good = "<plist><dict><key>PLModelVersion</key><integer>19607</integer></dict></plist>"
	refused := []struct {
		name string
		data []byte
	}{
		{"garbage", []byte("garbage")},
		{"OpenStep text", []byte(`{PLModelVersion = 19607;}`)},
		{"deepest text arrays", fillToLimit("", "(")},
		{"deepest text dictionaries", fillToLimit("", "{")},
		{"text before XML", []byte("garbage" + good)},
		{"no-break space before XML", []byte("\xC2\xA0" + good)},
		{"byte order mark only", []byte("\xEF\xBB\xBF")},
		{"whitespace only", []byte(" \t\r\n")},
		// Starts as XML and is refused after the text parser reads it as data.
		{"text data", []byte("<00>")},
	}
	for _, tt := range refused {
		var v any
		if err := decodePlist(tt.data, &v); !errors.Is(err, errPlistFormat) {
			t.Errorf("%s: decodePlist = %v, want %v", tt.name, err, errPlistFormat)
		}
	}

	accepted := []struct {
		name string
		data []byte
	}{
		{"XML", []byte(good)},
		{"whitespace before XML", []byte(" \t\r\n" + good)},
		{"byte order mark before XML", []byte("\xEF\xBB\xBF" + good)},
		{"byte order mark and whitespace before XML", []byte("\xEF\xBB\xBF\n " + good)},
	}
	for _, tt := range accepted {
		var v any
		err := decodePlist(tt.data, &v)
		dict, _ := v.(map[string]any)
		if err != nil || dict[modelKey] != uint64(19607) {
			t.Errorf("%s: decodePlist = %v, PLModelVersion %#v; want nil, uint64(19607)", tt.name, err, dict[modelKey])
		}
	}
}

type sharedCase struct {
	name string
	data []byte
	want error
}

func sharedCases() []sharedCase {
	str := func(s string) []byte { return append([]byte{0x50 | byte(len(s))}, s...) }
	twice := func(container []byte) []byte {
		return binaryPlist([][]byte{bpRefs(0xA, 2, 1, 1, 1), container}, 1)
	}
	// Past the 15 references that fit the marker.
	long := func(arrayRefs int) []byte {
		refs := append(repeatRef(2, 15), repeatRef(1, arrayRefs)...)
		return binaryPlist([][]byte{bpLongRefs(0xA, refs...), {0xA0}, str("x")}, 2)
	}
	longCount := func(marker byte, count uint64) []byte {
		return binaryPlist([][]byte{binary.BigEndian.AppendUint64([]byte{marker<<4 | 0xF, 0x13}, count)}, 1)
	}
	return []sharedCase{
		{"a shared array", twice([]byte{0xA0}), errSharedContainer},
		{"a shared ordered set", twice([]byte{0xB0}), errSharedContainer},
		{"a shared set", twice([]byte{0xC0}), errSharedContainer},
		{"a shared dictionary", twice([]byte{0xD0}), errSharedContainer},
		{"a dictionary key shared with its value", binaryPlist([][]byte{bpRefs(0xD, 1, 1, 1, 1), {0xA0}}, 1), errSharedContainer},
		{"a shared array after 15 references", long(2), errSharedContainer},
		{"a chain of shared arrays", binaryPlist(binaryArrayChain(0, 12, 2, 1), 1), errSharedContainer},
		{"600 indices of one array of 2000 references", aliasedArrays(600, 2000), errSharedContainer},
		{"a reference out of the list", binaryPlist([][]byte{bpRefs(0xA, 1, 1, 5), {0x10, 0x00}}, 1), errSharedContainer},
		{"a long count without an integer", binaryPlist([][]byte{{0xAF, 0x51, 'x'}}, 1), errSharedContainer},
		{"a long count of 16 bytes", binaryPlist([][]byte{append([]byte{0xAF, 0x14}, make([]byte, 64)...)}, 1), errSharedContainer},
		{"a long count cut short", binaryPlist([][]byte{{0xAF, 0x13, 0x00}}, 1), errSharedContainer},
		{"a count that does not fit", longCount(0xA, 1<<40), errSharedContainer},
		{"a dictionary count of 2^63", longCount(0xD, 1<<63), errSharedContainer},

		{"a shared string", binaryPlist([][]byte{bpRefs(0xD, 2, 1, 1, 2, 3, 3), str("a"), str("b"), str("x")}, 1), nil},
		{"an array after 15 references", long(1), nil},
		{"one index per array", aliasedArrays(1, 20), nil},
		{"an array of 4095 references", flatArray(maxBinaryObjects), nil},
	}
}

func TestDecodePlistShared(t *testing.T) {
	t.Parallel()
	for _, tt := range sharedCases() {
		var v any
		if err := decodePlist(tt.data, &v); !errors.Is(err, tt.want) {
			t.Errorf("%s: decodePlist = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func FuzzDecodePlist(f *testing.F) {
	for _, c := range sharedCases() {
		f.Add(c.data)
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		var v any
		_ = decodePlist(data, &v)
	})
}

// The inputs are under the size limit and a parser may find them expensive: it
// may refuse them, but must not take long. The binary ones pass
// checkBinaryTrailer and so reach the parser; binaryArrayChain adds an integer
// to the nesting maxBinaryObjects allows, modelChain a dictionary and a key.
func referenceCases() []detectCase {
	return []detectCase{
		{name: "shared references at the root", plist: binaryPlist(binaryArrayChain(0, 40, 2, 1), 1), errText: []string{msgNoKey, msgNotPlist}},
		{name: "shared references in PLModelVersion", plist: modelChain(40, 2, 1), errText: []string{msgNotInteger, msgNotPlist}},
		{name: "deepest binary arrays at the root", plist: binaryPlist(binaryArrayChain(0, maxBinaryObjects-1, 1, 2), 2), errText: []string{msgNoKey, msgNotPlist}},
		{name: "deepest binary arrays in PLModelVersion", plist: modelChain(maxBinaryObjects-3, 1, 2), errText: []string{msgNotInteger, msgNotPlist}},
		{name: "deepest text arrays", plist: fillToLimit("", "("), errText: []string{msgNotPlist}},
		{name: "deepest text dictionaries", plist: fillToLimit("", "{"), errText: []string{msgNotPlist}},
		{name: "deepest text arrays after <", plist: fillToLimit("<", "("), errText: []string{msgNotPlist}},
		{name: "unclosed XML arrays", plist: fillToLimit(xmlHead, "<array>"), errText: []string{msgNotPlist}},
		{name: "unclosed XML dictionaries", plist: fillToLimit(xmlHead, "<dict>"), errText: []string{msgNotPlist}},
		{name: "unclosed XML dictionaries under keys", plist: fillToLimit(xmlHead, "<dict><key>k</key>"), errText: []string{msgNotPlist}},
	}
}

// A decoder that copies a shared object once per path runs out of memory, and
// one quadratic in the depth of nesting takes minutes for a megabyte; go test
// -timeout stops such a run.
func TestDetectSchemaReferences(t *testing.T) {
	t.Parallel()
	for _, tt := range referenceCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if len(tt.plist) > maxPlistSize {
				t.Fatalf("input is %d bytes, over the size limit: it would test the limit, not the parser", len(tt.plist))
			}
			checkDetect(t, tt)
		})
	}
}

const (
	msgNotPlist   = "Z_METADATA.Z_PLIST is not a property list"
	msgTooLarge   = "Z_METADATA.Z_PLIST is larger than 1 MiB"
	msgNoKey      = "no PLModelVersion in Z_METADATA"
	msgNotInteger = "PLModelVersion is not an integer"
)

type detectCase struct {
	name      string
	plist     []byte
	wantModel int
	wantID    string
	errText   []string
}

func detectCases() []detectCase {
	return []detectCase{
		{name: "integer", plist: modelPlist("<integer>19607</integer>"), wantModel: 19607, wantID: "photos11_1"},
		{name: "decimal string", plist: modelPlist("<string>19320</string>"), wantModel: 19320, wantID: "photos11_1"},
		{name: "photos12", plist: modelPlist("<integer>270008501</integer>"), wantModel: 270008501, wantID: "photos12"},
		{name: "exactly 1 MiB", plist: paddedPlist(1 << 20), wantModel: 19607, wantID: "photos11_1"},

		{name: "macOS 15 beta 1", plist: modelPlist("<integer>18100</integer>"), errText: []string{"model version 18100"}},
		{name: "gap after photos10", plist: modelPlist("<integer>19050</integer>"), errText: []string{"model version 19050"}},
		{name: "after photos12", plist: modelPlist("<integer>271000000</integer>"), errText: []string{"model version 271000000"}},
		{name: "negative", plist: modelPlist("<integer>-1</integer>"), errText: []string{"model version -1"}},
		{name: "above MaxInt64", plist: modelPlist("<integer>9223372036854775808</integer>"), errText: []string{"model version 9223372036854775808"}},
		{name: "digits above MaxInt64", plist: modelPlist("<string>99999999999999999999</string>"), errText: []string{"model version 99999999999999999999"}},

		{name: "nil", plist: nil, errText: []string{msgNotPlist}},
		{name: "empty", plist: []byte{}, errText: []string{msgNotPlist}},
		{name: "garbage", plist: []byte("garbage"), errText: []string{msgNotPlist}},
		{name: "OpenStep text", plist: []byte(`{PLModelVersion = 19607;}`), errText: []string{msgNotPlist}},
		{name: "truncated XML header", plist: modelPlist("<integer>19607</integer>")[:120], errText: []string{msgNotPlist}},
		{name: "truncated XML value", plist: cutAfter(modelPlist("<integer>19607</integer>"), "<integer>196"), errText: []string{msgNotPlist}},
		{name: "XML without closing tags", plist: bytes.TrimSuffix(modelPlist("<integer>19607</integer>"), []byte("</dict></plist>\n")), errText: []string{msgNotPlist}},
		{name: "truncated binary", plist: []byte("bplist00\xd1\x01\x02"), errText: []string{msgNotPlist}},
		{name: "binary trailer declares 2^40 objects", plist: setTrailer(emptyDictBinary(), trailerObjects, 8, 1<<40), errText: []string{msgNotPlist}},
		{name: "1 MiB + 1", plist: paddedPlist(1<<20 + 1), errText: []string{msgTooLarge}},
		{name: "100000 nested arrays", plist: nestedArrays(100_000), errText: []string{msgTooLarge}},
		// Under the size limit, so it reaches the parser.
		{name: "60000 nested arrays", plist: nestedArrays(60_000), errText: []string{msgNoKey, msgNotPlist}},

		{name: "no key", plist: xmlPlist("<dict><key>Other</key><integer>19607</integer></dict>"), errText: []string{msgNoKey}},
		{name: "empty dict", plist: xmlPlist("<dict/>"), errText: []string{msgNoKey}},
		{name: "root array", plist: xmlPlist("<array><integer>19607</integer></array>"), errText: []string{msgNoKey}},
		{name: "root string", plist: xmlPlist("<string>19607</string>"), errText: []string{msgNoKey}},
		{name: "key only nested", plist: xmlPlist("<dict><key>Inner</key><dict><key>PLModelVersion</key><integer>19607</integer></dict></dict>"), errText: []string{msgNoKey}},

		{name: "true", plist: modelPlist("<true/>"), errText: []string{msgNotInteger}},
		{name: "real", plist: modelPlist("<real>1.5</real>"), errText: []string{msgNotInteger}},
		{name: "letters", plist: modelPlist("<string>abc</string>"), errText: []string{msgNotInteger}},
		{name: "plus sign", plist: modelPlist("<string>+19607</string>"), errText: []string{msgNotInteger}},
		{name: "leading space", plist: modelPlist("<string> 19607</string>"), errText: []string{msgNotInteger}},
		{name: "trailing space", plist: modelPlist("<string>19607 </string>"), errText: []string{msgNotInteger}},
		{name: "empty string", plist: modelPlist("<string></string>"), errText: []string{msgNotInteger}},
		{name: "fullwidth digits", plist: modelPlist("<string>１９６０７</string>"), errText: []string{msgNotInteger}},
		{name: "array value", plist: modelPlist("<array><integer>19607</integer></array>"), errText: []string{msgNotInteger}},
	}
}

func checkDetect(t *testing.T, tt detectCase) {
	t.Helper()
	model, s, err := detectSchema(tt.plist)
	if tt.errText == nil {
		if err != nil || model != tt.wantModel || s.ID != tt.wantID {
			t.Errorf("detectSchema = %d, %q, %v; want %d, %q, nil", model, s.ID, err, tt.wantModel, tt.wantID)
		}
		if want, _ := schema.ForModel(tt.wantModel); s != want {
			t.Errorf("detectSchema schema = %+v, want schema.ForModel(%d) = %+v", s, tt.wantModel, want)
		}
		return
	}
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("detectSchema error = %v, want one wrapping ErrUnsupportedSchema", err)
	}
	found := false
	for _, text := range tt.errText {
		found = found || strings.Contains(err.Error(), text)
	}
	if !found {
		t.Errorf("detectSchema error = %q, want one containing one of %q", err, tt.errText)
	}
	if model != 0 || s != (schema.Schema{}) {
		t.Errorf("detectSchema with error returned %d, %q; want zero values", model, s.ID)
	}
}

func TestDetectSchema(t *testing.T) {
	t.Parallel()
	for _, tt := range detectCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checkDetect(t, tt)
		})
	}
}

func marshal(t testing.TB, v any, format int) []byte {
	t.Helper()
	data, err := plist.Marshal(v, format)
	if err != nil {
		t.Fatalf("plist.Marshal(%#v): %v", v, err)
	}
	return data
}

// A real Z_PLIST is binary, so the values are encoded in both formats.
func binaryCases(t testing.TB) []detectCase {
	t.Helper()
	values := []struct {
		name      string
		value     any
		wantModel int
		wantID    string
		errText   string
	}{
		{name: "int", value: 19607, wantModel: 19607, wantID: "photos11_1"},
		{name: "int64", value: int64(270008501), wantModel: 270008501, wantID: "photos12"},
		{name: "uint8", value: uint8(200), errText: "model version 200"},
		{name: "decimal string", value: "19320", wantModel: 19320, wantID: "photos11_1"},
		{name: "macOS 15 beta 1", value: 18100, errText: "model version 18100"},
		{name: "gap", value: 19050, errText: "model version 19050"},
		{name: "after photos12", value: 271000000, errText: "model version 271000000"},
		{name: "negative", value: -1, errText: "model version -1"},
		{name: "above MaxInt64", value: uint64(1 << 63), errText: "model version 9223372036854775808"},
		{name: "above MaxInt32", value: int64(1 << 40), errText: "model version 1099511627776"},
		{name: "MinInt64", value: int64(-1 << 63), errText: "model version -9223372036854775808"},
		{name: "bool", value: true, errText: msgNotInteger},
		{name: "real", value: 1.5, errText: msgNotInteger},
		{name: "letters", value: "abc", errText: msgNotInteger},
		{name: "fullwidth digits", value: "１９６０７", errText: msgNotInteger},
		{name: "data", value: []byte("19607"), errText: msgNotInteger},
	}
	var out []detectCase
	for _, format := range []struct {
		name string
		id   int
	}{{"binary", plist.BinaryFormat}, {"XML", plist.XMLFormat}} {
		for _, v := range values {
			c := detectCase{
				name:      format.name + " " + v.name,
				plist:     marshal(t, map[string]any{modelKey: v.value, "Other": "x"}, format.id),
				wantModel: v.wantModel,
				wantID:    v.wantID,
			}
			if v.errText != "" {
				c.errText = []string{v.errText}
			}
			out = append(out, c)
		}
		out = append(out,
			detectCase{name: format.name + " no key", plist: marshal(t, map[string]any{"Other": 19607}, format.id), errText: []string{msgNoKey}},
			detectCase{name: format.name + " empty dict", plist: marshal(t, map[string]any{}, format.id), errText: []string{msgNoKey}},
			detectCase{name: format.name + " root array", plist: marshal(t, []any{19607}, format.id), errText: []string{msgNoKey}},
			detectCase{name: format.name + " root string", plist: marshal(t, "19607", format.id), errText: []string{msgNoKey}},
			detectCase{name: format.name + " key only nested", plist: marshal(t, map[string]any{"Inner": map[string]any{modelKey: 19607}}, format.id), errText: []string{msgNoKey}},
		)
	}
	return out
}

func TestDetectSchemaFormats(t *testing.T) {
	t.Parallel()
	for _, tt := range binaryCases(t) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checkDetect(t, tt)
		})
	}
}

func FuzzDetectSchema(f *testing.F) {
	for _, name := range testlib.Libraries(f) {
		f.Add(readZPlist(f, testlib.OpenPristine(f, name)))
	}
	for _, c := range append(detectCases(), referenceCases()...) {
		if len(c.plist) <= 8<<10 { // the large cases slow the fuzzer down
			f.Add(c.plist)
		}
	}
	for _, c := range binaryCases(f) {
		f.Add(c.plist)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		model, s, err := detectSchema(data)
		if err != nil {
			if !errors.Is(err, ErrUnsupportedSchema) {
				t.Fatalf("detectSchema error %v does not wrap ErrUnsupportedSchema", err)
			}
			return
		}
		if want, ok := schema.ForModel(model); !ok || s != want {
			t.Fatalf("detectSchema = %d, %q; schema.ForModel(%d) = %q, %v", model, s.ID, model, want.ID, ok)
		}
	})
}

func readZPlist(t testing.TB, path string) []byte {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(path, "database", "Photos.sqlite")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var data []byte
	if err := db.QueryRowContext(t.Context(),
		"SELECT Z_PLIST FROM Z_METADATA ORDER BY Z_VERSION DESC LIMIT 1").Scan(&data); err != nil {
		t.Fatalf("%s: read Z_METADATA.Z_PLIST: %v", filepath.Base(path), err)
	}
	return data
}

// The values are the public ones of testdata/libraries/SOURCES.md.
var libraryModels = map[string]struct {
	model int
	id    string
}{
	"Test-13.0.0":       {16320, "photos8"},
	"Test-Cloud-13.1":   {16320, "photos8"},
	"Test-14.0.0":       {17120, "photos9"},
	"Test-14.6.0":       {17600, "photos9_14_6"},
	"Test-15.4.1":       {18508, "photos10"},
	"Test-16.0.0-beta":  {19063, "photos11"},
	"Test-26.1":         {19320, "photos11_1"},
	"Test-27.0_DevBeta": {270008501, "photos12"},
	"synthetic":         {19607, "photos11_1"},
}

// The names checked are those of the expected schema, not of the detected
// one, so a wrong name is red even when detection is wrong as well.
func TestDetectSchemaLibraries(t *testing.T) {
	t.Parallel()

	type library struct {
		name, path string
		model      int
		id         string
	}
	var libs []library
	for _, name := range testlib.Libraries(t) {
		want, ok := libraryModels[name]
		if !ok {
			t.Fatalf("test library %s is not in libraryModels; add its model and schema", name)
		}
		libs = append(libs, library{name, testlib.OpenPristine(t, name), want.model, want.id})
	}
	// TestGolden and the other tests take whatever libraries exist, so a
	// deleted library is caught here.
	if len(libs) != len(libraryModels) {
		t.Fatalf("%d test libraries in testdata/libraries, %d in libraryModels", len(libs), len(libraryModels))
	}

	for _, lib := range libs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()
			model, s, err := detectSchema(readZPlist(t, lib.path))
			if err != nil || model != lib.model || s.ID != lib.id {
				t.Errorf("%s: detectSchema = %d, %q, %v; want %d, %q, nil", lib.name, model, s.ID, err, lib.model, lib.id)
			}

			db, err := sql.Open("sqlite", "file:"+filepath.Join(lib.path, "database", "Photos.sqlite")+"?mode=ro")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			checkNames(t, db, lib.name, schemaByID(t, lib.id))
		})
	}
}

func schemaByID(t *testing.T, id string) schema.Schema {
	t.Helper()
	for _, s := range schema.All() {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no schema %q in schema.All()", id)
	return schema.Schema{}
}

// In these databases the column of the other era (ZMASTERFINGERPRINT against
// ZORIGINALSTABLEHASH, ZHASADJUSTMENTS against ZADJUSTMENTSSTATE, ZPERSON
// against ZPERSONFORFACE, ZASSET against ZASSETFORFACE) does not exist, so
// existence catches a name of the wrong era. It does not catch two names of
// one table swapped with each other (album and asset columns, person and
// asset of a face): both exist. TestTable in internal/schema checks what
// each name stands for.
func checkNames(t *testing.T, db *sql.DB, lib string, s schema.Schema) {
	t.Helper()
	tables := []struct {
		table   string
		columns []string
	}{
		{s.MemberTable, []string{s.MemberAlbumColumn, s.MemberAssetColumn, s.MemberOrderColumn}},
		{"Z_1KEYWORDS", []string{"Z_1ASSETATTRIBUTES", s.KeywordColumn}},
		{"ZADDITIONALASSETATTRIBUTES", []string{s.FingerprintColumn}},
		{"ZASSET", []string{s.AdjustmentsColumn}},
		{"ZDETECTEDFACE", []string{s.FacePersonColumn, s.FaceAssetColumn}},
	}
	for _, tt := range tables {
		have := columns(t, db, tt.table)
		if len(have) == 0 {
			t.Errorf("%s (%s): no table %s", lib, s.ID, tt.table)
			continue
		}
		for _, c := range tt.columns {
			if !have[c] {
				t.Errorf("%s (%s): no column %s.%s", lib, s.ID, tt.table, c)
			}
		}
	}
}

func columns(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		t.Fatalf("pragma_table_info(%s): %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	have := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("pragma_table_info(%s): %v", table, err)
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("pragma_table_info(%s): %v", table, err)
	}
	return have
}
