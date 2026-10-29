package sqlread

import (
	"context"
	"database/sql"
)

// Exif is one ZEXTENDEDATTRIBUTES row (_photosdb_process_exif.py:45-51).
type Exif struct {
	PK, AssetPK int64 // Z_PK, ZASSET

	FlashFired, ISO, MeteringMode, SampleRate, TrackFormat, WhiteBalance sql.NullInt64 // ZFLASHFIRED, ZISO, ZMETERINGMODE, ZSAMPLERATE, ZTRACKFORMAT, ZWHITEBALANCE

	Aperture, BitRate, Duration, ExposureBias, FocalLength, FPS sql.NullFloat64 // ZAPERTURE, ZBITRATE, ZDURATION, ZEXPOSUREBIAS, ZFOCALLENGTH, ZFPS

	Latitude, Longitude, ShutterSpeed sql.NullFloat64 // ZLATITUDE, ZLONGITUDE, ZSHUTTERSPEED

	CameraMake, CameraModel, Codec, LensModel sql.NullString // ZCAMERAMAKE, ZCAMERAMODEL, ZCODEC, ZLENSMODEL

	DateCreated sql.NullFloat64 // ZDATECREATED
	TZOffset    sql.NullInt64   // ZTIMEZONEOFFSET
	TZName      sql.NullString  // ZTIMEZONENAME
}

// The order is that of the Scan in Exif.
func exifSelect() []column {
	e := func(name string) column { return column{"e", "ZEXTENDEDATTRIBUTES", name} }
	return []column{
		e("Z_PK"), e("ZASSET"),
		e("ZFLASHFIRED"), e("ZISO"), e("ZMETERINGMODE"), e("ZSAMPLERATE"), e("ZTRACKFORMAT"), e("ZWHITEBALANCE"),
		e("ZAPERTURE"), e("ZBITRATE"), e("ZDURATION"), e("ZEXPOSUREBIAS"), e("ZFOCALLENGTH"), e("ZFPS"),
		e("ZLATITUDE"), e("ZLONGITUDE"), e("ZSHUTTERSPEED"),
		e("ZCAMERAMAKE"), e("ZCAMERAMODEL"), e("ZCODEC"), e("ZLENSMODEL"),
		e("ZDATECREATED"), e("ZTIMEZONEOFFSET"), e("ZTIMEZONENAME"),
	}
}

// Exif returns the rows with a ZASSET, ordered by ZASSET, then Z_PK. Upstream
// joins them to ZASSET; the caller drops a row whose ZASSET is no asset.
func (d *DB) Exif(ctx context.Context) ([]Exif, error) {
	q := "SELECT " + selectList(exifSelect()) +
		" FROM ZEXTENDEDATTRIBUTES e WHERE e.ZASSET IS NOT NULL ORDER BY e.ZASSET, e.Z_PK"
	var out []Exif
	err := d.readRows(ctx, "exif", q, func(rows *sql.Rows) error {
		var r Exif
		if err := rows.Scan(&r.PK, &r.AssetPK,
			&r.FlashFired, &r.ISO, &r.MeteringMode, &r.SampleRate, &r.TrackFormat, &r.WhiteBalance,
			&r.Aperture, &r.BitRate, &r.Duration, &r.ExposureBias, &r.FocalLength, &r.FPS,
			&r.Latitude, &r.Longitude, &r.ShutterSpeed,
			&r.CameraMake, &r.CameraModel, &r.Codec, &r.LensModel,
			&r.DateCreated, &r.TZOffset, &r.TZName); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
