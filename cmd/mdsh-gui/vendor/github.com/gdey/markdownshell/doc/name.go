package doc

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"uuid"
)

// Rename is a block id that NameBlocks replaced.
type Rename struct {
	Old, New string
}

// NewBlockID returns a new id for a block that has none:
// block-<uuidv7>. Unlike block-N, it doesn't change when blocks move.
func NewBlockID() string { return "block-" + uuid.NewV7().String() }

// Named reports whether b's id is written in its fence, rather than
// block-N from its place in the document.
func (b *Block) Named() bool { return b.Attrs.Has("#") }

// NameBlocks writes an id from newID into the fence of each block that
// has none and that want picks, and carries what pointed at its block-N
// id over to the new one: its saved output, output copies, tapes, the
// placeholders and conditions that name it, and links to its files under
// <doc>.assets/. Every *Block in d keeps its identity, as with SetBlock.
// It returns the ids it replaced, in document order.
func (d *Document) NameBlocks(want func(*Block) bool, newID func() string) ([]Rename, error) {
	var renames []Rename
	named := map[*Block]string{}
	for _, b := range d.Blocks {
		if b.Named() || b.IsInclude() || b.IsTape() || !want(b) {
			continue
		}
		id := newID()
		renames = append(renames, Rename{b.ID, id})
		named[b] = id
	}
	if len(renames) == 0 {
		return nil, nil
	}
	if err := d.rename(renames, named, true); err != nil {
		return nil, err
	}
	return renames, nil
}

// historyID is the id the History pane gives a command it adds.
var historyID = regexp.MustCompile(`^history-[0-9]+$`)

// IsHistoryID reports whether id is one the History pane gives the
// blocks it adds, history-N.
func IsHistoryID(id string) bool { return historyID.MatchString(id) }

// RenameBlock gives b, a block whose id is written in its fence, the id
// id, and carries over what names it: its saved output and output
// copies (for=), tapes, placeholders, conditions and other blocks'
// depends=. Links to its files under <doc>.assets/ keep the old name,
// since the files keep it until the block runs again. Every *Block in d
// keeps its identity, as with NameBlocks.
func (d *Document) RenameBlock(b *Block, id string) error {
	if !b.Named() || b.IsInclude() || b.IsTape() {
		return errors.New("can't rename block " + d.Label(b) + ": only a block with an id of its own can be renamed")
	}
	if b.ID == id {
		return nil
	}
	if d.Block(id) != nil {
		return errors.New("can't rename block " + d.Label(b) + ": #" + id + " is taken")
	}
	return d.rename([]Rename{{b.ID, id}}, map[*Block]string{b: id}, false)
}

// rename rewrites the source for renames, where named is the blocks
// whose fences get their new ids. links says whether links to asset
// files follow too.
func (d *Document) rename(renames []Rename, named map[*Block]string, links bool) error {
	// Each region of the source that can name a block gets the
	// rewrites that apply there.
	type region struct {
		span   Span
		assets bool   // holds output, so links to asset files
		id     string // the block whose opening fence line this is
		b      *Block
	}
	var regions []region
	for _, b := range d.Blocks {
		first := Span{b.Span.Start, lineEnd(d.Src, b.Span.Start)}
		regions = append(regions, region{span: first, id: named[b], b: b})
		if b.IsTape() && !b.IsInclude() {
			regions = append(regions, region{span: b.BodySpan})
		}
		if b.HasOutput() {
			regions = append(regions, region{span: b.Output, assets: true})
		}
	}
	for _, c := range d.Copies {
		regions = append(regions, region{span: c.Span, assets: true})
	}
	for _, p := range d.Placeholders {
		regions = append(regions, region{span: p.Span})
	}
	for _, s := range d.Sections {
		regions = append(regions, region{span: s.If})
	}
	slices.SortFunc(regions, func(a, b region) int { return a.span.Start - b.span.Start })

	type rewrite struct {
		re   *regexp.Regexp
		with string
	}
	var refs, assets []rewrite
	for _, r := range renames {
		old := regexp.QuoteMeta(r.Old)
		refs = append(refs,
			rewrite{regexp.MustCompile(`\{\{#` + old + `/`), "{{#" + r.New + "/"},
			rewrite{regexp.MustCompile(`(for=)("?)` + old + `("|[\s}]|$)`), "${1}${2}" + r.New + "${3}"})
		if links {
			assets = append(assets, rewrite{regexp.MustCompile(`(\.assets/)` + old + `\.`), "${1}" + r.New + "."})
		}
	}
	news := map[string]string{}
	for _, r := range renames {
		news[r.Old] = r.New
	}

	var src bytes.Buffer
	pos := 0
	for _, g := range regions {
		if g.span.Start < pos {
			return errors.New("can't name blocks: parts of the document overlap")
		}
		src.Write(d.Src[pos:g.span.Start])
		s := string(d.Src[g.span.Start:g.span.End])
		for _, r := range refs {
			s = r.re.ReplaceAllString(s, r.with)
		}
		if g.assets {
			for _, r := range assets {
				s = r.re.ReplaceAllString(s, r.with)
			}
		}
		if g.b != nil {
			s = renameDepends(s, news)
		}
		if g.id != "" {
			s = setID(s, g.b, g.id)
		}
		src.WriteString(s)
		pos = g.span.End
	}
	src.Write(d.Src[pos:])

	nd, err := Parse(src.Bytes())
	if err != nil {
		return err
	}
	if len(nd.Blocks) != len(d.Blocks) {
		return errors.New("can't name blocks: naming them would change the document's blocks")
	}
	for i, nb := range nd.Blocks {
		b := d.Blocks[i]
		want := b.ID
		if id, ok := named[b]; ok {
			want = id
		}
		if nb.ID != want || nb.HasOutput() != b.HasOutput() {
			return errors.New("can't name blocks: block " + b.ID + " didn't keep its place or output")
		}
	}
	blocks := d.Blocks
	for i, nb := range nd.Blocks {
		*blocks[i] = *nb
	}
	*d = *nd
	d.Blocks = blocks
	return nil
}

// dependsAttr is a depends= attribute on a fence line.
var dependsAttr = regexp.MustCompile(`(\bdepends=)("[^"]*"|[^\s}]*)`)

// renameDepends renames the ids in line's depends= that news maps.
func renameDepends(line string, news map[string]string) string {
	return dependsAttr.ReplaceAllStringFunc(line, func(m string) string {
		key, val, _ := strings.Cut(m, "=")
		q := strings.HasPrefix(val, `"`)
		val = strings.Trim(val, `"`)
		ids := strings.Split(val, ",")
		for i, id := range ids {
			t := strings.TrimSpace(id)
			if n, ok := news[t]; ok {
				ids[i] = strings.Replace(id, t, n, 1)
			}
		}
		val = strings.Join(ids, ",")
		if q {
			val = `"` + val + `"`
		}
		return key + "=" + val
	})
}

// setID sets #id in the info string on line, b's opening fence line:
// in place of the id written there, or first.
func setID(line string, b *Block, id string) string {
	at := strings.Index(line, b.Fence) + len(b.Fence)
	rest := line[at:]
	end := len(strings.TrimRight(rest, "\r\n"))
	info := strings.TrimSpace(rest[:end])
	in := SplitInfo(info)
	if k := slices.IndexFunc(in.Tokens, func(t string) bool { return tokenKey(t) == "#" }); k >= 0 {
		in.Tokens[k] = "#" + id
	} else {
		in.Tokens = append([]string{"#" + id}, in.Tokens...)
	}
	lead := rest[:len(rest[:end])-len(strings.TrimLeft(rest[:end], " \t"))]
	return line[:at] + lead + in.String() + rest[end:]
}

// Label is how messages name b: its id, or "at line N" when mdsh made
// the id up, as block-N or block-<uuidv7>, since those tell a reader
// nothing. "block " + Label reads well either way.
func (d *Document) Label(b *Block) string {
	if b.Named() && !GeneratedID(b.ID) {
		return b.ID
	}
	return fmt.Sprintf("at line %d", lineNo(d.Src, b.Span.Start))
}

// GeneratedID reports whether id is one NewBlockID makes.
func GeneratedID(id string) bool {
	rest, ok := strings.CutPrefix(id, "block-")
	if !ok {
		return false
	}
	_, err := uuid.Parse(rest)
	return err == nil
}
