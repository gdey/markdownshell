package render

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

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
		first = retag(first, k, region)
		if rename && !region {
			first = spillFile.ReplaceAllStringFunc(first, func(m string) string {
				sm := spillFile.FindStringSubmatch(m)
				v := sm[2]
				quoted := len(v) >= 2 && v[0] == '"'
				if quoted {
					v = strings.ReplaceAll(v[1:len(v)-1], `\"`, `"`)
				}
				if v == "" || strings.HasPrefix(v, "/") || strings.Contains(strings.TrimPrefix(v, run.ResScheme), ":") {
					return m
				}
				return sm[1] + attrValue(run.HistoryAssetName(v, at))
			})
		}
		b.WriteString(first)
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

// spillFile matches the file= of an out=file output fence: the part
// before the value, and the value, quoted or not.
var spillFile = regexp.MustCompile(`(<include\s+file=)("(?:[^"\\]|\\.)*"|[^\s}]+)`)

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

// DropEarlierRuns is src with the earlier runs of block b that drop
// says to drop taken out from under its output, and the rest numbered
// again from 1, newest first. Dropping all of them leaves the current
// output as it is. src must be the source b was parsed from.
func DropEarlierRuns(src []byte, b *doc.Block, drop func(e doc.Entry) bool) []byte {
	if len(b.History) == 0 {
		return src
	}
	var entries []string
	for _, e := range b.History {
		if !drop(e) {
			entries = append(entries, entryText(src, e.Parts, len(entries)+1, e.At, false))
		}
	}
	cur := b.CurrentOutput().End
	out := append([]byte(nil), src[:cur]...)
	if len(entries) > 0 {
		gap := strings.TrimRight(b.Prefix, " ") + "\n"
		out = append(out, gap+strings.Join(entries, gap)...)
	}
	return append(out, src[b.Output.End:]...)
}

// printedEarlier is block b's earlier runs as printing shows them with
// Options.History: newest first, at most cap of them, each under an
// emphasized line saying how far back it is, when it ran and its exit
// status. With push, the output the file holds comes first, as the run
// before this one. It returns "" when there are none.
func printedEarlier(src []byte, b *doc.Block, push bool, cap int) string {
	type earlier struct {
		parts  []doc.Span
		at     string
		rename bool
	}
	var runs []earlier
	if push && len(b.CurrentParts) > 0 {
		runs = append(runs, earlier{b.CurrentParts, b.CurrentAt, true})
	}
	for _, e := range b.History {
		runs = append(runs, earlier{e.Parts, e.At, false})
	}
	if len(runs) > cap {
		runs = runs[:cap]
	}
	gap := strings.TrimRight(b.Prefix, " ") + "\n"
	var out []string
	for i, r := range runs {
		out = append(out, b.Prefix+runLine(src, i+1, r.parts, r.at)+"\n"+gap+entryText(src, r.parts, i+1, r.at, r.rename))
	}
	return strings.Join(out, gap)
}

// runLine is the line printing puts over earlier run k, whose parts
// are parts and whose at= is at: *Run 2 back, 2026-11-12 19:02 UTC,
// exit 7*. Output that goes in a region, such as an image, says its
// exit status only when it failed.
func runLine(src []byte, k int, parts []doc.Span, at string) string {
	says := []string{fmt.Sprintf("Run %d back", k)}
	if t, err := time.Parse(time.RFC3339, at); err == nil {
		says = append(says, t.UTC().Format("2006-01-02 15:04 MST"))
	}
	for _, p := range parts {
		first := string(src[p.Start:p.End])
		if nl := strings.IndexByte(first, '\n'); nl >= 0 {
			first = first[:nl]
		}
		if strings.Contains(first, "stream=stderr") {
			continue
		}
		exit := exitAttr.FindStringSubmatch(first)
		exited := exitedAttr.MatchString(first)
		region := strings.Contains(first, "<!-- mdsh:output")
		switch {
		case exit != nil && (!region || exit[1] != "0"):
			s := "exit " + exit[1]
			if exited {
				s += ", exited"
			}
			says = append(says, s)
		case exited:
			says = append(says, "exited")
		}
	}
	return "*" + strings.Join(says, ", ") + "*"
}

var (
	exitAttr   = regexp.MustCompile(`[\s{]exit="?(-?\d+)`)
	exitedAttr = regexp.MustCompile(`[\s{]exited[\s}"]`)
)
