package osxphotos_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// extensionMarker marks an exported name that has no counterpart in
// osxphotos.
const extensionMarker = "extension: not in osxphotos"

// TestUpstreamExtensions keeps the "Extensions" section of UPSTREAM.md and
// the code in agreement: the names in backticks there are exactly the
// exported types, fields, methods, functions, constants and variables of the
// public packages whose doc or line comment carries extensionMarker, written
// as pkg.Type, pkg.Type.Field, pkg.Type.Method(), pkg.Func() or pkg.Name.
func TestUpstreamExtensions(t *testing.T) {
	t.Parallel()

	inCode := codeExtensions(t, ".")
	data, err := os.ReadFile("UPSTREAM.md")
	if err != nil {
		t.Fatalf("read UPSTREAM.md: %v", err)
	}
	inDoc, err := docExtensions(string(data))
	if err != nil {
		t.Fatalf("UPSTREAM.md: %v; the section must list, one per item in backticks:\n%s", err, strings.Join(inCode, "\n"))
	}

	var missing, stale []string
	for _, name := range inCode {
		if !slices.Contains(inDoc, name) {
			missing = append(missing, name)
		}
	}
	for _, name := range inDoc {
		if !slices.Contains(inCode, name) {
			stale = append(stale, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("marked %q in the code but not listed in the Extensions section of UPSTREAM.md:\n%s",
			extensionMarker, strings.Join(missing, "\n"))
	}
	if len(stale) > 0 {
		t.Errorf("listed in the Extensions section of UPSTREAM.md but not an exported name marked %q in the code:\n%s",
			extensionMarker, strings.Join(stale, "\n"))
	}
}

// codeExtensions returns the sorted marked names of the public packages
// under root: every directory except internal/, testdata/ and those
// starting with "." or "_", files without _test.go, packages other than main.
func codeExtensions(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (name == "internal" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if f.Name.Name != "main" {
			names = append(names, fileExtensions(f)...)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the module: %v", err)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// fileExtensions returns the marked names declared in f.
func fileExtensions(f *ast.File) []string {
	pkg := f.Name.Name
	var out []string
	add := func(name string, groups ...*ast.CommentGroup) {
		for _, g := range groups {
			if g != nil && strings.Contains(g.Text(), extensionMarker) {
				out = append(out, name)
				return
			}
		}
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			if d.Recv == nil {
				add(pkg+"."+d.Name.Name+"()", d.Doc)
			} else if recv := receiverName(d.Recv); ast.IsExported(recv) {
				add(pkg+"."+recv+"."+d.Name.Name+"()", d.Doc)
			}
		case *ast.GenDecl:
			// The doc of a single-spec declaration is on the GenDecl.
			var declDoc *ast.CommentGroup
			if !d.Lparen.IsValid() {
				declDoc = d.Doc
			}
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if !s.Name.IsExported() {
						continue
					}
					typeName := pkg + "." + s.Name.Name
					add(typeName, declDoc, s.Doc, s.Comment)
					if st, ok := s.Type.(*ast.StructType); ok {
						for _, field := range st.Fields.List {
							for _, n := range fieldNames(field) {
								if ast.IsExported(n) {
									add(typeName+"."+n, field.Doc, field.Comment)
								}
							}
						}
					}
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if n.IsExported() {
							add(pkg+"."+n.Name, declDoc, s.Doc, s.Comment)
						}
					}
				}
			}
		}
	}
	return out
}

// receiverName is the type name of a method receiver: T or *T.
func receiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return typeName(recv.List[0].Type)
}

// typeName is the name of T or *T. The public packages have no generic
// receivers and no embedded types of other packages; for those forms it
// gives "", and the name is not collected.
func typeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return typeName(x.X)
	}
	return ""
}

// fieldNames returns the names of a struct field; an embedded field is
// named by its type.
func fieldNames(field *ast.Field) []string {
	if len(field.Names) == 0 {
		return []string{typeName(field.Type)}
	}
	var out []string
	for _, n := range field.Names {
		out = append(out, n.Name)
	}
	return out
}

var (
	extensionsHeading = regexp.MustCompile(`(?m)^#{2,6} Extensions\b.*$`)
	anyHeading        = regexp.MustCompile(`(?m)^#{1,6} `)
	codeSpan          = regexp.MustCompile("`([^`\n]+)`")
	listedName        = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[A-Z][A-Za-z0-9_]*)+(\(\))?$`)
)

// russianAnchor starts the Russian half of UPSTREAM.md; the section is read
// from the English half before it.
const russianAnchor = `<a id="upstream-osxphotos-ru">`

// docExtensions returns the sorted names in backticks on the list items of
// the English "Extensions" section of UPSTREAM.md, up to the next heading.
func docExtensions(doc string) ([]string, error) {
	if i := strings.Index(doc, russianAnchor); i >= 0 {
		doc = doc[:i]
	}
	loc := extensionsHeading.FindStringIndex(doc)
	if loc == nil {
		return nil, errNoSection
	}
	section := doc[loc[1]:]
	if next := anyHeading.FindStringIndex(section); next != nil {
		section = section[:next[0]]
	}
	var names []string
	for line := range strings.SplitSeq(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "* ") && !strings.HasPrefix(trimmed, "|") {
			continue
		}
		for _, m := range codeSpan.FindAllStringSubmatch(trimmed, -1) {
			if listedName.MatchString(m[1]) {
				names = append(names, m[1])
			}
		}
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

var errNoSection = errors.New(`no "## Extensions" section before ` + russianAnchor)

// TestDocExtensions checks the reading of the Extensions section on a
// literal document.
func TestDocExtensions(t *testing.T) {
	t.Parallel()

	doc := "# Upstream\n\n## Intentional differences\n\n- `photosdb.Info.Other` is not an extension\n\n" +
		"## Extensions\n\nIntro with `query.Options` outside a list item.\n\n" +
		"- `photoinfo.PhotoInfo.Resources`: resources of the asset.\n" +
		"- `photoinfo.Resource` — the whole type.\n" +
		"- `photoinfo.PhotoInfo.BurstUnselected()`; marked `// extension: not in osxphotos`\n" +
		"| `albuminfo.AlbumInfo.CloudGUID` | table row |\n\n" +
		"## Next\n\n- `photosdb.Info.After`\n\n" +
		russianAnchor + "\n\n## Extensions\n\n- `photosdb.Info.Russian`\n"
	got, err := docExtensions(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"albuminfo.AlbumInfo.CloudGUID",
		"photoinfo.PhotoInfo.BurstUnselected()",
		"photoinfo.PhotoInfo.Resources",
		"photoinfo.Resource",
	}
	if !slices.Equal(got, want) {
		t.Errorf("docExtensions = %q, want %q", got, want)
	}

	if _, err := docExtensions("## Other\n\n" + russianAnchor + "\n## Extensions\n- `a.B`\n"); err == nil {
		t.Error("docExtensions found a section only in the Russian half")
	}
}

// TestFileExtensions checks the reading of markers on a literal source file.
func TestFileExtensions(t *testing.T) {
	t.Parallel()

	src := `package demo

// Plain is not marked.
type Plain struct {
	A int // a plain field
	// B is marked in its doc.
	// extension: not in osxphotos.
	B int
	C, D string // extension: not in osxphotos — two names
	e    int    // extension: not in osxphotos — unexported
	*Whole      // extension: not in osxphotos — embedded
}

// Whole is marked as a type.
// extension: not in osxphotos.
type Whole struct{ X int }

type (
	// Grouped is marked inside a group.
	// extension: not in osxphotos.
	Grouped int
	Other   int
)

// Method is marked.
// extension: not in osxphotos.
func (p *Plain) Method() bool { return false }

// Unmarked is not marked.
func (Plain) Unmarked() {}

// Func is marked.
// extension: not in osxphotos.
func Func() {}

// Limit is marked.
// extension: not in osxphotos.
const Limit = 1
`
	f, err := parser.ParseFile(token.NewFileSet(), "demo.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	got := fileExtensions(f)
	slices.Sort(got)
	want := []string{
		"demo.Func()",
		"demo.Grouped",
		"demo.Limit",
		"demo.Plain.B",
		"demo.Plain.C",
		"demo.Plain.D",
		"demo.Plain.Method()",
		"demo.Plain.Whole",
		"demo.Whole",
	}
	if !slices.Equal(got, want) {
		t.Errorf("fileExtensions = %q, want %q", got, want)
	}
}
