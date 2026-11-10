package searchinfo

// The categories of groups.category of psi.sqlite for Photos 8 and later
// (_constants.py:530-629, SearchCategory_Photos8); the port reads only
// libraries of Photos 8 and later, so the older table (422-528) has no use.
// The categories of the last group exist only in leo.sqlite and keep the leo
// category id (620-629).
const (
	CategoryPlaceName         = 1
	CategoryStreet            = 2
	CategoryNeighborhood      = 3
	CategoryLocality4         = 4
	CategoryCity              = 5
	CategorySubLocality6      = 6
	CategoryNamedArea         = 7
	CategoryLocality8         = 8
	CategoryAreaOfInterest    = 9
	CategoryState             = 10
	CategoryStateAbbreviation = 11
	CategoryCountry           = 12
	CategoryCountryCode       = 13
	CategoryBodyOfWater       = 14

	CategoryHome      = 1000
	CategoryWork      = 1001
	CategoryContinent = 1007
	CategoryRegion    = 1008
	CategoryMonth     = 1100
	CategoryYear      = 1101
	CategoryHoliday   = 1103
	CategorySeason    = 1104
	CategoryTimeOfDay = 1106
	CategoryWeekpart  = 1107

	CategoryKeywords       = 1200
	CategoryTitle          = 1201
	CategoryDescription    = 1202
	CategoryDetectedText   = 1203
	CategoryTextFound      = 1205
	CategoryPerson         = 1300
	CategoryPet            = 1330
	CategoryLabel          = 1500
	CategoryLandmark       = 1520
	CategoryActivity       = 1600
	CategoryTrip           = 1610
	CategoryVenue          = 1700
	CategoryVenueType      = 1701
	CategoryEvent          = 1800
	CategoryEventPerformer = 1801
	CategoryEventType      = 1802

	CategoryPhotoTypeVideo               = 1901
	CategoryPhotoTypeRaw                 = 1902
	CategoryPhotoTypeCinematic           = 1903
	CategoryPhotoTypeSlomo               = 1905
	CategoryPhotoTypeLive                = 1906
	CategoryPhotoTypeScreenshot          = 1907
	CategoryPhotoTypePanorama            = 1908
	CategoryPhotoTypeTimelapse           = 1909
	CategoryPhotoTypeScreenRecordingsAlt = 1910
	CategoryPhotoTypeLongExposure        = 1911
	CategoryPhotoTypeAnimated            = 1912
	CategoryPhotoTypeBursts              = 1913
	CategoryPhotoTypePortrait            = 1914
	CategoryPhotoTypeSelfies             = 1915
	CategoryPhotoTypeScreenRecordings    = 1916
	CategoryPhotoTypeLiveDepth           = 1918
	CategoryPhotoTypeSpatial             = 1919
	CategoryPhotoTypeFavorites           = 2000

	CategoryPhotoName    = 2100
	CategorySource       = 2200
	CategoryCamera       = 2300
	CategorySound        = 2500
	CategoryHumanAction  = 2600
	CategoryDocumentType = 2800

	CategoryDayOfWeek          = 1070
	CategoryAgeGroup           = 3030
	CategoryIDDocumentType     = 4130
	CategoryPhotographicStyle  = 5200
	CategoryFileType           = 8051
	CategoryCapturedByMe       = 8170
	CategoryIDDocumentCardType = 11000
	CategoryIDDocumentName     = 11010
)

// ALL_LOCALITY (_constants.py:541-546).
var allLocality = []int{CategoryLocality4, CategorySubLocality6, CategoryLocality8, CategoryNamedArea}

// MEDIA_TYPES (_constants.py:595-613).
var mediaTypes = []int{
	CategoryPhotoTypeVideo, CategoryPhotoTypeSlomo, CategoryPhotoTypeLive, CategoryPhotoTypeScreenshot,
	CategoryPhotoTypeScreenRecordings, CategoryPhotoTypeScreenRecordingsAlt, CategoryPhotoTypePanorama,
	CategoryPhotoTypeTimelapse, CategoryPhotoTypeBursts, CategoryPhotoTypePortrait, CategoryPhotoTypeSelfies,
	CategoryPhotoTypeFavorites, CategoryPhotoTypeAnimated, CategoryPhotoTypeCinematic,
	CategoryPhotoTypeLongExposure, CategoryPhotoTypeLiveDepth, CategoryPhotoTypeSpatial,
}
