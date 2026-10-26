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

const modelKey = "PLModelVersion"

// data is the Z_PLIST of the Z_METADATA row with the highest Z_VERSION
// (photosdb_utils.py:88-118). Every failure wraps ErrUnsupportedSchema.
//
// Unlike osxphotos, which takes the latest schema when the model is missing
// or unknown (photosdb_utils.py:131-136, 160-163), an unknown model is an
// error: the names of another schema would read wrong columns.
func detectSchema(data []byte) (model int, s schema.Schema, err error) {
	if len(data) > maxPlistSize {
		return 0, schema.Schema{}, fmt.Errorf("%w: Z_METADATA.Z_PLIST is larger than 1 MiB", ErrUnsupportedSchema)
	}
	var root any
	if err := decodePlist(data, &root); err != nil {
		return 0, schema.Schema{}, fmt.Errorf("%w: Z_METADATA.Z_PLIST is not a property list: %w", ErrUnsupportedSchema, err)
	}
	dict, _ := root.(map[string]any)
	v, ok := dict[modelKey]
	if !ok {
		return 0, schema.Schema{}, fmt.Errorf("%w: no %s in Z_METADATA", ErrUnsupportedSchema, modelKey)
	}
	model, err = modelVersion(v)
	if err != nil {
		return 0, schema.Schema{}, err
	}
	s, ok = schema.ForModel(model)
	if !ok {
		return 0, schema.Schema{}, unknownModel(strconv.Itoa(model))
	}
	return model, s, nil
}

// A string of ASCII decimal digits is accepted as upstream int() of a str
// (photosdb_utils.py:137), but without its sign, spaces and non-ASCII digits.
// howett.net/plist decodes integers as uint64 or int64. A number outside int32,
// far beyond every known model, is an unknown model, not a type error.
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
	errEmptyPlist      = errors.New("empty")
	errPlistFormat     = errors.New("not an XML or binary property list")
	errBinaryTrailer   = errors.New("binary property list trailer does not fit the input")
	errBinaryObjects   = fmt.Errorf("binary property list has more than %d objects", maxBinaryObjects)
	errSharedContainer = errors.New("binary property list references an array or a dictionary more than once")
)

// Only XML and binary lists are accepted, the two formats that osxphotos reads
// with plistlib. The text formats that howett.net/plist also accepts
// (OpenStep, GNUstep) are refused: in them almost any text, such as "garbage",
// is a valid string. An input that does not start as binary or XML is refused
// before the parser, which would read it as text: there one byte opens one
// level of nesting, and 1 MiB of "(" took 0.6 s, 0.5 GB of allocations and a
// 256 MB stack (1.6 s and 512 MB with the race detector). An XML input that
// the XML parser refuses still falls back to the text parser and is refused
// after it; as text it starts with "<", which opens data, not nesting. v may
// be any: sharesContainers refuses the lists that any would expand.
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
	if sharesContainers(data) {
		return errSharedContainer
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

func startsAsXML(data []byte) bool {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	data = bytes.TrimLeft(data, " \t\r\n")
	return len(data) > 0 && data[0] == '<'
}

const (
	bplistHeaderSize  = 8 // "bplist" and a two-character version
	bplistTrailerSize = 32
)

// A binary property list whose trailer does not fit the input is refused
// before the parser allocates anything from it: a trailer of a few bytes can
// declare 2^40 objects. Every object takes at least one byte
// and one offset table entry, so a real input has fewer objects than bytes.
// The bounds come from the input alone and do not depend on how a version
// of howett.net/plist checks the trailer. A list of more than
// maxBinaryObjects objects is refused as well, so that parsing it is cheap
// however deep it nests.
func checkBinaryTrailer(data []byte) error {
	if !bytes.HasPrefix(data, []byte("bplist")) {
		return nil
	}
	if len(data) < bplistHeaderSize+bplistTrailerSize {
		return errBinaryTrailer
	}
	// Trailer: 5 unused bytes, sort version, offset size, reference size,
	// number of objects, root object, offset of the offset table.
	t := data[len(data)-bplistTrailerSize:]
	offsetSize, refSize := uint64(t[6]), uint64(t[7])
	numObjects := binary.BigEndian.Uint64(t[8:16])
	topObject := binary.BigEndian.Uint64(t[16:24])
	tableOffset := binary.BigEndian.Uint64(t[24:32])
	limit := uint64(len(data)) - bplistTrailerSize // the offset table ends here at the latest
	switch {
	case offsetSize < 1 || offsetSize > 8, refSize < 1 || refSize > 8:
		return errBinaryTrailer
	case numObjects == 0, numObjects > limit, topObject >= numObjects:
		return errBinaryTrailer
	// numObjects <= limit and offsetSize <= 8, so the product cannot overflow.
	case tableOffset < bplistHeaderSize, tableOffset > limit, numObjects*offsetSize > limit-tableOffset:
		return errBinaryTrailer
	case numObjects > maxBinaryObjects:
		return errBinaryObjects
	}
	return nil
}

// howett.net/plist decodes a container that several references share into
// any once per path, so a few KiB can grow exponentially; Apple's writer
// shares only strings, numbers, dates and data. The offset table need not be
// one-to-one, so containers of distinct indices may still be one object: the
// references are bounded by the bytes that can hold them as well. A reference
// that does not fit the input counts as shared. data has passed
// checkBinaryTrailer.
func sharesContainers(data []byte) bool {
	if !bytes.HasPrefix(data, []byte("bplist")) {
		return false
	}
	t := data[len(data)-bplistTrailerSize:]
	offsetSize, refSize, n := uint64(t[6]), uint64(t[7]), binary.BigEndian.Uint64(t[8:16])
	table, limit := binary.BigEndian.Uint64(t[24:32]), uint64(len(data))-bplistTrailerSize
	object := func(i uint64) (offset uint64, container bool) {
		offset = bplistUint(data[table+i*offsetSize:], offsetSize)
		return offset, offset < limit && data[offset]>>4 >= 0xA && data[offset]>>4 <= 0xD
	}
	seen := make([]bool, n)
	refs := limit / refSize
	for i := range n {
		off, container := object(i)
		if !container {
			continue
		}
		// CFBinaryPList: a count of 15 or more is the integer object after
		// the marker, and a dictionary holds its keys, then its values.
		count, pos := uint64(data[off]&0xF), off+1
		if count == 0xF {
			if pos >= limit || data[pos]>>4 != 1 || data[pos]&0xF > 3 || 1<<(data[pos]&0xF) > limit-pos-1 {
				return true
			}
			size := uint64(1) << (data[pos] & 0xF)
			count, pos = bplistUint(data[pos+1:], size), pos+1+size
		}
		if data[off]>>4 == 0xD {
			count = min(count, limit) * 2
		}
		if count > (limit-pos)/refSize || count > refs {
			return true
		}
		refs -= count
		for j := range count {
			ref := bplistUint(data[pos+j*refSize:], refSize)
			if ref >= n || seen[ref] {
				return true
			}
			_, seen[ref] = object(ref)
		}
	}
	return false
}

func bplistUint(b []byte, size uint64) uint64 {
	var v [8]byte
	copy(v[8-size:], b[:size])
	return binary.BigEndian.Uint64(v[:])
}
