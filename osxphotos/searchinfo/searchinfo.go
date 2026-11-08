package searchinfo

import "slices"

// Record is one search term of a photo: a row of groups joined through ga in
// psi.sqlite, or one lexeme of an item in leo.sqlite
// (_photosdb_process_searchinfo.py:233-247, 326-337). Content is
// content_string, Normalized normalized_string, both NFC and without NUL.
type Record struct {
	Category            int
	Content, Normalized string
}

// SearchInfo is the SearchInfo of osxphotos for one photo (searchinfo.py:8-29):
// the records of the photo, read either as content or, after Normalized, as
// the normalized strings (lower case). Every property sorts the strings of its
// category (401-413) and is nil without any; a single-valued property is the
// first of them or "" (396-399). A zero SearchInfo has no records, as a photo
// without search data.
type SearchInfo struct {
	records    []Record
	normalized bool
}

// New returns the SearchInfo of records, which it keeps and the caller does not
// change.
func New(records []Record) SearchInfo { return SearchInfo{records: records} }

// Normalized returns s reading the normalized strings: search_info_normalized
// (photoinfo.py:1554-1567).
func (s SearchInfo) Normalized() SearchInfo {
	s.normalized = true
	return s
}

// text is _get_text_for_category (401-413) for each category in turn: several
// categories make the union upstream builds for locality_names and media_types
// (52-57, 126-132), each category sorted on its own.
func (s SearchInfo) text(categories ...int) []string {
	var out []string
	for _, c := range categories {
		start := len(out)
		for _, r := range s.records {
			if r.Category != c {
				continue
			}
			if s.normalized {
				out = append(out, r.Normalized)
			} else {
				out = append(out, r.Content)
			}
		}
		slices.Sort(out[start:])
	}
	return out
}

func (s SearchInfo) first(category int) string {
	if t := s.text(category); len(t) > 0 {
		return t[0]
	}
	return ""
}

// Labels is labels (searchinfo.py:32).
func (s SearchInfo) Labels() []string { return s.text(CategoryLabel) }

// PlaceNames is place_names (searchinfo.py:37).
func (s SearchInfo) PlaceNames() []string { return s.text(CategoryPlaceName) }

// Streets is streets (searchinfo.py:42).
func (s SearchInfo) Streets() []string { return s.text(CategoryStreet) }

// Neighborhoods is neighborhoods (searchinfo.py:47).
func (s SearchInfo) Neighborhoods() []string { return s.text(CategoryNeighborhood) }

// LocalityNames is locality_names (searchinfo.py:52): the locality categories
// in turn, each sorted on its own.
func (s SearchInfo) LocalityNames() []string { return s.text(allLocality...) }

// City is city (searchinfo.py:60).
func (s SearchInfo) City() string { return s.first(CategoryCity) }

// State is state (searchinfo.py:66).
func (s SearchInfo) State() string { return s.first(CategoryState) }

// StateAbbreviation is state_abbreviation (searchinfo.py:72).
func (s SearchInfo) StateAbbreviation() string { return s.first(CategoryStateAbbreviation) }

// Country is country (searchinfo.py:78).
func (s SearchInfo) Country() string { return s.first(CategoryCountry) }

// Month is month (searchinfo.py:84).
func (s SearchInfo) Month() string { return s.first(CategoryMonth) }

// Year is year (searchinfo.py:90).
func (s SearchInfo) Year() string { return s.first(CategoryYear) }

// BodiesOfWater is bodies_of_water (searchinfo.py:96).
func (s SearchInfo) BodiesOfWater() []string { return s.text(CategoryBodyOfWater) }

// Holidays is holidays (searchinfo.py:101).
func (s SearchInfo) Holidays() []string { return s.text(CategoryHoliday) }

// Activities is activities (searchinfo.py:106).
func (s SearchInfo) Activities() []string { return s.text(CategoryActivity) }

// Season is season (searchinfo.py:111).
func (s SearchInfo) Season() string { return s.first(CategorySeason) }

// Venues is venues (searchinfo.py:117).
func (s SearchInfo) Venues() []string { return s.text(CategoryVenue) }

// VenueTypes is venue_types (searchinfo.py:122).
func (s SearchInfo) VenueTypes() []string { return s.text(CategoryVenueType) }

// MediaTypes is media_types (searchinfo.py:127): the media type categories in
// turn, each sorted on its own.
func (s SearchInfo) MediaTypes() []string { return s.text(mediaTypes...) }

// DetectedText is detected_text (searchinfo.py:135).
func (s SearchInfo) DetectedText() []string { return s.text(CategoryDetectedText) }

// TextFound is text_found (searchinfo.py:142): a list, as upstream returns.
func (s SearchInfo) TextFound() []string { return s.text(CategoryTextFound) }

// Camera is camera (searchinfo.py:149).
func (s SearchInfo) Camera() string { return s.first(CategoryCamera) }

// Source is source (searchinfo.py:157).
func (s SearchInfo) Source() string { return s.first(CategorySource) }

// TimesOfDay is times_of_day (searchinfo.py:165).
func (s SearchInfo) TimesOfDay() []string { return s.text(CategoryTimeOfDay) }

// WeekPart is week_part (searchinfo.py:170).
func (s SearchInfo) WeekPart() string { return s.first(CategoryWeekpart) }

// DayOfWeek is day_of_week (searchinfo.py:175).
func (s SearchInfo) DayOfWeek() string { return s.first(CategoryDayOfWeek) }

// Home is home (searchinfo.py:180).
func (s SearchInfo) Home() string { return s.first(CategoryHome) }

// AreasOfInterest is areas_of_interest (searchinfo.py:185).
func (s SearchInfo) AreasOfInterest() []string { return s.text(CategoryAreaOfInterest) }

// CountryCode is country_code (searchinfo.py:190).
func (s SearchInfo) CountryCode() string { return s.first(CategoryCountryCode) }

// Region is region (searchinfo.py:195).
func (s SearchInfo) Region() string { return s.first(CategoryRegion) }

// Continent is continent (searchinfo.py:200).
func (s SearchInfo) Continent() string { return s.first(CategoryContinent) }

// Events is events (searchinfo.py:205).
func (s SearchInfo) Events() []string { return s.text(CategoryEvent) }

// EventPerformers is event_performers (searchinfo.py:210).
func (s SearchInfo) EventPerformers() []string { return s.text(CategoryEventPerformer) }

// EventTypes is event_types (searchinfo.py:215).
func (s SearchInfo) EventTypes() []string { return s.text(CategoryEventType) }

// Pets is pets (searchinfo.py:220).
func (s SearchInfo) Pets() []string { return s.text(CategoryPet) }

// AgeGroups is age_groups (searchinfo.py:225).
func (s SearchInfo) AgeGroups() []string { return s.text(CategoryAgeGroup) }

// Landmarks is landmarks (searchinfo.py:230).
func (s SearchInfo) Landmarks() []string { return s.text(CategoryLandmark) }

// Sounds is sounds (searchinfo.py:235).
func (s SearchInfo) Sounds() []string { return s.text(CategorySound) }

// HumanActions is human_actions (searchinfo.py:240).
func (s SearchInfo) HumanActions() []string { return s.text(CategoryHumanAction) }

// DocumentTypes is document_types (searchinfo.py:245).
func (s SearchInfo) DocumentTypes() []string { return s.text(CategoryDocumentType) }

// Trip is trip (searchinfo.py:250).
func (s SearchInfo) Trip() string { return s.first(CategoryTrip) }

// PhotographicStyle is photographic_style (searchinfo.py:255).
func (s SearchInfo) PhotographicStyle() string { return s.first(CategoryPhotographicStyle) }

// FileType is file_type (searchinfo.py:260).
func (s SearchInfo) FileType() string { return s.first(CategoryFileType) }

// CapturedByMe is captured_by_me (searchinfo.py:265).
func (s SearchInfo) CapturedByMe() string { return s.first(CategoryCapturedByMe) }

// IDDocumentTypes is id_document_types (searchinfo.py:270): the sorted set of
// two categories, not included in All.
func (s SearchInfo) IDDocumentTypes() []string {
	t := s.text(CategoryIDDocumentType, CategoryIDDocumentCardType)
	slices.Sort(t)
	return slices.Compact(t)
}

// IDDocumentNames is id_document_names (searchinfo.py:281): not included in All.
func (s SearchInfo) IDDocumentNames() []string { return s.text(CategoryIDDocumentName) }

// All is all (searchinfo.py:287): the lists in turn, then the non-empty
// single values. TextFound, Source and the ID document properties are not in it.
func (s SearchInfo) All() []string {
	var all []string
	for _, l := range [][]string{
		s.Labels(), s.PlaceNames(), s.Streets(), s.Neighborhoods(), s.LocalityNames(), s.BodiesOfWater(), s.Holidays(),
		s.Activities(), s.Venues(), s.VenueTypes(), s.MediaTypes(), s.DetectedText(), s.TimesOfDay(), s.AreasOfInterest(),
		s.Events(), s.EventPerformers(), s.EventTypes(), s.Pets(), s.AgeGroups(), s.Landmarks(), s.Sounds(),
		s.HumanActions(), s.DocumentTypes(),
	} {
		all = append(all, l...)
	}
	for _, v := range []string{
		s.City(), s.State(), s.StateAbbreviation(), s.Country(), s.Month(), s.Year(), s.Season(), s.Camera(),
		s.WeekPart(), s.DayOfWeek(), s.Home(), s.CountryCode(), s.Region(), s.Continent(), s.Trip(),
		s.PhotographicStyle(), s.FileType(), s.CapturedByMe(),
	} {
		if v != "" {
			all = append(all, v)
		}
	}
	return all
}

// AsDict is asdict (searchinfo.py:348): the 44 keys of upstream, each a
// []string or a string. An empty list is nil (upstream []).
func (s SearchInfo) AsDict() map[string]any {
	return map[string]any{
		"labels": s.Labels(), "place_names": s.PlaceNames(), "streets": s.Streets(), "neighborhoods": s.Neighborhoods(),
		"city": s.City(), "locality_names": s.LocalityNames(), "state": s.State(), "state_abbreviation": s.StateAbbreviation(),
		"country": s.Country(), "bodies_of_water": s.BodiesOfWater(), "month": s.Month(), "year": s.Year(),
		"holidays": s.Holidays(), "activities": s.Activities(), "season": s.Season(), "venues": s.Venues(),
		"venue_types": s.VenueTypes(), "media_types": s.MediaTypes(), "detected_text": s.DetectedText(), "camera": s.Camera(),
		"source": s.Source(), "times_of_day": s.TimesOfDay(), "week_part": s.WeekPart(), "day_of_week": s.DayOfWeek(),
		"home": s.Home(), "areas_of_interest": s.AreasOfInterest(), "country_code": s.CountryCode(), "region": s.Region(),
		"continent": s.Continent(), "events": s.Events(), "event_performers": s.EventPerformers(), "event_types": s.EventTypes(),
		"pets": s.Pets(), "age_groups": s.AgeGroups(), "landmarks": s.Landmarks(), "sounds": s.Sounds(),
		"human_actions": s.HumanActions(), "document_types": s.DocumentTypes(), "trip": s.Trip(),
		"photographic_style": s.PhotographicStyle(), "file_type": s.FileType(), "captured_by_me": s.CapturedByMe(),
		"id_document_types": s.IDDocumentTypes(), "id_document_names": s.IDDocumentNames(),
	}
}
