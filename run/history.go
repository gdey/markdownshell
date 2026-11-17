package run

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/gdey/markdownshell/doc"
)

// At is r's start time as an output's at= writes it: UTC, RFC 3339 to
// the second. It is "" for a result with no start time.
func (r *Result) At() string {
	if r.Start.IsZero() {
		return ""
	}
	return r.Start.UTC().Format("2006-01-02T15:04:05Z07:00")
}

// historyStamp is how an earlier run's time goes in its file's name:
// the at= with its colons dropped, or "earlier" when it has none.
func historyStamp(at string) string {
	if at == "" {
		return "earlier"
	}
	return strings.ReplaceAll(at, ":", "")
}

// HistoryAssetName is the name an earlier run's file takes when
// history= pushes its output down: path with the run's time before its
// extension, as d.md.assets/check.2026-11-13T190800Z.svg. A run with no
// at= takes "earlier" for its time.
func HistoryAssetName(path, at string) string {
	ext := pathExt(path)
	return strings.TrimSuffix(path, ext) + "." + historyStamp(at) + ext
}

// pathExt is path's extension, by / or \ alike.
func pathExt(path string) string {
	base := path[strings.LastIndexAny(path, `/\`)+1:]
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		return base[i:]
	}
	return ""
}

// rotated matches a name HistoryAssetName made: the stem, the time and
// the extension.
var rotated = regexp.MustCompile(`^(.+)\.(\d{4}-\d\d-\d\dT\d{6}Z|earlier)(\.[^./\\]+)$`)

// imageLink finds the target of the first image link in s.
var imageLink = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)

// ownAsset reports whether rel, a path relative to opts.Dir, is a file
// mdsh writes for block b: its default <doc>.assets/<id>.<ext>, or the
// path its out= names, or the cast saved beside a .gif. Only these are
// renamed and deleted for history, so a document can't have mdsh move
// files it doesn't own.
func ownAsset(b *doc.Block, rel string, opts Options) bool {
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	_, arg := outKind(b)
	if arg == "" {
		dir, name := filepath.ToSlash(filepath.Dir(rel)), filepath.Base(rel)
		ext, ok := strings.CutPrefix(name, b.ID+".")
		return ok && dir == filepath.ToSlash(filepath.Clean(assetsDir(opts))) && ext != "" && !strings.Contains(ext, ".")
	}
	want, err := resolveRes(arg, opts)
	if err != nil {
		return false
	}
	want = filepath.ToSlash(filepath.Clean(filepath.FromSlash(want)))
	if strings.EqualFold(filepath.Ext(want), ".gif") && rel == strings.TrimSuffix(want, filepath.Ext(want))+".cast" {
		return true
	}
	return rel == want
}

// linkPath turns an image link in the document into a path relative to
// opts.Dir, or "" for a data: URI, a URL or an absolute path.
func linkPath(link string) string {
	link, _, _ = strings.Cut(link, "#")
	if link == "" || strings.Contains(link, ":") || filepath.IsAbs(link) || strings.HasPrefix(link, "/") {
		return ""
	}
	return link
}

// lastImages remembers, for each block that saved a file this session,
// its link and the run's at=, so the next run can rename it.
type lastImages struct {
	mu sync.Mutex
	m  map[*doc.Block][2]string
}

func (l *lastImages) set(b *doc.Block, link, at string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[*doc.Block][2]string{}
	}
	l.m[b] = [2]string{link, at}
}

func (l *lastImages) get(b *doc.Block) (link, at string, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.m[b]
	return v[0], v[1], ok
}

// rotateAssets runs before block b, if it keeps a history, so that the
// file its last output links to isn't overwritten: the file is renamed
// as HistoryAssetName says, with that output's at=, and write-back
// links the earlier run to it. The last output is this session's last
// run of b, or else the output the file holds. Only files mdsh writes
// for b are renamed (see ownAsset), and never over another file.
func (s *Session) rotateAssets(b *doc.Block) {
	if b.HistoryCap(s.d.Options) == 0 {
		return
	}
	dir := s.opts.Dir
	link, at, ok := s.images.get(b)
	if ok && s.opts.LinkDir != "" {
		dir = s.opts.LinkDir
	}
	if !ok {
		for _, p := range b.CurrentParts {
			if m := imageLink.FindSubmatch(s.d.Src[p.Start:p.End]); m != nil {
				link = string(m[1])
				break
			}
		}
		at = b.CurrentAt
	}
	rel := linkPath(link)
	if rel == "" {
		return
	}
	if dir != s.opts.Dir {
		abs, err := filepath.Abs(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return
		}
		base, err := filepath.Abs(s.opts.Dir)
		if err != nil {
			return
		}
		if rel, err = filepath.Rel(base, abs); err != nil {
			return
		}
	}
	files := []string{rel}
	if strings.EqualFold(filepath.Ext(rel), ".gif") {
		files = append(files, strings.TrimSuffix(rel, filepath.Ext(rel))+".cast")
	}
	for _, f := range files {
		if !ownAsset(b, f, s.opts) {
			continue
		}
		from := filepath.Join(s.opts.Dir, filepath.FromSlash(f))
		to := HistoryAssetName(from, at)
		if info, err := os.Lstat(from); err != nil || !info.Mode().IsRegular() {
			continue
		}
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		os.Rename(from, to)
	}
}

// TidyHistoryAssets deletes the files of the earlier runs that saving
// dropped: those linked from an earlier run in before, the document as
// it was, but not in after, the document as saved. Only files mdsh
// renamed for history are deleted (see HistoryAssetName and ownAsset).
// opts.Dir is the document's folder, by default opts.File's.
func TidyHistoryAssets(before, after []byte, opts Options) error {
	if opts.Dir == "" && opts.File != "" {
		opts.Dir = filepath.Dir(opts.File)
	}
	old, err := doc.Parse(before)
	if err != nil {
		return nil
	}
	cur, err := doc.Parse(after)
	if err != nil {
		return nil
	}
	kept := map[string]bool{}
	for _, b := range cur.Blocks {
		for _, e := range b.History {
			for _, m := range imageLink.FindAllSubmatch(cur.Src[e.Span.Start:e.Span.End], -1) {
				kept[b.ID+"\x00"+string(m[1])] = true
			}
		}
	}
	var errs []error
	for _, b := range old.Blocks {
		for _, e := range b.History {
			for _, m := range imageLink.FindAllSubmatch(old.Src[e.Span.Start:e.Span.End], -1) {
				link := string(m[1])
				rel := linkPath(link)
				if rel == "" || kept[b.ID+"\x00"+link] {
					continue
				}
				files := []string{rel}
				if strings.EqualFold(filepath.Ext(rel), ".gif") {
					files = append(files, strings.TrimSuffix(rel, filepath.Ext(rel))+".cast")
				}
				for _, f := range files {
					p := rotated.FindStringSubmatch(f)
					if p == nil || !ownAsset(b, p[1]+p[3], opts) {
						continue
					}
					if err := os.Remove(filepath.Join(opts.Dir, filepath.FromSlash(f))); err != nil && !errors.Is(err, fs.ErrNotExist) {
						errs = append(errs, err)
					}
				}
			}
		}
	}
	return errors.Join(errs...)
}
