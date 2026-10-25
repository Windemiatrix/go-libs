package photosdb

// Info describes an open library.
type Info struct {
	LibraryPath   string // the .photoslibrary directory given to Open
	ModelVersion  int    // PLModelVersion of Z_METADATA (photosdb_utils.py:88-118)
	SchemaID      string // "photos8" … "photos12"
	LibraryUUID   string // extension: not in osxphotos — Z_METADATA.Z_UUID, the identifier of the library
	SkippedAssets int    // extension: not in osxphotos — assets without a ZADDITIONALASSETATTRIBUTES row; osxphotos drops them silently
}
