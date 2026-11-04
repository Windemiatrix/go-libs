package photosdb

import (
	"cmp"
	"database/sql"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/sqlread"
	"github.com/Windemiatrix/go-libs/osxphotos/personinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// ZPERSON.ZTYPE of a favorite person (personinfo.py:83-86).
const personFavorite = 1

// PersonInfo returns every person of the library, those without a face
// included, in ZPERSON.Z_PK order, as osxphotos PhotosDB.person_info
// (photosdb/photosdb.py:559-568, 1790-1812); a repeated ZPERSONUUID is listed
// once per row. The slice is new on every call.
func (db *DB) PersonInfo() []personinfo.PersonInfo { return slices.Clone(db.persons) }

// Person returns the person of uuid; of a repeated ZPERSONUUID, the row with
// the highest Z_PK.
func (db *DB) Person(uuid string) (personinfo.PersonInfo, bool) {
	p, ok := db.personByUUID[uuid]
	return p, ok
}

// PersonInfoOf returns the person of each face of p, the ZPERSON row of the
// Z_PK of the face, ordered by Name, then UUID, then the Z_PK of the face: a
// person once per face, as p.Persons lists a name (osxphotos
// PhotoInfo.person_info, photoinfo.py:605-611). The slice is new on every call
// and nil without a face.
func (db *DB) PersonInfoOf(p *photoinfo.PhotoInfo) []personinfo.PersonInfo {
	return slices.Clone(db.personsOf[p.UUID])
}

// FaceInfoOf returns the faces of p ordered by Name, then Z_PK (osxphotos
// PhotoInfo.face_info, photoinfo.py:613-623). The slice is new on every call
// and nil without a face.
func (db *DB) FaceInfoOf(p *photoinfo.PhotoInfo) []personinfo.FaceInfo {
	return slices.Clone(db.facesOf[p.UUID])
}

// PersonPhotos returns the photos with a face of the person of uuid, each
// once, in UUID order (osxphotos PersonInfo.photos, personinfo.py:58-65); of a
// repeated ZPERSONUUID, those of the row that Person returns. The slice is new
// on every call and nil for a person without a photo or an unknown uuid; its
// values are shared: do not modify them.
func (db *DB) PersonPhotos(uuid string) []*photoinfo.PhotoInfo {
	var out []*photoinfo.PhotoInfo
	for _, id := range db.personPhotos[db.personPK[uuid]] {
		out = append(out, db.byUUID[id])
	}
	return out
}

// PersonFaces returns the faces of the person of uuid, those on an asset
// without a photo included, ordered by Quality, descending, then Z_PK
// (osxphotos PersonInfo.face_info, personinfo.py:67-81); of a repeated
// ZPERSONUUID, those of the row that Person returns. The slice is new on every
// call and nil for a person without a face or an unknown uuid.
func (db *DB) PersonFaces(uuid string) []personinfo.FaceInfo {
	return slices.Clone(db.personFaces[db.personPK[uuid]])
}

type personData struct {
	persons      []personinfo.PersonInfo            // every ZPERSON row, Z_PK order
	personByUUID map[string]personinfo.PersonInfo   // the row with the highest Z_PK for a repeated ZPERSONUUID
	personPK     map[string]int64                   // the Z_PK of the row of personByUUID
	personsOf    map[string][]personinfo.PersonInfo // by photo UUID, a person per face, in the order of PersonInfoOf
	facesOf      map[string][]personinfo.FaceInfo   // by photo UUID, in the order of FaceInfoOf

	// By ZPERSON.Z_PK, as upstream keys them (_dbfaces_pk, _db_faceinfo_person).
	personPhotos map[int64][]string              // photo UUIDs, sorted, without repeats
	personFaces  map[int64][]personinfo.FaceInfo // in the order of PersonFaces
}

// Upstream names a person without a name "_UNKNOWN_" (photosdb.py:1792-1796).
func nameOrUnknown(v sql.NullString) string {
	if name := nfc(v.String); name != "" {
		return name
	}
	return photoinfo.UnknownPerson
}

// Upstream keys a person by Z_PK, the person of a face too (photoinfo.py:605-611;
// personinfo.py:35), and never checks the UUID (photosdb.py:1797-1812): a
// repeated or empty ZPERSONUUID stays, and personByUUID holds the last row. The
// rows of t.feedback come by (ZPERSON, Z_PK), so the first of a person has the
// lowest Z_PK, where upstream takes the first row SQLite returns
// (personinfo.py:99-108). The faces of an asset without attributes are faces of
// its person but of no photo: upstream joins the asset table only
// (_photosdb_process_faceinfo.py:228-230; photosdb.py:1845-1852).
func loadPersons(t tables, c *catalog) personData {
	featureLess := map[int64]bool{}
	for _, f := range t.feedback {
		if _, seen := featureLess[f.PersonPK]; !seen {
			featureLess[f.PersonPK] = f.Type.Int64 != 0
		}
	}
	d := personData{
		personByUUID: make(map[string]personinfo.PersonInfo, len(t.persons)),
		personPK:     make(map[string]int64, len(t.persons)),
		personsOf:    map[string][]personinfo.PersonInfo{},
		facesOf:      map[string][]personinfo.FaceInfo{},
		personPhotos: map[int64][]string{},
		personFaces:  map[int64][]personinfo.FaceInfo{},
	}
	byPK := make(map[int64]personinfo.PersonInfo, len(t.persons))
	for _, r := range t.persons {
		p := personinfo.PersonInfo{
			UUID: r.UUID.String, Name: nameOrUnknown(r.FullName), DisplayName: nfc(r.DisplayName.String),
			KeyFace: r.KeyFace.Int64, FaceCount: intOf(r.FaceCount), Favorite: is(r.Type, personFavorite), SortOrder: intOf(r.ManualOrder),
			FeatureLess: featureLess[r.PK],
		}
		if _, ok := c.byUUID[r.KeyAssetUUID.String]; ok {
			p.KeyPhotoUUID = r.KeyAssetUUID.String
		}
		d.persons = append(d.persons, p)
		d.personByUUID[p.UUID], d.personPK[p.UUID], byPK[r.PK] = p, r.PK, p
	}

	for _, f := range t.faces {
		face := faceInfo(f)
		d.personFaces[f.PersonPK] = append(d.personFaces[f.PersonPK], face)
		// By the Z_PK of the asset, as groupFaces: an asset without attributes may share the ZUUID of a photo.
		if a, ok := c.byUUID[f.AssetUUID.String]; ok && a.PK == f.AssetPK {
			d.personsOf[a.UUID] = append(d.personsOf[a.UUID], byPK[f.PersonPK])
			d.facesOf[a.UUID] = append(d.facesOf[a.UUID], face)
			d.personPhotos[f.PersonPK] = append(d.personPhotos[f.PersonPK], a.UUID)
		}
	}
	// The faces come by Z_PK, so a stable sort keeps it for equal keys.
	for _, g := range d.personsOf {
		slices.SortStableFunc(g, func(a, b personinfo.PersonInfo) int {
			return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.UUID, b.UUID))
		})
	}
	for _, g := range d.facesOf {
		slices.SortStableFunc(g, func(a, b personinfo.FaceInfo) int { return strings.Compare(a.Name, b.Name) })
	}
	for _, g := range d.personFaces {
		slices.SortStableFunc(g, func(a, b personinfo.FaceInfo) int { return cmp.Compare(b.Quality, a.Quality) })
	}
	for pk, g := range d.personPhotos {
		d.personPhotos[pk] = sortedSet(g)
	}
	return d
}

// A face of a person without a name has the name "", not "_UNKNOWN_"
// (_photosdb_process_faceinfo.py:283).
func faceInfo(f sqlread.Face) personinfo.FaceInfo {
	return personinfo.FaceInfo{
		PK: f.PK, UUID: f.UUID.String, Name: nfc(f.FullName.String), AssetUUID: f.AssetUUID.String, PersonUUID: f.PersonUUID.String,
		CenterX: f.CenterX.Float64, CenterY: f.CenterY.Float64, Size: f.Size.Float64, Quality: f.Quality.Float64, Roll: f.Roll.Float64,
		SourceWidth: intOf(f.SourceWidth), SourceHeight: intOf(f.SourceHeight),
		HasSmile: is(f.HasSmile, flagSet), Manual: is(f.Manual, flagSet), InTrash: is(f.InTrash, flagSet),
		AgeType: intOf(f.AgeType), EyeMakeupType: intOf(f.EyeMakeupType), EyeState: intOf(f.EyeState), FacialHairType: intOf(f.FacialHairType),
		GenderType: intOf(f.GenderType), GlassesType: intOf(f.GlassesType), HairColorType: intOf(f.HairColorType),
		LipMakeupType: intOf(f.LipMakeupType), SmileType: intOf(f.SmileType),
	}
}
