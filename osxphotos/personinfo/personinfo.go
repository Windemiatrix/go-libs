package personinfo

// PersonInfo is a person of the library, the PersonInfo of osxphotos
// (personinfo.py:21-145): one ZPERSON row (photosdb/photosdb.py:1771-1812). A
// NULL integer is 0, a NULL string "".
type PersonInfo struct {
	UUID         string // ZPERSONUUID
	Name         string // ZFULLNAME in NFC; "_UNKNOWN_" for a person without a name, as upstream (photosdb.py:1796-1800)
	DisplayName  string // ZDISPLAYNAME in NFC
	KeyFace      int64  // ZKEYFACE, the Z_PK of the key face
	FaceCount    int    // ZFACECOUNT
	KeyPhotoUUID string // the ZUUID of the asset of the key face (photosdb.py:1821-1844) when the library has that asset (personinfo.py:44-58); "" otherwise
	Favorite     bool   // ZTYPE == 1 (85-88)
	SortOrder    int    // ZMANUALORDER (90-93)
	FeatureLess  bool   // "Feature This Person Less": ZTYPE != 0 of the ZUSERFEEDBACK row of the person with the lowest Z_PK; false without a row (95-110)
}

// FaceInfo is a face detected on a photo, the FaceInfo of osxphotos for Photos
// 5 and later (personinfo.py:148-212; _photosdb_process_faceinfo.py:176-358):
// one ZDETECTEDFACE row with a person and an asset. A NULL integer is 0, a
// NULL real 0, a NULL flag false. CenterX and CenterY are shares of the width
// and the height of the photo as Photos stores them, Size a share of its longer
// side (size_pixels, 223-232); the body region is in shares as the center is.
// The methods take Width, Height and Orientation of the photoinfo.PhotoInfo of
// the face; an orientation outside 0-8 leaves the point as Photos stores it,
// where upstream logs a warning (personinfo.py:408).
type FaceInfo struct {
	PK         int64  // Z_PK
	UUID       string // ZDETECTEDFACE.ZUUID
	Name       string // ZPERSON.ZFULLNAME in NFC; "" for a person without a name, where PersonInfo.Name is "_UNKNOWN_" (_photosdb_process_faceinfo.py:305)
	AssetUUID  string
	PersonPK   int64  // ZPERSON.Z_PK of the person, upstream _person_pk
	PersonUUID string // ZPERSONUUID of the person: the key of DB.Person, upstream person_info (personinfo.py:234-241)

	CenterX, CenterY, Size, Quality float64
	SourceWidth, SourceHeight       int
	HasSmile, Manual, InTrash       bool

	AgeType, EyeMakeupType, EyeState, FacialHairType, GenderType, GlassesType, HairColorType, LipMakeupType, SmileType int

	Roll float64 // ZROLL, radians; pitch and yaw are 0 for Photos 5 and later upstream (_photosdb_process_faceinfo.py:341-344) and have no field here

	DetectionType int // ZDETECTIONTYPE: 1 a person, 3 a dog, 4 a cat (_constants.py:55-57)

	BodyCenterX, BodyCenterY, BodyWidth, BodyHeight float64 // 0 for a face without a body region
}

// ZDETECTEDFACE.ZDETECTIONTYPE of a pet (_constants.py:55-57).
const (
	detectionTypeDog = 3
	detectionTypeCat = 4
)

// IsPet is is_pet (personinfo.py:195-198): a dog or a cat.
func (f FaceInfo) IsPet() bool {
	return f.DetectionType == detectionTypeDog || f.DetectionType == detectionTypeCat
}

// Area is MWG_RS_Area and MPRI_Reg_Rect of osxphotos (personinfo.py:17-18), in
// shares of the photo; X, Y is the center for MWGArea and the top left corner
// for MPRIRect (259-263, 294-298).
type Area struct{ X, Y, H, W float64 }
