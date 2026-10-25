package photosdb

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"howett.net/plist"
)

// libraryPreferences are the photolibraryd preferences that may record the
// system library, relative to the home directory, in the order of
// utils.py:180-186.
var libraryPreferences = []string{
	"Library/Group Containers/group.com.apple.photolibraryd.private/Library/Preferences/group.com.apple.photolibraryd.private.plist",
	"Library/Containers/com.apple.photolibraryd/Data/Library/Preferences/com.apple.photolibraryd.plist",
}

// SystemLibraryPath returns the path of the system Photos library recorded
// by photolibraryd (utils.py:162-206): the SystemLibraryPath key of the
// group container preferences (macOS 27 and later), then of the container
// preferences (macOS 10.15 to 26 according to utils.py:176-178). When
// neither file holds a usable value, it returns ErrLibraryNotFound: the
// private PhotoKit fallback of osxphotos (utils.py:206) is not ported.
//
// Only an absolute path without NUL is accepted. Whether the path exists is
// not checked: the caller opens it as any path given by the user.
func SystemLibraryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrLibraryNotFound, err)
	}
	return systemLibraryPath(home)
}

// systemLibraryPath is SystemLibraryPath for the home directory home.
func systemLibraryPath(home string) (string, error) {
	var denied []string
	for _, rel := range libraryPreferences {
		path, err := recordedLibraryPath(filepath.Join(home, filepath.FromSlash(rel)))
		if path != "" {
			return path, nil
		}
		if errors.Is(err, fs.ErrPermission) {
			denied = append(denied, rel)
		}
	}
	// The files are named relative to the home directory: its path holds the
	// login name, and the message may end up in a log or a bug report.
	msg := "SystemLibraryPath is not recorded in the photolibraryd preferences"
	for _, rel := range denied {
		msg += "; cannot read ~/" + rel + ": permission denied (the process may lack Full Disk Access)"
	}
	return "", fmt.Errorf("%w: %s", ErrLibraryNotFound, msg)
}

// recordedLibraryPath returns the SystemLibraryPath value of the
// preferences file, or "" when the file cannot be read, is larger than
// maxPlistSize, is not a property list, or has no usable value. The error is
// that of reading the file, if any.
func recordedLibraryPath(file string) (string, error) {
	data, err := os.ReadFile(file) //nolint:gosec // G304: fixed path under the home directory
	if err != nil || len(data) > maxPlistSize {
		return "", err
	}
	var prefs struct {
		Path string `plist:"SystemLibraryPath"`
	}
	if _, err := plist.Unmarshal(data, &prefs); err != nil {
		return "", nil // a broken file is skipped like a missing one, as upstream
	}
	if !filepath.IsAbs(prefs.Path) || strings.ContainsRune(prefs.Path, 0) {
		return "", nil
	}
	return prefs.Path, nil
}
