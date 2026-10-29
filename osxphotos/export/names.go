package export

import (
	"cmp"
	"path"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/uti"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// previewSuffix is the default preview_suffix of ExportOptions
// (_constants.py:646), the only one ported.
const previewSuffix = "_preview"

// Files are the names osxphotos export gives the companions of the file
// name of one photo, exported into one directory. name is the name of the
// exported photo after its collision is resolved: upstream builds every
// companion from dest.stem after _get_dest_path (photoexporter.py:308, 371,
// 392, 413-442, 445-471, 476-479, 493-500, 1923-1929). Call IncrementName
// for the name of the photo first, then Names(p, resolved); for an export of
// the edited photo, resolve EditedName(p) the same way. Preview is numbered
// once more on its own upstream (457-461) unless the export overwrites or
// updates. stem is name without its last extension, as pathlib.Path.stem, in
// NFC; upstream normalizes some of the names to NFD on macOS (unicode.py:18).
// A name is "" only when upstream names no such file for p; Preview is never
// "", and RAW and AAE are named even when the file is missing on disk, as
// upstream reports them missing (438, 491).
type Files struct {
	Live        string // stem + ".mov" when Live (371, 392; _constants.py:683)
	RAW         string // stem + the extension of the base name of PathRaw; without PathRaw, stem + "." + the preferred extension of UTIRaw in the table of uti.py (uti.py:583-614), or stem + ".raw" (413-431); "" without HasRAW
	Preview     string // stem + "_preview" + the extension of the base name of PathDerivatives[0]; without previews, ".jpeg" (445-471)
	AAE         string // stem + ".AAE" when HasAdjustments (476-479); upstream exports it with the original only (476)
	OriginalAAE string // stem + "_O.AAE" when OriginalAdjustmentsPath is not "" (493-500, 1923-1929); original only (494)
}

// Names returns the companions of the file name of p. name is not empty: the
// caller passes OriginalFilename (302) or EditedName(p) (297-300), numbered by
// IncrementName first (308); an empty name gives companions with an empty
// stem (".mov", "_preview.jpeg"). name and the names in p come from outside:
// the caller sanitizes them before it writes a file.
func Names(p *photoinfo.PhotoInfo, name string) Files {
	stem, _ := splitSuffix(norm.NFC.String(name))
	f := Files{Preview: stem + previewSuffix + ".jpeg"}
	if p.Live {
		f.Live = stem + ".mov"
	}
	if p.HasRAW {
		f.RAW = stem + rawSuffix(p)
	}
	if len(p.PathDerivatives) > 0 {
		f.Preview = stem + previewSuffix + baseSuffix(p.PathDerivatives[0])
	}
	if p.HasAdjustments {
		f.AAE = stem + ".AAE"
	}
	if p.OriginalAdjustmentsPath != "" {
		f.OriginalAAE = stem + "_O.AAE"
	}
	return f
}

// EditedName is the default name of an export of the edited photo
// (_get_edited_filename, photoexporter.py:297-300, 554-565): the stem of
// OriginalFilename + "_edited" + the extension of the base name of
// PathEdited, or "." + the preferred extension of UTIEdited (UTI when
// UTIEdited is "") in the table of uti.py (uti.py:583-614), which is
// "_edited." when the table has no extension, as f".{ext}" upstream. An
// export with a given filename names the edited file as is (298).
// HasAdjustments is not checked: the caller decides what to export.
// OriginalFilename and PathEdited come from outside: the caller sanitizes the
// result before it writes a file.
func EditedName(p *photoinfo.PhotoInfo) string {
	stem, _ := splitSuffix(p.OriginalFilename)
	ext := "." + uti.PreferredExtension(cmp.Or(p.UTIEdited, p.UTI))
	if p.PathEdited != "" {
		ext = baseSuffix(p.PathEdited)
	}
	return norm.NFC.String(stem + "_edited" + ext)
}

// rawSuffix follows photoexporter.py:413-431.
func rawSuffix(p *photoinfo.PhotoInfo) string {
	if p.PathRaw != "" {
		return baseSuffix(p.PathRaw)
	}
	return "." + cmp.Or(uti.PreferredExtension(p.UTIRaw), "raw")
}

// baseSuffix takes the suffix of the last element only: a dot in a directory
// name is no extension (pathlib.Path(p).suffix).
func baseSuffix(p string) string {
	_, suffix := splitSuffix(path.Base(p))
	return suffix
}

// splitSuffix follows pathlib.PurePath.stem and .suffix.
func splitSuffix(name string) (stem, suffix string) {
	if i := strings.LastIndexByte(name, '.'); i > 0 && i < len(name)-1 {
		return name[:i], name[i:]
	}
	return name, ""
}
