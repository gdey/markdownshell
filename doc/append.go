package doc

import (
	"bytes"
	"errors"
	"fmt"
)

// AppendBlock adds a fenced block of lang holding body to the end of d,
// after a blank line, and returns it. Earlier blocks keep their spans, so
// results for them stay valid. It fails if the end of d would swallow
// the block, such as an unclosed fence.
func (d *Document) AppendBlock(lang, body string) (*Block, error) {
	if body == "" {
		return nil, errors.New("empty block")
	}
	if body[len(body)-1] != '\n' {
		body += "\n"
	}
	fence := "```"
	for bytes.Contains([]byte(body), []byte(fence)) {
		fence += "`"
	}
	src := bytes.TrimRight(d.Src, "\n")
	src = append(append([]byte(nil), src...), "\n\n"+fence+lang+"\n"+body+fence+"\n"...)
	if len(bytes.TrimSpace(d.Src)) == 0 {
		src = src[2:]
	}
	nd, err := Parse(src)
	if err != nil {
		return nil, err
	}
	if len(nd.Blocks) != len(d.Blocks)+1 {
		return nil, fmt.Errorf("the end of the document doesn't take a new block")
	}
	b := nd.Blocks[len(nd.Blocks)-1]
	if b.Prefix != "" || b.Lang != lang {
		return nil, fmt.Errorf("the end of the document doesn't take a new block")
	}
	d.Src = src
	d.Blocks = append(d.Blocks, b)
	return b, nil
}
