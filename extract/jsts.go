package extract

import "regexp"

// jsExtractor covers the JavaScript/TypeScript family. Only relative
// specifiers become edges — a bare "react" names a registry package, not a
// file in this tree.
type jsExtractor struct{}

func (jsExtractor) Exts() []string {
	return []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".vue", ".svelte", ".astro"}
}

// The four ways a module specifier appears: `from "x"`, `import("x")`,
// a side-effect `import "x"`, and `require("x")`.
var jsRefRe = regexp.MustCompile(
	`\bfrom\s*['"]([^'"\n]+)['"]` +
		`|\bimport\s*\(\s*['"]([^'"\n]+)['"]` +
		`|\bimport\s+['"]([^'"\n]+)['"]` +
		`|\brequire\s*\(\s*['"]([^'"\n]+)['"]`)

// jsTryExts is precedence order for a specifier written without one.
var jsTryExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts", ".vue", ".svelte"}

func (jsExtractor) Refs(src []byte, fc FileContext) []Ref {
	src = stripComments(src)
	var refs []Ref

	for _, m := range jsRefRe.FindAllSubmatchIndex(src, -1) {
		spec := firstGroup(src, m)
		if spec == "" || !isRelativeSpec(spec) {
			continue
		}
		refs = append(refs, Ref{
			Spec:     spec,
			Kind:     "import",
			Line:     lineOf(src, m[0]),
			Strategy: RelPath,
			TryExts:  jsTryExts,
		})
	}
	return refs
}

// firstGroup returns the one alternation branch that actually matched.
func firstGroup(src []byte, m []int) string {
	for g := 1; g*2+1 < len(m); g++ {
		if m[g*2] >= 0 {
			return string(src[m[g*2]:m[g*2+1]])
		}
	}
	return ""
}
