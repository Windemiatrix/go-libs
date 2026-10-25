// Package photosdb reads a Photos library: it copies the library database
// (Photos.sqlite with its write-ahead log) and reads the copy, never the
// original. A database of an unknown schema is an error
// (ErrUnsupportedSchema), not a guess.
//
// SystemLibraryPath finds the system Photos library of the current user, and
// Open reads a library into a DB.
package photosdb
