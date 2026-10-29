package placeinfo_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/placeinfo"
)

func TestNew(t *testing.T) {
	p := func(typ int, name string, area float64) placeinfo.Place {
		return placeinfo.Place{Type: typ, Name: name, Area: area}
	}
	tests := []struct {
		name   string
		places []placeinfo.Place
		want   string
		names  placeinfo.Names
	}{
		{
			name: "area of interest, city, state and country",
			places: []placeinfo.Place{
				p(1, "Exampleland", 4), p(2, "Example State", 3), p(4, "Townsville", 2), p(8, "Example Park", 1),
			},
			want: "Example Park, Townsville, Example State, Exampleland",
			names: placeinfo.Names{
				Country: []string{"Exampleland"}, StateProvince: []string{"Example State"},
				City: []string{"Townsville"}, AreaOfInterest: []string{"Example Park"},
			},
		},
		{
			name:   "no area of interest and no region",
			places: []placeinfo.Place{p(4, "Townsville", 1), p(2, "Example State", 2), p(1, "Exampleland", 3)},
			want:   "Townsville, Example State, Exampleland",
			names: placeinfo.Names{
				Country: []string{"Exampleland"}, StateProvince: []string{"Example State"}, City: []string{"Townsville"},
			},
		},
		{
			name:   "region comes before the city",
			places: []placeinfo.Place{p(4, "Townsville", 1), p(11, "Example Island", 2)},
			want:   "Example Island, Townsville",
			names:  placeinfo.Names{City: []string{"Townsville"}, Region: []string{"Example Island"}},
		},
		{
			name:   "only an ocean",
			places: []placeinfo.Place{p(7, "Example Sea", 1)},
			want:   "Example Sea",
			names:  placeinfo.Names{Ocean: []string{"Example Sea"}, BodyOfWater: []string{"Example Sea"}},
		},
		{
			name:   "inland water before the ocean in the name, the ocean first in the body of water",
			places: []placeinfo.Place{p(9, "Example Lake", 1), p(7, "Example Sea", 2), p(1, "Exampleland", 3)},
			want:   "Exampleland, Example Lake, Example Sea",
			names: placeinfo.Names{
				Country: []string{"Exampleland"}, Ocean: []string{"Example Sea"}, InlandWater: []string{"Example Lake"},
				BodyOfWater: []string{"Example Sea", "Example Lake"},
			},
		},
		{
			name:   "places of one type by ascending area",
			places: []placeinfo.Place{p(4, "Big Town", 10), p(4, "Small Town", 5)},
			want:   "Small Town",
			names:  placeinfo.Names{City: []string{"Small Town", "Big Town"}},
		},
		{
			name:   "equal areas keep the order of the archive",
			places: []placeinfo.Place{p(4, "Town C", 5), p(4, "Town A", 5), p(4, "Town B", 5)},
			want:   "Town C",
			names:  placeinfo.Names{City: []string{"Town C", "Town A", "Town B"}},
		},
		{
			name:   "a type outside 0-17 is left out",
			places: []placeinfo.Place{p(18, "Example Sea", 1), p(-1, "Example Lake", 1), p(1, "Exampleland", 1)},
			want:   "Exampleland",
			names:  placeinfo.Names{Country: []string{"Exampleland"}},
		},
		{
			name:   "an empty first name skips the kind",
			places: []placeinfo.Place{p(8, "", 1), p(8, "Example Park", 2), p(4, "Townsville", 3)},
			want:   "Townsville",
			names:  placeinfo.Names{AreaOfInterest: []string{"", "Example Park"}, City: []string{"Townsville"}},
		},
		{
			name:   "only empty names",
			places: []placeinfo.Place{p(1, "", 1)},
			want:   "",
			names:  placeinfo.Names{Country: []string{""}},
		},
		{
			name: "every type fills its field",
			places: []placeinfo.Place{
				p(0, "n0", 1), p(1, "n1", 1), p(2, "n2", 1), p(3, "n3", 1), p(4, "n4", 1), p(5, "n5", 1),
				p(6, "n6", 1), p(7, "n7", 1), p(8, "n8", 1), p(9, "n9", 1), p(10, "n10", 1), p(11, "n11", 1),
				p(12, "n12", 1), p(13, "n13", 1), p(14, "n14", 1), p(15, "n15", 1), p(16, "n16", 1), p(17, "n17", 1),
			},
			want: "n8, n11, n4, n2, n1, n9, n7",
			names: placeinfo.Names{
				Field0: []string{"n0"}, Country: []string{"n1"}, StateProvince: []string{"n2"},
				SubAdministrativeArea: []string{"n3"}, City: []string{"n4"}, Field5: []string{"n5"},
				AdditionalCityInfo: []string{"n6"}, Ocean: []string{"n7"}, AreaOfInterest: []string{"n8"},
				InlandWater: []string{"n9"}, Field10: []string{"n10"}, Region: []string{"n11"},
				SubThroughfare: []string{"n12"}, Field13: []string{"n13"}, PostalCode: []string{"n14"},
				Field15: []string{"n15"}, Field16: []string{"n16"}, StreetAddress: []string{"n17"},
				BodyOfWater: []string{"n7", "n9"},
			},
		},
		{name: "no places", places: nil, want: "", names: placeinfo.Names{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := placeinfo.New(tt.places, "", "", false, placeinfo.Address{})
			if got.Name != tt.want {
				t.Errorf("Name = %q, want %q", got.Name, tt.want)
			}
			if !reflect.DeepEqual(got.Names, tt.names) {
				t.Errorf("Names = %+v, want %+v", got.Names, tt.names)
			}
		})
	}
}

func TestNewKeepsTheOrderOfEqualAreas(t *testing.T) {
	// slices.SortFunc leaves a slice already in order as it is and sorts up to 12
	// places by insertion, both stable; interleaved areas over 40 places make it
	// partition, which reorders equal areas.
	var places []placeinfo.Place
	var small, big []string
	for i := range 40 {
		name := fmt.Sprintf("Town %02d", i)
		places = append(places, placeinfo.Place{Type: 4, Name: name, Area: float64(1 + i%2)})
		if i%2 == 0 {
			small = append(small, name)
		} else {
			big = append(big, name)
		}
	}
	want := slices.Concat(small, big)
	got := placeinfo.New(places, "", "", false, placeinfo.Address{})
	if !reflect.DeepEqual(got.Names.City, want) {
		t.Errorf("City = %v, want %v", got.Names.City, want)
	}
}

func TestNewDoesNotChangeItsInput(t *testing.T) {
	places := []placeinfo.Place{{Type: 4, Name: "Big Town", Area: 10}, {Type: 4, Name: "Small Town", Area: 5}}
	placeinfo.New(places, "", "", false, placeinfo.Address{})
	if places[0].Name != "Big Town" || places[1].Name != "Small Town" {
		t.Errorf("places = %v, want the order of the call", places)
	}
}

func TestNewBodyOfWaterIsNotShared(t *testing.T) {
	got := placeinfo.New([]placeinfo.Place{{Type: 7, Name: "Example Sea", Area: 1}}, "", "", false, placeinfo.Address{})
	got.Names.BodyOfWater[0] = "changed"
	if got.Names.Ocean[0] != "Example Sea" {
		t.Errorf("Ocean = %v, want it apart from BodyOfWater", got.Names.Ocean)
	}
}

func TestNewCarriesTheRest(t *testing.T) {
	address := placeinfo.Address{
		Street: "1 Example Street", SubLocality: "Example District", City: "Townsville",
		SubAdministrativeArea: "Example County", StateProvince: "Example State", PostalCode: "12345",
		Country: "Exampleland", ISOCountryCode: "EX",
	}
	got := placeinfo.New(nil, "EX", "1 Example Street, Townsville", true, address)
	want := placeinfo.PlaceInfo{
		CountryCode: "EX", IsHome: true, AddressStr: "1 Example Street, Townsville", Address: address,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("New = %+v, want %+v", got, want)
	}
}
