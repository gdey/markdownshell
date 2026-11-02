// Package lsp is mdsh's language server: for the markdown files mdsh
// runs, it completes fences' attributes and depends= names, explains an
// attribute on hover and reports mistakes in them as diagnostics. It
// speaks the Language Server Protocol over a stream, as `mdsh lsp` and
// mdsh-gui's Neovim pane run it.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
)

// Serve answers LSP requests read from r on w until the client exits or
// r ends. File, if set, is the document's path for those opened with a
// URI that isn't a file:// one, for completing paths beside it.
func Serve(r io.Reader, w io.Writer, file string) error {
	s := &server{out: w, docs: map[string]string{}, file: file, enc: "utf-16"}
	in := bufio.NewReader(r)
	for {
		msg, err := read(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if s.handle(msg) {
			return nil
		}
	}
}

type server struct {
	mu   sync.Mutex
	out  io.Writer
	docs map[string]string // open documents' text, by URI
	file string
	enc  string // position encoding: utf-8 or utf-16
	down bool   // shutdown was asked
}

type message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

// read reads one message: headers, a blank line, then Content-Length
// bytes of JSON.
func read(r *bufio.Reader) (message, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if line == "" && length < 0 {
				return message{}, io.EOF
			}
			return message{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Content-Length") {
			if length, err = strconv.Atoi(strings.TrimSpace(v)); err != nil {
				return message{}, fmt.Errorf("bad Content-Length %q", v)
			}
		}
	}
	if length < 0 {
		return message{}, errors.New("a message without Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return message{}, err
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return message{}, err
	}
	return m, nil
}

func (s *server) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(b), b)
}

func (s *server) reply(id json.RawMessage, result any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *server) fail(id json.RawMessage, code int, msg string) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

func (s *server) notify(method string, params any) {
	s.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type rng struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type docID struct {
	URI string `json:"uri"`
}

type docPosition struct {
	TextDocument docID    `json:"textDocument"`
	Position     position `json:"position"`
}

// handle answers m and reports whether the client is done.
func (s *server) handle(m message) bool {
	isRequest := len(m.ID) > 0
	switch m.Method {
	case "initialize":
		var p struct {
			Capabilities struct {
				General struct {
					PositionEncodings []string `json:"positionEncodings"`
				} `json:"general"`
			} `json:"capabilities"`
			InitializationOptions struct {
				File string `json:"file"`
			} `json:"initializationOptions"`
		}
		json.Unmarshal(m.Params, &p)
		if slices.Contains(p.Capabilities.General.PositionEncodings, "utf-8") {
			s.enc = "utf-8"
		}
		if f := p.InitializationOptions.File; f != "" {
			s.file = f
		}
		s.reply(m.ID, map[string]any{
			"capabilities": map[string]any{
				"positionEncoding": s.enc,
				"textDocumentSync": map[string]any{"openClose": true, "change": 1},
				"completionProvider": map[string]any{
					"triggerCharacters": []string{"{", " ", "=", ",", "#", `"`},
				},
				"hoverProvider": true,
			},
			"serverInfo": map[string]any{"name": "mdsh"},
		})
	case "shutdown":
		s.down = true
		s.reply(m.ID, nil)
	case "exit":
		return true
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			s.docs[p.TextDocument.URI] = p.TextDocument.Text
			s.publish(p.TextDocument.URI)
		}
	case "textDocument/didChange":
		var p struct {
			TextDocument   docID `json:"textDocument"`
			ContentChanges []struct {
				Range *rng   `json:"range"`
				Text  string `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			text := s.docs[p.TextDocument.URI]
			for _, c := range p.ContentChanges {
				if c.Range == nil {
					text = c.Text
					continue
				}
				from := s.offset(text, c.Range.Start)
				to := s.offset(text, c.Range.End)
				if from <= to {
					text = text[:from] + c.Text + text[to:]
				}
			}
			s.docs[p.TextDocument.URI] = text
			s.publish(p.TextDocument.URI)
		}
	case "textDocument/didClose":
		var p struct {
			TextDocument docID `json:"textDocument"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			delete(s.docs, p.TextDocument.URI)
			s.notify("textDocument/publishDiagnostics", map[string]any{"uri": p.TextDocument.URI, "diagnostics": []any{}})
		}
	case "textDocument/completion":
		var p docPosition
		if err := json.Unmarshal(m.Params, &p); err != nil {
			s.fail(m.ID, -32602, err.Error())
			break
		}
		s.reply(m.ID, s.complete(p))
	case "textDocument/hover":
		var p docPosition
		if err := json.Unmarshal(m.Params, &p); err != nil {
			s.fail(m.ID, -32602, err.Error())
			break
		}
		s.reply(m.ID, s.hover(p))
	default:
		if isRequest {
			s.fail(m.ID, -32601, "mdsh doesn't do "+m.Method)
		}
	}
	return false
}

// docFile is the file a document's URI names, or the server's file.
func (s *server) docFile(uri string) string {
	if u, err := url.Parse(uri); err == nil && u.Scheme == "file" {
		return filepath.FromSlash(u.Path)
	}
	return s.file
}

var completionKinds = map[string]int{"text": 1, "property": 10, "enum": 13, "keyword": 14, "variable": 6, "constant": 21}

func (s *server) complete(p docPosition) any {
	text, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return []any{}
	}
	line := lineOf(text, p.Position.Line)
	col := s.byteCol(line, p.Position.Character)
	from, items := Complete(text, s.docFile(p.TextDocument.URI), p.Position.Line, col)
	out := make([]map[string]any, 0, len(items))
	edit := rng{Start: position{p.Position.Line, s.charCol(line, from)}, End: p.Position}
	for i, it := range items {
		insert := it.Insert
		if insert == "" {
			insert = it.Label
		}
		c := map[string]any{
			"label":      it.Label,
			"kind":       completionKinds[it.Kind],
			"filterText": it.Label,
			"sortText":   fmt.Sprintf("%04d", i),
			"textEdit":   map[string]any{"range": edit, "newText": insert},
		}
		if it.Detail != "" {
			c["detail"] = it.Detail
		}
		if it.Doc != "" {
			c["documentation"] = it.Doc
		}
		out = append(out, c)
	}
	return map[string]any{"isIncomplete": false, "items": out}
}

func (s *server) hover(p docPosition) any {
	text, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return nil
	}
	line := lineOf(text, p.Position.Line)
	spec, from, to, ok := Help(text, p.Position.Line, s.byteCol(line, p.Position.Character))
	if !ok {
		return nil
	}
	return map[string]any{
		"contents": map[string]any{"kind": "markdown", "value": HelpText(spec)},
		"range":    rng{Start: position{p.Position.Line, s.charCol(line, from)}, End: position{p.Position.Line, s.charCol(line, to)}},
	}
}

// publish sends the diagnostics for the document at uri.
func (s *server) publish(uri string) {
	text := s.docs[uri]
	list := []map[string]any{}
	for _, d := range Check(text) {
		line := lineOf(text, d.Line)
		severity := 2
		if d.Severity == "error" {
			severity = 1
		}
		list = append(list, map[string]any{
			"range":    rng{Start: position{d.Line, s.charCol(line, d.From)}, End: position{d.Line, s.charCol(line, d.To)}},
			"severity": severity,
			"source":   "mdsh",
			"message":  d.Message,
		})
	}
	s.notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "diagnostics": list})
}

// lineOf is line n of text, from 0.
func lineOf(text string, n int) string {
	for i := 0; i < n; i++ {
		j := strings.IndexByte(text, '\n')
		if j < 0 {
			return ""
		}
		text = text[j+1:]
	}
	if j := strings.IndexByte(text, '\n'); j >= 0 {
		text = text[:j]
	}
	return strings.TrimSuffix(text, "\r")
}

// charCol is the client's column for byte column b of line.
func (s *server) charCol(line string, b int) int {
	b = max(0, min(b, len(line)))
	if s.enc == "utf-8" {
		return b
	}
	n := 0
	for _, r := range line[:b] {
		n += utf16.RuneLen(r)
	}
	return n
}

// byteCol is the byte column for the client's column c of line.
func (s *server) byteCol(line string, c int) int {
	if s.enc == "utf-8" {
		return max(0, min(c, len(line)))
	}
	n := 0
	for i, r := range line {
		if n >= c {
			return i
		}
		n += utf16.RuneLen(r)
	}
	return len(line)
}

// offset is the byte offset in text of the client's position p.
func (s *server) offset(text string, p position) int {
	at := 0
	for i := 0; i < p.Line; i++ {
		j := strings.IndexByte(text[at:], '\n')
		if j < 0 {
			return len(text)
		}
		at += j + 1
	}
	line := text[at:]
	if j := strings.IndexByte(line, '\n'); j >= 0 {
		line = line[:j]
	}
	return at + s.byteCol(line, p.Character)
}
