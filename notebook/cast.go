package notebook

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
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
