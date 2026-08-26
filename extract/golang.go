package extract

import (
	"go/parser"
	"go/token"
	"strconv"
)

// goExtractor reads imports with the standard library's own parser, so what
// lands in the graph is exactly what the compiler sees.
type goExtractor struct{}

func (goExtractor) Exts() []string { return []string{".go"} }

func (goExtractor) Refs(src []byte, fc FileContext) []Ref {
	fset := token.NewFileSet()
	// ImportsOnly stops at the import block; a syntax error further down the
	// file still leaves the imports we care about populated.
	f, _ := parser.ParseFile(fset, fc.Name, src, parser.ImportsOnly)
	if f == nil {
		return nil
	}

	refs := make([]Ref, 0, len(f.Imports))
	for _, imp := range f.Imports {
		spec, err := strconv.Unquote(imp.Path.Value)
		if err != nil || spec == "" {
			continue
		}
		refs = append(refs, Ref{
			Spec:     spec,
			Kind:     "import",
			Line:     fset.Position(imp.Pos()).Line,
			Strategy: GoPkg,
		})
	}
	return refs
}
