package photosdb

import "errors"

var (
	// ErrUnsupportedSchema is wrapped by every error about a database whose
	// schema this module does not read: an unknown model version or a
	// missing table or column.
	ErrUnsupportedSchema = errors.New("osxphotos: unsupported Photos.sqlite schema")

	// ErrLibraryNotFound is wrapped by the error of SystemLibraryPath when
	// the path of the system library cannot be found.
	ErrLibraryNotFound = errors.New("osxphotos: photos library not found")
)
