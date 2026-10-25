package photosdb

import (
	"cmp"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/uti"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

const (
	originalsDir = "originals"

	// _constants.py:352.
	sharedDerivativesDir = "scopes/cloudsharing/resources/derivatives/masters"

	thumbSuffix = "_4_5005_c.jpeg"
)

// Errors of Lstat and ReadDir are dropped, not returned: their text holds
// names from the database.
func setPaths(libraryPath string, assets []sqlread.Asset, c *catalog) error {
	root, err := os.OpenRoot(libraryPath)
	if err != nil {
		return fmt.Errorf("photosdb: %s: the library directory: %w", libraryPath, err)
	}
	defer func() { _ = root.Close() }() // opened for reading only
	o := &libraryFiles{lib: libraryPath, root: root, fsys: root.FS(), dirs: map[string]*dirNames{}}
	for _, a := range assets {
		// An asset without attributes is not in c, and its ZUUID may be that
		// of a photo.
		if a.HasAttributes {
			p := c.byUUID[a.UUID.String]
			o.set(p, a.Directory.String)
			o.setLibraryFiles(p)
		}
	}
	return nil
}

type libraryFiles struct {
	lib  string
	root *os.Root
	fsys fs.FS
	dirs map[string]*dirNames // nil when the directory cannot be read
}

type dirNames struct {
	names, folded []string
}

// set follows photoinfo.py:189-263, 514-572, 1051-1076. Shared photos and
// "Shared with you" photos not saved to the library have no Path and
// PathLive here; upstream looks for them elsewhere (photoinfo.py:211-225),
// but not for their RAW file (photoinfo.py:532-540). The shared moment
// directory is not searched (photoinfo.py:213-222).
func (o *libraryFiles) set(p *photoinfo.PhotoInfo, dir string) {
	if p.IsMissing {
		return
	}
	if name, ok := relName(dir, p.Filename); ok && !p.Shared && (!p.Syndicated || p.SavedToLibrary) && o.isFile(name) {
		p.Path = o.abs(name)
		if p.Live && !p.IsReference {
			if live := path.Join(path.Dir(name), stem(name)+"_3.mov"); o.isFile(live) {
				p.PathLive = o.abs(live)
			}
		}
	}
	if p.HasRAW && !p.IsReference {
		if name, ok := o.rawName(dir, p.Filename); ok && o.isFile(name) {
			p.PathRaw = o.abs(name)
			p.UTIRaw = uti.ForExtension(suffix(name))
		}
	}
}

// setLibraryFiles follows photoinfo.py:323-382, 495-512, 743-781, 1172-1274.
// The files do not depend on IsMissing, as upstream.
func (o *libraryFiles) setLibraryFiles(p *photoinfo.PhotoInfo) {
	// A "/" would name a file of another directory.
	if strings.ContainsRune(p.UUID, '/') {
		return
	}
	r, _ := utf8.DecodeRuneInString(p.UUID)
	x := string(r)
	render := "resources/renders/" + x + "/" + p.UUID
	if p.HasAdjustments {
		switch {
		case p.IsPhoto && p.UTI == "public.heic":
			p.PathEdited = o.found(render + "_1_201_a.heic")
		case p.IsPhoto:
			p.PathEdited = o.found(render + "_1_201_a.jpeg")
		case p.IsMovie:
			p.PathEdited = o.found(render + "_2_0_a.mov")
		}
		p.AdjustmentsPath = o.found(render + ".plist")
		if p.Live {
			p.PathEditedLive = o.found(render + "_2_100_a.mov")
		}
	}
	p.OriginalAdjustmentsPath = o.found(originalsDir + "/" + x + "/" + p.UUID + "_5.aae")
	p.PathDerivatives = o.derivatives(p, x)
}

type derivative struct {
	path string
	size int64
}

// derivatives follows photoinfo.py:1172-1229, 1260-1274 and utils.py:734-779.
// Upstream lists a directory in the order of the file system, so the order of
// files of one size is that of the names here.
func (o *libraryFiles) derivatives(p *photoinfo.PhotoInfo, x string) []string {
	if p.Shared {
		if thumb := o.found(sharedDerivativesDir + "/" + x + "/" + p.UUID + thumbSuffix); thumb != "" {
			return []string{thumb}
		}
		return nil
	}
	base := "resources/derivatives"
	switch {
	case p.SharedMoment:
		base = "scopes/momentshared/resources/derivatives"
	case p.Syndicated && !p.SavedToLibrary:
		base = "scopes/syndication/resources/derivatives"
	}
	out := o.previews(base+"/"+x, p.UUID)
	if thumb := o.found(base + "/masters/" + x + "/" + p.UUID + thumbSuffix); thumb != "" {
		out = append(out, thumb)
	}
	// A photo's first preview is an image even when the video of a Live photo
	// is larger (photoinfo.py:1221-1227; the upstream comment states the
	// opposite of its code).
	if p.IsPhoto && len(out) > 1 && (strings.HasSuffix(out[0], ".mov") || strings.HasSuffix(out[0], ".MOV")) {
		out[0], out[1] = out[1], out[0]
	}
	return out
}

// previews is find_files_by_prefix of utils.py:734-779 with ".THM" ignored.
func (o *libraryFiles) previews(dir, prefix string) []string {
	d := o.names(dir)
	if d == nil {
		return nil
	}
	var files []derivative
	i, _ := slices.BinarySearch(d.names, prefix) // the names are sorted
	for ; i < len(d.names) && strings.HasPrefix(d.names[i], prefix); i++ {
		name := dir + "/" + d.names[i]
		if info, err := o.root.Lstat(name); err == nil && info.Mode().IsRegular() && !strings.EqualFold(path.Ext(name), ".THM") {
			files = append(files, derivative{o.abs(name), info.Size()})
		}
	}
	slices.SortStableFunc(files, func(a, b derivative) int { return cmp.Compare(b.size, a.size) })
	var out []string
	for _, f := range files {
		out = append(out, f.path)
	}
	return out
}

// dir and file come from the database: a path that could leave the root is
// refused.
func relName(dir, file string) (string, bool) {
	name := dir + "/" + file
	if dir == "" || file == "" || strings.ContainsRune(name, 0) || !fs.ValidPath(name) {
		return "", false
	}
	return originalsDir + "/" + name, true
}

func (o *libraryFiles) isFile(name string) bool {
	info, err := o.root.Lstat(name)
	return err == nil && info.Mode().IsRegular()
}

func (o *libraryFiles) found(name string) string {
	if !o.isFile(name) {
		return ""
	}
	return o.abs(name)
}

func (o *libraryFiles) abs(name string) string {
	return filepath.Join(o.lib, filepath.FromSlash(name))
}

func (o *libraryFiles) names(dir string) *dirNames {
	d, ok := o.dirs[dir]
	if !ok {
		d = readNames(o.fsys, dir)
		o.dirs[dir] = d
	}
	return d
}

// rawName follows photoinfo.py:545-558 and utils.py:327-396.
func (o *libraryFiles) rawName(dir, file string) (string, bool) {
	if _, ok := relName(dir, file); !ok {
		return "", false
	}
	full := originalsDir + "/" + dir
	d := o.names(full)
	if d == nil {
		return "", false
	}
	prefix := fold(stem(file) + "_4")
	for i, f := range d.folded {
		if strings.HasPrefix(f, prefix) {
			return full + "/" + d.names[i], true
		}
	}
	return "", false
}

func readNames(fsys fs.FS, dir string) *dirNames {
	entries, err := fs.ReadDir(fsys, dir) // sorted by name
	if err != nil {
		return nil
	}
	d := &dirNames{names: make([]string, len(entries)), folded: make([]string, len(entries))}
	for i, e := range entries {
		d.names[i] = e.Name()
		d.folded[i] = fold(e.Name())
	}
	return d
}

// fold is the form in which upstream compares file names on macOS: NFD,
// then lower case (unicode.py:18, 67-73; utils.py:359-368).
func fold(s string) string { return strings.ToLower(norm.NFD.String(s)) }

// lastElement is the last element of a slash-separated path, as the name of
// a Python pathlib path: trailing slashes are ignored.
func lastElement(p string) string {
	p = strings.TrimRight(p, "/")
	return p[strings.LastIndex(p, "/")+1:]
}

// suffix is pathlib.PurePath.suffix of Python 3.12: the part of the last
// element from its last ".", when that "." is neither the first nor the last
// character; otherwise "". filepath.Ext differs for "a." and ".x".
func suffix(p string) string {
	name := lastElement(p)
	if i := strings.LastIndex(name, "."); 0 < i && i < len(name)-1 {
		return name[i:]
	}
	return ""
}

// stem is pathlib.PurePath.stem of Python 3.12: the last element without
// its suffix.
func stem(p string) string {
	name := lastElement(p)
	return strings.TrimSuffix(name, suffix(name))
}
