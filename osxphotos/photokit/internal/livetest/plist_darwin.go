//go:build darwin && cgo

package livetest

// Without NSPhotoLibraryUsageDescription in the Info.plist of the process,
// macOS ends it on the first request for Photos access. The section is linked
// into the test binary of this package only: a program that uses the library
// carries its own Info.plist.

// #cgo LDFLAGS: -Wl,-sectcreate,__TEXT,__info_plist,${SRCDIR}/Info.plist
import "C"
