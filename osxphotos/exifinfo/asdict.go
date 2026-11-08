package exifinfo

// AsDict is dataclasses.asdict of ExifInfo (exifinfo.py:18-39), which
// photoinfo.py:2123 calls: a nil pointer and an empty string are nil, as None
// upstream. The map is new on every call.
func (e ExifInfo) AsDict() map[string]any {
	return map[string]any{
		"flash_fired": orNil(e.FlashFired), "iso": orNil(e.ISO), "metering_mode": orNil(e.MeteringMode),
		"sample_rate": orNil(e.SampleRate), "track_format": orNil(e.TrackFormat), "white_balance": orNil(e.WhiteBalance),
		"aperture": orNil(e.Aperture), "bit_rate": orNil(e.BitRate), "duration": orNil(e.Duration),
		"exposure_bias": orNil(e.ExposureBias), "focal_length": orNil(e.FocalLength), "fps": orNil(e.FPS),
		"latitude": orNil(e.Latitude), "longitude": orNil(e.Longitude), "shutter_speed": orNil(e.ShutterSpeed),
		"camera_make": strOrNil(e.CameraMake), "camera_model": strOrNil(e.CameraModel), "codec": strOrNil(e.Codec),
		"lens_model": strOrNil(e.LensModel), "date": orNil(e.Date), "tzoffset": orNil(e.TZOffset), "tzname": strOrNil(e.TZName),
	}
}

// A nil pointer in an any is not nil, which None is.
func orNil[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// The port holds None of upstream as "" here.
func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
