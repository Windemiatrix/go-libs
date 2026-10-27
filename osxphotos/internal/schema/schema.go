// Package schema holds what differs between the Photos.sqlite schemas that
// this module reads: the model version range of each schema and the names of
// version-dependent tables and columns (_constants.py:55-68, 129-264). Data
// only: no database access.
package schema

import "slices"

// Schema is one Photos.sqlite schema. Names are the table and column names
// that differ between schemas; the rest is the same in all of them.
type Schema struct {
	ID                 string  // "photos8", "photos9", "photos9_14_6", "photos10", "photos11", "photos11_1", "photos12"
	MinModel, MaxModel int     // inclusive range of PLModelVersion
	PhotosVersion      float64 // photos_version of osxphotos (photosdb_utils.py:138-159)

	MemberTable       string // album membership (ASSET_ALBUM_TABLE), e.g. Z_33ASSETS
	MemberAlbumColumn string // its album column (ASSET_ALBUM_JOIN), e.g. Z_33ALBUMS
	MemberAssetColumn string // its asset column (ALBUM_JOIN), e.g. Z_3ASSETS
	MemberOrderColumn string // its order column (ALBUM_SORT_ORDER), e.g. Z_FOK_3ASSETS
	KeywordColumn     string // Z_1KEYWORDS column of the keyword (KEYWORD_JOIN), e.g. Z_52KEYWORDS
	FingerprintColumn string // ZADDITIONALASSETATTRIBUTES column (MASTER_FINGERPRINT)
	AdjustmentsColumn string // ZASSET column (HAS_ADJUSTMENTS)
	FacePersonColumn  string // ZDETECTEDFACE column of the person (DETECTED_FACE_PERSON_FK)
	FaceAssetColumn   string // ZDETECTEDFACE column of the asset (DETECTED_FACE_ASSET_FK)

	ParticipantShareColumn string // ZSHAREPARTICIPANT column of the share (ZSHAREPARTICIPANT_ZXX_SHARE), e.g. Z66_SHARE
}

// All returns every schema in ascending model order (a copy).
func All() []Schema {
	return slices.Clone(schemas)
}

// ForModel returns the schema whose range contains model.
func ForModel(model int) (Schema, bool) {
	for _, s := range schemas {
		if s.MinModel <= model && model <= s.MaxModel {
			return s, true
		}
	}
	return Schema{}, false
}
