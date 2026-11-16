package httpblock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"
)

// Masked stands for a credential header's value in the saved output.
const Masked = "(secret)"

// SecretHeaders are the headers whose values are always masked in the
// output, since they carry a credential. The front matter's
// mdsh.http.secret_headers adds to them; nothing takes one off.
var SecretHeaders = []string{"Set-Cookie", "Authorization", "Proxy-Authorization", "WWW-Authenticate", "X-API-Key"}

// Format is the response as the block's output shows it and the file
// saves it: the status line, the headers sorted by name with the
// credential ones (SecretHeaders and more) masked, a blank line and the
// body. A JSON body is pretty-printed unless raw; a body that isn't
// text is described, not shown.
func Format(resp *Response, more []string, raw bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", resp.Proto, resp.Status)
	writeHeaders(&b, resp.Header, more)
	body := Body(resp, raw)
	if body != "" {
		b.WriteString("\n")
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// Headers is the response's header lines, as Format writes them.
func Headers(resp *Response, more []string) string {
	var b strings.Builder
	writeHeaders(&b, resp.Header, more)
	return b.String()
}

func writeHeaders(b *strings.Builder, h http.Header, more []string) {
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		for _, v := range h[k] {
			if IsSecretHeader(k, more) {
				v = Masked
			}
			fmt.Fprintf(b, "%s: %s\n", k, v)
		}
	}
}

// IsSecretHeader reports whether header name's value is masked:
// it is one of SecretHeaders or more, in any case.
func IsSecretHeader(name string, more []string) bool {
	for _, list := range [][]string{SecretHeaders, more} {
		for _, s := range list {
			if strings.EqualFold(strings.TrimSpace(s), name) {
				return true
			}
		}
	}
	return false
}

// Body is the response body as Format writes it.
func Body(resp *Response, raw bool) string {
	if len(resp.Body) == 0 {
		return ""
	}
	ct := resp.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ct)
	if !utf8.Valid(resp.Body) || bytes.IndexByte(resp.Body, 0) >= 0 {
		what := mt
		if what == "" {
			what = "binary data"
		}
		return fmt.Sprintf("(%d bytes of %s, not shown)\n", len(resp.Body), what)
	}
	body := string(resp.Body)
	if !raw && IsJSON(mt) {
		var out bytes.Buffer
		if json.Indent(&out, resp.Body, "", "  ") == nil {
			body = out.String()
		}
	}
	if resp.Truncated {
		body = strings.TrimRight(body, "\n") + fmt.Sprintf("\n(cut at %d MB)\n", MaxBody>>20)
	}
	return body
}

// IsJSON reports whether media type mt is JSON: application/json, or a
// type ending in +json.
func IsJSON(mt string) bool {
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}
