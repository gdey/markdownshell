package httpblock

import (
	"fmt"
	"net/url"
	"strings"
)

// Curl is r as a curl command line, for pasting into a shell: r is
// usually expanded first, so it holds the values the request would send.
func Curl(r *Request) string {
	args := []string{"curl"}
	if r.Method != "GET" || len(r.Form) > 0 || r.Body != "" || r.BodyFile != "" {
		if !(r.Method == "POST" && (len(r.Form) > 0 || r.Body != "" || r.BodyFile != "")) {
			args = append(args, "-X", r.Method)
		}
	}
	u := r.URL
	if len(r.Query) > 0 {
		q := url.Values{}
		for _, p := range r.Query {
			q.Add(p.Name, p.Value)
		}
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + q.Encode()
	}
	args = append(args, shellQuote(u))
	for _, h := range r.Headers {
		args = append(args, "-H", shellQuote(h.Name+": "+h.Value))
	}
	switch {
	case len(r.Form) > 0:
		for _, p := range r.Form {
			args = append(args, "--data-urlencode", shellQuote(p.Name+"="+p.Value))
		}
	case r.BodyFile != "":
		args = append(args, "--data-binary", shellQuote("@"+r.BodyFile))
	case r.Body != "":
		hasCT := false
		for _, h := range r.Headers {
			hasCT = hasCT || strings.EqualFold(h.Name, "Content-Type")
		}
		if !hasCT && looksJSON(r.Body) {
			args = append(args, "-H", shellQuote("Content-Type: application/json"))
		}
		args = append(args, "--data-raw", shellQuote(r.Body))
	}
	if r.Insecure {
		args = append(args, "-k")
	}
	if r.Follow {
		args = append(args, "-L")
	}
	if r.Timeout > 0 {
		args = append(args, "--max-time", fmt.Sprint(r.Timeout.Seconds()))
	}
	return strings.Join(args, " ")
}

func looksJSON(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")
}

// shellQuote quotes s for sh, unless it needs no quotes.
func shellQuote(s string) string {
	safe := s != ""
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_./:@%+=,", c)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Split cuts a response as Format wrote it, or as a file saved it, into
// its status line, its header lines and its body. ok is false when text
// doesn't start with an HTTP status line.
func Split(text string) (status, headers, body string, ok bool) {
	first, rest, _ := strings.Cut(text, "\n")
	if !strings.HasPrefix(first, "HTTP/") {
		return "", "", "", false
	}
	_, status, _ = strings.Cut(first, " ")
	headers, body, found := strings.Cut(rest, "\n\n")
	if !found {
		headers, body = rest, ""
	} else {
		headers += "\n"
	}
	if strings.HasPrefix(rest, "\n") {
		headers, body = "", rest[1:]
	}
	return status, headers, body, true
}
