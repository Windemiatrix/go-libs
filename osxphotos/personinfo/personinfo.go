package personinfo

// PersonInfo is a person of the library, the PersonInfo of osxphotos
// (personinfo.py:19-143): one ZPERSON row (photosdb/photosdb.py:1767-1808). A
// NULL integer is 0, a NULL string "".
type PersonInfo struct {
	UUID         string // ZPERSONUUID
	Name         string // ZFULLNAME in NFC; "_UNKNOWN_" for a person without a name, as upstream (photosdb.py:1792-1796)
	DisplayName  string // ZDISPLAYNAME in NFC
	KeyFace      int64  // ZKEYFACE, the Z_PK of the key face
	FaceCount    int    // ZFACECOUNT
	KeyPhotoUUID string // the ZUUID of the asset of the key face (photosdb.py:1817-1840) when the library has that asset (personinfo.py:42-56); "" otherwise
	Favorite     bool   // ZTYPE == 1 (83-86)
	SortOrder    int    // ZMANUALORDER (88-91)
	FeatureLess  bool   // "Feature This Person Less": ZTYPE != 0 of the ZUSERFEEDBACK row of the person with the lowest Z_PK; false without a row (93-108)
}

// FaceInfo is a face detected on a photo, the FaceInfo of osxphotos for Photos
// 5 and later (personinfo.py:146-187; _photosdb_process_faceinfo.py:171-331):
// one ZDETECTEDFACE row with a person and an asset. A NULL integer is 0, a
// NULL real 0, a NULL flag false. CenterX and CenterY are shares of the width
// and the height of the photo as Photos stores them, Size a share of its longer
// side (size_pixels, 197-206). The methods take Width, Height and Orientation
// of the photoinfo.PhotoInfo of the face; an orientation outside 0-8 leaves the
// point as Photos stores it, where upstream logs a warning (personinfo.py:373).
type FaceInfo struct {
	PK         int64  // Z_PK
	UUID       string // ZDETECTEDFACE.ZUUID
	Name       string // ZPERSON.ZFULLNAME in NFC; "" for a person without a name, where PersonInfo.Name is "_UNKNOWN_" (_photosdb_process_faceinfo.py:283)
	AssetUUID  string
	PersonPK   int64  // ZPERSON.Z_PK of the person, upstream _person_pk
	PersonUUID string // ZPERSONUUID of the person: the key of DB.Person, upstream person_info (personinfo.py:208-215)

	CenterX, CenterY, Size, Quality float64
	SourceWidth, SourceHeight       int
	HasSmile, Manual, InTrash       bool

	AgeType, EyeMakeupType, EyeState, FacialHairType, GenderType, GlassesType, HairColorType, LipMakeupType, SmileType int

	Roll float64 // ZROLL, radians; pitch and yaw are 0 for Photos 5 and later upstream (_photosdb_process_faceinfo.py:314-317) and have no field here
}

// Area is MWG_RS_Area and MPRI_Reg_Rect of osxphotos (personinfo.py:15-16), in
// shares of the photo; X, Y is the center for MWGArea and the top left corner
// for MPRIRect (233-237, 264-268).
type Area struct{ X, Y, H, W float64 }
