package extract

import (
	"regexp"
	"strings"
)

// pythonExtractor reads import statements. Both regexes anchor to the start of
// a line, which is also what keeps `# import foo` out of the graph.
type pythonExtractor struct{}

func (pythonExtractor) Exts() []string { return []string{".py", ".pyi"} }

var (
	pyFromRe   = regexp.MustCompile(`(?m)^[ \t]*from[ \t]+([.\w]+)[ \t]+import\b`)
	pyImportRe = regexp.MustCompile(`(?m)^[ \t]*import[ \t]+([^\n#]+)`)
)

func (pythonExtractor) Refs(src []byte, fc FileContext) []Ref {
	var refs []Ref

	for _, m := range pyFromRe.FindAllSubmatchIndex(src, -1) {
		spec := string(src[m[2]:m[3]])
		if spec == "" {
			continue
		}
		refs = append(refs, Ref{
			Spec:     spec,
			Kind:     "import",
			Line:     lineOf(src, m[0]),
			Strategy: PyModule,
		})
	}

	for _, m := range pyImportRe.FindAllSubmatchIndex(src, -1) {
		line := lineOf(src, m[0])
		// `import a.b as x, c` — one statement, several modules.
		for _, part := range strings.Split(string(src[m[2]:m[3]]), ",") {
			spec := strings.TrimSpace(part)
			if i := strings.Index(spec, " as "); i >= 0 {
				spec = strings.TrimSpace(spec[:i])
			}
			if spec == "" || strings.ContainsAny(spec, " \t()") {
				continue
			}
			refs = append(refs, Ref{
				Spec:     spec,
				Kind:     "import",
				Line:     line,
				Strategy: PyModule,
			})
		}
	}

	return refs
}
