package template

import (
	"fmt"
	"maps"
	"strings"
)

// PHOTO_VIDEO_TYPE_DEFAULTS and MEDIA_TYPE_DEFAULTS (phototemplate.py:56-70).
var (
	photoVideoDefaults = map[string]string{"photo": "photo", "video": "video"}
	mediaTypeDefaults  = map[string]string{
		"selfie": "selfie", "time_lapse": "time_lapse", "panorama": "panorama", "slow_mo": "slow_mo",
		"screenshot": "screenshot", "screen_recording": "screen_recording", "portrait": "portrait",
		"live_photo": "live_photo", "burst": "burst", "photo": "photo", "video": "video",
	}
)

// get_media_type and get_photo_video_type (phototemplate.py:1550-1580). The
// Photo gives the kind, so that it needs no table; upstream picks it from the
// flags of the photo (1559-1580), always a key of the table, so it has no error
// for one outside it.
func (s *rendering) mediaType(name string, def []string) ([]string, error) {
	v, ok := s.p.Field(name)
	if !ok {
		return nil, unsupported(ErrUnsupportedField, name)
	}
	table := mediaTypeDefaults
	if name == "photo_or_video" {
		table = photoVideoDefaults
	}
	if len(v.Strings) != 1 || table[v.Strings[0]] == "" {
		return nil, fmt.Errorf("%w: the photo gives %s outside the kinds of upstream", ErrRender, name)
	}
	if out := parseDefaultKV(def, table)[v.Strings[0]]; out != "" {
		return []string{out}, nil
	}
	return nil, nil // "" is None (phototemplate.py:1012)
}

// parse_default_kv (phototemplate.py:1587-1609), whose unpacking raises the
// ValueError it passes for a pair without exactly one "=" (1603-1608).
func parseDefaultKV(def []string, table map[string]string) map[string]string {
	out := maps.Clone(table)
	if len(def) == 0 {
		return out
	}
	for kv := range strings.SplitSeq(def[0], ";") {
		if p := strings.Split(kv, "="); len(p) == 2 {
			out[strings.TrimFunc(p[0], isSpace)] = strings.TrimFunc(p[1], isSpace)
		}
	}
	return out
}
