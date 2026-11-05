package golden

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

const (
	templatesFile = "templates.txt"
	templatesJSON = "templates.json"
)

// TemplateLines reads dir/templates.txt: the ids t001, t002, … by the order of
// its template lines ("#" comments and empty lines skipped), each with its
// template text.
func TemplateLines(dir string) (map[string]string, error) {
	data, err := readFile(filepath.Join(dir, templatesFile))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			out[fmt.Sprintf("t%03d", len(out)+1)] = line
		}
	}
	return out, nil
}

// Templates reads dir/<library>/templates.json, the osxphotos rendering of the
// templates of TemplateLines for every photo of the library: by photo UUID,
// the values of each id; a string is one value and a list several
// (cli/print_photo_info.py:138-149). library is checked as in Load, before any
// file is opened. Errors name the file, the element number and the lowest bad
// id, never a UUID or a value.
func Templates(dir, library string) (map[string]map[string][]string, error) {
	if err := checkLibrary(library); err != nil {
		return nil, err
	}
	items, err := readArray(filepath.Join(dir, library, templatesJSON), templatesJSON)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string][]string, len(items))
	for i, item := range items {
		var obj map[string]json.RawMessage
		if json.Unmarshal(item, &obj) != nil {
			return nil, fmt.Errorf("golden: %s: element %d is not an object", templatesJSON, i+1)
		}
		var uuid string
		if json.Unmarshal(obj["uuid"], &uuid) != nil || uuid == "" {
			return nil, fmt.Errorf("golden: %s: element %d: uuid is not a string", templatesJSON, i+1)
		}
		if _, ok := out[uuid]; ok {
			return nil, fmt.Errorf("golden: %s: element %d: uuid repeated", templatesJSON, i+1)
		}
		values := make(map[string][]string, len(obj)-1)
		for _, id := range slices.Sorted(maps.Keys(obj)) {
			if id == "uuid" {
				continue
			}
			v, ok := templateValues(obj[id])
			if !ok {
				return nil, fmt.Errorf("golden: %s: element %d: id %s is neither a string nor a list of strings", templatesJSON, i+1, id)
			}
			values[id] = v
		}
		out[uuid] = values
	}
	return out, nil
}

// A null, alone or in a list, is no value: it would decode to an empty string,
// which osxphotos never prints.
func templateValues(raw json.RawMessage) ([]string, bool) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil, false
	}
	switch v := v.(type) {
	case string:
		return []string{v}, true
	case []any:
		out := make([]string, len(v))
		for i, e := range v {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out[i] = s
		}
		return out, true
	}
	return nil, false
}
