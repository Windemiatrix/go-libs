package placeinfo

import (
	"cmp"
	"slices"
	"strings"
)

// PlaceInfo is the reverse geolocation of a photo as Photos stored it, the
// PlaceInfo of osxphotos for Photos 5 and later (placeinfo.py:573-716): what
// the archive of ZADDITIONALASSETATTRIBUTES.ZREVERSELOCATIONDATA holds. Name is
// the name Photos shows, built from Names as _process_place_info does
// (629-686); "" when no part of it is known. A string is "" where upstream has
// None. Address is zero when the archive has no postal address (622-625).
type PlaceInfo struct {
	Name        string
	Names       Names
	CountryCode string // countryCode, such as "GB" (589-591), not normalized
	IsHome      bool   // isHome (594-596)
	AddressStr  string // addressString (584-586)
	Address     Address
}

// Names are the place names by kind, each a list in ascending order of area,
// the PlaceNames of osxphotos (placeinfo.py:51-74): the field of index k holds
// the names of the places of placeType k of sortedPlaceInfos (643-653), and
// BodyOfWater is Ocean followed by InlandWater (655-656). The fields named
// FieldN are kinds upstream leaves unnamed and reports as unused (50). Empty is
// no place of that kind.
type Names struct {
	Field0, Country, StateProvince, SubAdministrativeArea, City, Field5 []string

	AdditionalCityInfo, Ocean, AreaOfInterest, InlandWater, Field10, Region []string

	SubThroughfare, Field13, PostalCode, Field15, Field16, StreetAddress []string

	BodyOfWater []string
}

// Address is the postal address of the place, the PostalAddress of osxphotos
// (placeinfo.py:30-42, 609-627); ISOCountryCode is not normalized, the other
// parts are in NFC (251-257).
type Address struct {
	Street, SubLocality, City, SubAdministrativeArea, StateProvince, PostalCode, Country, ISOCountryCode string
}

// Place is one PLRevGeoMapItemAdditionalPlaceInfo of sortedPlaceInfos
// (placeinfo.py:197-233): Type is placeType, Area the area it covers.
type Place struct {
	Type int
	Name string
	Area float64
}

// New builds Names and Name from places as _process_place_info does
// (placeinfo.py:629-686). It does not modify places and does not normalize the
// strings: upstream brings them to NFC when it decodes the archive (95-100,
// 202, 251-257). Without places Name is "" and Names is zero. Equal Area keeps
// the order of places, as Python sorted does (649); a Type outside 0-17 is left
// out (643).
func New(places []Place, countryCode, addressStr string, isHome bool, address Address) PlaceInfo {
	sorted := slices.Clone(places)
	slices.SortStableFunc(sorted, func(a, b Place) int { return cmp.Compare(a.Area, b.Area) })

	var byType [18][]string
	for _, p := range sorted {
		if p.Type >= 0 && p.Type < len(byType) {
			byType[p.Type] = append(byType[p.Type], p.Name)
		}
	}

	names := Names{
		byType[0], byType[1], byType[2], byType[3], byType[4], byType[5],
		byType[6], byType[7], byType[8], byType[9], byType[10], byType[11],
		byType[12], byType[13], byType[14], byType[15], byType[16], byType[17],
		slices.Concat(byType[7], byType[9]),
	}

	var parts []string
	for _, list := range [][]string{
		names.AreaOfInterest, names.Region, names.City, names.StateProvince,
		names.Country, names.InlandWater, names.Ocean,
	} {
		if len(list) > 0 && list[0] != "" {
			parts = append(parts, list[0])
		}
	}

	return PlaceInfo{
		Name:        strings.Join(parts, ", "),
		Names:       names,
		CountryCode: countryCode,
		IsHome:      isHome,
		AddressStr:  addressStr,
		Address:     address,
	}
}
