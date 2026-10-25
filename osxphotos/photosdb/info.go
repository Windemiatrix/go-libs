package photosdb

// Info describes an open library.
type Info struct {
	// LibraryPath is the library that the paths of photos refer to: Options.LibraryPath,
	// else the directory given to Open, else the parent of the directory of the
	// database. As given, not cleaned or resolved; derived from a database path
	// it is absolute (photosdb/photosdb.py:452-458).
	LibraryPath   string
	DBPath        string  // db_path: the absolute path of the Photos.sqlite that was read, not of the copy; symbolic links are not resolved, upstream resolves them (photosdb/photosdb.py:698-701)
	ModelVersion  int     // PLModelVersion of Z_METADATA (photosdb_utils.py:88-118)
	PhotosVersion float64 // photos_version: 8, 9, 9.6, 10, 11, 11.1 or 12 (photosdb/photosdb.py:708-711, photosdb_utils.py:138-159)
	SchemaID      string  // "photos8" … "photos12"
	LibraryUUID   string  // extension: not in osxphotos — Z_METADATA.Z_UUID, the identifier of the library
	SkippedAssets int     // extension: not in osxphotos — assets without a ZADDITIONALASSETATTRIBUTES row; osxphotos drops them silently
}
