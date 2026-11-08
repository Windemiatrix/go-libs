package placeinfo

// AsDict is asdict of PlaceInfo5 (placeinfo.py:708-716): name, names (the 19
// lists of Names by the keys of PlaceNames, [] when empty), country_code,
// ishome, address_str and address (the 8 parts of Address by the keys of
// PostalAddress, an object also without a postal address, 622-625). name,
// country_code, address_str and each part of address are nil for "" (None
// upstream). The map and its lists are new on every call.
func (pl PlaceInfo) AsDict() map[string]any {
	n, a := pl.Names, pl.Address
	return map[string]any{
		"name": strOrNil(pl.Name),
		"names": map[string]any{
			"field0": listOf(n.Field0), "country": listOf(n.Country), "state_province": listOf(n.StateProvince),
			"sub_administrative_area": listOf(n.SubAdministrativeArea), "city": listOf(n.City), "field5": listOf(n.Field5),
			"additional_city_info": listOf(n.AdditionalCityInfo), "ocean": listOf(n.Ocean), "area_of_interest": listOf(n.AreaOfInterest),
			"inland_water": listOf(n.InlandWater), "field10": listOf(n.Field10), "region": listOf(n.Region),
			"sub_throughfare": listOf(n.SubThroughfare), "field13": listOf(n.Field13), "postal_code": listOf(n.PostalCode),
			"field15": listOf(n.Field15), "field16": listOf(n.Field16), "street_address": listOf(n.StreetAddress),
			"body_of_water": listOf(n.BodyOfWater),
		},
		"country_code": strOrNil(pl.CountryCode),
		"ishome":       pl.IsHome,
		"address_str":  strOrNil(pl.AddressStr),
		"address": map[string]any{
			"street": strOrNil(a.Street), "sub_locality": strOrNil(a.SubLocality), "city": strOrNil(a.City),
			"sub_administrative_area": strOrNil(a.SubAdministrativeArea), "state_province": strOrNil(a.StateProvince),
			"postal_code": strOrNil(a.PostalCode), "country": strOrNil(a.Country), "iso_country_code": strOrNil(a.ISOCountryCode),
		},
	}
}

// json writes a nil slice as null, where upstream writes the empty list as [].
func listOf(s []string) []string { return append([]string{}, s...) }

// PlaceInfo holds "" where upstream has None, which json writes as null.
func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
