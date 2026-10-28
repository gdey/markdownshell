package doc

import "testing"

func TestAppendBlock(t *testing.T) {
	d, err := Parse([]byte("# T\n\n```sh\necho one\n```\n\nprose\n"))
	if err != nil {
		t.Fatal(err)
	}
	first := d.Blocks[0].Span
	b, err := d.AppendBlock("sh", "echo two")
	if err != nil {
		t.Fatal(err)
	}
	want := "# T\n\n```sh\necho one\n```\n\nprose\n\n```sh\necho two\n```\n"
	if string(d.Src) != want {
		t.Errorf("src = %q, want %q", d.Src, want)
	}
	if b.ID != "block-2" || b.Body != "echo two\n" || len(d.Blocks) != 2 || d.Blocks[0].Span != first {
		t.Errorf("block %q body %q, %d blocks", b.ID, b.Body, len(d.Blocks))
	}

	// A body with a fence in it gets a longer fence.
	if b, err = d.AppendBlock("sh", "cat <<'EOF'\n```\nEOF"); err != nil || b.Body != "cat <<'EOF'\n```\nEOF\n" {
		t.Errorf("body %q, err %v", b.Body, err)
	}

	open, _ := Parse([]byte("```sh\necho open\n"))
	if _, err := open.AppendBlock("sh", "echo x"); err == nil {
		t.Error("appended inside an unclosed fence")
	}
}
