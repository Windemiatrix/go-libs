package photosdb

import (
	"cmp"
	"database/sql"
	"slices"
	"strings"

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

// Upstream names a person without a name "_UNKNOWN_" (photosdb.py:1792-1796).
func nameOrUnknown(v sql.NullString) string {
	if name := nfc(v.String); name != "" {
		return name
	}
	return photoinfo.UnknownPerson
}

// Upstream keys a person by Z_PK, the person of a face too (photoinfo.py:605-611;
// personinfo.py:35), and never checks the UUID (photosdb.py:1797-1812): a
// repeated or empty ZPERSONUUID stays, and byUUID holds the last row. The rows
// of t.feedback come by (ZPERSON, Z_PK), so the first of a person has the
// lowest Z_PK, where upstream takes the first row SQLite returns
// (personinfo.py:99-108).
func loadPersons(t tables, c *catalog) (persons []personinfo.PersonInfo, byUUID map[string]personinfo.PersonInfo, personsOf map[string][]personinfo.PersonInfo) {
	featureLess := map[int64]bool{}
	for _, f := range t.feedback {
		if _, seen := featureLess[f.PersonPK]; !seen {
			featureLess[f.PersonPK] = f.Type.Int64 != 0
		}
	}
	byUUID = make(map[string]personinfo.PersonInfo, len(t.persons))
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
		persons = append(persons, p)
		byUUID[p.UUID] = p
		byPK[r.PK] = p
	}

	personsOf = map[string][]personinfo.PersonInfo{}
	for _, f := range t.faces {
		// By the Z_PK of the asset, as groupFaces: an asset without attributes may share the ZUUID of a photo.
		if a, ok := c.byUUID[f.AssetUUID.String]; ok && a.PK == f.AssetPK {
			personsOf[a.UUID] = append(personsOf[a.UUID], byPK[f.PersonPK])
		}
	}
	for _, g := range personsOf {
		slices.SortStableFunc(g, func(a, b personinfo.PersonInfo) int {
			return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.UUID, b.UUID))
		})
	}
	return persons, byUUID, personsOf
}
