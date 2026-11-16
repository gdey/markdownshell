package run

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/docroot"
	"github.com/gdey/markdownshell/httpblock"
)

// runHTTP makes http block b's request, with its placeholders filled in
// from lookup, and puts what came back in r: the response as the file
// saves it on stdout, with the credential headers masked, a line per
// failed check on stderr, status 0 when every check passed and 1
// otherwise, and the captures as the block's variables. A request that
// gets no response is r.Err.
func runHTTP(ctx context.Context, d *doc.Document, b *doc.Block, r *Result, opts Options, emit func(Event), lookup doc.Lookup) {
	r.Ran = true
	req, err := b.Request()
	if err != nil {
		r.Err = err
		return
	}
	r.Request = req
	req, err = req.Expand(func(s string) (string, error) { return doc.Expand(s, lookup) })
	if err != nil {
		r.Err = err
		return
	}
	cfg := httpblock.Config{
		Transport: opts.HTTPTransport,
		ReadFile: func(name string) ([]byte, error) {
			o := opts
			if o.Dir == "" && o.File != "" {
				o.Dir = filepath.Dir(o.File)
			}
			path, err := resolveRes(name, o)
			if err != nil {
				return nil, err
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(o.Dir, filepath.FromSlash(path))
			}
			data, err := docroot.ReadFile(docRootFor(name, o), path, 0)
			if pe := (*fs.PathError)(nil); errors.As(err, &pe) {
				err = pe.Err
			}
			return data, err
		},
	}
	if t, ok := b.Attrs["timeout"]; ok {
		if cfg.Timeout, err = time.ParseDuration(t); err != nil {
			r.Err = fmt.Errorf("timeout: %w", err)
			return
		}
	}
	resp, err := httpblock.Do(ctx, req, cfg)
	if err != nil {
		r.Err = err
		return
	}
	r.HTTP = resp
	out := httpblock.Format(resp, d.Options.HTTP.SecretHeaders, b.Attrs.Has("raw"))
	errs := ""
	if f := resp.Failures(); len(f) > 0 {
		errs = strings.Join(f, "\n") + "\n"
		r.Status = 1
	}
	// A secret capture's value stays out of the output too, wherever
	// the response held it.
	for _, name := range resp.Secret {
		if v := resp.Vars[name]; v != "" {
			out = strings.ReplaceAll(out, v, httpblock.Masked)
			errs = strings.ReplaceAll(errs, v, httpblock.Masked)
		}
	}
	r.Stdout = []byte(out)
	emit(Event{Kind: Stdout, Block: b, Data: r.Stdout})
	if errs != "" {
		r.Stderr = []byte(errs)
		emit(Event{Kind: Stderr, Block: b, Data: r.Stderr})
	}
	r.Vars = resp.Vars
}
