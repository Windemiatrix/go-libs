package photosdb

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
)

// placeObjects is the $objects of a PLRevGeoLocationInfo archive with the keys
// that PlaceInfo5 reads (placeinfo.py:80-325), without $class, which placeOf
// does not check. The root is $objects[1]. A property list decodes a
// non-negative integer as uint64, a negative one as int64 and a real of four
// bytes as float32, so the areas take each of these types. Every string has a
// letter that NFD decomposes and is stored in NFD, so that a field that is not
// normalized differs from examplePlace.
func placeObjects() []any {
	root := map[string]any{"isHome": true}
	objects := []any{"$null", root}
	ref := func(v any) plist.UID {
		if s, ok := v.(string); ok {
			v = norm.NFD.String(s)
		}
		objects = append(objects, v)
		return plist.UID(len(objects) - 1)
	}
	place := func(typ int, name string, area any) plist.UID {
		return ref(map[string]any{"placeType": typ, "name": ref(name), "area": area})
	}
	infos := []any{
		place(4, "Townsvill\U000000e9", 100), place(8, "Example Caf\U000000e9", 0.5), place(4, "Old T\U000000f6wn", int64(-5)),
		place(2, "Example St\U000000e2te", uint64(5000)), place(1, "Exampl\U000000e9land", float32(90000)), place(9, "Example L\U000000e1ke", 20.0),
	}
	root["mapItem"] = ref(map[string]any{"sortedPlaceInfos": ref(map[string]any{"NS.objects": infos})})
	root["addressString"] = ref("1 Example Street, Caf\U000000e9 Town")
	root["countryCode"] = ref("\U000000c9X")
	root["postalAddress"] = ref(map[string]any{
		"_street": ref("1 Exampl\U000000e9 Street"), "_subLocality": ref("Old T\U000000f6wn"), "_city": ref("Caf\U000000e9 Town"),
		"_subAdministrativeArea": ref("Example C\U000000f6unty"), "_state": ref("Example St\U000000e2te"), "_postalCode": ref("\U000000c9X1 2AB"),
		"_country": ref("Exampl\U000000e9land"), "_ISOCountryCode": ref("\U000000c9X"),
	})
	return objects
}

// The country codes stay in NFD (placeinfo.py:96, 250); the other strings are
// in NFC.
var examplePlace = placeinfo.PlaceInfo{
	Name: "Example Caf\U000000e9, Old T\U000000f6wn, Example St\U000000e2te, Exampl\U000000e9land, Example L\U000000e1ke",
	Names: placeinfo.Names{
		Country: []string{"Exampl\U000000e9land"}, StateProvince: []string{"Example St\U000000e2te"}, City: []string{"Old T\U000000f6wn", "Townsvill\U000000e9"},
		AreaOfInterest: []string{"Example Caf\U000000e9"}, InlandWater: []string{"Example L\U000000e1ke"}, BodyOfWater: []string{"Example L\U000000e1ke"},
	},
	CountryCode: "E\U00000301X",
	IsHome:      true,
	AddressStr:  "1 Example Street, Caf\U000000e9 Town",
	Address: placeinfo.Address{
		Street: "1 Exampl\U000000e9 Street", SubLocality: "Old T\U000000f6wn", City: "Caf\U000000e9 Town", SubAdministrativeArea: "Example C\U000000f6unty",
		StateProvince: "Example St\U000000e2te", PostalCode: "\U000000c9X1 2AB", Country: "Exampl\U000000e9land", ISOCountryCode: "E\U00000301X",
	},
}

func keyedArchiveOf(t testing.TB, objects []any) []byte {
	t.Helper()
	b, err := plist.Marshal(map[string]any{
		"$archiver": "NSKeyedArchiver", "$version": 100000,
		"$top": map[string]any{"root": plist.UID(1)}, "$objects": objects,
	}, plist.BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Upstream gives None where PlaceInfo5 raises (photoinfo.py:1349-1358).
func TestPlaceOf(t *testing.T) {
	t.Parallel()

	dict := func(o []any, ref any) map[string]any { return o[ref.(plist.UID)].(map[string]any) }
	root := func(o []any) map[string]any { return o[1].(map[string]any) }
	mapItem := func(o []any) map[string]any { return dict(o, root(o)["mapItem"]) }
	infos := func(o []any) map[string]any { return dict(o, mapItem(o)["sortedPlaceInfos"]) }
	info := func(o []any, i int) map[string]any { return dict(o, infos(o)["NS.objects"].([]any)[i]) }
	const townsville, oldTown = 0, 2
	repeat := func(o []any, n int) {
		infos(o)["NS.objects"] = slices.Repeat(infos(o)["NS.objects"].([]any)[townsville:townsville+1], n)
	}

	without := func(edit func(*placeinfo.PlaceInfo)) *placeinfo.PlaceInfo {
		p := examplePlace
		edit(&p)
		return &p
	}
	noAddress := without(func(p *placeinfo.PlaceInfo) { p.Address = placeinfo.Address{} })
	noTownsville := without(func(p *placeinfo.PlaceInfo) { p.Names.City = []string{"Old T\U000000f6wn"} })
	noOldTownName := without(func(p *placeinfo.PlaceInfo) {
		p.Names.City = []string{"", "Townsvill\U000000e9"}
		p.Name = "Example Caf\U000000e9, Example St\U000000e2te, Exampl\U000000e9land, Example L\U000000e1ke"
	})

	for _, tt := range []struct {
		name string
		edit func(o []any)
		want *placeinfo.PlaceInfo
	}{
		{"every field", func([]any) {}, &examplePlace},
		{"postalAddress $null", func(o []any) { root(o)["postalAddress"] = plist.UID(0) }, noAddress},
		{"no postalAddress", func(o []any) { delete(root(o), "postalAddress") }, noAddress},
		{"an inline postalAddress", func(o []any) { root(o)["postalAddress"] = dict(o, root(o)["postalAddress"]) }, noAddress},
		{"no isHome", func(o []any) { delete(root(o), "isHome") }, without(func(p *placeinfo.PlaceInfo) { p.IsHome = false })},
		// _process_place_info keys a place by its placeType and reads the keys 0-17 (placeinfo.py:633-653).
		{"a placeType that is the real 4.0", func(o []any) { info(o, townsville)["placeType"] = 4.0 }, &examplePlace},
		{"a placeType that is 4.5", func(o []any) { info(o, townsville)["placeType"] = 4.5 }, noTownsville},
		{"a placeType of -4", func(o []any) { info(o, townsville)["placeType"] = int64(-4) }, noTownsville},
		{"a placeType of 99", func(o []any) { info(o, townsville)["placeType"] = 99 }, noTownsville},
		{"a placeType that is a string", func(o []any) { info(o, townsville)["placeType"] = "4" }, noTownsville},
		{"no placeType", func(o []any) { delete(info(o, townsville), "placeType") }, noTownsville},
		// normalize_unicode keeps None, which the name of Photos skips (unicode.py:79-80, placeinfo.py:683).
		{"a name that is $null", func(o []any) { info(o, oldTown)["name"] = plist.UID(0) }, noOldTownName},
		{"no name", func(o []any) { delete(info(o, oldTown), "name") }, noOldTownName},
		{"no places", func(o []any) { infos(o)["NS.objects"] = []any{} }, without(func(p *placeinfo.PlaceInfo) { p.Name, p.Names = "", placeinfo.Names{} })},
		{"4000 references to one place", func(o []any) { repeat(o, 4000) }, &placeinfo.PlaceInfo{
			Name: "Townsvill\U000000e9", Names: placeinfo.Names{City: slices.Repeat([]string{"Townsvill\U000000e9"}, 4000)}, CountryCode: examplePlace.CountryCode,
			IsHome: true, AddressStr: examplePlace.AddressStr, Address: examplePlace.Address,
		}},
		// bpylist2 indexes $objects and raises IndexError.
		{"a reference at len($objects)", func(o []any) { root(o)["countryCode"] = plist.UID(len(o)) }, nil},
		{"a reference far outside $objects", func(o []any) { info(o, oldTown)["name"] = plist.UID(9999) }, nil},
		{"a mapItem that is the root", func(o []any) { root(o)["mapItem"] = plist.UID(1) }, nil},
		{"no mapItem", func(o []any) { delete(root(o), "mapItem") }, nil},
		{"sortedPlaceInfos that is not an array", func(o []any) { mapItem(o)["sortedPlaceInfos"] = "places" }, nil},
		{"a place that is $null", func(o []any) { infos(o)["NS.objects"].([]any)[oldTown] = plist.UID(0) }, nil},
		// An inline dictionary stays a dict for bpylist2, without the attributes upstream reads.
		{"an inline place", func(o []any) { infos(o)["NS.objects"].([]any)[oldTown] = info(o, oldTown) }, nil},
		{"an area that is not a number", func(o []any) { info(o, oldTown)["area"] = "large" }, nil},
		{"a root that is not a dictionary", func(o []any) { o[1] = "root" }, nil},
	} {
		objects := placeObjects()
		tt.edit(objects)
		if got := placeOf(keyedArchiveOf(t, objects)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: %+v, want %+v", tt.name, got, tt.want)
		}
	}

	// These archives decode, so only the bounds of placeOf refuse them. XML,
	// because a binary property list of 4097 references is refused by its
	// object limit before placeOf.
	for name, edit := range map[string]func(o []any){
		"4097 references to one place": func(o []any) { repeat(o, maxBinaryObjects+1) },
		"names over 1 MiB": func(o []any) {
			o[info(o, townsville)["name"].(plist.UID)] = strings.Repeat("a", 400_000)
			repeat(o, 3)
		},
	} {
		objects := placeObjects()
		edit(objects)
		archive := marshal(t, map[string]any{
			"$archiver": "NSKeyedArchiver", "$version": 100000,
			"$top": map[string]any{"root": plist.UID(1)}, "$objects": objects,
		}, plist.XMLFormat)
		if plistAny(archive) == nil || placeOf(archive) != nil {
			t.Errorf("%s: the archive does not decode, or gives a place", name)
		}
	}

	o := placeObjects()
	inlineRoot := marshal(t, map[string]any{"$top": map[string]any{"root": o[1]}, "$objects": o}, plist.BinaryFormat)
	for _, archive := range [][]byte{nil, {}, []byte("not a list"), {0}, inlineRoot} {
		if got := placeOf(archive); got != nil {
			t.Errorf("%q: %+v, want nil", archive, got)
		}
	}
}

func BenchmarkPlaceOf(b *testing.B) {
	db := testlib.OpenDB(b, filepath.Join(testlib.OpenPristine(b, "Test-26.1"), "database", "Photos.sqlite"))
	rows := testlib.Rows(b, db, "SELECT ZREVERSELOCATIONDATA FROM ZADDITIONALASSETATTRIBUTES WHERE ZREVERSELOCATIONDATA IS NOT NULL")
	library := make([][]byte, len(rows))
	for i, r := range rows {
		library[i] = []byte(r)
	}
	for name, archives := range map[string][][]byte{"Test-26.1": library, "placeObjects": {keyedArchiveOf(b, placeObjects())}} {
		b.Run(name, func(b *testing.B) {
			if len(archives) == 0 {
				b.Skip("no archive")
			}
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				placeOf(archives[i%len(archives)])
			}
		})
	}
}
