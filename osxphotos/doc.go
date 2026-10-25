// Package osxphotos is an unofficial Go port of the parts of osxphotos
// (https://github.com/RhetTbull/osxphotos) that are needed to read a macOS
// Photos library without Python.
//
// This root package carries no code; the module is split into small packages:
//
//   - photosdb opens a copy of Photos.sqlite and exposes the catalog.
//   - photoinfo and albuminfo describe photos, albums and folders.
//   - query selects and orders photos.
//   - planned: template (path templates), export (export file names),
//     photokit (PhotoKit on macOS).
//
// Status: early development; see README.md for what works today.
//
// The read packages never change a Photos library and read the database only
// from a copy. Changing the library will be possible only through the
// photokit package, on explicit use.
//
// The upstream version that the behavior follows, the differences and the
// extensions are listed in UPSTREAM.md.
package osxphotos
