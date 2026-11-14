package run_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/audit"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

// apiServer is a small API for http blocks: POST /sessions logs in,
// GET /me says who is logged in.
func apiServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sessions":
			var in struct{ User string }
			json.NewDecoder(r.Body).Decode(&in)
			w.Header().Set("Set-Cookie", "session=cookie123")
			w.Header().Set("X-Session-Id", "sid456")
			w.WriteHeader(201)
			fmt.Fprintf(w, `{"token":"tok-%s-789","id":42}`, in.User)
		case "/me":
			user := "guest"
			if r.Header.Get("Authorization") == "Bearer tok-j-789" {
				user = "j"
			}
			fmt.Fprintf(w, `{"user":%q}`, user)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

const httpDoc = "---\nmdsh:\n  env:\n    share: [API]\n  http:\n    secret_headers: [X-Session-Id]\n---\n" +
	"```sh {#setup}\nNAME=j\n```\n\n" +
	"```http {#login}\nPOST {{$API}}/sessions\n\n{\"user\": \"{{#setup/env(NAME)}}\"}\n\nHTTP 201\n[Captures]\ntoken: secret jsonpath $.token\nid: jsonpath $.id\n```\n\n" +
	"```http {#whoami}\nGET {{$API}}/me\nAuthorization: Bearer {{#login/env(token)}}\n\nHTTP 200\n[Asserts]\njsonpath $.user == \"j\"\n```\n\n" +
	"```http {#anon}\nGET {{$API}}/me\n\nHTTP 200\n[Asserts]\njsonpath $.user == \"j\"\n```\n\n" +
	"Logged in as {{#login/env(id)}} with {{#login/env(token)}}; anon got {{#anon/?}}.\n"

func TestHTTPBlocks(t *testing.T) {
	srv := apiServer(t)
	var log bytes.Buffer
	d, results := runDoc(t, httpDoc, run.Options{
		Environ: []string{"PATH=" + os.Getenv("PATH"), "API=" + srv.URL, "MDSH_EXPOSE=API"},
		Audit:   audit.New(&log),
	})
	r := byID(results)
	if r["login"].Err != nil || r["login"].Status != 0 {
		t.Fatalf("login: %v %d %s", r["login"].Err, r["login"].Status, r["login"].Stderr)
	}
	if r["whoami"].Failed() {
		t.Errorf("whoami failed: %v %s", r["whoami"].Err, r["whoami"].Stderr)
	}
	if got := string(r["anon"].Stderr); r["anon"].Status != 1 || got != "assert jsonpath $.user == \"j\": got \"guest\"\n" {
		t.Errorf("anon: status %d, stderr %q", r["anon"].Status, got)
	}
	out := string(render.Markdown(d, results, render.Options{WriteBack: true}))
	for _, want := range []string{
		"```output {for=login exit=0}\nHTTP/1.1 201 Created\n",
		"Set-Cookie: (secret)\n",
		"X-Session-Id: (secret)\n",
		"\"token\": \"(secret)\"",
		"```output {for=anon stream=stderr}\nassert jsonpath $.user == \"j\": got \"guest\"\n```",
		"Logged in as {{#login/env(id):\"42\"}} with {{#login/env(token)}}; anon got {{#anon/?:\"1\"}}.",
		"POST {{$API}}/sessions\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("write-back lacks %q:\n%s", want, out)
		}
	}
	for _, secret := range []string{"cookie123", "sid456", "tok-j-789", srv.URL} {
		if strings.Contains(out, secret) {
			t.Errorf("write-back holds %q:\n%s", secret, out)
		}
		if strings.Contains(log.String(), secret) {
			t.Errorf("audit log holds %q:\n%s", secret, log.String())
		}
	}
	if !strings.Contains(log.String(), `"method":"POST"`) || !strings.Contains(log.String(), `"url":"{{$API}}/sessions"`) || !strings.Contains(log.String(), `"status":201`) {
		t.Errorf("audit log lacks the request:\n%s", log.String())
	}
}

func TestHTTPNoResponse(t *testing.T) {
	srv := apiServer(t)
	url := srv.URL
	srv.Close()
	_, results := runDoc(t, "```http {#a}\nGET "+url+"/me\n```\n", run.Options{})
	if r := results[0]; r.Err == nil || !r.Failed() {
		t.Errorf("err = %v", r.Err)
	}
}

func TestHTTPBadRequest(t *testing.T) {
	d, results := runDoc(t, "```http {#a}\nFETCH https://example.com\n```\n", run.Options{})
	if r := results[0]; r.Err == nil || !strings.Contains(r.Err.Error(), "unknown method") {
		t.Errorf("err = %v", r.Err)
	}
	if len(d.Warnings) != 1 || !strings.Contains(d.Warnings[0], "unknown method") {
		t.Errorf("warnings = %q", d.Warnings)
	}
}
