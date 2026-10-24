package photosdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// maxPlistSize bounds every property list this package parses: Z_PLIST of a
// real database and the photolibraryd preferences are a few KiB.
const maxPlistSize = 1 << 20

// modelKey is the key of the model version in Z_METADATA.Z_PLIST.
const modelKey = "PLModelVersion"

// detectSchema returns PLModelVersion from data (the Z_PLIST of the
// Z_METADATA row with the highest Z_VERSION, photosdb_utils.py:88-118) and
// the schema of that model. Every failure wraps ErrUnsupportedSchema.
//
// Unlike osxphotos, which takes the latest schema when the model is missing
// or unknown (photosdb_utils.py:131-136, 160-163), an unknown model is an
// error: the names of another schema would read wrong columns.
func detectSchema(data []byte) (model int, s schema.Schema, err error) {
	if len(data) > maxPlistSize {
		return 0, schema.Schema{}, fmt.Errorf("%w: Z_METADATA.Z_PLIST is larger than 1 MiB", ErrUnsupportedSchema)
	}
	var md metadata
	if err := decodePlist(data, &md); err != nil {
		return 0, schema.Schema{}, fmt.Errorf("%w: Z_METADATA.Z_PLIST is not a property list: %w", ErrUnsupportedSchema, err)
	}
	if !md.model.present {
		return 0, schema.Schema{}, fmt.Errorf("%w: no %s in Z_METADATA", ErrUnsupportedSchema, modelKey)
	}
	model, err = modelVersion(md.model.value)
	if err != nil {
		return 0, schema.Schema{}, err
	}
	s, ok := schema.ForModel(model)
	if !ok {
		return 0, schema.Schema{}, unknownModel(strconv.Itoa(model))
	}
	return model, s, nil
}

// metadata is the part of Z_PLIST that detectSchema reads. A root that is
// not a dictionary decodes as one without PLModelVersion.
type metadata struct {
	model modelField
}

// UnmarshalPlist implements plist.Unmarshaler.
func (m *metadata) UnmarshalPlist(unmarshal func(any) error) error {
	var dict struct {
		Model modelField `plist:"PLModelVersion"`
	}
	if unmarshal(&dict) == nil {
		m.model = dict.Model
	}
	return nil
}

// modelField is the value of PLModelVersion. Only a scalar is decoded: a
// container fails the typed probes without being walked. A binary plist may
// reference one object many times, and decoding such a value into any would
// copy it once per path, exponentially in the depth.
type modelField struct {
	present bool
	value   any // as decoded into any; nil when not a number or a string
}

// UnmarshalPlist implements plist.Unmarshaler. The probes only tell a scalar
// from a container; the value itself is decoded into any, as before.
func (f *modelField) UnmarshalPlist(unmarshal func(any) error) error {
	f.present = true
	var (
		u uint64
		i int64
		s string
	)
	if unmarshal(&u) == nil || unmarshal(&i) == nil || unmarshal(&s) == nil {
		return unmarshal(&f.value)
	}
	return nil
}

// modelVersion converts the value of PLModelVersion: an integer, or a string
// of ASCII decimal digits as upstream int() of a str (photosdb_utils.py:137)
// but without its sign, spaces and non-ASCII digits. howett.net/plist
// decodes integers as uint64 or int64. A number outside int32, far beyond
// every known model, is an unknown model, not a type error.
func modelVersion(v any) (int, error) {
	switch n := v.(type) {
	case uint64:
		if n > math.MaxInt32 {
			return 0, unknownModel(strconv.FormatUint(n, 10))
		}
		return int(n), nil
	case int64:
		if n < math.MinInt32 || n > math.MaxInt32 {
			return 0, unknownModel(strconv.FormatInt(n, 10))
		}
		return int(n), nil
	case string:
		if !asciiDigits(n) {
			return 0, notInteger()
		}
		m, err := strconv.Atoi(n)
		if err != nil { // digits only, so out of range
			return 0, unknownModel(n)
		}
		return m, nil
	}
	return 0, notInteger()
}

func notInteger() error {
	return fmt.Errorf("%w: %s is not an integer", ErrUnsupportedSchema, modelKey)
}

func unknownModel(model string) error {
	return fmt.Errorf("%w: model version %s", ErrUnsupportedSchema, model)
}

func asciiDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// maxBinaryObjects bounds the number of objects of a binary property list,
// read from its trailer before parsing. howett.net/plist v1.0.1 parses
// nested containers in time quadratic in the depth, and the depth can be as
// large as the number of objects: 100000 nested arrays in 900 KB took 2.5 s
// (about 20 s with the race detector), 4096 take a few milliseconds. A real
// Z_PLIST has fewer than 200 objects (144 to 189 in the test libraries). The
// bound applies to every caller of decodePlist: the photolibraryd
// preferences that SystemLibraryPath reads as well.
const maxBinaryObjects = 1 << 12

var (
	errEmptyPlist    = errors.New("empty")
	errPlistFormat   = errors.New("not an XML or binary property list")
	errBinaryTrailer = errors.New("binary property list trailer does not fit the input")
	errBinaryObjects = fmt.Errorf("binary property list has more than %d objects", maxBinaryObjects)
)

// decodePlist decodes an XML or a binary property list into v, the two
// formats that osxphotos reads with plistlib. The text formats that
// howett.net/plist also accepts (OpenStep, GNUstep) are refused: in them
// almost any text, such as "garbage", is a valid string. An input that does
// not start as binary or XML is refused before the parser, which would read
// it as text: there one byte opens one level of nesting, and 1 MiB of "("
// took 0.6 s, 0.5 GB of allocations and a 256 MB stack (1.6 s and 512 MB
// with the race detector). An XML input that the XML parser refuses still
// falls back to the text parser and is refused after it; as text it starts
// with "<", which opens data, not nesting. v is a struct or an Unmarshaler
// with only the keys the caller needs, never any: see modelField.
func decodePlist(data []byte, v any) error {
	if len(data) == 0 {
		return errEmptyPlist
	}
	if !bytes.HasPrefix(data, []byte("bplist")) && !startsAsXML(data) {
		return errPlistFormat
	}
	if err := checkBinaryTrailer(data); err != nil {
		return err
	}
	format, err := plist.Unmarshal(data, v)
	if err != nil {
		return err
	}
	if format != plist.XMLFormat && format != plist.BinaryFormat {
		return errPlistFormat
	}
	return nil
}

// startsAsXML reports whether the first byte of data after an optional UTF-8
// byte order mark and XML whitespace is "<".
func startsAsXML(data []byte) bool {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	data = bytes.TrimLeft(data, " \t\r\n")
	return len(data) > 0 && data[0] == '<'
}

// bplistHeader and bplistTrailer are the sizes of the header ("bplist" and a
// two-character version) and of the trailer of a binary property list.
const (
	bplistHeader  = 8
	bplistTrailer = 32
)

// checkBinaryTrailer refuses a binary property list whose trailer does not
// fit the input, before the parser allocates anything from it: a trailer of
// a few bytes can declare 2^40 objects. Every object takes at least one byte
// and one offset table entry, so a real input has fewer objects than bytes.
// The bounds come from the input alone and do not depend on how a version
// of howett.net/plist checks the trailer. A list of more than
// maxBinaryObjects objects is refused as well, so that parsing it is cheap
// however deep it nests. Other inputs are left to the parser.
func checkBinaryTrailer(data []byte) error {
	if !bytes.HasPrefix(data, []byte("bplist")) {
		return nil
	}
	if len(data) < bplistHeader+bplistTrailer {
		return errBinaryTrailer
	}
	// Trailer: 5 unused bytes, sort version, offset size, reference size,
	// number of objects, root object, offset of the offset table.
	t := data[len(data)-bplistTrailer:]
	offsetSize, refSize := uint64(t[6]), uint64(t[7])
	numObjects := binary.BigEndian.Uint64(t[8:16])
	topObject := binary.BigEndian.Uint64(t[16:24])
	tableOffset := binary.BigEndian.Uint64(t[24:32])
	limit := uint64(len(data)) - bplistTrailer // the offset table ends here at the latest
	switch {
	case offsetSize < 1 || offsetSize > 8, refSize < 1 || refSize > 8:
		return errBinaryTrailer
	case numObjects == 0, numObjects > limit, topObject >= numObjects:
		return errBinaryTrailer
	// numObjects <= limit and offsetSize <= 8, so the product cannot overflow.
	case tableOffset < bplistHeader, tableOffset > limit, numObjects*offsetSize > limit-tableOffset:
		return errBinaryTrailer
	case numObjects > maxBinaryObjects:
		return errBinaryObjects
	}
	return nil
}
