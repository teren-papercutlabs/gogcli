package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"regexp"
	"testing"
)

func TestGogMarkdownFixedRegexReuse(t *testing.T) {
	patterns := []struct {
		re      *regexp.Regexp
		literal string
	}{
		{gogMarkdownNumberedListRe, `^(\d+)\.\s+(.+)`},
		{gogMarkdownLinkRe, `\[([^\]]+)\]\(([^)]+)\)`},
		{gogMarkdownInlineCodeRe, "`([^`]+)`"},
		{gogMarkdownBoldItalicRe, `\*\*\*([^*]+)\*\*\*`},
		{gogMarkdownBoldRe, `\*\*([^*]+)\*\*`},
		{gogMarkdownItalicRe, `\*([^*]+)\*`},
		{gogMarkdownHeadingRe, `^(#{1,6})\s+(.+)$`},
	}
	for _, p := range patterns {
		if p.re == nil || p.re.String() != p.literal {
			t.Errorf("fixed regex differs from %q", p.literal)
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "docs_markdown.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	mustCompile := func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return false
		}
		s, ok := c.Fun.(*ast.SelectorExpr)
		if !ok || s.Sel.Name != "MustCompile" {
			return false
		}
		id, ok := s.X.(*ast.Ident)
		return ok && id.Name == "regexp"
	}
	compiles := 0
	ast.Inspect(file, func(n ast.Node) bool {
		if mustCompile(n) {
			compiles++
		}
		return true
	})
	if compiles != 7 {
		t.Errorf("got %d compile sites, want seven", compiles)
	}
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			ast.Inspect(fn, func(n ast.Node) bool {
				if mustCompile(n) {
					t.Errorf("per-call compile in %s", fn.Name.Name)
				}
				return true
			})
		}
	}
}

func TestGogMarkdownRegexBehavior(t *testing.T) {
	block := "###### H\n0. zero\n- bullet\n| A | B |\n| --- | --- |\n| x | y |\nAfter"
	want := []MarkdownElement{
		{Type: MDHeading6, Content: "H"},
		{Type: MDNumberedList, Content: "zero"},
		{Type: MDListItem, Content: "bullet"},
		{Type: MDTable, TableCells: [][]string{{"A", "B"}, {"x", "y"}}},
		{Type: MDParagraph, Content: "After"},
	}
	if got := ParseMarkdown(block); !reflect.DeepEqual(got, want) {
		t.Errorf("elements=%#v want=%#v", got, want)
	}
	cases := []struct {
		input, plain string
		styles       []TextStyle
	}{
		{"", "", []TextStyle{}},
		{"*i*", "i", []TextStyle{{Italic: true, Start: 0, End: 1}}},
		{"***both*** **b** *i* `c` [l](u)", "both b *i* c l", []TextStyle{
			{Bold: true, Italic: true, Start: 0, End: 4},
			{Bold: true, Start: 5, End: 6},
			{Code: true, Start: 11, End: 12},
			{Link: "u", Start: 13, End: 14},
		}},
		{"[**b**](u) `**c**` ***d***", "**b** **c** d", []TextStyle{
			{Link: "u", Start: 0, End: 5}, {Code: true, Start: 6, End: 11}, {Bold: true, Italic: true, Start: 12, End: 13},
		}},
		{"é🎯 **b** x", "é🎯 b x", []TextStyle{{Bold: true, Start: 4, End: 5}}},
		{"`\xff` x", "\xff x", []TextStyle{{Code: true, Start: 0, End: 1}}},
	}
	// These goldens preserve observed precedence, including the unparsed *i*
	// beside bold above. Unicode has an ASCII suffix for the held nextRune bug.
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			styles, plain := ParseInlineFormatting(c.input)
			if plain != c.plain || !reflect.DeepEqual(styles, c.styles) {
				t.Errorf("text=%q styles=%#v want text=%q styles=%#v", plain, styles, c.plain, c.styles)
			}
		})
	}
}
