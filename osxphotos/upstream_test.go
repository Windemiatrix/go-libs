package osxphotos_test

import (
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

	"github.com/Windemiatrix/go-libs/osxphotos/template"
)

const extensionMarker = "extension: not in osxphotos"

// {osxphotos_version} renders the release the port follows.
func TestUpstreamTemplateVersion(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("UPSTREAM.md")
	if err != nil {
		t.Fatalf("read UPSTREAM.md: %v", err)
	}
	m := regexp.MustCompile(`(?m)^Upstream version: (\S+)$`).FindSubmatch(data)
	if m == nil || string(m[1]) != template.UpstreamVersion {
		t.Errorf("UPSTREAM.md gives the upstream version %q, template.UpstreamVersion is %q", m, template.UpstreamVersion)
	}
}

func TestUpstreamExtensions(t *testing.T) {
	t.Parallel()

	inCode := codeExtensions(t, ".")
	data, err := os.ReadFile("UPSTREAM.md")
	if err != nil {
		t.Fatalf("read UPSTREAM.md: %v", err)
	}
	inDoc := docExtensions(string(data))

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
							for _, n := range field.Names {
								if n.IsExported() {
									add(typeName+"."+n.Name, field.Doc, field.Comment)
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

func receiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return typeName(recv.List[0].Type)
}

// The public packages have no generic receivers; for that form typeName gives
// "", and the name is not collected.
func typeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return typeName(x.X)
	}
	return ""
}

var listedName = regexp.MustCompile("(?m)^- `([^`\n]+)`")

func docExtensions(doc string) []string {
	_, section, _ := strings.Cut(doc, "\n## Extensions\n")
	section, _, _ = strings.Cut(section, "\n## ")
	var names []string
	for _, m := range listedName.FindAllStringSubmatch(section, -1) {
		names = append(names, m[1])
	}
	return names
}
