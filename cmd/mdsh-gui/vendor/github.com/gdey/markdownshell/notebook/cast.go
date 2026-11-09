package notebook

import (
	"bytes"
	"encoding/base64"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/docroot"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termrec"
)

// SetCastFrame writes frame=n, where a recording's player was left, so
// the file opens at that frame: on block i, a tty block, or with i < 0
// on the first cast fence whose recording is body. n <= 0 takes frame=
// out, for the last frame. Frames set one after another are one undo.
func (n *Notebook) SetCastFrame(i int, body string, frame int) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	var b *doc.Block
	switch {
	case i >= len(n.d.Blocks):
		return fmt.Errorf("no block %d", i)
	case i >= 0:
		b = n.d.Blocks[i]
		if run.OutKind(b) != "recording" {
			return fmt.Errorf("block %s isn't a tty block", n.d.Label(b))
		}
	default:
		want := strings.TrimSpace(body)
		for _, c := range n.d.Blocks {
			if c.Lang == "cast" && strings.TrimSpace(c.Body) == want {
				b = c
				break
			}
		}
		if b == nil {
			return fmt.Errorf("no cast fence holds that recording")
		}
	}
	info := doc.SplitInfo(b.Info)
	if frame > 0 {
		info.Set("frame", strconv.Itoa(frame), false)
	} else {
		info.Del("frame")
	}
	at := n.d.InfoSpan(b)
	src := append(append(append([]byte(nil), n.d.Src[:at.Start]...), info.String()...), n.d.Src[at.End:]...)
	return n.change("move the recording's frame", "frame:"+strconv.Itoa(at.Start), src, nil)
}

// castLink finds the recording in an output region: the ![alt](link)
// line of a tty block's cast.
var castLink = regexp.MustCompile(`!\[([^\]\n]*)\]\(([^)\s]+)\)`)

// ReplaceCast swaps tty block i's recording, in its output, for what
// as names: "text", the text frame n shows, counting from 1 (0 is the
// last), in an output fence; or "gif", an animated GIF made from it,
// saved beside the cast (or inline, for an inline cast) and linked in
// its place. The block still records a cast when it runs again. It is a
// rendered-view edit, so Undo puts the recording back.
func (n *Notebook) ReplaceCast(i int, as string, frame int) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	if i < 0 || i >= len(n.d.Blocks) {
		return fmt.Errorf("no block %d", i)
	}
	b := n.d.Blocks[i]
	if n.bg[b] {
		return ErrBusy
	}
	if run.OutKind(b) != "recording" {
		return fmt.Errorf("block %s isn't a tty block", n.d.Label(b))
	}
	start, end, link, err := n.castRegion(b)
	if err != nil {
		return err
	}
	dest, frag, _ := strings.Cut(link, "#")
	data, path, err := n.readCast(dest)
	if err != nil {
		return err
	}
	rec, err := termrec.ReadCast(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("the recording: %w", err)
	}
	old := n.results[i]
	fresh := *old
	var repl, what string
	switch as {
	case "text":
		text, err := termrec.StillText(rec, frame)
		if err != nil {
			return fmt.Errorf("the recording: %w", err)
		}
		attrs := "for=" + attrValue(b.ID)
		if old.Ran {
			attrs += " exit=" + strconv.Itoa(old.Status)
		}
		// The region's lines go, with the fence in their place.
		start, end = regionLines(n.d.Src, b, start, end)
		repl = prefixLines(render.Fence("output", attrs, text), b.Prefix)
		fresh.Image, fresh.Stdout = "", []byte(text)
		what = "turn the recording into text"
	case "gif":
		data, err := termrec.GIF(rec)
		if err != nil {
			return fmt.Errorf("the recording: %w", err)
		}
		if path == "" {
			repl = "data:image/gif;base64," + base64.StdEncoding.EncodeToString(data)
		} else {
			gifPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".gif"
			if err := docroot.WriteFile(docroot.Root(filepath.Dir(n.file)), gifPath, data); err != nil {
				return err
			}
			repl = strings.TrimSuffix(dest, pathpkg.Ext(dest)) + ".gif"
		}
		fresh.Image = repl
		if frag != "" {
			repl += "#" + frag
		}
		what = "turn the recording into a GIF"
	default:
		return fmt.Errorf("can't turn a recording into %q", as)
	}
	src := append(append(append([]byte(nil), n.d.Src[:start]...), repl...), n.d.Src[end:]...)
	if err := n.change(what, "", src, nil); err != nil {
		return err
	}
	// A run this session is written back from its result, so the result
	// holds the swap too.
	if old.Ran {
		nb := n.d.Blocks[i]
		fresh.Block = nb
		n.results[i] = &fresh
		n.sess.SetResult(i, &fresh)
		if len(n.undo) > 0 {
			top := &n.undo[len(n.undo)-1]
			top.res = append(top.res, resChange{nb, old, &fresh})
		}
	}
	return nil
}

// castRegion finds b's recording in its output: the link's offsets in
// the source, and the link.
func (n *Notebook) castRegion(b *doc.Block) (start, end int, link string, err error) {
	out := n.d.Src[b.Output.Start:b.Output.End]
	for _, m := range castLink.FindAllSubmatchIndex(out, -1) {
		l := string(out[m[4]:m[5]])
		dest, _, _ := strings.Cut(l, "#")
		if strings.HasPrefix(dest, "data:"+run.CastMIME) || strings.EqualFold(pathpkg.Ext(dest), ".cast") {
			return b.Output.Start + m[4], b.Output.Start + m[5], l, nil
		}
	}
	return 0, 0, "", fmt.Errorf("block %s has no recording in its output", n.d.Label(b))
}

// readCast reads the cast at dest, a data: URI or a path relative to
// the document, and returns the file's path, "" for a data: URI. A file
// must be inside the document's root: see package docroot.
func (n *Notebook) readCast(dest string) (data []byte, path string, err error) {
	if rest, ok := strings.CutPrefix(dest, "data:"); ok {
		meta, payload, _ := strings.Cut(rest, ",")
		if !strings.HasSuffix(meta, ";base64") {
			return nil, "", fmt.Errorf("the recording isn't base64")
		}
		data, err = base64.StdEncoding.DecodeString(payload)
		return data, "", err
	}
	path = filepath.FromSlash(dest)
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(n.file), path)
	}
	data, err = docroot.ReadFile(docroot.Root(filepath.Dir(n.file)), path, 0)
	return data, path, err
}

// regionLines widens [start, end), a link in b's output, to the whole
// lines of the output region holding it, from its opening comment to its
// closing one.
func regionLines(src []byte, b *doc.Block, start, end int) (int, int) {
	if o := bytes.LastIndex(src[b.Output.Start:start], []byte("<!-- mdsh:output for=")); o >= 0 {
		start = b.Output.Start + o
	}
	if c := bytes.Index(src[end:b.Output.End], []byte(doc.RegionEnd)); c >= 0 {
		end += c + len(doc.RegionEnd)
	}
	start = lineStart(src, start)
	if nl := bytes.IndexByte(src[end:], '\n'); nl >= 0 {
		end += nl + 1
	} else {
		end = len(src)
	}
	return start, end
}

// prefixLines starts each line of s with prefix, as a block nested in a
// quote or list does, trimming it on blank lines.
func prefixLines(s, prefix string) string {
	if prefix == "" {
		return s
	}
	lines := strings.SplitAfter(s, "\n")
	var b strings.Builder
	for _, l := range lines {
		if l == "" {
			continue
		}
		if strings.TrimSpace(l) == "" {
			b.WriteString(strings.TrimRight(prefix, " ") + l)
		} else {
			b.WriteString(prefix + l)
		}
	}
	return b.String()
}

// attrValue quotes an attribute value that needs it.
func attrValue(s string) string {
	if strings.ContainsAny(s, " \t\"{}") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}
