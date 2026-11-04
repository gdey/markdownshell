package doc

// SetSource replaces d's source with src and parses it again, as when
// the whole file is edited at once. A block whose info string and body
// are unchanged keeps its identity, wherever it moved, with its spans
// moved to match src, so results that point at it stay valid. Edited and
// new blocks are new *Blocks, and blocks no longer in src are dropped.
// A nosave input's session value (see SetValue) is kept by id, so what
// was typed survives an edit elsewhere in the file. If src doesn't
// parse, d is left as it was.
func (d *Document) SetSource(src []byte) error {
	nd, err := Parse(src)
	if err != nil {
		return err
	}
	session := map[string]*string{}
	for _, b := range d.Blocks {
		if b.session != nil {
			session[b.ID] = b.session
		}
	}
	// Equal blocks are matched in order, so duplicates keep theirs.
	old := map[string][]*Block{}
	for _, b := range d.Blocks {
		k := sameKey(b)
		old[k] = append(old[k], b)
	}
	for i, nb := range nd.Blocks {
		k := sameKey(nb)
		if bs := old[k]; len(bs) > 0 {
			b := bs[0]
			old[k] = bs[1:]
			*b = *nb
			nd.Blocks[i] = b
		}
	}
	for _, nb := range nd.Blocks {
		if v, ok := session[nb.ID]; ok && nb.IsInput() && nb.NoSave() {
			nb.session = v
		}
	}
	*d = *nd
	return nil
}

func sameKey(b *Block) string { return b.Info + "\x00" + b.Body }
