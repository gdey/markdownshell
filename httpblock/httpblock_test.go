package httpblock

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	r, err := Parse(`POST {{$API}}/sessions
Content-Type: application/json
X-Trace: 1

{"user": "j",
 "password": "{{$PW}}"}

HTTP 201
[Asserts]
header Content-Type contains application/json
jsonpath "$.token" exists
jsonpath $.n not == 3
[Captures]
token: secret jsonpath $.token
id: jsonpath $.id redact
code: status
`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "POST" || r.URL != "{{$API}}/sessions" {
		t.Errorf("request line = %s %s", r.Method, r.URL)
	}
	if want := []Pair{{"Content-Type", "application/json"}, {"X-Trace", "1"}}; !reflect.DeepEqual(r.Headers, want) {
		t.Errorf("headers = %v", r.Headers)
	}
	if r.Body != "{\"user\": \"j\",\n \"password\": \"{{$PW}}\"}" {
		t.Errorf("body = %q", r.Body)
	}
	if r.Status != 201 || r.StatusLine != "HTTP 201" {
		t.Errorf("status = %d %q", r.Status, r.StatusLine)
	}
	if len(r.Asserts) != 3 {
		t.Fatalf("asserts = %v", r.Asserts)
	}
	if a := r.Asserts[1]; a.What != "jsonpath" || a.Arg != "$.token" || a.Op != "exists" {
		t.Errorf("assert 2 = %+v", a)
	}
	if a := r.Asserts[2]; !a.Not || a.Op != "==" || a.Value != "3" {
		t.Errorf("assert 3 = %+v", a)
	}
	want := []Capture{
		{Name: "token", Query: Query{What: "jsonpath", Arg: "$.token"}, Secret: true},
		{Name: "id", Query: Query{What: "jsonpath", Arg: "$.id"}, Secret: true},
		{Name: "code", Query: Query{What: "status"}},
	}
	if !reflect.DeepEqual(r.Captures, want) {
		t.Errorf("captures = %+v", r.Captures)
	}
}

func TestParseSections(t *testing.T) {
	r, err := Parse(`GET https://example.com/search
[QueryStringParams]
q: mdsh shell
page: 2
[Options]
insecure: true
follow: true
timeout: 5s
`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Pair{{"q", "mdsh shell"}, {"page", "2"}}; !reflect.DeepEqual(r.Query, want) {
		t.Errorf("query = %v", r.Query)
	}
	if !r.Insecure || !r.Follow || r.Timeout != 5*time.Second {
		t.Errorf("options = %v %v %v", r.Insecure, r.Follow, r.Timeout)
	}
	r, err = Parse("POST https://example.com/login\n[FormParams]\nuser: j\n")
	if err != nil || !reflect.DeepEqual(r.Form, []Pair{{"user", "j"}}) {
		t.Errorf("form = %v, %v", r, err)
	}
	r, err = Parse("PUT https://example.com/x\n\n@data.json\n")
	if err != nil || r.BodyFile != "data.json" {
		t.Errorf("body file = %v, %v", r, err)
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"", "no request"},
		{"FETCH https://x", "unknown method"},
		{"GET", "want METHOD URL"},
		{"GET https://x\nnot a header\n", "want a header"},
		{"GET https://x\n[Multipart]\n", "multipart"},
		{"GET https://x\n[Bogus]\n", "unknown section"},
		{"GET https://x\nHTTP 200\n[QueryStringParams]\n", "belongs to the request"},
		{"GET https://x\nHTTP 200\n[Asserts]\nxpath //a exists\n", "unknown query"},
		{"GET https://x\nHTTP 200\n[Asserts]\nstatus isAwesome\n", "unknown predicate"},
		{"GET https://x\nHTTP 200\n[Asserts]\nstatus ==\n", "needs a value"},
		{"GET https://x\nHTTP 200\n[Asserts]\nbody exists 3\n", "takes no value"},
		{"GET https://x\nHTTP 200\n[Asserts]\njsonpath .x exists\n", "starting with $"},
		{"GET https://x\nHTTP 200\n[Captures]\nx: jsonpath $.a\nx: status\n", "captured twice"},
		{"GET https://x\n[Options]\nretry: 5\n", "isn't supported yet"},
		{"GET https://x\nHTTP 200\nHTTP 201\n", "second HTTP line"},
		{"POST https://x\n[FormParams]\na: 1\n\n{}\n", "not both"},
	} {
		_, err := Parse(tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Parse(%q) = %v, want an error with %q", tc.src, err, tc.want)
		}
	}
}

func TestJSONPath(t *testing.T) {
	doc := map[string]any{
		"user":  "j",
		"items": []any{map[string]any{"id": 1.0}, map[string]any{"id": 2.0}},
		"a.b":   true,
	}
	for _, tc := range []struct {
		path string
		want []any
	}{
		{"$.user", []any{"j"}},
		{"$['a.b']", []any{true}},
		{"$.items[1].id", []any{2.0}},
		{"$.items[-1].id", []any{2.0}},
		{"$.items[*].id", []any{1.0, 2.0}},
		{"$..id", []any{1.0, 2.0}},
		{"$.nope", nil},
	} {
		p, err := compilePath(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.eval(doc); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestDo(t *testing.T) {
	var gotBody, gotCT, gotQuery string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotCT, gotQuery = string(b), r.Header.Get("Content-Type"), r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=abc")
		w.Header().Set("X-Session-Id", "s1")
		w.Header().Set("X-Count", "3")
		w.WriteHeader(201)
		fmt.Fprint(w, `{"token":"t0k","user":"j","items":[1,2,3],"n":4.5}`)
	})
	src := `POST {{URL}}/sessions
X-Trace: 1
[QueryStringParams]
q: a b

{"user": "j"}

HTTP 201
[Asserts]
header Content-Type contains json
header X-Count == 3
jsonpath $.token exists
jsonpath $.user == "j"
jsonpath $.user == j
jsonpath $.items count == 3
jsonpath $.items contains 2
jsonpath $.n > 4
jsonpath $.n isNumber
jsonpath $.missing not exists
body matches "t0k"
status < 300
duration < 10000
[Captures]
token: secret jsonpath $.token
items: jsonpath $.items
`
	req, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	req, err = req.Expand(func(s string) (string, error) { return strings.ReplaceAll(s, "{{URL}}", srv.URL), nil })
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Do(context.Background(), req, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"user": "j"}` || gotCT != "application/json" || gotQuery != "q=a+b" {
		t.Errorf("server got %q %q %q", gotBody, gotCT, gotQuery)
	}
	if resp.Failed() {
		t.Errorf("failures: %v", resp.Failures())
	}
	if len(resp.Checks) != 14 {
		t.Errorf("%d checks, want 14", len(resp.Checks))
	}
	if want := map[string]string{"token": "t0k", "items": "[1,2,3]"}; !reflect.DeepEqual(resp.Vars, want) {
		t.Errorf("vars = %v", resp.Vars)
	}
	if !reflect.DeepEqual(resp.Secret, []string{"token"}) {
		t.Errorf("secret = %v", resp.Secret)
	}
	out := Format(resp, []string{"x-session-id"}, false)
	for _, want := range []string{"HTTP/1.1 201 Created\n", "Set-Cookie: (secret)\n", "X-Session-Id: (secret)\n", "X-Count: 3\n", "\n{\n  \"token\": \"t0k\",\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "abc") || strings.Contains(out, "s1") {
		t.Errorf("output holds a secret header:\n%s", out)
	}
	if raw := Format(resp, nil, true); !strings.Contains(raw, `{"token":"t0k"`) {
		t.Errorf("raw output:\n%s", raw)
	}
}

func TestDoFailures(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		fmt.Fprint(w, `{"user":"guest"}`)
	})
	req, err := Parse("GET " + srv.URL + "\nHTTP 200\n[Asserts]\njsonpath $.user == \"j\"\njsonpath $.id exists\n[Captures]\nid: jsonpath $.id\n")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Do(context.Background(), req, Config{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"HTTP 200: got 404",
		`assert jsonpath $.user == "j": got "guest"`,
		"assert jsonpath $.id exists: got nothing",
		"capture id: jsonpath $.id found nothing",
	}
	if got := resp.Failures(); !reflect.DeepEqual(got, want) {
		t.Errorf("failures =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDoRedirectAndForm(t *testing.T) {
	var form string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		r.ParseForm()
		form = r.PostForm.Get("user") + " " + r.Header.Get("Content-Type")
		fmt.Fprint(w, "ok")
	})
	req, _ := Parse("GET " + srv.URL + "/old\nHTTP 302\n")
	resp, err := Do(context.Background(), req, Config{})
	if err != nil || resp.Failed() {
		t.Errorf("no follow: %v %v", err, resp.Failures())
	}
	req, _ = Parse("GET " + srv.URL + "/old\n[Options]\nfollow: true\nHTTP 200\n")
	resp, err = Do(context.Background(), req, Config{})
	if err != nil || resp.Failed() {
		t.Errorf("follow: %v %v", err, resp.Failures())
	}
	req, _ = Parse("POST " + srv.URL + "/form\n[FormParams]\nuser: j d\n")
	if _, err := Do(context.Background(), req, Config{}); err != nil || form != "j d application/x-www-form-urlencoded" {
		t.Errorf("form: %v %q", err, form)
	}
}

func TestDoTimeout(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	req, _ := Parse("GET " + srv.URL + "/slow?key=secret\n")
	_, err := Do(context.Background(), req, Config{Timeout: 50 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "no response after 50ms") || strings.Contains(err.Error(), "secret") {
		t.Errorf("err = %v", err)
	}
}

func TestExpandRefusesLineBreaks(t *testing.T) {
	req, _ := Parse("GET https://x\nX-A: {{v}}\n")
	_, err := req.Expand(func(s string) (string, error) { return strings.ReplaceAll(s, "{{v}}", "a\r\nX-Evil: 1"), nil })
	if err == nil || !strings.Contains(err.Error(), "line break") {
		t.Errorf("err = %v", err)
	}
}

func TestBinaryBody(t *testing.T) {
	resp := &Response{Proto: "HTTP/1.1", Status: "200 OK", Header: http.Header{"Content-Type": {"image/png"}}, Body: []byte{0x89, 'P', 'N', 'G', 0, 1}}
	if got := Body(resp, false); got != "(6 bytes of image/png, not shown)\n" {
		t.Errorf("body = %q", got)
	}
}
