package photosdb

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/internal/uti"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

const originalsDir = "originals"

// Files are looked up only inside libraryPath/originals, through os.Root: a
// path never leaves that directory and a symbolic link is not a file. Only
// Lstat and ReadDir are used; no file is opened for reading. An error of
// Lstat or ReadDir makes the path "" and is not returned: its text holds
// names from the database.
func setPaths(libraryPath string, assets []sqlread.Asset, c *catalog) error {
	root, err := os.OpenRoot(filepath.Join(libraryPath, originalsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("photosdb: %s: the originals directory: %w", libraryPath, err)
	}
	defer func() { _ = root.Close() }() // opened for reading only
	o := &originals{lib: libraryPath, root: root, fsys: root.FS(), dirs: map[string]*dirNames{}}
	for _, a := range assets {
		// An asset without attributes is not in c, and its ZUUID may be that
		// of a photo.
		if a.HasAttributes {
			o.set(c.byUUID[a.UUID.String], a.Directory.String)
		}
	}
	return nil
}

type originals struct {
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
func (o *originals) set(p *photoinfo.PhotoInfo, dir string) {
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

// dir and file come from the database: a path that could leave the root is
// refused.
func relName(dir, file string) (string, bool) {
	name := dir + "/" + file
	if dir == "" || file == "" || strings.ContainsRune(name, 0) || !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}

func (o *originals) isFile(name string) bool {
	info, err := o.root.Lstat(name)
	return err == nil && info.Mode().IsRegular()
}

func (o *originals) abs(name string) string {
	return filepath.Join(o.lib, originalsDir, filepath.FromSlash(name))
}

// rawName finds the RAW file of a RAW+JPEG pair: the first name of dir, in
// name order, that starts with <stem of file>_4, compared in NFD and lower
// case (photoinfo.py:545-558, utils.py:327-396).
func (o *originals) rawName(dir, file string) (string, bool) {
	if _, ok := relName(dir, file); !ok {
		return "", false
	}
	d, ok := o.dirs[dir]
	if !ok {
		d = readNames(o.fsys, dir)
		o.dirs[dir] = d
	}
	if d == nil {
		return "", false
	}
	prefix := fold(stem(file) + "_4")
	for i, f := range d.folded {
		if strings.HasPrefix(f, prefix) {
			return dir + "/" + d.names[i], true
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
