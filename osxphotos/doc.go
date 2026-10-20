// Package osxphotos is a Go port of the parts of osxphotos
// (https://github.com/RhetTbull/osxphotos) that are needed to read a macOS
// Photos library without Python.
//
// The module is split into small packages; this root package only documents
// them and carries no code of its own:
//
//   - photosdb opens a copy of Photos.sqlite and exposes the catalog.
//   - photoinfo and albuminfo describe photos, albums and folders.
//   - query selects photos from the catalog.
//   - template renders path templates from photo fields.
//   - export computes file names for exported originals.
//   - photokit talks to the Photos app through PhotoKit on macOS.
//
// The library only reads the database, and only from a copy: it never writes
// to a Photos library. The upstream version that the behavior follows is
// recorded in UPSTREAM.md, which also lists the differences and the
// extensions that go beyond upstream.
package osxphotos
