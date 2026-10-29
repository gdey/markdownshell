package notebook

import (
	"strconv"
	"strings"
	"testing"
)

func TestCheckInfo(t *testing.T) {
	for _, c := range []struct {
		info string
		want string // "from-to severity" of each problem
	}{
		{`sh {#a timeout=5m confirm="Go?"}`, ""},
		{`sh {#a timeout=5 out=html}`, "7-16 error, 17-25 error"},
		{`sh {#a confrim}`, "7-14 warning"},
		{`go {<include file=x.go from=a}`, "23-29 error"},
		{`go {<include #x file=x.go tty}`, "13-15 error, 26-29 error"},
		{`sh {file=x}`, "4-10 warning"},
		{`sh {run={{$X}`, "4-12 error"},
	} {
		var got []string
		for _, p := range CheckInfo(c.info) {
			got = append(got, strings.Join([]string{strconv.Itoa(p.From) + "-" + strconv.Itoa(p.To), p.Severity}, " "))
			if c.info[p.From:p.To] == "" {
				t.Errorf("%s: empty range", c.info)
			}
		}
		if g := strings.Join(got, ", "); g != c.want {
			t.Errorf("CheckInfo(%s) = %s, want %s", c.info, g, c.want)
		}
	}
}
