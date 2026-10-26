package photosdb

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/adjustmentsinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Adjustments returns the edit data in the AdjustmentsPath of p
// (photoinfo.py:783-793), read on every call. It is nil when AdjustmentsPath
// is "" or the file is no longer a regular file inside the library or cannot
// be read.
func (db *DB) Adjustments(p *photoinfo.PhotoInfo) *adjustmentsinfo.AdjustmentsInfo {
	return db.readAdjustments(p.AdjustmentsPath)
}

// OriginalAdjustments is Adjustments of OriginalAdjustmentsPath
// (photoinfo.py:795-800).
func (db *DB) OriginalAdjustments(p *photoinfo.PhotoInfo) *adjustmentsinfo.AdjustmentsInfo {
	return db.readAdjustments(p.OriginalAdjustmentsPath)
}

// The file may have changed since Open, so it is checked again as by setPaths.
// Lstat comes first because os.Root follows a symbolic link inside the library;
// O_NONBLOCK keeps a named pipe swapped in after Lstat from blocking the open.
func (db *DB) readAdjustments(path string) *adjustmentsinfo.AdjustmentsInfo {
	rel, err := filepath.Rel(db.info.LibraryPath, path)
	if path == "" || err != nil {
		return nil
	}
	root, err := os.OpenRoot(db.info.LibraryPath)
	if err != nil {
		return nil
	}
	defer func() { _ = root.Close() }() // opened for reading only
	if info, err := root.Lstat(rel); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }() // opened for reading only
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(f, maxPlistSize+1))
	if err != nil {
		return nil
	}
	return decodeAdjustments(data)
}

// decodeAdjustments follows adjustmentsinfo.py:35-52, 117-132 and 169-208.
func decodeAdjustments(file []byte) *adjustmentsinfo.AdjustmentsInfo {
	dict, _ := plistAny(file).(map[string]any)
	a := &adjustmentsinfo.AdjustmentsInfo{Plist: dict, BaseVersion: dict["adjustmentBaseVersion"], FormatVersion: dict["adjustmentFormatVersion"]}
	a.Editor, _ = dict["adjustmentEditorBundleID"].(string)
	a.FormatID, _ = dict["adjustmentFormatIdentifier"].(string)
	if t, ok := dict["adjustmentTimestamp"].(time.Time); ok {
		t = t.UTC() // adjustmentsinfo.py:44-46 is always UTC
		a.Timestamp = &t
	}
	a.Data, _ = dict["adjustmentData"].([]byte)
	if s, ok := dict["adjustmentData"].(string); ok {
		a.Data = []byte(s)
	}
	data, _ := decodeAdjustmentData(a.Data).(map[string]any)
	a.Adjustments, _ = data["adjustments"].([]any)
	a.Metadata, _ = data["metadata"].(map[string]any)
	a.Orientation = 0
	if v, ok := a.Metadata["orientation"]; ok {
		a.Orientation = v
	}
	a.AdjustmentFormatVersion = data["formatVersion"]
	a.VersionInfo = data["versionInfo"]
	return a
}

// decodeAdjustmentData follows adjustmentsinfo.py:54-115. Unlike upstream,
// inflated data over maxPlistSize is not decoded: the file comes from
// outside, and 1 MiB of it can inflate to a gigabyte.
func decodeAdjustmentData(data []byte) any {
	if v := decodeBytes(data); v != nil {
		return v
	}
	if v := decodeBytes(inflate(flate.NewReader(bytes.NewReader(data)))); v != nil {
		return v
	}
	z, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return decodeBytes(inflate(z))
}

func inflate(r io.Reader) []byte {
	b, err := io.ReadAll(io.LimitReader(r, maxPlistSize+1))
	if err != nil || len(b) > maxPlistSize {
		return nil
	}
	return b
}

func decodeBytes(b []byte) any {
	v := plistAny(b)
	if v == nil && json.Unmarshal(b, &v) != nil {
		return nil
	}
	return v
}

func plistAny(data []byte) any {
	var v any
	if len(data) > maxPlistSize || decodePlist(data, &v) != nil {
		return nil
	}
	return v
}
