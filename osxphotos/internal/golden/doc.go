// Package golden compares what this module reads with the output of
// `osxphotos query --json` of osxphotos 0.77.2 kept in testdata/golden of the
// repository. The rules are in testdata/golden/NORMALIZATION.md. Only tests
// use it.
//
// The package reads files only: the reference data of testdata/golden or the
// paths its caller names. What to compare it gets as plain values
// (photoinfo.PhotoInfo, albuminfo.AlbumInfo), so it does not import photosdb.
package golden
