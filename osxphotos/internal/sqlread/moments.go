package sqlread

import (
	"context"
	"database/sql"
)

// Moment is one ZMOMENT row (photosdb/photosdb.py:2747-2792).
type Moment struct {
	PK                                   int64
	TZOffset                             sql.NullInt64
	Latitude, Longitude                  sql.NullFloat64 // ZAPPROXIMATELATITUDE, ZAPPROXIMATELONGITUDE
	Start, End, Representative, Modified sql.NullFloat64 // ZSTARTDATE, ZENDDATE, ZREPRESENTATIVEDATE, ZMODIFICATIONDATE
	Title, Subtitle                      sql.NullString
}

// The order is that of the Scan in Moments. Upstream also reads ZTRASHEDSTATE
// and ZUUID but exposes neither (photosdb.py:2783, 2792).
func momentSelect() []column {
	m := func(name string) column { return column{"m", "ZMOMENT", name} }
	return []column{
		m("Z_PK"), m("ZTIMEZONEOFFSET"), m("ZAPPROXIMATELATITUDE"), m("ZAPPROXIMATELONGITUDE"),
		m("ZSTARTDATE"), m("ZENDDATE"), m("ZREPRESENTATIVEDATE"), m("ZMODIFICATIONDATE"), m("ZTITLE"), m("ZSUBTITLE"),
	}
}

// Moments returns every ZMOMENT row in Z_PK order.
func (d *DB) Moments(ctx context.Context) ([]Moment, error) {
	q := "SELECT " + selectList(momentSelect()) + " FROM ZMOMENT m ORDER BY m.Z_PK"
	var out []Moment
	err := d.readRows(ctx, "moments", q, func(rows *sql.Rows) error {
		var m Moment
		if err := rows.Scan(&m.PK, &m.TZOffset, &m.Latitude, &m.Longitude,
			&m.Start, &m.End, &m.Representative, &m.Modified, &m.Title, &m.Subtitle); err != nil {
			return err
		}
		out = append(out, m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
