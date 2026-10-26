// Package adjustmentsinfo describes the edit data of a photo as a plain value.
// It does no I/O.
package adjustmentsinfo

import "time"

// AdjustmentsInfo is the property list of an edit, osxphotos AdjustmentsInfo
// (adjustmentsinfo.py:34-231). A field is the upstream property in CamelCase
// without adj_, except AdjustmentFormatVersion (adj_format_version), as
// FormatVersion is format_version. Values in any are as howett.net/plist
// (uint64 or int64 for an integer) or encoding/json (float64) decode them.
// The fields after Timestamp are nil, Orientation 0, when adjustmentData does
// not decode.
type AdjustmentsInfo struct {
	Plist                                map[string]any // the whole file; nil when photosdb does not accept it as a dictionary property list (see UPSTREAM.md)
	Data                                 []byte         // adjustmentData, a string as its UTF-8 bytes; nil without bytes or a string there
	Editor, FormatID                     string         // adjustmentEditorBundleID, adjustmentFormatIdentifier; "" without a string there
	BaseVersion, FormatVersion           any            // adjustmentBaseVersion, adjustmentFormatVersion; nil without them
	Timestamp                            *time.Time     // adjustmentTimestamp in UTC; nil when it is not a date
	Adjustments                          []any          // "adjustments" of the decoded adjustmentData; nil when it is not a list
	Metadata                             map[string]any // "metadata" of the decoded adjustmentData; nil when it is not a dictionary
	Orientation                          any            // "orientation" of Metadata as is; 0 when Metadata has no such key
	AdjustmentFormatVersion, VersionInfo any            // "formatVersion", "versionInfo" of the decoded adjustmentData; nil without them
}
