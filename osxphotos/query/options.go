package query

import (
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Options is osxphotos QueryOptions (photoquery.py:47-241), field by field;
// UPSTREAM.md lists the fields that are not here. The zero value is
// QueryOptions(): what `osxphotos query` selects without options — the photos
// and movies outside the trash, without the unselected frames of bursts.
type Options struct {
	UUID []string // uuid: only these photos (photosdb/photosdb.py:3300-3305)
	// --only-photos and --only-movies, which set QueryOptions movies and photos
	// to False (photoquery.py:338-343): the zero value keeps both, as
	// QueryOptions does.
	OnlyPhotos, OnlyMovies bool
	FromDate, ToDate       time.Time // from_date (Date at or after), to_date (Date before); the zero time is not set
	Deleted, DeletedOnly   bool      // deleted: the trash too; deleted_only: only the trash (photoquery.py:426-445)

	IgnoreCase          bool // ignore_case, for Keyword, Person, Title, Description, Place, Name, Album, Folder
	Keyword             []string
	NoKeyword           bool
	Person              []string // a name of Persons, "_UNKNOWN_" included (photoquery.py:467-470)
	Title               []string // substrings of Title
	NoTitle             bool
	Description         []string // substrings of Description
	NoDescription       bool
	Place               []string // place: each value is a substring of a name in any list of PhotoInfo.Place.Names; "" matches a photo with a name that is not "" (photoquery.py:518-548)
	NoPlace             bool     // no_place: PhotoInfo.Place is nil; ignored with Place (photoquery.py:549-550)
	Name                []string // substrings of OriginalFilename (photoquery.py:685-720)
	UTI                 string   // a substring of UTIOriginal, case kept (photoquery.py:585-586)
	Album               []string // "Folder/Subfolder/Album"; a "/" of a title is written "//"
	Folder              []string // "Folder/Subfolder": the albums in it and in its subfolders
	InAlbum, NotInAlbum bool

	Edited, NotEdited                   bool
	ExternalEdit                        bool
	Favorite, NotFavorite               bool
	Hidden, NotHidden                   bool
	Missing, NotMissing                 bool // by Path, not by IsMissing (photoquery.py:570-573)
	Shared, NotShared                   bool
	Burst, NotBurst                     bool
	Live, NotLive                       bool
	Portrait, NotPortrait               bool
	Spatial, NotSpatial                 bool // Spatial is not 0 (photoquery.py:603-606)
	Screenshot, NotScreenshot           bool
	ScreenRecording, NotScreenRecording bool
	SlowMo, NotSlowMo                   bool
	TimeLapse, NotTimeLapse             bool
	HDR, NotHDR                         bool
	Selfie, NotSelfie                   bool
	Panorama, NotPanorama               bool
	CloudAsset, NotCloudAsset           bool
	InCloud, NotInCloud                 bool // of the photos that are not Shared (photoquery.py:648-651)
	HasRAW                              bool
	HasComment, NoComment               bool // comments of a shared album on the photo, by Catalog.Comments (photoquery.py:656-659)
	HasLikes, NoLikes                   bool // likes of a shared album on the photo, by Catalog.Likes (photoquery.py:661-664)
	IsReference, NotReference           bool
	Location, NoLocation                bool
	Syndicated, NotSyndicated           bool
	SavedToLibrary, NotSavedToLibrary   bool // of the Syndicated photos (photoquery.py:844-847)
	SharedMoment, NotSharedMoment       bool
	SharedLibrary, NotSharedLibrary     bool

	Year                       []int          // year: Date.Year() in the zone of Date (photoquery.py:682-683)
	FromTime, ToTime           *time.Duration // from_time, to_time: the time of day of Date since midnight, in its zone; nil is not set, as midnight is a value (photoquery.py:676-680)
	AddedAfter, AddedBefore    time.Time      // added_after (DateAdded after), added_before (DateAdded before); strict, the zero time is not set (photoquery.py:822-832)
	AddedInLast                time.Duration  // added_in_last: DateAdded after now minus it; 0 is not set (photoquery.py:834-837)
	MinSize, MaxSize           int64          // min_size, max_size: OriginalFilesize at least, at most, in bytes; 0 is not set (photoquery.py:722-730)
	Duplicate                  bool           // duplicate: the photos that have Duplicates, and those duplicates from the whole catalog; only the later filters (photoquery.py:773-857) and Deleted, DeletedOnly apply to them (photoquery.py:754-771)
	BurstPhotos, MissingBursts bool           // burst_photos: add the other frames of the bursts of the result; missing_bursts: the frames that are IsMissing too (photoquery.py:863-883)

	// function: run in order on the result of the filters
	// (photoquery.py:859-861). The first gets the filtered photos in the order
	// of the photos given to Apply; with Duplicate, each photo that has
	// duplicates, in the order of Sort, followed by its duplicates in the order
	// of Sort, each photo once (photoquery.py:754-765). Their values are shared
	// with the DB: a function must not modify them. Apply sorts the slice it
	// returns in place, the one a function returns included.
	Function []func([]*photoinfo.PhotoInfo) []*photoinfo.PhotoInfo

	NewestFirst bool // newest_first: the order of Sort, reversed (photoquery.py:888-898)
}
