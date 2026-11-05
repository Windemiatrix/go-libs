package photosdb

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
	"github.com/Windemiatrix/go-libs/osxphotos/template"
)

// Fields returns p as a template.Photo: the values of the template fields of
// osxphotos read from p and the library (phototemplate.py:930-1000,
// 1261-1317), on demand, by their upstream name; a name the port does not give
// yet, such as photo.X, place.X, exif.X, searchinfo.X, label or
// label_normalized, gives false. The Photo is valid as long as db is; the
// caller may change the values it gives.
func (db *DB) Fields(p *photoinfo.PhotoInfo) template.Photo { return photoFields{db, p} }

// RenderTemplate renders t for p with one Renderer of the library, as
// osxphotos PhotoInfo.render_template does (photoinfo.py:1866-1880): {counter}
// grows from photo to photo for as long as db lives; a caller that needs to
// reset it renders with its own template.Renderer and Fields(p). t is parsed
// once by the caller, where upstream parses the string on each call. The
// results and the errors are those of template.Renderer.Render.
func (db *DB) RenderTemplate(p *photoinfo.PhotoInfo, t *template.Template, o template.RenderOptions) (values, unmatched []string, err error) {
	return db.renderer.Render(t, db.Fields(p), o)
}

type photoFields struct {
	db *DB
	p  *photoinfo.PhotoInfo
}

func (f photoFields) Field(name string) (template.Value, bool) {
	db, p := f.db, f.p
	switch name {
	case "name":
		return singleValue(p.Filename), true
	case "original_name":
		return singleValue(p.OriginalFilename), true
	case "title":
		return singleValue(p.Title), true
	case "descr", "caption":
		return singleValue(p.Description), true
	case "media_type":
		return singleValue(mediaKindOf(p)), true
	case "photo_or_video": // 1547-1550
		if p.IsPhoto {
			return singleValue("photo"), true
		}
		return singleValue("video"), true
	case "hdr":
		return flagValue(p.HDR, name), true
	case "edited":
		return flagValue(p.HasAdjustments, name), true
	case "favorite":
		return flagValue(p.Favorite, name), true
	case "created":
		d := p.Date
		return template.Value{Time: &d}, true
	case "modified":
		if p.DateModified == nil {
			return template.Value{}, true
		}
		d := *p.DateModified
		return template.Value{Time: &d}, true
	case "uuid":
		return singleValue(p.UUID), true
	case "id":
		pk := p.PK
		return template.Value{Int: &pk}, true
	case "imported_by.name":
		return singleValue(p.ImportedByName), true
	case "imported_by.id":
		return singleValue(p.ImportedByBundleID), true
	case "moment": // 971-972
		m, _ := db.MomentInfoOf(p)
		return singleValue(m.Title), true
	case "burst": // 995-1000
		if key, ok := db.BurstKeyPhoto(p); ok {
			return singleValue(key.OriginalFilename), true
		}
		return template.Value{}, true
	// burst_albums and burst_album_info are albums and album_info for a photo
	// outside a burst (photoinfo.py:646-671), so the Burst*Of methods stand for
	// both branches of phototemplate.py:1262-1266, 1270, 1290-1293.
	case "album":
		return template.Value{Strings: db.BurstAlbumTitlesOf(p)}, true
	case "project":
		return template.Value{Strings: projectTitles(db.ProjectInfoOf(p))}, true
	case "album_project": // upstream also appends the projects to the cached albums of a photo outside a burst (1270-1271)
		v := slices.Concat(db.BurstAlbumTitlesOf(p), projectTitles(db.ProjectInfoOf(p)))
		slices.Sort(v)
		return template.Value{Strings: v}, true
	case "keyword":
		return multiValue(p.Keywords), true
	case "person":
		return multiValue(p.Persons), true
	case "comment": // f"{comment.user}: …" prints the user None as "None" (1314-1317)
		var v []string
		for _, c := range db.Comments(p) {
			v = append(v, cmp.Or(c.User, "None")+": "+c.Text)
		}
		slices.Sort(v)
		return template.Value{Strings: v}, true
	case "folder_album":
		return albumRefsOf(db.BurstAlbumInfoOf(p)), true
	case "folder_album_project":
		return albumRefsOf(slices.Concat(db.BurstAlbumInfoOf(p), db.ProjectInfoOf(p))), true
	case "album_seq": // album_info, not the albums of the burst (983, 985)
		albums := db.AlbumInfoOf(p)
		v := albumRefsOf(albums)
		for i, a := range albums {
			v.Albums[i].Index, _ = db.PhotoIndex(a, p) // -1 is the ValueError of photo_index (albuminfo.py:285-287)
		}
		return v, true
	}
	return template.Value{}, false
}

// get_media_type (phototemplate.py:1552-1577) tests the flags in this order.
func mediaKindOf(p *photoinfo.PhotoInfo) string {
	switch {
	case p.Selfie:
		return "selfie"
	case p.TimeLapse:
		return "time_lapse"
	case p.Panorama:
		return "panorama"
	case p.SlowMo:
		return "slow_mo"
	case p.Screenshot:
		return "screenshot"
	case p.ScreenRecording:
		return "screen_recording"
	case p.Portrait:
		return "portrait"
	case p.Live:
		return "live_photo"
	case p.Burst:
		return "burst"
	case p.IsMovie:
		return "video"
	}
	return "photo"
}

// A single-valued field never holds "" (phototemplate.py:1012).
func singleValue(s string) template.Value {
	if s == "" {
		return template.Value{}
	}
	return template.Value{Strings: []string{s}}
}

// The caller of Fields may change the values: the slices of p stay out of its reach.
func multiValue(s []string) template.Value { return template.Value{Strings: slices.Clone(s)} }

// phototemplate.py:942-949
func flagValue(on bool, name string) template.Value {
	if on {
		return singleValue(name)
	}
	return template.Value{}
}

// Upstream keeps a repeated project title (phototemplate.py:1268).
func projectTitles(projects []*albuminfo.AlbumInfo) []string {
	var out []string
	for _, a := range projects {
		out = append(out, a.Title)
	}
	return out
}

func albumRefsOf(albums []*albuminfo.AlbumInfo) template.Value {
	var refs []template.AlbumRef
	for _, a := range albums {
		refs = append(refs, template.AlbumRef{Folders: slices.Clone(a.FolderNames), Title: a.Title})
	}
	return template.Value{Albums: refs}
}
