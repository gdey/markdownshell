package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/gdey/markdownshell/ui"
)

// method is one of ui.App's methods, which the pages call by name.
type method struct {
	fn  reflect.Value
	ctx bool           // it takes the call's context first
	in  []reflect.Type // the arguments after that
	err bool           // its last result is an error
	val bool           // it returns a value before that
}

var (
	ctxType = reflect.TypeFor[context.Context]()
	errType = reflect.TypeFor[error]()
)

// methods are a's exported methods, by name. They are all calls the
// frontend makes; ui.Control holds the rest.
func methods(a *ui.App) map[string]method {
	v := reflect.ValueOf(a)
	t := v.Type()
	ms := map[string]method{}
	for i := range t.NumMethod() {
		m := t.Method(i)
		mt := m.Type
		var mm method
		mm.fn = v.Method(i)
		for j := 1; j < mt.NumIn(); j++ {
			in := mt.In(j)
			if j == 1 && in == ctxType {
				mm.ctx = true
				continue
			}
			mm.in = append(mm.in, in)
		}
		n := mt.NumOut()
		if n > 0 && mt.Out(n-1) == errType {
			mm.err = true
			n--
		}
		if n > 1 {
			continue // not a call the frontend could make
		}
		mm.val = n == 1
		ms[m.Name] = mm
	}
	return ms
}

// callRequest is what Call.ByName in runtime.js posts: the name the
// bindings use, such as "github.com/gdey/markdownshell/ui.App.Cells",
// and the arguments.
type callRequest struct {
	Name string            `json:"name"`
	Args []json.RawMessage `json:"args"`
}

// badCall is a call that names no method or doesn't fit it.
type badCall struct{ msg string }

func (e badCall) Error() string { return e.msg }

// invoke calls the method req names and returns its result as JSON.
func (s *Server) invoke(ctx context.Context, req callRequest) ([]byte, error) {
	name := req.Name[strings.LastIndex(req.Name, ".")+1:]
	if !strings.HasSuffix(req.Name, "ui.App."+name) {
		return nil, badCall{fmt.Sprintf("no call %q", req.Name)}
	}
	m, ok := s.calls[name]
	if !ok {
		return nil, badCall{fmt.Sprintf("no call %q", req.Name)}
	}
	if len(req.Args) != len(m.in) {
		return nil, badCall{fmt.Sprintf("%s takes %d arguments, got %d", name, len(m.in), len(req.Args))}
	}
	args := make([]reflect.Value, 0, len(m.in)+1)
	if m.ctx {
		args = append(args, reflect.ValueOf(ctx))
	}
	for i, t := range m.in {
		p := reflect.New(t)
		if err := json.Unmarshal(req.Args[i], p.Interface()); err != nil {
			return nil, badCall{fmt.Sprintf("%s argument %d: %v", name, i+1, err)}
		}
		args = append(args, p.Elem())
	}
	out := m.fn.Call(args)
	if m.err {
		if err, _ := out[len(out)-1].Interface().(error); err != nil {
			return nil, err
		}
	}
	if !m.val {
		return []byte("null"), nil
	}
	return json.Marshal(out[0].Interface())
}

// decodeJSON reads one JSON value from r into v, up to 64 MiB: SetText
// sends the whole file.
func decodeJSON(r io.Reader, v any) error {
	b, err := io.ReadAll(io.LimitReader(r, 64<<20))
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	return d.Decode(v)
}
