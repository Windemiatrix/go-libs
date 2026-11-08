package adjustmentsinfo

// AsDict is asdict of AdjustmentsInfo (adjustmentsinfo.py:210-228).
// adjustments is [] when nil; editor and format_id are nil for "" (None
// upstream); any other absent value is nil. timestamp is a time.Time, where
// upstream writes its isoformat(). Nested values are shared with a, as upstream
// shares them.
func (a AdjustmentsInfo) AsDict() map[string]any {
	d := map[string]any{
		"data": nil, "editor": strOrNil(a.Editor), "format_id": strOrNil(a.FormatID), "base_version": a.BaseVersion,
		"format_version": a.FormatVersion, "adjustments": append([]any{}, a.Adjustments...), "metadata": nil,
		"orientation": a.Orientation, "adjustment_format_version": a.AdjustmentFormatVersion,
		"version_info": a.VersionInfo, "timestamp": nil,
	}
	// A nil slice or map in an any is not nil, which None is.
	if a.Data != nil {
		d["data"] = a.Data
	}
	if a.Metadata != nil {
		d["metadata"] = a.Metadata
	}
	if a.Timestamp != nil {
		d["timestamp"] = *a.Timestamp
	}
	return d
}

// AdjustmentsInfo holds "" where upstream has None (41-42), which json writes
// as null.
func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
