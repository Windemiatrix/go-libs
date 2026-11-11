package template

import (
	"cmp"
	"fmt"
	"strings"
)

// The attributes of pathlib.PurePosixPath that are a string or a path. suffixes
// and parts are a list and a tuple, whose repr upstream renders; the others are
// methods or objects whose str is no value.
var pathProperties = set("name stem suffix parent anchor root drive")

// Checked before any value of the filter (phototemplate.py:1867-1874) and of a
// set field (1642-1652).
func pathProps(name string) ([]string, error) {
	_, rest, dotted := strings.Cut(name, ".")
	if !dotted {
		return nil, nil
	}
	props := strings.Split(rest, ".")
	for _, p := range props {
		if p == "suffixes" || p == "parts" {
			return nil, unsupported(ErrUnsupportedConstruct, name)
		} else if !pathProperties[p] {
			return nil, fmt.Errorf("%w: unknown path property in %s", ErrRender, name)
		}
	}
	return props, nil
}

// get_template_values_pathlib (phototemplate.py:1853-1886).
func (s *rendering) paths(name string, vals []string) ([]string, error) {
	props, err := pathProps(name)
	if err != nil {
		return nil, err
	}
	return vals, s.each(vals, func(v string) string {
		if props == nil {
			return joinPath(splitPath(v))
		}
		return s.pathProperty(v, props)
	})
}

// _get_pathlib_value (phototemplate.py:1634-1657); an unset path is no value,
// whatever its properties (1037-1039).
func (s *rendering) pathField(name, stem string) ([]string, error) {
	p := s.o.Filepath
	if stem == "export_dir" {
		p = s.o.ExportDir
	}
	if p == "" {
		return nil, nil
	}
	props, err := pathProps(name)
	if err != nil {
		return nil, err
	}
	vals := []string{p}
	return vals, s.each(vals, func(v string) string {
		if v = s.pathProperty(v, props); s.o.Quote {
			return shellQuote(v)
		}
		return v
	})
}

// Each property is read from a path again, as the field form does
// (phototemplate.py:1646-1650); the filter form would fail on a string after
// name, stem or suffix (1880-1881).
func (s *rendering) pathProperty(v string, props []string) string {
	for _, prop := range props {
		if s.bytesLeft -= len(v); s.bytesLeft < 0 { // every step splits v again, and each counts only the result
			return v
		}
		root, parts := splitPath(v)
		name := ""
		if n := len(parts); n > 0 {
			name, parts = parts[n-1], parts[:n-1]
		}
		switch stem, suffix := splitSuffix(name); prop {
		case "name":
			v = name
		case "stem":
			v = stem
		case "suffix":
			v = suffix
		case "parent":
			v = joinPath(root, parts)
		case "anchor", "root": // drive is always "" on POSIX, so anchor is root
			v = root
		case "drive":
			v = ""
		}
	}
	return v
}

// pathlib.PurePosixPath, not path.Clean: POSIX leaves the meaning of exactly
// two leading "/" to the system, and ".." after a link is not the parent.
func splitPath(p string) (root string, parts []string) {
	switch {
	case strings.HasPrefix(p, "//") && !strings.HasPrefix(p, "///"):
		root = "//"
	case strings.HasPrefix(p, "/"):
		root = "/"
	}
	for part := range strings.SplitSeq(p, "/") {
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}
	return root, parts
}

func joinPath(root string, parts []string) string {
	return cmp.Or(root+strings.Join(parts, "/"), ".")
}

// pathlib.PurePath.stem and .suffix; a copy of splitSuffix of export, so that
// template depends on no other package of the module.
func splitSuffix(name string) (stem, suffix string) {
	if i := strings.LastIndexByte(name, '.'); i > 0 && i < len(name)-1 {
		return name[:i], name[i:]
	}
	return name, ""
}
