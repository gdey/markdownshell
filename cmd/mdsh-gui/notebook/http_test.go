package notebook

import (
	"testing"

	"github.com/gdey/markdownshell/doc"
)

func TestHTTPCellFromSavedOutput(t *testing.T) {
	src := "```http {#a}\nGET https://example.com\n```\n\n```output {for=a exit=1}\nHTTP/1.1 404 Not Found\nContent-Type: text/plain\nSet-Cookie: (secret)\n\nnope\n```\n\n```output {for=a stream=stderr}\nHTTP 200: got 404\n```\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	b := d.Blocks[0]
	c := httpCell(b, nil, d.Src[b.Span.End:b.Output.End])
	if c == nil || c.Status != "404 Not Found" || c.Code != 404 || c.Headers != "Content-Type: text/plain\nSet-Cookie: (secret)\n" || c.Body != "nope\n" || c.Checks != nil {
		t.Errorf("httpCell = %+v", c)
	}
	if c := httpCell(&doc.Block{Lang: "sh"}, nil, nil); c != nil {
		t.Errorf("a sh block got %+v", c)
	}
}
