package photosdb

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func TestSuffixStem(t *testing.T) {
	t.Parallel()

	// Python 3.12: pathlib.PurePath(name).suffix and .stem.
	tests := []struct{ name, suffix, stem string }{
		{"IMG_1.final.jpg", ".jpg", "IMG_1.final"},
		{"a.", "", "a."},
		{".hidden", "", ".hidden"},
		{"noext", "", "noext"},
		{"dir.x/file", "", "file"},
		{"dir/IMG_1.HEIC", ".HEIC", "IMG_1"},
		{"a/b/", "", "b"},
		{"..", "", ".."},
		{"", "", ""},
		{"x..y", ".y", "x."},
	}
	for _, tt := range tests {
		if got := suffix(tt.name); got != tt.suffix {
			t.Errorf("suffix(%q) = %q, want %q", tt.name, got, tt.suffix)
		}
		if got := stem(tt.name); got != tt.stem {
			t.Errorf("stem(%q) = %q, want %q", tt.name, got, tt.stem)
		}
	}
}

func FuzzSuffix(f *testing.F) {
	for _, s := range []string{"IMG_1.final.jpg", "a.", ".hidden", "noext", "dir.x/file", "a/b/", "..", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if got := stem(name) + suffix(name); got != lastElement(name) {
			t.Errorf("stem + suffix of %q = %q, want %q", name, got, lastElement(name))
		}
	})
}

func marked(t *testing.T, lib, marker string) (pk int64, uuid, dir, file string) {
	t.Helper()
	err := roDB(t, lib).QueryRowContext(t.Context(),
		"SELECT a.Z_PK, a.ZUUID, a.ZDIRECTORY, a.ZFILENAME FROM ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK "+
			"WHERE x.ZTITLE = ?", marker).Scan(&pk, &uuid, &dir, &file)
	if err != nil {
		t.Fatalf("marker %s: %v", marker, err)
	}
	return pk, uuid, dir, file
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func pathsLibrary(t *testing.T) (lib, uuid, dir, file string) {
	t.Helper()
	lib = testlib.Open(t, "Test-14.0.0")
	pk, uuid, dir, file := marked(t, lib, "mig:resources-original")
	change(t, lib,
		"UPDATE ZASSET SET ZKINDSUBTYPE = 2 WHERE Z_PK = "+itoa(pk),
		"INSERT INTO ZINTERNALRESOURCE (Z_PK, Z_ENT, Z_OPT, ZASSET, ZDATASTORESUBTYPE, ZRESOURCETYPE, ZLOCALAVAILABILITY) "+
			"SELECT MAX(Z_PK) + 1, MAX(Z_ENT), 1, "+itoa(pk)+", 17, 4, 1 FROM ZINTERNALRESOURCE")
	originals := filepath.Join(lib, "originals", dir)
	writeFile(t, filepath.Join(originals, file))
	writeFile(t, filepath.Join(originals, stem(file)+"_3.mov"))
	writeFile(t, filepath.Join(originals, stem(file)+"_4.DNG"))
	writeFile(t, filepath.Join(originals, stem(file)+"_4b.CR2")) // a later name that also matches
	return lib, uuid, dir, file
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func openPhoto(t *testing.T, lib, uuid string) *photoInfo {
	t.Helper()
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	p, ok := db.Photo(uuid)
	if !ok {
		t.Fatalf("Photo(%s): not found", uuid)
	}
	return p
}

func TestPaths(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()
		lib, uuid, dir, file := pathsLibrary(t)
		p := openPhoto(t, lib, uuid)
		originals := filepath.Join(lib, "originals", dir)
		if want := filepath.Join(originals, file); p.Path != want {
			t.Errorf("Path = %q, want %q", p.Path, want)
		}
		if want := filepath.Join(originals, stem(file)+"_3.mov"); p.PathLive != want {
			t.Errorf("PathLive = %q, want %q", p.PathLive, want)
		}
		if want := filepath.Join(originals, stem(file)+"_4.DNG"); p.PathRaw != want || p.UTIRaw != "com.adobe.raw-image" {
			t.Errorf("PathRaw, UTIRaw = %q, %q; want %q, com.adobe.raw-image", p.PathRaw, p.UTIRaw, want)
		}
	})

	t.Run("not files", func(t *testing.T) {
		t.Parallel()
		lib, uuid, dir, file := pathsLibrary(t)
		originals := filepath.Join(lib, "originals", dir)
		for _, name := range []string{file, stem(file) + "_3.mov", stem(file) + "_4.DNG", stem(file) + "_4b.CR2"} {
			if err := os.Remove(filepath.Join(originals, name)); err != nil {
				t.Fatal(err)
			}
		}
		outside := filepath.Join(t.TempDir(), "outside.jpeg")
		writeFile(t, outside)
		if err := os.Symlink(outside, filepath.Join(originals, file)); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(originals, stem(file)+"_3.mov"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(originals, "real.DNG"))
		if err := os.Symlink("real.DNG", filepath.Join(originals, stem(file)+"_4.DNG")); err != nil {
			t.Fatal(err)
		}
		p := openPhoto(t, lib, uuid)
		if p.Path != "" || p.PathLive != "" || p.PathRaw != "" || p.UTIRaw != "" {
			t.Errorf("paths %q, %q, %q, UTIRaw %q; want all empty", p.Path, p.PathLive, p.PathRaw, p.UTIRaw)
		}
	})

	// A shared photo has no Path here (upstream looks in the shared album
	// directory), but its RAW file is looked up as for any photo
	// (photoinfo.py:532-540).
	t.Run("shared", func(t *testing.T) {
		t.Parallel()
		lib, uuid, dir, file := pathsLibrary(t)
		change(t, lib, "UPDATE ZASSET SET ZCLOUDBATCHPUBLISHDATE = 1 WHERE ZUUID = '"+uuid+"'")
		p := openPhoto(t, lib, uuid)
		if want := filepath.Join(lib, "originals", dir, stem(file)+"_4.DNG"); !p.Shared || p.Path != "" || p.PathLive != "" || p.PathRaw != want {
			t.Errorf("Shared %v, paths %q, %q, %q; want true, \"\", \"\", %q", p.Shared, p.Path, p.PathLive, p.PathRaw, want)
		}
	})

	// The RAW file name may differ from ZFILENAME in case or in Unicode
	// normalization (photoinfo.py:545-558, utils.py:359-368).
	for _, tt := range []struct{ name, dbFile, rawFile string }{
		{"RAW name in lower case", "", ""},
		{"RAW name in NFD, ZFILENAME in NFC", "Caf\U000000e9.jpeg", "Cafe\U00000301_4.DNG"},
		{"RAW name in NFC, ZFILENAME in NFD", "Cafe\U00000301.jpeg", "Caf\U000000e9_4.DNG"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib, uuid, dir, file := pathsLibrary(t)
			originals := filepath.Join(lib, "originals", dir)
			for _, name := range []string{stem(file) + "_4.DNG", stem(file) + "_4b.CR2"} {
				if err := os.Remove(filepath.Join(originals, name)); err != nil {
					t.Fatal(err)
				}
			}
			raw := tt.rawFile
			if tt.dbFile == "" {
				raw = strings.ToLower(stem(file)) + "_4.dng"
				if raw == stem(file)+"_4.dng" {
					t.Fatalf("the stem of %q has no upper-case letter", file)
				}
			} else {
				change(t, lib, "UPDATE ZASSET SET ZFILENAME = '"+tt.dbFile+"' WHERE ZUUID = '"+uuid+"'")
			}
			writeFile(t, filepath.Join(originals, raw))
			if got, want := openPhoto(t, lib, uuid).PathRaw, filepath.Join(originals, raw); got != want {
				t.Errorf("PathRaw = %q, want %q", got, want)
			}
		})
	}

	t.Run("directory cannot be read", func(t *testing.T) {
		t.Parallel()
		lib, uuid, dir, _ := pathsLibrary(t)
		originals := filepath.Join(lib, "originals", dir)
		info, err := os.Stat(originals)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(originals, 0o300); err != nil { //nolint:gosec // G302: a directory that can be searched but not read
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(originals, info.Mode().Perm()) })
		if entries, err := os.ReadDir(originals); err == nil {
			t.Skipf("the directory is readable without the read bit (%d entries): running as root?", len(entries))
		}
		// os.Root opens every directory of a path, so Lstat fails as well as
		// ReadDir: every path is "", and Open does not fail.
		if p := openPhoto(t, lib, uuid); p.Path != "" || p.PathLive != "" || p.PathRaw != "" || p.UTIRaw != "" {
			t.Errorf("paths %q, %q, %q, UTIRaw %q; want all empty", p.Path, p.PathLive, p.PathRaw, p.UTIRaw)
		}
	})

	t.Run("Live video is a directory", func(t *testing.T) {
		t.Parallel()
		lib, uuid, dir, file := pathsLibrary(t)
		live := filepath.Join(lib, "originals", dir, stem(file)+"_3.mov")
		if err := os.Remove(live); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(live, 0o700); err != nil {
			t.Fatal(err)
		}
		if p := openPhoto(t, lib, uuid); p.Path == "" || p.PathLive != "" {
			t.Errorf("Path %q, PathLive %q; want a path and \"\"", p.Path, p.PathLive)
		}
	})

	for _, tt := range []struct{ name, dir string }{
		{"directory with ..", "../originals/"},
		{"absolute directory", "/"},
		{"empty directory", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib, uuid, dir, _ := pathsLibrary(t)
			newDir := tt.dir
			if tt.dir != "" {
				newDir = tt.dir + dir
			}
			if tt.name == "absolute directory" {
				newDir = filepath.Join(lib, "originals", dir)
			}
			change(t, lib, "UPDATE ZASSET SET ZDIRECTORY = '"+strings.ReplaceAll(newDir, "'", "''")+"' WHERE ZUUID = '"+uuid+"'")
			if p := openPhoto(t, lib, uuid); p.Path != "" || p.PathLive != "" || p.PathRaw != "" {
				t.Errorf("ZDIRECTORY %q: paths %q, %q, %q; want all empty", newDir, p.Path, p.PathLive, p.PathRaw)
			}
		})
	}

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		lib := testlib.Open(t, "Test-14.0.0")
		pk, uuid, dir, file := marked(t, lib, "mig:resources-missing")
		change(t, lib,
			"UPDATE ZASSET SET ZKINDSUBTYPE = 2 WHERE Z_PK = "+itoa(pk),
			"INSERT INTO ZINTERNALRESOURCE (Z_PK, Z_ENT, Z_OPT, ZASSET, ZDATASTORESUBTYPE, ZRESOURCETYPE, ZLOCALAVAILABILITY) "+
				"SELECT MAX(Z_PK) + 1, MAX(Z_ENT), 1, "+itoa(pk)+", 17, 4, 1 FROM ZINTERNALRESOURCE")
		originals := filepath.Join(lib, "originals", dir)
		for _, name := range []string{file, stem(file) + "_3.mov", stem(file) + "_4.DNG"} {
			writeFile(t, filepath.Join(originals, name))
		}
		p := openPhoto(t, lib, uuid)
		if !p.IsMissing || p.Path != "" || p.PathLive != "" || p.PathRaw != "" {
			t.Errorf("IsMissing %v, paths %q, %q, %q; want true and all empty", p.IsMissing, p.Path, p.PathLive, p.PathRaw)
		}
	})

	t.Run("no originals directory", func(t *testing.T) {
		t.Parallel()
		db, err := Open(t.Context(), testlib.OpenPristine(t, "Test-14.0.0"), Options{TempDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range db.Photos() {
			if p.Path != "" || p.PathLive != "" || p.PathRaw != "" || p.UTIRaw != "" {
				t.Fatalf("%s: paths %q, %q, %q without originals", p.UUID, p.Path, p.PathLive, p.PathRaw)
			}
		}
	})

	t.Run("originals is a file", func(t *testing.T) {
		t.Parallel()
		lib := testlib.OpenPristine(t, "Test-14.0.0")
		writeFile(t, filepath.Join(lib, "originals"))
		_, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
		if err == nil || !strings.Contains(err.Error(), lib) {
			t.Errorf("Open error %v, want one naming the library", err)
		}
	})
}
