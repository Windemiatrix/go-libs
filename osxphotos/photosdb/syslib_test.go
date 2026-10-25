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

func libraryPlist(v string) []byte {
	return xmlPlist("<dict><key>Other</key><integer>1</integer><key>SystemLibraryPath</key>" + v + "</dict>")
}

func pathPlist(path string) []byte {
	return libraryPlist("<string>" + path + "</string>")
}

func sizedPathPlist(size int, path string) []byte {
	body := func(pad string) []byte {
		return xmlPlist("<dict><key>Pad</key><string>" + pad + "</string><key>SystemLibraryPath</key><string>" + path + "</string></dict>")
	}
	return body(strings.Repeat("x", size-len(body(""))))
}

type prefsFile struct {
	data       []byte
	unreadable bool
	isDir      bool
}

func writeHome(t *testing.T, group, container prefsFile) string {
	t.Helper()
	home := t.TempDir()
	for rel, f := range map[string]prefsFile{groupPlist: group, containerPlist: container} {
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

	none := prefsFile{}
	tests := []struct {
		name             string
		group, container prefsFile
		want             string
		wantErrText      []string
	}{
		{name: "group file first", group: prefsFile{data: pathPlist(groupLibrary)}, container: prefsFile{data: pathPlist(containerLibrary)}, want: groupLibrary},
		{name: "group only", group: prefsFile{data: pathPlist(groupLibrary)}, container: none, want: groupLibrary},
		{name: "group without key", group: prefsFile{data: xmlPlist("<dict><key>Other</key><string>/x</string></dict>")}, container: prefsFile{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "container only", group: none, container: prefsFile{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "no files", group: none, container: none, wantErrText: []string{"not recorded"}},
		{name: "broken group", group: prefsFile{data: []byte("garbage")}, container: prefsFile{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "integer value", group: prefsFile{data: libraryPlist("<integer>1</integer>")}, container: prefsFile{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "empty string", group: prefsFile{data: pathPlist("")}, container: prefsFile{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "root array", group: prefsFile{data: xmlPlist("<array><string>" + groupLibrary + "</string></array>")}, container: prefsFile{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "relative path", group: prefsFile{data: pathPlist("Pictures/Photos Library.photoslibrary")}, container: prefsFile{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "every value rejected", group: prefsFile{data: pathPlist("relative/path")}, container: prefsFile{data: libraryPlist("<true/>")}, wantErrText: []string{"not recorded"}},
		{name: "directory instead of file", group: prefsFile{isDir: true}, container: prefsFile{data: pathPlist(containerLibrary)}, want: containerLibrary},
		{name: "spaces and missing path unchanged", group: prefsFile{data: pathPlist("/Volumes/No Such/Photos Library.photoslibrary")}, container: none, want: "/Volumes/No Such/Photos Library.photoslibrary"},
		{name: "exactly 1 MiB", group: prefsFile{data: sizedPathPlist(1<<20, groupLibrary)}, container: prefsFile{data: pathPlist(containerLibrary)}, want: groupLibrary},
		{name: "1 MiB + 1", group: prefsFile{data: sizedPathPlist(1<<20+1, groupLibrary)}, container: prefsFile{data: pathPlist(containerLibrary)}, want: containerLibrary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := systemLibraryPath(writeHome(t, tt.group, tt.container))
			checkLibraryPath(t, got, err, tt.want, tt.wantErrText)
		})
	}
}

func TestSystemLibraryPathBinary(t *testing.T) {
	t.Parallel()

	bin := func(v any) prefsFile {
		return prefsFile{data: marshal(t, map[string]any{"Other": 1, "SystemLibraryPath": v}, plist.BinaryFormat)}
	}
	container := prefsFile{data: pathPlist(containerLibrary)}
	tests := []struct {
		name  string
		group prefsFile
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
		home := writeHome(t, prefsFile{}, prefsFile{data: pathPlist(containerLibrary), unreadable: true})
		got, err := systemLibraryPath(home)
		checkLibraryPath(t, got, err, "", []string{"not recorded", "permission denied", "Full Disk Access",
			"~/" + containerPlist})
		if strings.Contains(err.Error(), home) {
			t.Errorf("systemLibraryPath error = %q names the home directory %s; want paths relative to ~", err, home)
		}
	})
	t.Run("first unreadable, second with key", func(t *testing.T) {
		t.Parallel()
		home := writeHome(t, prefsFile{data: pathPlist(groupLibrary), unreadable: true}, prefsFile{data: pathPlist(containerLibrary)})
		got, err := systemLibraryPath(home)
		checkLibraryPath(t, got, err, containerLibrary, nil)
	})
}
