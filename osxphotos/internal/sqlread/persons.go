package sqlread

import (
	"context"
	"database/sql"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/schema"
)

// Person is one ZPERSON row with the ZUUID of the asset of its key face
// (photosdb.py:1767-1778, 1817-1826).
type Person struct {
	PK                                        int64
	UUID, FullName, DisplayName, KeyAssetUUID sql.NullString
	FaceCount, KeyFace, Type, ManualOrder     sql.NullInt64
}

// The order is that of the Scan in Persons.
func personSelect() []column {
	p := func(name string) column { return column{"p", "ZPERSON", name} }
	return []column{
		p("Z_PK"), p("ZPERSONUUID"), p("ZFULLNAME"), p("ZDISPLAYNAME"), {"a", "ZASSET", "ZUUID"},
		p("ZFACECOUNT"), p("ZKEYFACE"), p("ZTYPE"), p("ZMANUALORDER"),
	}
}

// Persons returns the rows of ZPERSON in Z_PK order. KeyAssetUUID is the ZUUID
// of the asset of ZKEYFACE, NULL for a person without a key face or whose key
// face has no asset. Upstream reads the key face with a second query
// (photosdb.py:1817-1840).
func (d *DB) Persons(ctx context.Context, s schema.Schema) ([]Person, error) {
	q := "SELECT " + selectList(personSelect()) + " FROM ZPERSON p LEFT JOIN ZDETECTEDFACE kf ON kf.Z_PK = p.ZKEYFACE " +
		"LEFT JOIN ZASSET a ON a.Z_PK = kf." + s.FaceAssetColumn + " ORDER BY p.Z_PK"
	var out []Person
	err := d.readRows(ctx, "persons", q, func(rows *sql.Rows) error {
		var p Person
		if err := rows.Scan(&p.PK, &p.UUID, &p.FullName, &p.DisplayName, &p.KeyAssetUUID,
			&p.FaceCount, &p.KeyFace, &p.Type, &p.ManualOrder); err != nil {
			return err
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Face is one detected face of an asset with a person, the columns of
// _photosdb_process_faceinfo.py:189-248 that FaceInfo exposes. AssetPK is
// schema.Schema.FaceAssetColumn, FullName ZPERSON.ZFULLNAME, UUID
// ZDETECTEDFACE.ZUUID, AssetUUID ZASSET.ZUUID and PersonUUID
// ZPERSON.ZPERSONUUID.
type Face struct {
	PK, AssetPK, PersonPK                   int64
	AssetUUID, UUID, PersonUUID, FullName   sql.NullString
	AgeType, EyeMakeupType, EyeState        sql.NullInt64
	FacialHairType, GenderType, GlassesType sql.NullInt64
	HairColorType, HasSmile, InTrash        sql.NullInt64
	LipMakeupType, Manual, SmileType        sql.NullInt64
	SourceHeight, SourceWidth               sql.NullInt64
	CenterX, CenterY, Quality, Roll, Size   sql.NullFloat64
	DetectionType                           sql.NullInt64
	BodyCenterX, BodyCenterY                sql.NullFloat64
	BodyWidth, BodyHeight                   sql.NullFloat64
}

// The order is that of the Scan in Faces.
func faceSelect(s schema.Schema) []column {
	f := func(name string) column { return column{"f", "ZDETECTEDFACE", name} }
	return []column{
		f("Z_PK"), f(s.FaceAssetColumn), {"a", "ZASSET", "ZUUID"}, f("ZUUID"),
		f(s.FacePersonColumn), {"p", "ZPERSON", "ZPERSONUUID"}, {"p", "ZPERSON", "ZFULLNAME"},
		f("ZAGETYPE"), f("ZEYEMAKEUPTYPE"), f("ZEYESSTATE"), f("ZFACIALHAIRTYPE"), f("ZGENDERTYPE"), f("ZGLASSESTYPE"),
		f("ZHAIRCOLORTYPE"), f("ZHASSMILE"), f("ZISINTRASH"), f("ZLIPMAKEUPTYPE"), f("ZMANUAL"), f("ZSMILETYPE"),
		f("ZSOURCEHEIGHT"), f("ZSOURCEWIDTH"), f("ZCENTERX"), f("ZCENTERY"), f("ZQUALITY"), f("ZROLL"), f("ZSIZE"),
		f("ZDETECTIONTYPE"), f("ZBODYCENTERX"), f("ZBODYCENTERY"), f("ZBODYWIDTH"), f("ZBODYHEIGHT"),
	}
}

// Faces returns the detected faces that have a row in ZASSET and in ZPERSON,
// one row per face, in Z_PK order (photosdb.py:1848-1857;
// _photosdb_process_faceinfo.py:189-248). The detection type and the body
// region exist from Photos 7, so every schema of the port has them (189-198).
func (d *DB) Faces(ctx context.Context, s schema.Schema) ([]Face, error) {
	q := "SELECT " + selectList(faceSelect(s)) + " FROM ZDETECTEDFACE f JOIN ZASSET a ON a.Z_PK = f." + s.FaceAssetColumn +
		" JOIN ZPERSON p ON p.Z_PK = f." + s.FacePersonColumn + " ORDER BY f.Z_PK"
	var out []Face
	err := d.readRows(ctx, "faces", q, func(rows *sql.Rows) error {
		var f Face
		if err := rows.Scan(&f.PK, &f.AssetPK, &f.AssetUUID, &f.UUID, &f.PersonPK, &f.PersonUUID, &f.FullName,
			&f.AgeType, &f.EyeMakeupType, &f.EyeState, &f.FacialHairType, &f.GenderType, &f.GlassesType,
			&f.HairColorType, &f.HasSmile, &f.InTrash, &f.LipMakeupType, &f.Manual, &f.SmileType,
			&f.SourceHeight, &f.SourceWidth, &f.CenterX, &f.CenterY, &f.Quality, &f.Roll, &f.Size,
			&f.DetectionType, &f.BodyCenterX, &f.BodyCenterY, &f.BodyWidth, &f.BodyHeight); err != nil {
			return err
		}
		out = append(out, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Feedback is one ZUSERFEEDBACK row of a person (personinfo.py:99-106).
type Feedback struct {
	PK, PersonPK int64
	Type         sql.NullInt64
}

func feedbackSelect() []column {
	u := func(name string) column { return column{"u", "ZUSERFEEDBACK", name} }
	return []column{u("Z_PK"), u("ZPERSON"), u("ZTYPE")}
}

// UserFeedback returns the rows with a person, ordered by ZPERSON, then Z_PK.
func (d *DB) UserFeedback(ctx context.Context) ([]Feedback, error) {
	q := "SELECT " + selectList(feedbackSelect()) + " FROM ZUSERFEEDBACK u WHERE u.ZPERSON IS NOT NULL ORDER BY u.ZPERSON, u.Z_PK"
	var out []Feedback
	err := d.readRows(ctx, "user feedback", q, func(rows *sql.Rows) error {
		var f Feedback
		if err := rows.Scan(&f.PK, &f.PersonPK, &f.Type); err != nil {
			return err
		}
		out = append(out, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
