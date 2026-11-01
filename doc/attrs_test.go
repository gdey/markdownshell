package doc

import "testing"

func TestSetFlag(t *testing.T) {
	for _, tc := range []struct {
		info string
		on   bool
		want string
	}{
		{"sh", true, "sh {collapse}"},
		{"sh", false, "sh"},
		{"", true, "{collapse}"},
		{"sh {#a}", true, "sh {#a collapse}"},
		{"sh {#a  }", true, "sh {#a collapse}"},
		{"sh {}", true, "sh {collapse}"},
		{"sh{#a}", true, "sh{#a collapse}"},
		{"{.sh #a}", true, "{.sh #a collapse}"},
		{"sh {#a collapse}", true, "sh {#a collapse}"},
		{"sh {#a collapse}", false, "sh {#a}"},
		{"sh {collapse #a}", false, "sh {#a}"},
		{"sh {#a  collapse   env=x}", false, "sh {#a   env=x}"},
		{"sh {collapse}", false, "sh"},
		{"{collapse}", false, ""},
		{"sh {#a collapse=}", false, "sh {#a}"},
		{"sh {#a collapse={{$?}}==0}", false, "sh {#a collapse={{$?}}==0}"},
		{"sh {#a collapse={{$?}}==0}", true, "sh {#a collapse={{$?}}==0}"},
		{`sh {#a x="collapse y"}`, true, `sh {#a x="collapse y" collapse}`},
		{"sh {#a collapse-output}", false, "sh {#a collapse-output}"},
		{"sh foo", true, "sh foo"},
	} {
		if got := SetFlag(tc.info, "collapse", tc.on); got != tc.want {
			t.Errorf("SetFlag(%q, %v) = %q, want %q", tc.info, tc.on, got, tc.want)
		}
	}
}

func TestInfoSpan(t *testing.T) {
	src := "> 1. ```` sh {#a}\n>    echo hi\n>    ````\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	b := d.Blocks[0]
	at := d.InfoSpan(b)
	got := string(d.Src[:at.Start]) + SetFlag(b.Info, "collapse", true) + string(d.Src[at.End:])
	if want := "> 1. ```` sh {#a collapse}\n>    echo hi\n>    ````\n"; got != want {
		t.Errorf("set info = %q, want %q", got, want)
	}
}
