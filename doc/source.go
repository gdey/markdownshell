package doc

import (
	"bytes"
	"reflect"
	"sort"
)

// SetSource replaces d's source with src and parses it again, as when
// the whole file is edited at once. A block whose info string and body
// are unchanged keeps its identity, wherever it moved, with its spans
// moved to match src, so results that point at it stay valid. Edited and
// new blocks are new *Blocks, and blocks no longer in src are dropped.
// A nosave input's session value (see SetValue) is kept by id, so what
// was typed survives an edit elsewhere in the file. If src doesn't
// parse, d is left as it was.
func (d *Document) SetSource(src []byte) error {
	if d.splice(src) {
		return nil
	}
	return d.reparse(src)
}

// reparse is SetSource by a parse of the whole of src.
func (d *Document) reparse(src []byte) error {
	nd, err := Parse(src)
	if err != nil {
		return err
	}
	session := map[string]*string{}
	for _, b := range d.Blocks {
		if b.session != nil {
			session[b.ID] = b.session
		}
	}
	// Equal blocks are matched in order, so duplicates keep theirs.
	old := map[string][]*Block{}
	for _, b := range d.Blocks {
		k := sameKey(b)
		old[k] = append(old[k], b)
	}
	for i, nb := range nd.Blocks {
		k := sameKey(nb)
		if bs := old[k]; len(bs) > 0 {
			b := bs[0]
			old[k] = bs[1:]
			*b = *nb
			nd.Blocks[i] = b
		}
	}
	for _, nb := range nd.Blocks {
		if v, ok := session[nb.ID]; ok && nb.IsInput() && nb.NoSave() {
			nb.session = v
		}
	}
	*d = *nd
	return nil
}

func sameKey(b *Block) string { return b.Info + "\x00" + b.Body }

// splice is SetSource for an edit to the prose between two blocks, as
// typing in prose makes, or to the body of a plain block, as typing in a
// block makes, without parsing the whole of src: it parses from the
// fence of the block above the edit, or of the block edited, to that of
// the block below, and if that finds what it did before, moved, but for
// the edited block's body, d takes src with everything after the edit
// moved. The edited block is a new *Block with the old one's id. It
// reports whether it could; when it can't, as when the edit changed a
// fence or one of the lines mdsh reads in prose, d is left as it was. A
// parse of the whole file was most of the time an edit to a long one
// took.
func (d *Document) splice(src []byte) bool {
	was := d.Src
	// The edit took was[p:wasEnd] and put src[p:end] there.
	p := commonPrefix(was, src)
	q := commonSuffix(was[p:], src[p:])
	wasEnd, end := len(was)-q, len(src)-q
	delta := len(src) - len(was)
	k := sort.Search(len(d.Blocks), func(i int) bool { return d.Blocks[i].Span.Start >= p })
	if k < len(d.Blocks) && d.Blocks[k].Span.Start < wasEnd {
		return false
	}
	// The parse starts at the fence of the block above, which starts a
	// line on its own, outside any list or quote, so the parse can start
	// there as at the top of the file; or, above the first block, where
	// Parse starts, after the shebang and front matter.
	from := 0
	if d.Shebang != "" {
		from = lineEnd(was, 0)
	}
	// An edit to the first line can start a shebang or front matter.
	if p < lineEnd(was, from) || d.FrontMatterErr != nil {
		return false
	}
	from = max(from, d.FrontMatter.End)
	var prev *Block
	edited := false // the edit is in prev's body
	if k > 0 {
		prev = d.Blocks[k-1]
		if prev.quote != 0 || prev.col != 0 || prev.Lead != "" || prev.Prefix != "" || prev.IsInput() {
			return false
		}
		if p < prev.Span.End {
			// The body of a block that nothing else reads, between its
			// fences.
			if p < prev.BodySpan.Start || wasEnd > prev.BodySpan.End || prev.BodySpan.End == prev.Span.End ||
				prev.IsInclude() || prev.IsTape() || prev.IsHTTP() {
				return false
			}
			edited = true
		} else if prev.Output.End > p {
			return false
		}
		from = prev.Span.Start
	} else if p < from {
		return false
	}
	stop, oldStop := len(src), len(was)
	var next *Block
	if k < len(d.Blocks) {
		next = d.Blocks[k]
		if next.IsInput() {
			return false
		}
		oldStop = next.Span.Start + 1
		stop = oldStop + delta
	}
	lines := bytes.Count(src[p:end], []byte{'\n'}) - bytes.Count(was[p:wasEnd], []byte{'\n'})
	if lines != 0 {
		// Warnings name lines.
		if len(d.Warnings) > 0 {
			return false
		}
		for _, b := range d.Blocks {
			if b.VarTaken != "" {
				return false
			}
		}
	}
	nd := &Document{Src: src, Options: d.Options}
	c := countsOf(d.Blocks[:max(k-1, 0)])
	if _, err := parseLines(nd, from, stop, c); err != nil {
		return false
	}
	// Where something of was's is in src, if the edit left it whole.
	move := func(s Span) (Span, bool) {
		switch {
		case s.End <= p:
			return s, true
		case s.Start >= wasEnd:
			return Span{s.Start + delta, s.End + delta}, true
		}
		return s, false
	}
	in := func(s Span) bool { return s.Start >= from && s.Start < oldStop }
	var want []*Block
	if prev != nil {
		want = append(want, prev)
	}
	if next != nil {
		want = append(want, next)
	}
	if len(nd.Blocks) != len(want) || (next != nil && !sameFence(nd.Blocks[len(want)-1], next, delta)) {
		return false
	}
	if edited {
		if !sameEdited(nd.Blocks[0], prev, delta) || d.twin(prev, nd.Blocks[0]) {
			return false
		}
	} else if prev != nil && !sameBlock(nd.Blocks[0], prev, 0) {
		return false
	}
	var copies []*Copy
	for _, c := range d.Copies {
		if in(c.Span) {
			copies = append(copies, c)
		}
	}
	if len(copies) != len(nd.Copies) {
		return false
	}
	for i, c := range copies {
		m, ok := move(c.Span)
		n := *nd.Copies[i]
		n.Span = c.Span
		if !ok || m != nd.Copies[i].Span || n != *c {
			return false
		}
	}
	var folds []*Fold
	for _, f := range d.Folds {
		if in(f.Span) {
			folds = append(folds, f)
		}
	}
	if len(folds) != len(nd.Folds) {
		return false
	}
	for i, f := range folds {
		if m, ok := move(f.Span); !ok || m != nd.Folds[i].Span || !reflect.DeepEqual(f.Cond, nd.Folds[i].Cond) {
			return false
		}
	}
	var sections []*Section
	for _, s := range d.Sections {
		if in(s.If) || in(s.End) {
			sections = append(sections, s)
		}
	}
	if len(sections) != len(nd.Sections) {
		return false
	}
	for i, s := range sections {
		ns := nd.Sections[i]
		mi, ok := move(s.If)
		me, ok2 := move(s.End)
		if !ok || !ok2 || mi != ns.If || me != ns.End || !reflect.DeepEqual(s.Cond, ns.Cond) {
			return false
		}
	}
	var holders []*Placeholder
	for _, h := range d.Placeholders {
		if in(h.Span) {
			holders = append(holders, h)
		}
	}
	if len(holders) != len(nd.Placeholders) {
		return false
	}
	for i, h := range holders {
		n := *nd.Placeholders[i]
		m, ok := move(h.Span)
		n.Span = h.Span
		if !ok || m != nd.Placeholders[i].Span || !reflect.DeepEqual(n, *h) {
			return false
		}
	}

	// It is the same document, moved.
	d.Src = src
	if edited {
		d.Blocks[k-1] = nd.Blocks[0]
	}
	if delta == 0 {
		return true
	}
	at := func(s *Span) {
		if s.Start >= wasEnd {
			s.Start += delta
			s.End += delta
		}
	}
	for _, b := range d.Blocks[k:] {
		b.move(delta)
	}
	for _, c := range d.Copies {
		at(&c.Span)
	}
	for _, f := range d.Folds {
		at(&f.Span)
	}
	for _, s := range d.Sections {
		at(&s.If)
		at(&s.End)
	}
	for _, h := range d.Placeholders {
		at(&h.Span)
	}
	return true
}

// move moves b's spans by delta bytes.
func (b *Block) move(delta int) {
	m := func(s *Span) {
		s.Start += delta
		s.End += delta
	}
	m(&b.Span)
	m(&b.BodySpan)
	m(&b.Output)
	for i := range b.CurrentParts {
		m(&b.CurrentParts[i])
	}
	for i := range b.History {
		e := &b.History[i]
		m(&e.Span)
		for j := range e.Parts {
			m(&e.Parts[j])
		}
	}
}

// sameBlock says a, from a parse of part of the file, is b, moved by
// delta, but for what a parse of part of the file can't know: its id
// when it has none of its own, and the var= it binds.
func sameBlock(a, b *Block, delta int) bool {
	x := *b
	x.move(delta)
	x.ID, x.VarTaken, x.MirrorOf, x.session = a.ID, a.VarTaken, a.MirrorOf, a.session
	return reflect.DeepEqual(*a, x)
}

// sameEdited says a, from a parse of part of the file, is b with its
// body edited, the edit moving what is after the body by delta.
func sameEdited(a, b *Block, delta int) bool {
	x := *b
	x.move(delta)
	x.Span.Start, x.BodySpan.Start = b.Span.Start, b.BodySpan.Start
	x.Body = a.Body
	return reflect.DeepEqual(*a, x)
}

// twin reports whether a block of d's other than b is the same as b or
// as nb, b edited, by sameKey: SetSource would match them up otherwise.
func (d *Document) twin(b, nb *Block) bool {
	for _, o := range d.Blocks {
		if o != b && o.Info == b.Info && (o.Body == b.Body || o.Body == nb.Body) {
			return true
		}
	}
	return false
}

// sameFence says a is b's fence, moved by delta: a parse that stopped at
// b's fence has b's block but not the output under it.
func sameFence(a, b *Block, delta int) bool {
	return a.Span == (Span{b.Span.Start + delta, b.Span.End + delta}) && a.BodySpan == (Span{b.BodySpan.Start + delta, b.BodySpan.End + delta}) &&
		a.Output.Start == b.Output.Start+delta && a.Info == b.Info && a.Body == b.Body && a.Fence == b.Fence && a.Lang == b.Lang &&
		a.Prefix == b.Prefix && a.Lead == b.Lead && a.quote == b.quote && a.col == b.col && (a.ID == b.ID || !b.Named())
}

// chunk is how many bytes commonPrefix and commonSuffix compare at a
// time, as bytes.Equal is much quicker than a loop.
const chunk = 4096

// commonPrefix is how many bytes a and b start with alike.
func commonPrefix(a, b []byte) int {
	m := min(len(a), len(b))
	i := 0
	for i+chunk <= m && bytes.Equal(a[i:i+chunk], b[i:i+chunk]) {
		i += chunk
	}
	for i < m && a[i] == b[i] {
		i++
	}
	return i
}

// commonSuffix is how many bytes a and b end with alike.
func commonSuffix(a, b []byte) int {
	m := min(len(a), len(b))
	i := 0
	for i+chunk <= m && bytes.Equal(a[len(a)-i-chunk:len(a)-i], b[len(b)-i-chunk:len(b)-i]) {
		i += chunk
	}
	for i < m && a[len(a)-1-i] == b[len(b)-1-i] {
		i++
	}
	return i
}
