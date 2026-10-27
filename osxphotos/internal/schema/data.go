package schema

// Ascending model order. Model ranges are _constants.py:59-68, names are
// _DB_TABLE_NAMES of _constants.py:129-264.
//
// Not supported, so not in the table: models below 16320 (Photos 7 and
// earlier, and the Ventura previews 16000-16319 without the shared library
// columns, _constants.py:74), 18000-18200 (macOS 15 beta 1, photos10b1,
// _constants.py:62, 180-196), and the gaps 19000-19062 and
// 20000-269999999.
var schemas = []Schema{
	{
		// macOS 13: _constants.py:59, 74, 129-145.
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
		// macOS 14.0-14.5: _constants.py:60, 146-162.
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
		// macOS 14.6: _constants.py:61, 163-179.
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
		// macOS 15: _constants.py:63, 197-213.
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
		// macOS 26.0: _constants.py:64, 214-230.
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
		// macOS 26.1 and later 26.x: _constants.py:65, 231-247.
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
		// macOS 27: _constants.py:66-68, 248-264.
		ID: "photos12", MinModel: 270000000, MaxModel: 270999999, PhotosVersion: 12,
		MemberTable: "Z_34ASSETS", MemberAlbumColumn: "Z_34ALBUMS",
		MemberAssetColumn: "Z_3ASSETS", MemberOrderColumn: "Z_FOK_3ASSETS",
		KeywordColumn:     "Z_53KEYWORDS",
		FingerprintColumn: "ZORIGINALSTABLEHASH",
		AdjustmentsColumn: "ZADJUSTMENTSSTATE",
		FacePersonColumn:  "ZPERSONFORFACE", FaceAssetColumn: "ZASSETFORFACE",

		ParticipantShareColumn: "Z71_SHARE",
	},
}
