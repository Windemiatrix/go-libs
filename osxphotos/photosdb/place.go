package photosdb

import (
	"math"

	"howett.net/plist"

	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
)

// keyedArchive reads an NSKeyedArchiver archive as bpylist2 unarchives it for
// upstream, but only along the keys its readers walk; the classes are not
// checked.
type keyedArchive struct {
	objects []any
	top     map[string]any
	outside bool // a reference was outside $objects, where bpylist2 raises IndexError
}

func newKeyedArchive(data []byte) *keyedArchive {
	keyed, _ := plistAny(data).(map[string]any)
	a := &keyedArchive{}
	a.objects, _ = keyed["$objects"].([]any)
	a.top, _ = keyed["$top"].(map[string]any)
	return a
}

// NSKeyedArchiver keeps numbers and booleans inline, and bpylist2 returns a
// value that is not a reference as it is.
func (a *keyedArchive) object(ref any) any {
	uid, isUID := ref.(plist.UID)
	if !isUID {
		return ref
	}
	if uint64(uid) >= uint64(len(a.objects)) {
		a.outside = true
		return nil
	}
	if a.objects[uid] == "$null" {
		return nil
	}
	return a.objects[uid]
}

// bpylist2 decodes only a referenced dictionary as an archived object; an
// inline one stays a dict, whose attributes upstream cannot read.
func (a *keyedArchive) dict(ref any) map[string]any {
	if _, isUID := ref.(plist.UID); !isUID {
		return nil
	}
	d, _ := a.object(ref).(map[string]any)
	return d
}

func (a *keyedArchive) text(ref any) string {
	s, _ := a.object(ref).(string)
	return s
}

func (a *keyedArchive) root() map[string]any { return a.dict(a.top["root"]) }

func (a *keyedArchive) array(ref any) ([]any, bool) {
	list, ok := a.dict(ref)["NS.objects"].([]any)
	return list, ok
}

// placeOf decodes the archive of ZREVERSELOCATIONDATA as PlaceInfo5 does
// (placeinfo.py:77-334, 573-686): nil where upstream raises and gives None
// (photoinfo.py:1349-1358). Unlike upstream, an area that is not a number
// gives nil too, where upstream drops the places of that type when it cannot
// sort them, and so do more than maxBinaryObjects places or more than
// maxPlistSize bytes of their names.
func placeOf(archive []byte) *placeinfo.PlaceInfo {
	a := newKeyedArchive(archive)
	root := a.root()
	infos, ok := a.array(a.dict(root["mapItem"])["sortedPlaceInfos"])
	if !ok || len(infos) > maxBinaryObjects {
		return nil
	}
	places := make([]placeinfo.Place, 0, len(infos))
	size := 0 // every reference to a shared place normalizes its name again
	for _, ref := range infos {
		info := a.dict(ref)
		area, isNumber := number(a.object(info["area"]))
		name := a.text(info["name"])
		size += len(name)
		if info == nil || !isNumber || size > maxPlistSize {
			return nil
		}
		places = append(places, placeinfo.Place{Type: placeType(a.object(info["placeType"])), Name: nfc(name), Area: area})
	}
	postal := a.dict(root["postalAddress"])
	part := func(key string) string { return nfc(a.text(postal[key])) }
	address := placeinfo.Address{
		Street: part("_street"), SubLocality: part("_subLocality"), City: part("_city"),
		SubAdministrativeArea: part("_subAdministrativeArea"), StateProvince: part("_state"),
		PostalCode: part("_postalCode"), Country: part("_country"), ISOCountryCode: a.text(postal["_ISOCountryCode"]),
	}
	isHome, _ := a.object(root["isHome"]).(bool)
	p := placeinfo.New(places, a.text(root["countryCode"]), nfc(a.text(root["addressString"])), isHome, address)
	if a.outside {
		return nil
	}
	return &p
}

// -1, a type that placeinfo.New leaves out, for a value that is not an
// integral number: upstream keys a place by its placeType and reads the keys
// 0-17, which 4.0 matches as well as 4 (placeinfo.py:633-653). The bound only
// keeps int(f) defined.
func placeType(v any) int {
	f, ok := number(v)
	if !ok || f != math.Trunc(f) || math.Abs(f) > math.MaxInt32 {
		return -1
	}
	return int(f)
}

// howett.net/plist decodes an integer as int64 or uint64 and a real of four
// bytes as float32.
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	}
	return 0, false
}
