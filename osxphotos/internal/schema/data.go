package schema

// Ascending model order. Model ranges are _constants.py:63-72, names are
// _DB_TABLE_NAMES of _constants.py:134-269.
//
// Not supported, so not in the table: models below 16320 (Photos 7 and
// earlier, and the Ventura previews 16000-16319 without the shared library
// columns, _constants.py:79), 18000-18200 (macOS 15 beta 1, photos10b1,
// _constants.py:66, 185-201), the gaps 19000-19062 and 20000-269999999, and
// 270300000 and above: upstream has the photos12_3 schema for macOS 27.3
// beta (_constants.py:73, 270-286), but a schema of a new macOS is not added
// without a test library of that version.
var schemas = []Schema{
	{
		// macOS 13: _constants.py:63, 79, 134-150.
		ID: "photos8", MinModel: 16320, MaxModel: 16999, PhotosVersion: 8,
		MemberTable: "Z_28ASSETS", MemberAlbumColumn: "Z_28ALBUMS",
		MemberAssetColumn: "Z_3ASSETS", MemberOrderColumn: "Z_FOK_3ASSETS",
		KeywordColumn:     "Z_40KEYWORDS",
		FingerprintColumn: "ZMASTERFINGERPRINT",
		AdjustmentsColumn: "ZHASADJUSTMENTS",
		FacePersonColumn:  "ZPERSON", FaceAssetColumn: "ZASSET",

		ParticipantShareColumn: "Z54_SHARE",
	},
	{
		// macOS 14.0-14.5: _constants.py:64, 151-167.
		ID: "photos9", MinModel: 17000, MaxModel: 17599, PhotosVersion: 9,
		MemberTable: "Z_28ASSETS", MemberAlbumColumn: "Z_28ALBUMS",
		MemberAssetColumn: "Z_3ASSETS", MemberOrderColumn: "Z_FOK_3ASSETS",
		KeywordColumn:     "Z_40KEYWORDS",
		FingerprintColumn: "ZMASTERFINGERPRINT",
		AdjustmentsColumn: "ZHASADJUSTMENTS",
		FacePersonColumn:  "ZPERSONFORFACE", FaceAssetColumn: "ZASSETFORFACE",

		ParticipantShareColumn: "Z54_SHARE",
	},
	{
		// macOS 14.6: _constants.py:65, 168-184.
		ID: "photos9_14_6", MinModel: 17600, MaxModel: 17999, PhotosVersion: 9.6,
		MemberTable: "Z_29ASSETS", MemberAlbumColumn: "Z_29ALBUMS",
		MemberAssetColumn: "Z_3ASSETS", MemberOrderColumn: "Z_FOK_3ASSETS",
		KeywordColumn:     "Z_41KEYWORDS",
		FingerprintColumn: "ZMASTERFINGERPRINT",
		AdjustmentsColumn: "ZHASADJUSTMENTS",
		FacePersonColumn:  "ZPERSONFORFACE", FaceAssetColumn: "ZASSETFORFACE",

		ParticipantShareColumn: "Z55_SHARE",
	},
	{
		// macOS 15: _constants.py:67, 202-218.
		ID: "photos10", MinModel: 18201, MaxModel: 18999, PhotosVersion: 10,
		MemberTable: "Z_30ASSETS", MemberAlbumColumn: "Z_30ALBUMS",
		MemberAssetColumn: "Z_3ASSETS", MemberOrderColumn: "Z_FOK_3ASSETS",
		KeywordColumn:     "Z_47KEYWORDS",
		FingerprintColumn: "ZORIGINALSTABLEHASH",
		AdjustmentsColumn: "ZADJUSTMENTSSTATE",
		FacePersonColumn:  "ZPERSONFORFACE", FaceAssetColumn: "ZASSETFORFACE",

		ParticipantShareColumn: "Z61_SHARE",
	},
	{
		// macOS 26.0: _constants.py:68, 219-235.
		ID: "photos11", MinModel: 19063, MaxModel: 19319, PhotosVersion: 11,
		MemberTable: "Z_32ASSETS", MemberAlbumColumn: "Z_32ALBUMS",
		MemberAssetColumn: "Z_3ASSETS", MemberOrderColumn: "Z_FOK_3ASSETS",
		KeywordColumn:     "Z_51KEYWORDS",
		FingerprintColumn: "ZORIGINALSTABLEHASH",
		AdjustmentsColumn: "ZADJUSTMENTSSTATE",
		FacePersonColumn:  "ZPERSONFORFACE", FaceAssetColumn: "ZASSETFORFACE",

		ParticipantShareColumn: "Z65_SHARE",
	},
	{
		// macOS 26.1 and later 26.x: _constants.py:69, 236-252.
		ID: "photos11_1", MinModel: 19320, MaxModel: 19999, PhotosVersion: 11.1,
		MemberTable: "Z_33ASSETS", MemberAlbumColumn: "Z_33ALBUMS",
		MemberAssetColumn: "Z_3ASSETS", MemberOrderColumn: "Z_FOK_3ASSETS",
		KeywordColumn:     "Z_52KEYWORDS",
		FingerprintColumn: "ZORIGINALSTABLEHASH",
		AdjustmentsColumn: "ZADJUSTMENTSSTATE",
		FacePersonColumn:  "ZPERSONFORFACE", FaceAssetColumn: "ZASSETFORFACE",

		ParticipantShareColumn: "Z66_SHARE",
	},
	{
		// macOS 27.0: _constants.py:70-72, 253-269.
		ID: "photos12", MinModel: 270000000, MaxModel: 270299999, PhotosVersion: 12,
		MemberTable: "Z_34ASSETS", MemberAlbumColumn: "Z_34ALBUMS",
		MemberAssetColumn: "Z_3ASSETS", MemberOrderColumn: "Z_FOK_3ASSETS",
		KeywordColumn:     "Z_53KEYWORDS",
		FingerprintColumn: "ZORIGINALSTABLEHASH",
		AdjustmentsColumn: "ZADJUSTMENTSSTATE",
		FacePersonColumn:  "ZPERSONFORFACE", FaceAssetColumn: "ZASSETFORFACE",

		ParticipantShareColumn: "Z71_SHARE",
	},
}
