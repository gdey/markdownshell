package run

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gdey/markdownshell/doc"
)

// Win is the lines an include shows.
type Win struct {
	// Text is the lines, as the file has them.
	Text []byte
	// First is the line number of Text's first line, counting from 1. It
	// is 0 when nothing matched.
	First int
	// Warning says why the window is empty, when nothing matched, or
	// that more than one definition did. It doesn't fail the include.
	Warning string
}

// Window returns the lines of data that inc asks for: from= to to=, or
// the first line matching match= or the definition symbol= names, with
// before=, lines= and until= around it. lang is the include's language,
// which with the file's name says whether symbol= can read it as Go.
//
// Nothing matching is not an error: the window is empty and Warning
// says so. An error is a window that can't be cut, as from= past the
// end of the file.
func Window(data []byte, inc doc.Include, lang string) (Win, error) {
	if !inc.Windowed() && inc.Before == 0 && inc.Lines == 0 && inc.Until == "" {
		text, err := Lines(data, inc.From, inc.To)
		if err != nil {
			return Win{}, err
		}
		return Win{Text: text, First: max(inc.From, 1)}, nil
	}
	starts := lineStarts(data)
	n := len(starts)
	line := func(i int) []byte { // line i, counting from 1, without its line end
		end := len(data)
		if i < n {
			end = starts[i]
		}
		return bytes.TrimRight(data[starts[i-1]:end], "\r\n")
	}
	var w Win
	start, end := max(inc.From, 1), n
	if inc.To > 0 {
		end = min(inc.To, n)
	}
	switch {
	case inc.Match != "":
		re := regexp.MustCompile(inc.Match) // doc checked it
		start = 0
		for i := 1; i <= n; i++ {
			if re.Match(line(i)) {
				start = i
				break
			}
		}
		if start == 0 {
			return Win{Warning: fmt.Sprintf("include: no line of %s matches %q", inc.File, inc.Match)}, nil
		}
	case inc.Symbol != "":
		s, e, warn := symbolLines(data, inc, lang)
		if s == 0 {
			return Win{Warning: warn}, nil
		}
		start, end, w.Warning = s, e, warn
	default:
		if start > n {
			return Win{}, fmt.Errorf("include: from=%d is past the end of the file (%d lines)", start, n)
		}
	}
	switch {
	case inc.Lines > 0:
		end = min(start+inc.Lines-1, n)
	case inc.Until != "":
		re := regexp.MustCompile(inc.Until)
		end = n
		for i := start; i <= n; i++ {
			if re.Match(line(i)) {
				end = i
				break
			}
		}
	}
	start = max(start-inc.Before, 1)
	stop := len(data)
	if end < n {
		stop = starts[end]
	}
	w.Text, w.First = data[starts[start-1]:stop], start
	return w, nil
}

// lineStarts is the offset in data of each line's first byte.
func lineStarts(data []byte) []int {
	starts := make([]int, 0, LineCount(data))
	for i := 0; i < len(data); {
		starts = append(starts, i)
		j := bytes.IndexByte(data[i:], '\n')
		if j < 0 {
			break
		}
		i += j + 1
	}
	return starts
}

// symbolLines finds the definition inc.Symbol names in data, as lines
// start through end, counting from 1. start is 0 when there is none,
// and warn says why. warn can also be set with a definition, when more
// than one has the name: the first is used.
func symbolLines(data []byte, inc doc.Include, lang string) (start, end int, warn string) {
	if lang != "go" && filepath.Ext(inc.File) != ".go" {
		return 0, 0, fmt.Sprintf("include: symbol= finds only Go definitions so far, and %s isn't Go", inc.File)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, inc.File, data, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return 0, 0, fmt.Sprintf("include: symbol=%s: %s doesn't read as Go: %v", inc.Symbol, inc.File, err)
	}
	recv, name := splitSymbol(inc.Symbol)
	type span struct{ from, to token.Pos }
	var found []span
	add := func(node ast.Node, comment *ast.CommentGroup) {
		s := span{node.Pos(), node.End()}
		if comment != nil && !inc.NoComment {
			s.from = comment.Pos()
		}
		found = append(found, s)
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == name && recvName(d) == recv {
				add(d, d.Doc)
			}
		case *ast.GenDecl:
			if recv != "" {
				continue
			}
			for _, spec := range d.Specs {
				var names []*ast.Ident
				var comment *ast.CommentGroup
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names, comment = []*ast.Ident{s.Name}, s.Doc
				case *ast.ValueSpec:
					names, comment = s.Names, s.Doc
				}
				for _, id := range names {
					if id.Name != name {
						continue
					}
					// Alone, the spec is the whole declaration, keyword and
					// all; in a group it is its own lines.
					if !d.Lparen.IsValid() {
						add(d, d.Doc)
					} else {
						add(spec, comment)
					}
				}
			}
		}
	}
	if len(found) == 0 {
		return 0, 0, fmt.Sprintf("include: %s defines no %s", inc.File, inc.Symbol)
	}
	if len(found) > 1 {
		warn = fmt.Sprintf("include: %s defines %s %d times; showing the first", inc.File, inc.Symbol, len(found))
	}
	return fset.Position(found[0].from).Line, fset.Position(found[0].to).Line, warn
}

// splitSymbol splits a symbol= name into its receiver type, "" for none,
// and its name: T.M and (*T).M are method M of T.
func splitSymbol(s string) (recv, name string) {
	i := strings.LastIndex(s, ".")
	if i < 0 {
		return "", s
	}
	recv = strings.TrimSuffix(strings.TrimPrefix(s[:i], "("), ")")
	return strings.TrimPrefix(recv, "*"), s[i+1:]
}

// recvName is the name of the type method d is on, without * or type
// parameters, or "" for a function.
func recvName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return ""
	}
	t := d.Recv.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
		case *ast.ParenExpr:
			t = x.X
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		case *ast.Ident:
			return x.Name
		default:
			return ""
		}
	}
}
