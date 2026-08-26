package extract

import (
	"regexp"
	"strings"
)

// markdownExtractor finds the two link forms that connect notes to each other:
// wiki-style [[links]] and inline [text](relative/path.md).
type markdownExtractor struct{}

func (markdownExtractor) Exts() []string {
	return []string{".md", ".mdx", ".markdown"}
}

var (
	wikiLinkRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)
	mdLinkRe   = regexp.MustCompile(`\]\(\s*<?([^)<>\s]+)>?(?:\s+"[^"]*")?\s*\)`)
	fenceRe    = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~|`[^`\n]*`")
	schemeRe   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:|^//`)
)

func (markdownExtractor) Refs(src []byte, fc FileContext) []Ref {
	src = blankOut(src, fenceRe)
	var refs []Ref

	for _, m := range wikiLinkRe.FindAllSubmatchIndex(src, -1) {
		target := string(src[m[2]:m[3]])
		// [[note|alias]] and [[note#heading]] both point at "note".
		if i := strings.IndexAny(target, "|#"); i >= 0 {
			target = target[:i]
		}
		if target = strings.TrimSpace(target); target == "" {
			continue
		}
		refs = append(refs, Ref{
			Spec:     target,
			Kind:     "reference",
			Line:     lineOf(src, m[0]),
			Strategy: Basename,
			TryExts:  []string{".md", ".markdown", ".mdx"},
		})
	}

	for _, m := range mdLinkRe.FindAllSubmatchIndex(src, -1) {
		target := string(src[m[2]:m[3]])
		if target == "" || schemeRe.MatchString(target) || strings.HasPrefix(target, "#") {
			continue // external URL or same-page anchor
		}
		refs = append(refs, Ref{
			Spec:     target,
			Kind:     "reference",
			Line:     lineOf(src, m[0]),
			Strategy: RelPath,
			TryExts:  []string{".md", ".markdown", ".mdx"},
		})
	}

	return refs
}

// blankOut replaces every match with spaces, keeping newlines so that byte
// offsets and line numbers survive.
func blankOut(src []byte, re *regexp.Regexp) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	for _, m := range re.FindAllIndex(out, -1) {
		for i := m[0]; i < m[1]; i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	}
	return out
}
