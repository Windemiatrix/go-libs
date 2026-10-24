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
	"time"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/gen"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// xmlHead is an XML property list up to its first value.
const xmlHead = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">`

// xmlPlist wraps body (one plist value) into an XML property list.
func xmlPlist(body string) []byte {
	return []byte(xmlHead + body + "</plist>\n")
}

// fill is head followed by as many copies of unit as fit in maxPlistSize
// bytes: the largest such input that reaches the parser.
func fill(head, unit string) []byte {
	return []byte(head + strings.Repeat(unit, (maxPlistSize-len(head))/len(unit)))
}

// modelPlist is an XML plist whose PLModelVersion is the XML value v.
func modelPlist(v string) []byte {
	return xmlPlist("<dict><key>PLModelVersion</key>" + v + "</dict>")
}

// paddedPlist is a valid XML plist of exactly size bytes with PLModelVersion
// 19607, padded by a string under another key.
func paddedPlist(size int) []byte {
	head := xmlPlist("<dict><key>PLModelVersion</key><integer>19607</integer><key>Pad</key><string></string></dict>")
	pad := size - len(head)
	return xmlPlist("<dict><key>PLModelVersion</key><integer>19607</integer><key>Pad</key><string>" +
		strings.Repeat("x", pad) + "</string></dict>")
}

// nestedArrays is an XML plist of depth nested arrays at the root.
func nestedArrays(depth int) []byte {
	return xmlPlist(strings.Repeat("<array>", depth) + strings.Repeat("</array>", depth))
}

// cutAfter returns data up to and including the first marker.
func cutAfter(data []byte, marker string) []byte {
	i := bytes.Index(data, []byte(marker))
	if i < 0 {
		panic("no " + marker)
	}
	return data[:i+len(marker)]
}

// binaryPlist assembles a binary property list (bplist00) from encoded
// objects; object 0 is the root. size is the width in bytes of object
// references and of offsets.
func binaryPlist(objects [][]byte, size int) []byte {
	out := []byte("bplist00")
	offsets := make([]uint64, len(objects))
	for i, o := range objects {
		offsets[i] = uint64(len(out))
		out = append(out, o...)
	}
	table := uint64(len(out))
	for _, off := range offsets {
		out = appendSized(out, off, size)
	}
	// Trailer: 5 unused bytes, sort version, offset size, reference size,
	// number of objects, root object, offset of the offset table.
	out = append(out, 0, 0, 0, 0, 0, 0, byte(size), byte(size))
	out = binary.BigEndian.AppendUint64(out, uint64(len(objects)))
	out = binary.BigEndian.AppendUint64(out, 0)
	return binary.BigEndian.AppendUint64(out, table)
}

// appendSized appends v big-endian in size bytes.
func appendSized(b []byte, v uint64, size int) []byte {
	for i := size - 1; i >= 0; i-- {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}

// bpRefs encodes an array (marker 0xA) or a dictionary (marker 0xD) of
// fewer than 15 entries; a dictionary takes its key references first.
func bpRefs(marker byte, count, size int, refs ...int) []byte {
	o := []byte{marker<<4 | byte(count)}
	for _, r := range refs {
		o = appendSized(o, uint64(r), size)
	}
	return o
}

// chain returns depth arrays starting at object first, followed by the
// integer 0: each array holds width references to the next object, so with
// width 2 every array is reachable along 2^k paths.
func chain(first, depth, width, size int) [][]byte {
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

// modelChain is a binary plist whose PLModelVersion is a chain.
func modelChain(depth, width, size int) []byte {
	key := append([]byte{0x50 | byte(len(modelKey))}, modelKey...)
	objects := [][]byte{bpRefs(0xD, 1, size, 1, 2), key}
	return binaryPlist(append(objects, chain(2, depth, width, size)...), size)
}

// flat is a binary plist of n objects (n ≥ 2, few enough for two-byte
// offsets) and depth 1: a root array that holds n-1 integers, with two-byte
// offsets and references.
func flat(n int) []byte {
	root := []byte{0xAF, 0x11} // array, count in the two-byte integer after it
	root = binary.BigEndian.AppendUint16(root, uint16(n-1))
	objects := [][]byte{nil}
	for i := 1; i < n; i++ {
		root = appendSized(root, uint64(i), 2)
		objects = append(objects, []byte{0x10, byte(i)})
	}
	objects[0] = root
	return binaryPlist(objects, 2)
}

// emptyDictBinary is the smallest binary plist: an empty dictionary, with
// one-byte offsets and references.
func emptyDictBinary() []byte {
	return binaryPlist([][]byte{bpRefs(0xD, 0, 1)}, 1)
}

// Offsets of the fields in a binary plist trailer.
const (
	trOffsetSize = 6
	trRefSize    = 7
	trObjects    = 8
	trTop        = 16
	trTable      = 24
)

// setTrailer returns a copy of data with the trailer field at offset field
// set to v, big-endian in width bytes.
func setTrailer(data []byte, field, width int, v uint64) []byte {
	out := bytes.Clone(data)
	copy(out[len(out)-bplistTrailer+field:], appendSized(nil, v, width))
	return out
}

// TestCheckBinaryTrailer checks that a trailer that does not fit the input,
// or declares more than maxBinaryObjects objects, is refused before the
// parser, through detectSchema as well, and that the valid inputs of the
// other tests, the reference cases among them, pass the check and so still
// test the parser.
func TestCheckBinaryTrailer(t *testing.T) {
	t.Parallel()
	base := emptyDictBinary()
	limit := uint64(len(base) - bplistTrailer)
	refused := []struct {
		name string
		data []byte
		want error
	}{
		{"truncated binary", []byte("bplist00\xd1\x01\x02"), errBinaryTrailer},
		{"39 bytes", append([]byte("bplist00"), make([]byte, bplistTrailer-1)...), errBinaryTrailer},
		{"2^40 objects", setTrailer(base, trObjects, 8, 1<<40), errBinaryTrailer},
		{"no objects", setTrailer(base, trObjects, 8, 0), errBinaryTrailer},
		{"more objects than bytes", setTrailer(base, trObjects, 8, limit+1), errBinaryTrailer},
		{"offset size 0", setTrailer(base, trOffsetSize, 1, 0), errBinaryTrailer},
		{"offset size 9", setTrailer(base, trOffsetSize, 1, 9), errBinaryTrailer},
		{"reference size 0", setTrailer(base, trRefSize, 1, 0), errBinaryTrailer},
		{"reference size 9", setTrailer(base, trRefSize, 1, 9), errBinaryTrailer},
		{"root object out of range", setTrailer(base, trTop, 8, 1), errBinaryTrailer},
		{"root object MaxUint64", setTrailer(base, trTop, 8, math.MaxUint64), errBinaryTrailer},
		{"offset table in the header", setTrailer(base, trTable, 8, bplistHeader-1), errBinaryTrailer},
		{"offset table after the trailer start", setTrailer(base, trTable, 8, limit+1), errBinaryTrailer},
		{"offset table at MaxUint64", setTrailer(base, trTable, 8, math.MaxUint64), errBinaryTrailer},
		{"offset table runs into the trailer", setTrailer(base, trObjects, 8, 2), errBinaryTrailer},
		{"wide offsets run into the trailer", setTrailer(base, trOffsetSize, 1, 2), errBinaryTrailer},
		// Consistent trailers: only the number of objects is refused.
		{"one object over the limit", flat(maxBinaryObjects + 1), errBinaryObjects},
		// 900 KB, under the size limit; the parser took 2.5 s on them.
		{"100000 nested arrays at the root", binaryPlist(chain(0, 100_000, 1, 4), 4), errBinaryObjects},
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
		{"offset table right after the header", setTrailer(base, trTable, 8, bplistHeader)},
		{"8-byte offsets and references", binaryPlist([][]byte{bpRefs(0xD, 0, 8)}, 8)},
		{"objects at the limit", flat(maxBinaryObjects)},
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

// TestDecodePlistFormat checks that an input that is neither binary nor XML
// is refused with errPlistFormat before the parser, which would otherwise
// read it as a text property list, and that XML after a byte order mark and
// whitespace is still read.
func TestDecodePlistFormat(t *testing.T) {
	t.Parallel()
	const good = "<plist><dict><key>PLModelVersion</key><integer>19607</integer></dict></plist>"
	refused := []struct {
		name string
		data []byte
	}{
		{"garbage", []byte("garbage")},
		{"OpenStep text", []byte(`{PLModelVersion = 19607;}`)},
		{"deepest text arrays", fill("", "(")},
		{"deepest text dictionaries", fill("", "{")},
		{"text before XML", []byte("garbage" + good)},
		{"no-break space before XML", []byte("\xC2\xA0" + good)},
		{"byte order mark only", []byte("\xEF\xBB\xBF")},
		{"whitespace only", []byte(" \t\r\n")},
		// Starts as XML and is refused after the text parser reads it as data.
		{"text data", []byte("<00>")},
	}
	for _, tt := range refused {
		var md metadata
		if err := decodePlist(tt.data, &md); !errors.Is(err, errPlistFormat) {
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
		var md metadata
		if err := decodePlist(tt.data, &md); err != nil || md.model.value != uint64(19607) {
			t.Errorf("%s: decodePlist = %v, PLModelVersion %#v; want nil, uint64(19607)", tt.name, err, md.model.value)
		}
	}
}

// referenceCases are inputs under the size limit that a parser may find
// expensive. Binary: objects referenced along many paths, and arrays nested
// as deep as maxBinaryObjects allows (chain adds an integer, modelChain a
// dictionary and a key); they pass checkBinaryTrailer and so reach the
// parser. Text: one byte per level of nesting, up to the size limit. XML:
// containers left open up to the size limit. The parser may refuse them; it
// must not take long.
func referenceCases() []detectCase {
	return []detectCase{
		{name: "shared references at the root", plist: binaryPlist(chain(0, 40, 2, 1), 1), errText: []string{msgNoKey, msgNotPlist}},
		{name: "shared references in PLModelVersion", plist: modelChain(40, 2, 1), errText: []string{msgNotInteger, msgNotPlist}},
		{name: "deepest binary arrays at the root", plist: binaryPlist(chain(0, maxBinaryObjects-1, 1, 2), 2), errText: []string{msgNoKey, msgNotPlist}},
		{name: "deepest binary arrays in PLModelVersion", plist: modelChain(maxBinaryObjects-3, 1, 2), errText: []string{msgNotInteger, msgNotPlist}},
		{name: "deepest text arrays", plist: fill("", "("), errText: []string{msgNotPlist}},
		{name: "deepest text dictionaries", plist: fill("", "{"), errText: []string{msgNotPlist}},
		{name: "deepest text arrays after <", plist: fill("<", "("), errText: []string{msgNotPlist}},
		{name: "unclosed XML arrays", plist: fill(xmlHead, "<array>"), errText: []string{msgNotPlist}},
		{name: "unclosed XML dictionaries", plist: fill(xmlHead, "<dict>"), errText: []string{msgNotPlist}},
		{name: "unclosed XML dictionaries under keys", plist: fill(xmlHead, "<dict><key>k</key>"), errText: []string{msgNotPlist}},
	}
}

// TestDetectSchemaReferences checks that the reference cases end, with an
// error, within a bound. A decoder that copies a shared object once per path
// runs out of memory instead, and parsing deep nesting takes time quadratic
// in the depth.
func TestDetectSchemaReferences(t *testing.T) {
	t.Parallel()
	for _, tt := range referenceCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if len(tt.plist) > maxPlistSize {
				t.Fatalf("input is %d bytes, over the size limit: it would test the limit, not the parser", len(tt.plist))
			}
			type result struct {
				model int
				s     schema.Schema
				err   error
			}
			done := make(chan result, 1)
			go func() {
				model, s, err := detectSchema(tt.plist)
				done <- result{model, s, err}
			}()
			select {
			case r := <-done:
				checkDetectResult(t, tt, r.model, r.s, r.err)
			case <-time.After(5 * time.Second):
				t.Fatalf("detectSchema of %d bytes did not return within 5 s", len(tt.plist))
			}
		})
	}
}

const (
	msgNotPlist   = "Z_METADATA.Z_PLIST is not a property list"
	msgTooLarge   = "Z_METADATA.Z_PLIST is larger than 1 MiB"
	msgNoKey      = "no PLModelVersion in Z_METADATA"
	msgNotInteger = "PLModelVersion is not an integer"
)

// detectCase is one input of detectSchema: either a model and schema or an
// error that wraps ErrUnsupportedSchema and contains one of errText.
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
		{name: "binary trailer declares 2^40 objects", plist: setTrailer(emptyDictBinary(), trObjects, 8, 1<<40), errText: []string{msgNotPlist}},
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
	checkDetectResult(t, tt, model, s, err)
}

func checkDetectResult(t *testing.T, tt detectCase, model int, s schema.Schema, err error) {
	t.Helper()
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

// marshal encodes v as a property list in format.
func marshal(t testing.TB, v any, format int) []byte {
	t.Helper()
	data, err := plist.Marshal(v, format)
	if err != nil {
		t.Fatalf("plist.Marshal(%#v): %v", v, err)
	}
	return data
}

// binaryCases are the typed values of PLModelVersion, encoded by
// howett.net/plist in both formats (a real Z_PLIST is binary).
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

// FuzzDetectSchema checks that no input panics, that every error wraps
// ErrUnsupportedSchema and that a result is the schema of its model. The
// seeds are the Z_PLIST of every test library and the cases above.
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

// readZPlist reads Z_PLIST of the Z_METADATA row with the highest Z_VERSION
// of the library at path.
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

// libraryModels is the model and schema of each test library, from the
// public values of testdata/libraries/SOURCES.md.
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
}

// TestDetectSchemaLibraries checks the schema table against real databases:
// the model of each test library and of the synthetic library selects the
// expected schema, and every name of that schema exists in the database.
// The selected names are those of the expected schema, not of the detected
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
	synthetic := filepath.Join(t.TempDir(), "synthetic.photoslibrary")
	cfg := gen.DefaultConfig()
	cfg.Assets = 0
	if err := gen.Generate(t.Context(), synthetic, cfg); err != nil {
		t.Fatalf("gen.Generate: %v", err)
	}
	libs = append(libs, library{"gen " + cfg.Schema, synthetic, 19607, "photos11_1"})

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

// checkNames checks that every table and column of s exists in db. In these
// databases the column of the other era (ZMASTERFINGERPRINT against
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

// columns returns the column names of table; empty when there is no table.
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
