// Package osxphotos is an unofficial Go port of parts of osxphotos
// (https://github.com/RhetTbull/osxphotos): reading a macOS Photos library,
// its template language and export file names, and PhotoKit.
//
// This root package carries no code; the module is split into packages:
//
//   - photosdb opens a copy of Photos.sqlite and exposes the catalog.
//   - photoinfo describes a photo.
//   - albuminfo describes albums, folders, import sessions and projects.
//   - adjustmentsinfo describes the edit data of a photo.
//   - exifinfo holds the EXIF data that Photos stores on import.
//   - placeinfo holds the reverse geolocation of a photo.
//   - personinfo holds persons and detected faces.
//   - momentinfo holds moments.
//   - scoreinfo holds the computed scores of a photo.
//   - searchinfo holds the search terms and labels of a photo.
//   - query selects and orders photos as osxphotos query does.
//   - template parses and renders the osxphotos template language.
//   - export names the files of an export as osxphotos export does.
//   - photokit reads and changes the system library through PhotoKit,
//     on macOS with cgo only.
//
// Only photosdb and photokit do I/O: photosdb reads a copy of the database and
// the adjustment files of the library and never changes it, photokit calls PhotoKit
// and is the only package that can change it. The other packages are pure.
//
// The behavior follows the osxphotos release named in UPSTREAM.md and is
// checked against its output on test libraries; UPSTREAM.md also lists the
// intentional differences and the extensions.
package osxphotos
