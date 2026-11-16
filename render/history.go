package render

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// earlierRuns is what write-back writes under block b's output: its
// earlier runs, newest first, numbered from 1, at most cap of them.
// With push, the output the file holds now is pushed on top, as the
// run before this one: marked history=1, and its image links pointing
// at the names run.HistoryAssetName gives their files. It returns ""
// when there are none.
func earlierRuns(src []byte, b *doc.Block, push bool, cap int) string {
	var entries []string
	if push && len(b.CurrentParts) > 0 {
		entries = append(entries, entryText(src, b.CurrentParts, 1, b.CurrentAt, true))
	}
	for _, e := range b.History {
		if len(entries) >= cap {
			break
		}
		entries = append(entries, entryText(src, e.Parts, len(entries)+1, e.At, false))
	}
	if len(entries) > cap {
		entries = entries[:cap]
	}
	return strings.Join(entries, strings.TrimRight(b.Prefix, " ")+"\n")
}

// emptyOutput is the output a run that showed nothing writes when its
// block keeps a history, so the run is still there, with its time: an
// empty fence, or an empty region for output that goes in one.
func emptyOutput(r *run.Result) string {
	if k := run.OutKind(r.Block); k != "text" {
		return doc.RegionStartAt(r.Block.ID, r.At()) + "\n" + doc.RegionEnd + "\n"
	}
	attrs := "for=" + attrValue(r.Block.ID) + " exit=" + strconv.Itoa(r.Status)
	if r.Exited {
		attrs += " exited"
	}
	if at := r.At(); at != "" {
		attrs += " at=" + at
	}
	return Fence("output", attrs, "")
}

// historyTrimmed reports whether write-back changes b's earlier runs
// without a run: there are more than cap, or they aren't numbered 1, 2,
// 3 and so on.
func historyTrimmed(b *doc.Block, cap int) bool {
	if len(b.History) > cap {
		return true
	}
	for i, e := range b.History {
		if e.K != i+1 {
			return true
		}
	}
	return false
}

// entryText is the parts, from src, as earlier run k: each part's
// opening line marked history=k, and the gaps between the parts kept.
// With rename, the image links in it are pointed at their files' names
// for an earlier run of at.
func entryText(src []byte, parts []doc.Span, k int, at string, rename bool) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.Write(src[parts[i-1].End:p.Start])
		}
		text := string(src[p.Start:p.End])
		nl := strings.IndexByte(text, '\n')
		if nl < 0 {
			nl = len(text) - 1
		}
		first, rest := text[:nl+1], text[nl+1:]
		region := strings.Contains(first, "<!-- mdsh:output")
		b.WriteString(retag(first, k, region))
		if rename && region {
			rest = imageTarget.ReplaceAllStringFunc(rest, func(m string) string {
				link := imageTarget.FindStringSubmatch(m)[1]
				path, frag, hasFrag := strings.Cut(link, "#")
				if path == "" || strings.Contains(path, ":") || strings.HasPrefix(path, "/") {
					return m
				}
				link = run.HistoryAssetName(path, at)
				if hasFrag {
					link += "#" + frag
				}
				return strings.Replace(m, imageTarget.FindStringSubmatch(m)[1], link, 1)
			})
		}
		b.WriteString(rest)
	}
	return b.String()
}

// imageTarget matches an image link, with its target.
var imageTarget = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)

var (
	fenceHistory  = regexp.MustCompile(`(\s)history=\S*?([\s}])`)
	regionHistory = regexp.MustCompile(`(\s)history="[^"]*"`)
)

// retag marks line, the opening line of an output fence or region,
// history=k, replacing the history= it has.
func retag(line string, k int, region bool) string {
	n := strconv.Itoa(k)
	if region {
		if regionHistory.MatchString(line) {
			return regionHistory.ReplaceAllString(line, `${1}history="`+n+`"`)
		}
		i := strings.LastIndex(line, " -->")
		if i < 0 {
			return line
		}
		return line[:i] + ` history="` + n + `"` + line[i:]
	}
	if fenceHistory.MatchString(line) {
		return fenceHistory.ReplaceAllString(line, "${1}history="+n+"${2}")
	}
	i := strings.LastIndexByte(line, '}')
	if i < 0 {
		return line
	}
	return line[:i] + " history=" + n + line[i:]
}
