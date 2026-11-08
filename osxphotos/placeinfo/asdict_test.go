package placeinfo_test

import (
	"reflect"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
)

// A distinct value in each list, so that two swapped keys fail.
func filledPlace() placeinfo.PlaceInfo {
	return placeinfo.PlaceInfo{
		Name: "Townsville",
		Names: placeinfo.Names{
			Field0: []string{"n0"}, Country: []string{"n1"}, StateProvince: []string{"n2"},
			SubAdministrativeArea: []string{"n3"}, City: []string{"n4"}, Field5: []string{"n5"},
			AdditionalCityInfo: []string{"n6"}, Ocean: []string{"n7"}, AreaOfInterest: []string{"n8"},
			InlandWater: []string{"n9"}, Field10: []string{"n10"}, Region: []string{"n11"},
			SubThroughfare: []string{"n12"}, Field13: []string{"n13"}, PostalCode: []string{"n14"},
			Field15: []string{"n15"}, Field16: []string{"n16"}, StreetAddress: []string{"n17"},
			BodyOfWater: []string{"n18"},
		},
		CountryCode: "EX", IsHome: true, AddressStr: "1 Example Street, Townsville",
		Address: placeinfo.Address{
			Street: "1 Example Street", SubLocality: "Example District", City: "Townsville",
			SubAdministrativeArea: "Example County", StateProvince: "Example State", PostalCode: "12345",
			Country: "Exampleland", ISOCountryCode: "EX",
		},
	}
}

func TestAsDict(t *testing.T) {
	empty := []string{}

	tests := []struct {
		name string
		pl   placeinfo.PlaceInfo
		want map[string]any
	}{
		{
			name: "filled",
			pl:   filledPlace(),
			want: map[string]any{
				"name": "Townsville",
				"names": map[string]any{
					"field0": []string{"n0"}, "country": []string{"n1"}, "state_province": []string{"n2"},
					"sub_administrative_area": []string{"n3"}, "city": []string{"n4"}, "field5": []string{"n5"},
					"additional_city_info": []string{"n6"}, "ocean": []string{"n7"}, "area_of_interest": []string{"n8"},
					"inland_water": []string{"n9"}, "field10": []string{"n10"}, "region": []string{"n11"},
					"sub_throughfare": []string{"n12"}, "field13": []string{"n13"}, "postal_code": []string{"n14"},
					"field15": []string{"n15"}, "field16": []string{"n16"}, "street_address": []string{"n17"},
					"body_of_water": []string{"n18"},
				},
				"country_code": "EX", "ishome": true, "address_str": "1 Example Street, Townsville",
				"address": map[string]any{
					"street": "1 Example Street", "sub_locality": "Example District", "city": "Townsville",
					"sub_administrative_area": "Example County", "state_province": "Example State", "postal_code": "12345",
					"country": "Exampleland", "iso_country_code": "EX",
				},
			},
		},
		{
			name: "zero: every list is [], address an object, every string nil",
			pl:   placeinfo.PlaceInfo{},
			want: map[string]any{
				"name": nil,
				"names": map[string]any{
					"field0": empty, "country": empty, "state_province": empty, "sub_administrative_area": empty,
					"city": empty, "field5": empty, "additional_city_info": empty, "ocean": empty,
					"area_of_interest": empty, "inland_water": empty, "field10": empty, "region": empty,
					"sub_throughfare": empty, "field13": empty, "postal_code": empty, "field15": empty,
					"field16": empty, "street_address": empty, "body_of_water": empty,
				},
				"country_code": nil, "ishome": false, "address_str": nil,
				"address": map[string]any{
					"street": nil, "sub_locality": nil, "city": nil, "sub_administrative_area": nil,
					"state_province": nil, "postal_code": nil, "country": nil, "iso_country_code": nil,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pl.AsDict(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AsDict =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

func TestAsDictIsNotShared(t *testing.T) {
	pl := filledPlace()
	for _, list := range pl.AsDict()["names"].(map[string]any) {
		list.([]string)[0] = "changed"
	}
	if want := filledPlace().Names; !reflect.DeepEqual(pl.Names, want) {
		t.Errorf("Names = %+v, want %+v, apart from the map of AsDict", pl.Names, want)
	}
}
