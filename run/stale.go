package run

import "github.com/gdey/markdownshell/doc"

// Stale reports, for each of d's blocks, whether its result in results
// is out of date: it ran, but a block its depends= names has run again
// since, had its run cleared, or is stale itself. A block that hasn't
// run isn't stale, nor is one whose dependency simply never ran.
func Stale(d *doc.Document, results []*Result) []bool {
	stale := make([]bool, len(d.Blocks))
	done := make([]bool, len(d.Blocks))
	index := make(map[*doc.Block]int, len(d.Blocks))
	for i, b := range d.Blocks {
		index[b] = i
	}
	// d has no depends= cycles: they are parse errors.
	var check func(i int) bool
	check = func(i int) bool {
		if done[i] {
			return stale[i]
		}
		done[i] = true
		r := resultAt(results, i)
		if r == nil || !r.Ran {
			return false
		}
		for _, id := range d.Blocks[i].Depends() {
			dep := d.Dep(id)
			if dep == nil {
				continue
			}
			j := index[dep]
			if seqOf(resultAt(results, j)) != r.Deps[id] || check(j) {
				stale[i] = true
				break
			}
		}
		return stale[i]
	}
	for i := range d.Blocks {
		check(i)
	}
	return stale
}

// depSeqs is the Seq of each block b's depends= names, as Result.Deps
// holds them.
func depSeqs(d *doc.Document, results []*Result, b *doc.Block) map[string]uint64 {
	ids := b.Depends()
	if len(ids) == 0 {
		return nil
	}
	seqs := make(map[string]uint64, len(ids))
	for _, id := range ids {
		if dep := d.Dep(id); dep != nil {
			for i, r := range results {
				if r != nil && r.Block == dep {
					seqs[id] = seqOf(results[i])
				}
			}
		}
	}
	return seqs
}

func resultAt(results []*Result, i int) *Result {
	if i < len(results) {
		return results[i]
	}
	return nil
}

func seqOf(r *Result) uint64 {
	if r == nil || !r.Ran {
		return 0
	}
	return r.Seq
}
