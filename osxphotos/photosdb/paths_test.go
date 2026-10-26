package photosdb

import (
	"os"
	"path/filepath"
	"slices"
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
		lib := testlib.Open(t, "Test-14.0.0")
		_, uuid, _, _ := marked(t, lib, "mig:resources-original")
		change(t, lib, "UPDATE ZASSET SET ZHASADJUSTMENTS = 1 WHERE ZUUID = '"+uuid+"'")
		writeFile(t, filepath.Join(lib, "originals"))
		want := filepath.Join(lib, "resources", "renders", uuid[:1], uuid+".plist")
		writeFile(t, want)
		if p := openPhoto(t, lib, uuid); p.AdjustmentsPath != want || p.Path != "" || p.OriginalAdjustmentsPath != "" {
			t.Errorf("AdjustmentsPath %q, Path %q, OriginalAdjustmentsPath %q; want %q, \"\", \"\"",
				p.AdjustmentsPath, p.Path, p.OriginalAdjustmentsPath, want)
		}
	})

	t.Run("library cannot be opened", func(t *testing.T) {
		t.Parallel()
		lib := filepath.Join(t.TempDir(), "none")
		if err := setPaths(lib, nil, nil); err == nil || !strings.Contains(err.Error(), lib) {
			t.Errorf("setPaths error %v, want one naming the library", err)
		}
	})
}

func editedLibrary(t *testing.T, asset, attrs string) (lib, uuid string) {
	t.Helper()
	lib, uuid, _, _ = pathsLibrary(t)
	stmts := []string{"UPDATE ZASSET SET " + asset + " WHERE ZUUID = '" + uuid + "'"}
	if attrs != "" {
		stmts = append(stmts, "UPDATE ZADDITIONALASSETATTRIBUTES SET "+attrs+
			" WHERE ZASSET = (SELECT Z_PK FROM ZASSET WHERE ZUUID = '"+uuid+"')")
	}
	change(t, lib, stmts...)
	return lib, uuid
}

func editedWrite(t *testing.T, lib, rel string, size int) {
	t.Helper()
	name := filepath.Join(lib, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(strings.Repeat("x", size)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func editedMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestEditedPaths(t *testing.T) {
	t.Parallel()

	allRenders := []string{"_1_201_a.heic", "_1_201_a.jpeg", "_2_0_a.mov", "_2_100_a.mov", ".plist"}
	const (
		heic = "ZHASADJUSTMENTS = 1, ZUNIFORMTYPEIDENTIFIER = 'public.heic'"
		jpeg = "ZHASADJUSTMENTS = 1, ZUNIFORMTYPEIDENTIFIER = 'public.jpeg'"
	)
	tests := []struct {
		name, asset                           string
		renders                               []string
		aae                                   bool
		editedSuffix, plistSuffix, liveSuffix string
	}{
		{"HEIC photo", "ZKIND = 0, ZKINDSUBTYPE = 2, " + heic, allRenders, true, "_1_201_a.heic", ".plist", "_2_100_a.mov"},
		{"JPEG photo", "ZKIND = 0, ZKINDSUBTYPE = 2, " + jpeg, allRenders, true, "_1_201_a.jpeg", ".plist", "_2_100_a.mov"},
		{"video", "ZKIND = 1, ZKINDSUBTYPE = 2, " + jpeg, allRenders, true, "_2_0_a.mov", ".plist", "_2_100_a.mov"},
		{"neither photo nor video", "ZKIND = 2, ZKINDSUBTYPE = 2, " + jpeg, allRenders, true, "", ".plist", "_2_100_a.mov"},
		{"not adjusted", "ZKIND = 0, ZKINDSUBTYPE = 2, ZHASADJUSTMENTS = 0", allRenders, true, "", "", ""},
		{"not Live", "ZKIND = 0, ZKINDSUBTYPE = 0, " + jpeg, allRenders, true, "_1_201_a.jpeg", ".plist", ""},
		{"no files", "ZKIND = 0, ZKINDSUBTYPE = 2, " + jpeg, nil, false, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib, uuid := editedLibrary(t, tt.asset, "")
			x := uuid[:1]
			for _, f := range tt.renders {
				editedWrite(t, lib, "resources/renders/"+x+"/"+uuid+f, 1)
			}
			aae := ""
			if tt.aae {
				editedWrite(t, lib, "originals/"+x+"/"+uuid+"_5.aae", 1)
				aae = filepath.Join(lib, "originals", x, uuid+"_5.aae")
			}
			render := func(suffix string) string {
				if suffix == "" {
					return ""
				}
				return filepath.Join(lib, "resources", "renders", x, uuid+suffix)
			}
			p := openPhoto(t, lib, uuid)
			got := []string{p.PathEdited, p.AdjustmentsPath, p.PathEditedLive, p.OriginalAdjustmentsPath}
			want := []string{render(tt.editedSuffix), render(tt.plistSuffix), render(tt.liveSuffix), aae}
			if !slices.Equal(got, want) {
				t.Errorf("PathEdited, AdjustmentsPath, PathEditedLive, OriginalAdjustmentsPath = %q, want %q", got, want)
			}
		})
	}

	outsideFile := filepath.Join(t.TempDir(), "outside.jpeg")
	writeFile(t, outsideFile)
	for _, tt := range []struct {
		name   string
		create func(t *testing.T, name string)
	}{
		{"links inside the library are not files", func(t *testing.T, name string) {
			writeFile(t, filepath.Join(filepath.Dir(name), "regular"))
			editedSymlink(t, "regular", name)
		}},
		{"links out of the library are not files", func(t *testing.T, name string) { editedSymlink(t, outsideFile, name) }},
		{"directories are not files", editedMkdir},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib, uuid := editedLibrary(t, "ZKIND = 0, ZKINDSUBTYPE = 2, "+jpeg, "")
			for _, name := range libraryFileNames(lib, uuid) {
				tt.create(t, name)
			}
			assertNoLibraryFiles(t, openPhoto(t, lib, uuid))
		})
	}

	// A ZUUID is not checked for its form, and os.Root lets ".." through while
	// the path stays inside the library.
	t.Run("UUID with a slash", func(t *testing.T) {
		t.Parallel()
		lib, uuid := editedLibrary(t, "ZKIND = 0, ZKINDSUBTYPE = 2, "+jpeg, "")
		x := uuid[:1]
		bad := x + "/../" + uuid
		change(t, lib, "UPDATE ZASSET SET ZUUID = '"+bad+"' WHERE ZUUID = '"+uuid+"'")
		for _, name := range libraryFileNames(lib, uuid) {
			writeFile(t, name)
			editedMkdir(t, filepath.Join(filepath.Dir(name), x))
		}
		assertNoLibraryFiles(t, openPhoto(t, lib, bad))
	})

	t.Run("missing photo", func(t *testing.T) {
		t.Parallel()
		lib := testlib.Open(t, "Test-14.0.0")
		_, uuid, _, _ := marked(t, lib, "mig:resources-missing")
		change(t, lib, "UPDATE ZASSET SET ZKIND = 0, "+jpeg+" WHERE ZUUID = '"+uuid+"'")
		want := filepath.Join(lib, "resources", "renders", uuid[:1], uuid+"_1_201_a.jpeg")
		writeFile(t, want)
		if p := openPhoto(t, lib, uuid); !p.IsMissing || p.PathEdited != want {
			t.Errorf("IsMissing %v, PathEdited %q; want true, %q", p.IsMissing, p.PathEdited, want)
		}
	})

	t.Run("directories link outside", func(t *testing.T) {
		t.Parallel()
		lib, uuid := editedLibrary(t, "ZKIND = 0, ZKINDSUBTYPE = 2, "+jpeg, "")
		x := uuid[:1]
		outside := t.TempDir()
		editedWrite(t, outside, uuid+"_1_201_a.jpeg", 1)
		editedWrite(t, outside, uuid+"_1.jpeg", 1)
		for _, dir := range []string{"resources/renders", "resources/derivatives"} {
			editedSymlink(t, outside, filepath.Join(lib, filepath.FromSlash(dir), x))
		}
		p := openPhoto(t, lib, uuid)
		if p.PathEdited != "" || len(p.PathDerivatives) != 0 {
			t.Errorf("paths %q, %q; want both empty", p.PathEdited, p.PathDerivatives)
		}
	})
}

func editedSymlink(t *testing.T, target, name string) {
	t.Helper()
	editedMkdir(t, filepath.Dir(name))
	if err := os.Symlink(target, name); err != nil {
		t.Fatal(err)
	}
}

func libraryFileNames(lib, uuid string) []string {
	x := uuid[:1]
	renders := filepath.Join(lib, "resources", "renders", x, uuid)
	derivatives := filepath.Join(lib, "resources", "derivatives")
	return []string{
		renders + "_1_201_a.jpeg",
		renders + "_2_100_a.mov",
		renders + ".plist",
		filepath.Join(lib, "originals", x, uuid+"_5.aae"),
		filepath.Join(derivatives, x, uuid+"_1.jpeg"),
		filepath.Join(derivatives, "masters", x, uuid+"_4_5005_c.jpeg"),
	}
}

func assertNoLibraryFiles(t *testing.T, p *photoInfo) {
	t.Helper()
	if p.PathEdited != "" || p.AdjustmentsPath != "" || p.PathEditedLive != "" || p.OriginalAdjustmentsPath != "" || len(p.PathDerivatives) != 0 {
		t.Errorf("paths %q, %q, %q, %q, %q; want all empty",
			p.PathEdited, p.AdjustmentsPath, p.PathEditedLive, p.OriginalAdjustmentsPath, p.PathDerivatives)
	}
}

func TestPathDerivatives(t *testing.T) {
	t.Parallel()

	const (
		regular   = "resources/derivatives/{x}/"
		masters   = "resources/derivatives/masters/{x}/{u}_4_5005_c.jpeg"
		moment    = "scopes/momentshared/resources/derivatives/"
		syndicate = "scopes/syndication/resources/derivatives/"
		shared    = "scopes/cloudsharing/resources/derivatives/masters/{x}/{u}_4_5005_c.jpeg"
		notSaved  = "ZSYNDICATIONIDENTIFIER = 'TEST-SYND', ZSYNDICATIONHISTORY = 0"
		saved     = "ZSYNDICATIONIDENTIFIER = 'TEST-SYND', ZSYNDICATIONHISTORY = 1"
	)
	type file struct {
		rel  string
		size int
	}
	tests := []struct {
		name, asset, attrs string
		files              []file
		want               []string
	}{
		{"largest first, the thumbnail last", "ZKIND = 0", "", []file{
			{regular + "{u}_1_100_c.jpeg", 200},
			{regular + "{u}_1_105_c.jpeg", 300},
			{regular + "{u}_1_101_c.THM", 900},
			{regular + "{u}_1_102_c.thm", 800},
			{regular + "{l}_1_103_c.jpeg", 700},
			{regular + "other_1.jpeg", 600},
			{masters, 1000},
		}, []string{regular + "{u}_1_105_c.jpeg", regular + "{u}_1_100_c.jpeg", masters}},
		{"equal sizes in name order", "ZKIND = 0", "", []file{
			{regular + "{u}_1_105_c.jpeg", 300},
			{regular + "{u}_1_104_c.jpeg", 300},
		}, []string{regular + "{u}_1_104_c.jpeg", regular + "{u}_1_105_c.jpeg"}},
		{"video of a photo goes second", "ZKIND = 0", "", []file{
			{regular + "{u}_2_0_c.MOV", 500},
			{regular + "{u}_1_0_c.jpeg", 100},
		}, []string{regular + "{u}_1_0_c.jpeg", regular + "{u}_2_0_c.MOV"}},
		{"video before the thumbnail", "ZKIND = 0", "", []file{
			{regular + "{u}_2_0_c.mov", 500},
			{masters, 50},
		}, []string{masters, regular + "{u}_2_0_c.mov"}},
		{"a single video", "ZKIND = 0", "", []file{
			{regular + "{u}_2_0_c.mov", 500},
		}, []string{regular + "{u}_2_0_c.mov"}},
		{"video of a video stays first", "ZKIND = 1", "", []file{
			{regular + "{u}_2_0_c.MOV", 500},
			{regular + "{u}_1_0_c.jpeg", 100},
		}, []string{regular + "{u}_2_0_c.MOV", regular + "{u}_1_0_c.jpeg"}},
		{"shared", "ZKIND = 0, ZCLOUDBATCHPUBLISHDATE = 1", "", []file{
			{shared, 10},
			{"scopes/cloudsharing/resources/derivatives/masters/{x}/{u}_other.jpeg", 20},
			{regular + "{u}_1.jpeg", 100},
		}, []string{shared}},
		{"shared without a thumbnail", "ZKIND = 0, ZCLOUDBATCHPUBLISHDATE = 1", "", []file{
			{"scopes/cloudsharing/resources/derivatives/masters/{x}/{u}_other.jpeg", 20},
			{regular + "{u}_1.jpeg", 100},
		}, nil},
		{"shared and shared moment", "ZKIND = 0, ZCLOUDBATCHPUBLISHDATE = 1, ZMOMENTSHARE = 1", "", []file{
			{shared, 10},
			{moment + "{x}/{u}_1.jpeg", 100},
		}, []string{shared}},
		{"shared moment", "ZKIND = 0, ZMOMENTSHARE = 1", "", []file{
			{moment + "{x}/{u}_1.jpeg", 100},
			{moment + "masters/{x}/{u}_4_5005_c.jpeg", 10},
			{regular + "{u}_2.jpeg", 500},
		}, []string{moment + "{x}/{u}_1.jpeg", moment + "masters/{x}/{u}_4_5005_c.jpeg"}},
		{"shared moment, shared with you, not saved", "ZKIND = 0, ZMOMENTSHARE = 1", notSaved, []file{
			{moment + "{x}/{u}_1.jpeg", 100},
			{moment + "masters/{x}/{u}_4_5005_c.jpeg", 10},
			{syndicate + "{x}/{u}_2.jpeg", 500},
			{syndicate + "masters/{x}/{u}_4_5005_c.jpeg", 20},
		}, []string{moment + "{x}/{u}_1.jpeg", moment + "masters/{x}/{u}_4_5005_c.jpeg"}},
		{"shared with you, not saved", "ZKIND = 0", notSaved, []file{
			{syndicate + "{x}/{u}_1.jpeg", 100},
			{syndicate + "masters/{x}/{u}_4_5005_c.jpeg", 10},
			{regular + "{u}_2.jpeg", 500},
		}, []string{syndicate + "{x}/{u}_1.jpeg", syndicate + "masters/{x}/{u}_4_5005_c.jpeg"}},
		{"shared with you, saved", "ZKIND = 0", saved, []file{
			{syndicate + "{x}/{u}_1.jpeg", 100},
			{regular + "{u}_2.jpeg", 500},
		}, []string{regular + "{u}_2.jpeg"}},
		{"no files", "ZKIND = 0", "", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lib, uuid := editedLibrary(t, tt.asset, tt.attrs)
			if strings.ToLower(uuid) == uuid {
				t.Fatalf("the UUID %q has no upper-case letter", uuid)
			}
			expand := strings.NewReplacer("{x}", uuid[:1], "{u}", uuid, "{l}", strings.ToLower(uuid)).Replace
			for _, f := range tt.files {
				editedWrite(t, lib, expand(f.rel), f.size)
			}
			want := make([]string, len(tt.want))
			for i, rel := range tt.want {
				want[i] = filepath.Join(lib, filepath.FromSlash(expand(rel)))
			}
			if got := openPhoto(t, lib, uuid).PathDerivatives; !slices.Equal(got, want) {
				t.Errorf("PathDerivatives = %q, want %q", got, want)
			}
		})
	}

	t.Run("directory cannot be read", func(t *testing.T) {
		t.Parallel()
		lib, uuid := editedLibrary(t, "ZKIND = 0", "")
		x := uuid[:1]
		editedWrite(t, lib, "resources/derivatives/"+x+"/"+uuid+"_1.jpeg", 10)
		editedWrite(t, lib, "resources/derivatives/masters/"+x+"/"+uuid+"_4_5005_c.jpeg", 1)
		dir := filepath.Join(lib, "resources", "derivatives", x)
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o300); err != nil { //nolint:gosec // G302: a directory that can be searched but not read
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, info.Mode().Perm()) })
		if entries, err := os.ReadDir(dir); err == nil {
			t.Skipf("the directory is readable without the read bit (%d entries): running as root?", len(entries))
		}
		want := []string{filepath.Join(lib, "resources", "derivatives", "masters", x, uuid+"_4_5005_c.jpeg")}
		if got := openPhoto(t, lib, uuid).PathDerivatives; !slices.Equal(got, want) {
			t.Errorf("PathDerivatives = %q, want %q", got, want)
		}
	})
}
