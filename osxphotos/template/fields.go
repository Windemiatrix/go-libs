package template

import "strings"

// The fields that run Python code or exiftool, call Vision or read the command
// line (phototemplate.py:859-874, 924-925, 1383-1384).
var never = set("exiftool function detected_text osxphotos_cmd_line")

// Fields of upstream, by the name before the first ".", not rendered yet.
var later = set("format counter album_seq folder_album_seq")

// PUNCTUATION (phototemplate.py:345-363).
var punctuation = map[string]string{
	"comma": ",", "semicolon": ";", "pipe": "|", "percent": "%", "ampersand": "&", "openbrace": "{",
	"closebrace": "}", "openparens": "(", "closeparens": ")", "openbracket": "[", "closebracket": "]",
	"questionmark": "?", "newline": "\n", "lf": "\n", "cr": "\r", "crlf": "\r\n", "tab": "\t",
}

// TEMPLATE_SUBSTITUTIONS (phototemplate.py:73-227) but the names whose part
// before the first "." is a name too, which get_field_values matches (851).
var singleFields = set(`name original_name title descr caption media_type photo_or_video hdr edited edited_version
	favorite burst created modified today place.name place.country_code place.name.country place.name.state_province
	place.name.city place.name.area_of_interest place.address place.address.street place.address.city
	place.address.state_province place.address.postal_code place.address.country place.address.country_code
	searchinfo.season exif.camera_make exif.camera_model exif.lens_model imported_by.name imported_by.id moment uuid
	shortuuid id counter album_seq folder_album_seq comma semicolon questionmark pipe percent ampersand openbrace
	closebrace openparens closeparens openbracket closebracket newline lf cr crlf tab osxphotos_version osxphotos_cmd_line`)

// TEMPLATE_SUBSTITUTIONS_MULTI_VALUED (phototemplate.py:235-276).
var multiFields = set(`album folder_album project album_project folder_album_project keyword person label
	label_normalized comment exiftool searchinfo.holiday searchinfo.activity searchinfo.venue searchinfo.venue_type
	photo detected_text shell_quote strip format function`)

// TEMPLATE_SUBSTITUTIONS_PATHLIB (phototemplate.py:229-232).
var pathFields = set("export_dir filepath")

// get_field_values puts every other name in unmatched (phototemplate.py:849-883);
// a name that starts with "photo" is a property of the photo (875).
func upstreamField(name, stem string) bool {
	return singleFields[name] || singleFields[stem] || multiFields[name] || strings.HasPrefix(name, "photo") || pathFields[stem]
}

func set(names string) map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Fields(names) {
		m[n] = true
	}
	return m
}
