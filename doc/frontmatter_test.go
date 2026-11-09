package doc

import (
	"slices"
	"testing"
)

func TestFullEnv(t *testing.T) {
	for _, tt := range []struct {
		share  []string
		full   bool
		except []string
		banner string
	}{
		{nil, false, nil, ""},
		{[]string{"-PATH"}, false, nil, ""},
		{[]string{"PATH", "HOME", "LC_ALL"}, false, nil, ""},
		{[]string{"GREETING", "-PATH"}, false, nil, "This file brings in GREETING from your environment"},
		{[]string{"GITHUB_TOKEN", "PATH", "AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN"}, false, nil, "This file brings in GITHUB_TOKEN, AWS_SECRET_ACCESS_KEY from your environment"},
		{[]string{"..."}, true, nil, "This file brings in all your environment variables"},
		{[]string{"*", "-A", "-B"}, true, []string{"A", "B"}, "This file brings in all your environment variables, except A, B"},
	} {
		var o Options
		o.Env.Share = tt.share
		full, except := o.FullEnv()
		if full != tt.full || !slices.Equal(except, tt.except) {
			t.Errorf("FullEnv(%q) = %v, %q, want %v, %q", tt.share, full, except, tt.full, tt.except)
		}
		if got := o.ShareBanner(); got != tt.banner {
			t.Errorf("ShareBanner(%q) = %q, want %q", tt.share, got, tt.banner)
		}
	}
}
