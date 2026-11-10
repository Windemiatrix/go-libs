package searchinfo_test

import (
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/searchinfo"
)

func rec(category int, content string) searchinfo.Record {
	return searchinfo.Record{Category: category, Content: content, Normalized: strings.ToLower(content)}
}

// The records are in an order a sort changes: Tree before Beach, Loc Z before
// Loc B, and the category 7 term before the category 4 terms.
func records() []searchinfo.Record {
	return []searchinfo.Record{
		rec(searchinfo.CategoryLabel, "Tree"),
		rec(searchinfo.CategoryLabel, "Beach"),
		rec(searchinfo.CategoryCity, "Townsville"),
		rec(searchinfo.CategoryCity, "Another City"),
		rec(searchinfo.CategoryNamedArea, "Area A"),
		rec(searchinfo.CategoryLocality4, "Loc Z"),
		rec(searchinfo.CategoryLocality4, "Loc B"),
		rec(searchinfo.CategoryIDDocumentType, "Passport"),
		rec(searchinfo.CategoryIDDocumentCardType, "Passport"),
		rec(searchinfo.CategoryIDDocumentCardType, "Driver License"),
		rec(searchinfo.CategoryHoliday, "Holiday"),
		rec(searchinfo.CategoryPhotoTypeFavorites, "Favorite"),
		rec(searchinfo.CategoryPhotoTypeVideo, "Video"),
		rec(searchinfo.CategoryWeekpart, "Weekend"),
		rec(searchinfo.CategorySeason, "Summer"),
		rec(searchinfo.CategoryCamera, "Example Camera"),
		rec(searchinfo.CategorySource, "Example App"),
		rec(searchinfo.CategoryTextFound, "Text"),
		rec(99999, "Other"),
	}
}

func isEmpty(v any) bool {
	switch v := v.(type) {
	case string:
		return v == ""
	case []string:
		return v == nil
	}
	return false
}

func TestSearchInfo(t *testing.T) {
	s := searchinfo.New(records())
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"labels sorted", s.Labels(), []string{"Beach", "Tree"}},
		{"city is the first of the sorted", s.City(), "Another City"},
		{"locality names category after category", s.LocalityNames(), []string{"Loc B", "Loc Z", "Area A"}},
		{"media types in the order of the list", s.MediaTypes(), []string{"Video", "Favorite"}},
		{"id document types are a sorted set", s.IDDocumentTypes(), []string{"Driver License", "Passport"}},
		{"holidays", s.Holidays(), []string{"Holiday"}},
		{"text found", s.TextFound(), []string{"Text"}},
		{"week part", s.WeekPart(), "Weekend"},
		{"source", s.Source(), "Example App"},
		{"no state", s.State(), ""},
		{"no pets", s.Pets(), []string(nil)},
		{"no id document names", s.IDDocumentNames(), []string(nil)},
		{"normalized labels", s.Normalized().Labels(), []string{"beach", "tree"}},
		{"normalized city", s.Normalized().City(), "another city"},
		{"normalized id document types", s.Normalized().IDDocumentTypes(), []string{"driver license", "passport"}},
		{
			"all: lists in turn, then the non-empty single values", s.All(),
			[]string{
				"Beach", "Tree", "Loc B", "Loc Z", "Area A", "Holiday", "Video", "Favorite",
				"Another City", "Summer", "Example Camera", "Weekend",
			},
		},
		{
			"normalized all", s.Normalized().All(),
			[]string{
				"beach", "tree", "loc b", "loc z", "area a", "holiday", "video", "favorite",
				"another city", "summer", "example camera", "weekend",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("got %#v, want %#v", tt.got, tt.want)
			}
		})
	}

	t.Run("Normalized does not change the receiver", func(t *testing.T) {
		n := s.Normalized()
		if got, want := s.Labels(), []string{"Beach", "Tree"}; !slices.Equal(got, want) {
			t.Errorf("receiver Labels() = %#v, want %#v", got, want)
		}
		if got, want := n.Labels(), []string{"beach", "tree"}; !slices.Equal(got, want) {
			t.Errorf("Normalized().Labels() = %#v, want %#v", got, want)
		}
	})
}

func TestSortSemantics(t *testing.T) {
	s := searchinfo.New([]searchinfo.Record{
		rec(1500, "apple"), rec(1500, "Zebra"), rec(1500, "Tree"), rec(1500, "Tree"),
	})
	if got, want := s.Labels(), []string{"Tree", "Tree", "Zebra", "apple"}; !slices.Equal(got, want) {
		t.Errorf("Labels() = %#v, want %#v", got, want)
	}
	if got, want := s.Normalized().Labels(), []string{"apple", "tree", "tree", "zebra"}; !slices.Equal(got, want) {
		t.Errorf("Normalized().Labels() = %#v, want %#v", got, want)
	}
}

func TestLocalityNamesOrder(t *testing.T) {
	var rs []searchinfo.Record
	for _, c := range []int{7, 8, 6, 5, 4} {
		rs = append(rs, rec(c, strconv.Itoa(c)))
	}
	if got, want := searchinfo.New(rs).LocalityNames(), []string{"4", "6", "8", "7"}; !slices.Equal(got, want) {
		t.Errorf("LocalityNames() = %#v, want %#v", got, want)
	}
}

func TestMediaTypesOrder(t *testing.T) {
	var rs []searchinfo.Record
	for _, c := range []int{
		2000, 1919, 1918, 1916, 1915, 1914, 1913, 1912, 1911, 1910, 1909, 1908, 1907, 1906, 1905, 1903, 1902, 1901,
	} {
		rs = append(rs, rec(c, strconv.Itoa(c)))
	}
	want := []string{
		"1901", "1905", "1906", "1907", "1916", "1910", "1908", "1909", "1913",
		"1914", "1915", "2000", "1912", "1903", "1911", "1918", "1919",
	}
	if got := searchinfo.New(rs).MediaTypes(); !slices.Equal(got, want) {
		t.Errorf("MediaTypes() = %#v, want %#v", got, want)
	}
}

type categoryRow struct {
	key        string
	categories []int
	list       bool
}

// The numbers are those of SearchCategory_Photos8 (_constants.py:530-629), not
// the package constants, so a wrong constant fails here.
var categoryRows = []categoryRow{
	{"labels", []int{1500}, true},
	{"place_names", []int{1}, true},
	{"streets", []int{2}, true},
	{"neighborhoods", []int{3}, true},
	{"city", []int{5}, false},
	{"locality_names", []int{4, 6, 8, 7}, true},
	{"state", []int{10}, false},
	{"state_abbreviation", []int{11}, false},
	{"country", []int{12}, false},
	{"bodies_of_water", []int{14}, true},
	{"month", []int{1100}, false},
	{"year", []int{1101}, false},
	{"holidays", []int{1103}, true},
	{"activities", []int{1600}, true},
	{"season", []int{1104}, false},
	{"venues", []int{1700}, true},
	{"venue_types", []int{1701}, true},
	{
		"media_types",
		[]int{1901, 1905, 1906, 1907, 1916, 1910, 1908, 1909, 1913, 1914, 1915, 2000, 1912, 1903, 1911, 1918, 1919},
		true,
	},
	{"detected_text", []int{1203}, true},
	{"camera", []int{2300}, false},
	{"source", []int{2200}, false},
	{"times_of_day", []int{1106}, true},
	{"week_part", []int{1107}, false},
	{"day_of_week", []int{1070}, false},
	{"home", []int{1000}, false},
	{"areas_of_interest", []int{9}, true},
	{"country_code", []int{13}, false},
	{"region", []int{1008}, false},
	{"continent", []int{1007}, false},
	{"events", []int{1800}, true},
	{"event_performers", []int{1801}, true},
	{"event_types", []int{1802}, true},
	{"pets", []int{1330}, true},
	{"age_groups", []int{3030}, true},
	{"landmarks", []int{1520}, true},
	{"sounds", []int{2500}, true},
	{"human_actions", []int{2600}, true},
	{"document_types", []int{2800}, true},
	{"trip", []int{1610}, false},
	{"photographic_style", []int{5200}, false},
	{"file_type", []int{8051}, false},
	{"captured_by_me", []int{8170}, false},
	{"id_document_types", []int{4130, 11000}, true},
	{"id_document_names", []int{11010}, true},
}

func TestCategoryOfEachKey(t *testing.T) {
	if len(categoryRows) != 44 {
		t.Fatalf("the table lists %d keys, upstream has 44", len(categoryRows))
	}
	for _, row := range categoryRows {
		for _, c := range row.categories {
			t.Run(row.key+"/"+strconv.Itoa(c), func(t *testing.T) {
				d := searchinfo.New([]searchinfo.Record{{Category: c, Content: row.key, Normalized: row.key}}).AsDict()
				var want any = row.key
				if row.list {
					want = []string{row.key}
				}
				if got := d[row.key]; !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %#v, want %#v", row.key, got, want)
				}
				for k, v := range d {
					if k != row.key && !isEmpty(v) {
						t.Errorf("%s = %#v, want empty", k, v)
					}
				}
			})
		}
	}
}

func TestUnreadCategoryNumbers(t *testing.T) {
	tests := []struct {
		name      string
		got, want int
	}{
		{"CategoryWork", searchinfo.CategoryWork, 1001},
		{"CategoryKeywords", searchinfo.CategoryKeywords, 1200},
		{"CategoryTitle", searchinfo.CategoryTitle, 1201},
		{"CategoryDescription", searchinfo.CategoryDescription, 1202},
		{"CategoryPerson", searchinfo.CategoryPerson, 1300},
		{"CategoryPhotoName", searchinfo.CategoryPhotoName, 2100},
		{"CategoryPhotoTypeRaw", searchinfo.CategoryPhotoTypeRaw, 1902},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %d, want %d", tt.got, tt.want)
			}
		})
	}

	t.Run("TextFound reads 1205", func(t *testing.T) {
		got := searchinfo.New([]searchinfo.Record{rec(1205, "Text")}).TextFound()
		if want := []string{"Text"}; !slices.Equal(got, want) {
			t.Errorf("TextFound() = %#v, want %#v", got, want)
		}
	})
}

func TestAllOrder(t *testing.T) {
	var rs []searchinfo.Record
	for _, row := range categoryRows {
		for _, c := range row.categories {
			rs = append(rs, rec(c, row.key))
		}
	}
	rs = append(rs, rec(searchinfo.CategoryTextFound, "text_found"))
	s := searchinfo.New(rs)

	want := slices.Concat(
		[]string{"labels", "place_names", "streets", "neighborhoods"},
		slices.Repeat([]string{"locality_names"}, 4),
		[]string{"bodies_of_water", "holidays", "activities", "venues", "venue_types"},
		slices.Repeat([]string{"media_types"}, 17),
		[]string{
			"detected_text", "times_of_day", "areas_of_interest", "events", "event_performers", "event_types", "pets",
			"age_groups", "landmarks", "sounds", "human_actions", "document_types",
		},
		[]string{
			"city", "state", "state_abbreviation", "country", "month", "year", "season", "camera", "week_part",
			"day_of_week", "home", "country_code", "region", "continent", "trip", "photographic_style", "file_type",
			"captured_by_me",
		},
	)
	if got := s.All(); !slices.Equal(got, want) {
		t.Errorf("All() = %#v, want %#v", got, want)
	}
	if got := s.Normalized().All(); !slices.Equal(got, want) {
		t.Errorf("Normalized().All() = %#v, want %#v", got, want)
	}
}

var wantKeys = []string{
	"labels", "place_names", "streets", "neighborhoods", "city", "locality_names", "state", "state_abbreviation",
	"country", "bodies_of_water", "month", "year", "holidays", "activities", "season", "venues", "venue_types",
	"media_types", "detected_text", "camera", "source", "times_of_day", "week_part", "day_of_week", "home",
	"areas_of_interest", "country_code", "region", "continent", "events", "event_performers", "event_types", "pets",
	"age_groups", "landmarks", "sounds", "human_actions", "document_types", "trip", "photographic_style", "file_type",
	"captured_by_me", "id_document_types", "id_document_names",
}

func TestAsDict(t *testing.T) {
	sortedKeys := slices.Sorted(slices.Values(wantKeys))
	if len(wantKeys) != 44 {
		t.Fatalf("the test lists %d keys, upstream has 44", len(wantKeys))
	}

	t.Run("the keys of upstream", func(t *testing.T) {
		if got := slices.Sorted(maps.Keys(searchinfo.New(records()).AsDict())); !slices.Equal(got, sortedKeys) {
			t.Errorf("keys = %v, want %v", got, sortedKeys)
		}
	})

	t.Run("values", func(t *testing.T) {
		d := searchinfo.New(records()).AsDict()
		want := map[string]any{
			"labels":            []string{"Beach", "Tree"},
			"city":              "Another City",
			"state":             "",
			"locality_names":    []string{"Loc B", "Loc Z", "Area A"},
			"media_types":       []string{"Video", "Favorite"},
			"source":            "Example App",
			"week_part":         "Weekend",
			"id_document_types": []string{"Driver License", "Passport"},
			"pets":              []string(nil),
		}
		for k, w := range want {
			if !reflect.DeepEqual(d[k], w) {
				t.Errorf("%s = %#v, want %#v", k, d[k], w)
			}
		}
	})

	t.Run("a zero SearchInfo has the keys and no terms", func(t *testing.T) {
		var s searchinfo.SearchInfo
		d := s.AsDict()
		if got := slices.Sorted(maps.Keys(d)); !slices.Equal(got, sortedKeys) {
			t.Errorf("keys = %v, want %v", got, sortedKeys)
		}
		for k, v := range d {
			if !isEmpty(v) {
				t.Errorf("%s = %#v, want \"\" or nil", k, v)
			}
		}
		if got := s.All(); got != nil {
			t.Errorf("All() = %#v, want nil", got)
		}
	})
}
