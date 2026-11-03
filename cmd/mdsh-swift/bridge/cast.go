package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/gif"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termrec"
)

// The cast player. A tty block's output, and a cast fence, is an
// asciinema recording, which Go links as an image: a /doc/<path>.cast
// link or a data:application/x-asciicast URI, with #frame=N for where
// the file says to open. The app asks castLoad for the recording made
// into frames, as termrec's GIF has them, so frame=N names the same
// screen here as in mdsh-gui and a PDF, then castFrame for each frame
// it shows, as a PNG, and castGIF to copy the whole as an animated GIF.

// castInfo is what castLoad answers: the recording's id, for castFrame
// and castGIF, its frames' delays in milliseconds, its size in pixels
// and its width in columns, and the frame the link names, counting from
// 1, or 0 for the last.
type castInfo struct {
	ID   string `json:"id"`
	Cols int    `json:"cols"`
	// Start is when the first frame shows, in milliseconds: the
	// recording's first write. The player counts its clock from it.
	Start  int   `json:"start"`
	Delays []int `json:"delays"`
	Width  int   `json:"width"`
	Height int   `json:"height"`
	Frame  int   `json:"frame"`
}

// casts keeps the recordings loaded, as GIFs, by id.
type casts struct {
	mu   sync.Mutex
	gifs map[string][]byte
}

// castBytes reads the recording src links to: a data: URI, or a
// /doc/<path> link to a .cast file, as the notebook writes them. It
// returns the frame its #frame=N names too.
func castBytes(src string) ([]byte, int, error) {
	src, frag, _ := strings.Cut(src, "#")
	frame := 0
	if v, ok := strings.CutPrefix(frag, "frame="); ok {
		frame, _ = strconv.Atoi(v)
	}
	if data, ok := strings.CutPrefix(src, "data:"+run.CastMIME+";base64,"); ok {
		b, err := base64.StdEncoding.DecodeString(data)
		return b, frame, err
	}
	path, ok := strings.CutPrefix(src, "/doc")
	if !ok {
		return nil, 0, fmt.Errorf("not a recording: %q", src)
	}
	path, _, _ = strings.Cut(path, "?")
	if p, err := url.PathUnescape(path); err == nil {
		path = p
	}
	if !strings.EqualFold(filepath.Ext(path), ".cast") {
		return nil, 0, fmt.Errorf("not a recording: %q", src)
	}
	b, err := os.ReadFile(path)
	return b, frame, err
}

// load makes the recording at src into a GIF, keeping it.
func (c *casts) load(src string) (castInfo, error) {
	data, frame, err := castBytes(src)
	if err != nil {
		return castInfo{}, err
	}
	if !termrec.IsCast(data) {
		return castInfo{}, errors.New("not an asciinema recording")
	}
	rec, err := termrec.ReadCast(bytes.NewReader(data))
	if err != nil {
		return castInfo{}, err
	}
	g, err := termrec.GIF(rec)
	if err != nil {
		return castInfo{}, err
	}
	dec, err := gif.DecodeAll(bytes.NewReader(g))
	if err != nil {
		return castInfo{}, err
	}
	sum := sha256.Sum256(data)
	start := 0
	if len(rec.Events) > 0 {
		start = int(rec.Events[0].At.Milliseconds())
	}
	info := castInfo{ID: hex.EncodeToString(sum[:8]), Cols: rec.Cols, Start: start, Width: dec.Config.Width, Height: dec.Config.Height, Frame: frame}
	for _, d := range dec.Delay {
		info.Delays = append(info.Delays, d*10)
	}
	if info.Frame > len(info.Delays) {
		info.Frame = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gifs == nil {
		c.gifs = map[string][]byte{}
	}
	c.gifs[info.ID] = g
	return info, nil
}

func (c *casts) gif(id string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	g, ok := c.gifs[id]
	if !ok {
		return nil, fmt.Errorf("no recording %q loaded", id)
	}
	return g, nil
}

// frame is frame n of recording id, counting from 1, as a PNG.
func (c *casts) frame(id string, n int) ([]byte, error) {
	g, err := c.gif(id)
	if err != nil {
		return nil, err
	}
	img, err := termrec.Frame(g, n)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
