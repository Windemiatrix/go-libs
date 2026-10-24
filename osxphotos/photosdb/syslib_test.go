package photosdb

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"howett.net/plist"
)

const (
	groupPlist     = "Library/Group Containers/group.com.apple.photolibraryd.private/Library/Preferences/group.com.apple.photolibraryd.private.plist"
	containerPlist = "Library/Containers/com.apple.photolibraryd/Data/Library/Preferences/com.apple.photolibraryd.plist"

	groupLibrary     = "/Users/someone/Pictures/Group.photoslibrary"
	containerLibrary = "/Users/someone/Pictures/Container.photoslibrary"
)

// libraryPlist is an XML plist with SystemLibraryPath set to the XML value v.
func libraryPlist(v string) []byte {
	return xmlPlist("<dict><key>Other</key><integer>1</integer><key>SystemLibraryPath</key>" + v + "</dict>")
}

func pathPlist(path string) []byte {
	return libraryPlist("<string>" + path + "</string>")
}

// sizedPathPlist is a valid XML plist of exactly size bytes with
// SystemLibraryPath set to path, padded by a string under another key.
func sizedPathPlist(size int, path string) []byte {
	body := func(pad string) []byte {
		return xmlPlist("<dict><key>Pad</key><string>" + pad + "</string><key>SystemLibraryPath</key><string>" + path + "</string></dict>")
	}
	return body(strings.Repeat("x", size-len(body(""))))
}

// file is one preferences file of a test home; the zero value means no file.
type file struct {
	data       []byte
	unreadable bool // mode 0000
	isDir      bool // a directory in place of the file
}

func writeHome(t *testing.T, group, container file) string {
	t.Helper()
	home := t.TempDir()
	for rel, f := range map[string]file{groupPlist: group, containerPlist: container} {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		switch {
		case f.isDir:
			if err := os.Mkdir(p, 0o700); err != nil {
				t.Fatal(err)
			}
		case f.data != nil:
			if err := os.WriteFile(p, f.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if f.unreadable {
				if err := os.Chmod(p, 0); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return home
}

func TestSystemLibraryPath(t *testing.T) {
	t.Parallel()

	none := file{}
	tests := []struct {
		name             string
		group, container file
		want             string
		errText          []string // "" want: error wrapping ErrLibraryNotFound with these
	}{
		{name: "group file first", group: file{data: pathPlist(groupLibrary)}, container: file{data: pathPlist(containerLibrary)}, want: groupLibrary},
		{name: "group only", group: file{data: pathPlist(groupLibrary)}, container: none, want: groupLibrary},
		{name: "group without key", group: file{data: xmlPlist("<dict><key>Other</key><string>/x</string></dict>")}, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "container only", group: none, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "no files", group: none, container: none, errText: []string{"not recorded"}},
		{name: "broken group", group: file{data: []byte("garbage")}, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "OpenStep text group", group: file{data: []byte(`{SystemLibraryPath = "/Users/someone/Pictures/Group.photoslibrary";}`)}, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "integer value", group: file{data: libraryPlist("<integer>1</integer>")}, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "empty string", group: file{data: pathPlist("")}, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "root array", group: file{data: xmlPlist("<array><string>" + groupLibrary + "</string></array>")}, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "relative path", group: file{data: pathPlist("Pictures/Photos Library.photoslibrary")}, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "every value rejected", group: file{data: pathPlist("relative/path")}, container: file{data: libraryPlist("<true/>")}, errText: []string{"not recorded"}},
		{name: "directory instead of file", group: file{isDir: true}, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "spaces and missing path unchanged", group: file{data: pathPlist("/Volumes/No Such/Photos Library.photoslibrary")}, container: none, want: "/Volumes/No Such/Photos Library.photoslibrary"},
		{name: "exactly 1 MiB", group: file{data: sizedPathPlist(1<<20, groupLibrary)}, container: file{data: pathPlist(containerLibrary)}, want: groupLibrary},
		{name: "1 MiB + 1", group: file{data: sizedPathPlist(1<<20+1, groupLibrary)}, container: file{data: pathPlist(containerLibrary)}, want: containerLibrary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := systemLibraryPath(writeHome(t, tt.group, tt.container))
			checkLibraryPath(t, got, err, tt.want, tt.errText)
		})
	}
}

func TestSystemLibraryPathBinary(t *testing.T) {
	t.Parallel()

	bin := func(v any) file {
		return file{data: marshal(t, map[string]any{"Other": 1, "SystemLibraryPath": v}, plist.BinaryFormat)}
	}
	container := file{data: pathPlist(containerLibrary)}
	tests := []struct {
		name  string
		group file
		want  string
	}{
		{name: "binary group", group: bin(groupLibrary), want: groupLibrary},
		{name: "binary relative NUL", group: bin("a\x00b"), want: containerLibrary},
		{name: "binary absolute with NUL", group: bin("/Users/someone/a\x00b.photoslibrary"), want: containerLibrary},
		{name: "binary integer", group: bin(1), want: containerLibrary},
		{name: "binary data", group: bin([]byte(groupLibrary)), want: containerLibrary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := systemLibraryPath(writeHome(t, tt.group, container))
			checkLibraryPath(t, got, err, tt.want, nil)
		})
	}
}

func checkLibraryPath(t *testing.T, got string, err error, want string, errText []string) {
	t.Helper()
	if errText == nil {
		if err != nil || got != want {
			t.Errorf("systemLibraryPath = %q, %v; want %q, nil", got, err, want)
		}
		return
	}
	if !errors.Is(err, ErrLibraryNotFound) || got != "" {
		t.Fatalf("systemLibraryPath = %q, %v; want \"\" and an error wrapping ErrLibraryNotFound", got, err)
	}
	for _, text := range errText {
		if !strings.Contains(err.Error(), text) {
			t.Errorf("systemLibraryPath error = %q, want it to contain %q", err, text)
		}
	}
}

func TestSystemLibraryPathPermission(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads files with mode 0000")
	}

	t.Run("second unreadable", func(t *testing.T) {
		t.Parallel()
		home := writeHome(t, file{}, file{data: pathPlist(containerLibrary), unreadable: true})
		got, err := systemLibraryPath(home)
		checkLibraryPath(t, got, err, "", []string{"not recorded", "permission denied", "Full Disk Access",
			"~/" + containerPlist})
		if strings.Contains(err.Error(), home) {
			t.Errorf("systemLibraryPath error = %q names the home directory %s; want paths relative to ~", err, home)
		}
	})
	t.Run("first unreadable, second with key", func(t *testing.T) {
		t.Parallel()
		home := writeHome(t, file{data: pathPlist(groupLibrary), unreadable: true}, file{data: pathPlist(containerLibrary)})
		got, err := systemLibraryPath(home)
		checkLibraryPath(t, got, err, containerLibrary, nil)
	})
}
