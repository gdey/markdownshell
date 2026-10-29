package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/gdey/markdownshell/settings"
)

func TestFillAddr(t *testing.T) {
	for in, want := range map[string]string{
		"":               DefaultAddr,
		"localhost:9000": "localhost:9000",
		"0.0.0.0":        "0.0.0.0:8080",
		"localhost:":     "localhost:8080",
		":9000":          ":9000",
		"::1":            "[::1]:8080",
		"[::1]:9000":     "[::1]:9000",
	} {
		got, err := fillAddr(in)
		if err != nil || got != want {
			t.Errorf("fillAddr(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

// start serves file on a test server and returns its URL and a client
// that keeps cookies, not yet holding the token.
func start(t *testing.T, file string) (*Server, *httptest.Server, *http.Client) {
	t.Helper()
	store, err := settings.Open(filepath.Join(t.TempDir(), "gui.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{Settings: store, Cwd: t.TempDir(), File: file, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() {
		ts.Close()
		s.shutdown()
	})
	jar, _ := cookiejar.New(nil)
	return s, ts, &http.Client{Jar: jar}
}

func TestGuard(t *testing.T) {
	_, ts, c := start(t, "")
	get := func(path string, h map[string]string) *http.Response {
		t.Helper()
		r, _ := http.NewRequest("GET", ts.URL+path, nil)
		for k, v := range h {
			if k == "Host" {
				r.Host = v
			} else {
				r.Header.Set(k, v)
			}
		}
		resp, err := c.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	if r := get("/style.css", nil); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("without the token: %s", r.Status)
	}
	if r := get("/?token=wrong", nil); r.StatusCode != http.StatusForbidden {
		t.Errorf("with a wrong token: %s", r.Status)
	}
	// The token gives a cookie, and the page is a window.
	r := get("/?token=secret", nil)
	if r.StatusCode != http.StatusOK || !strings.HasPrefix(r.Request.URL.RequestURI(), "/window.html?w=1") {
		t.Errorf("with the token: %s at %s", r.Status, r.Request.URL)
	}
	if r := get("/style.css", nil); r.StatusCode != http.StatusOK {
		t.Errorf("with the cookie: %s", r.Status)
	}
	if r := get("/index.html?tab=1", nil); r.StatusCode != http.StatusOK || r.Request.URL.Path != "/index.html" {
		t.Errorf("a tab's page: %s at %s", r.Status, r.Request.URL)
	}
	u, _ := url.Parse(ts.URL)
	if r := get("/style.css", map[string]string{"Host": "evil.example:" + u.Port()}); r.StatusCode != http.StatusForbidden {
		t.Errorf("from another host name: %s", r.Status)
	}
	if r := get("/style.css", map[string]string{"Origin": "http://evil.example"}); r.StatusCode != http.StatusForbidden {
		t.Errorf("from another site: %s", r.Status)
	}
}

// login gets c the token's cookie.
func login(t *testing.T, ts *httptest.Server, c *http.Client) {
	t.Helper()
	resp, err := c.Get(ts.URL + "/?token=secret")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// call calls the App as runtime.js does, from window 1 and tab, if not 0.
func call(t *testing.T, ts *httptest.Server, c *http.Client, tab, name string, args ...any) (int, string) {
	t.Helper()
	if args == nil {
		args = []any{}
	}
	body, _ := json.Marshal(map[string]any{"name": "github.com/gdey/markdownshell/ui.App." + name, "args": args})
	r, _ := http.NewRequest("POST", ts.URL+"/wails/call", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("x-mdsh-window", "1")
	if tab != "" {
		r.Header.Set("x-mdsh-tab", tab)
	}
	resp, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

func TestCalls(t *testing.T) {
	file := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(file, []byte("# Doc\n\n```sh\necho hi\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, ts, c := start(t, file)
	login(t, ts, c)

	if code, body := call(t, ts, c, "1", "Doc"); code != 200 || !strings.Contains(body, `"file":"`+file+`"`) {
		t.Errorf("Doc: %d %s", code, body)
	}
	if code, body := call(t, ts, c, "", "Tabs"); code != 200 || !strings.Contains(body, `"title":"doc.md"`) {
		t.Errorf("Tabs: %d %s", code, body)
	}
	if code, body := call(t, ts, c, "1", "CheckInfo", "sh {#a confrim}"); code != 200 || body == "null" {
		t.Errorf("CheckInfo: %d %s", code, body)
	}
	if code, _ := call(t, ts, c, "1", "Nope"); code != http.StatusBadRequest {
		t.Errorf("an unknown call: %d", code)
	}
	if code, _ := call(t, ts, c, "1", "RunBlock"); code != http.StatusBadRequest {
		t.Errorf("too few arguments: %d", code)
	}
	if code, body := call(t, ts, c, "9", "Doc"); code != http.StatusUnprocessableEntity || body != "no tab 9" {
		t.Errorf("a call's error: %d %q", code, body)
	}

	// A run sends its events to the window's page.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/wails/events?w=1"
	ws, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPClient: c, HTTPHeader: http.Header{"Origin": {ts.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	if code, body := call(t, ts, c, "1", "RunBlock", 0); code != 200 {
		t.Fatalf("RunBlock: %d %s", code, body)
	}
	var names []string
	for {
		_, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("after %v: %v", names, err)
		}
		var m message
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		names = append(names, m.Name)
		if m.Name == "block" && m.Sender != "w1" {
			t.Errorf("block event from %q, want w1", m.Sender)
		}
		if m.Name == "ran" {
			break
		}
	}
	if !strings.Contains(strings.Join(names, " "), "block") {
		t.Errorf("events: %v", names)
	}
}

// Asking about unsaved output shows a question in the page, and its
// answer closes the window.
func TestAsk(t *testing.T) {
	file := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(file, []byte("```sh\necho hi\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, ts, c := start(t, file)
	login(t, ts, c)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/wails/events?w=1"
	ws, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPClient: c, HTTPHeader: http.Header{"Origin": {ts.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	if code, body := call(t, ts, c, "1", "RunBlock", 0); code != 200 {
		t.Fatalf("RunBlock: %d %s", code, body)
	}
	if code, body := call(t, ts, c, "", "CloseTab", 1); code != 200 {
		t.Fatalf("CloseTab: %d %s", code, body)
	}
	next := func(name string) message {
		t.Helper()
		for {
			_, b, err := ws.Read(ctx)
			if err != nil {
				t.Fatalf("waiting for %s: %v", name, err)
			}
			var m message
			_ = json.Unmarshal(b, &m)
			if m.Name == name {
				return m
			}
		}
	}
	q := next("mdsh:ask").Data.(map[string]any)
	if q["message"] != "Save the output in doc.md before closing?" {
		t.Errorf("question: %v", q)
	}
	body, _ := json.Marshal(map[string]any{"id": q["id"], "value": "1"}) // Don't Save
	resp, err := c.Post(ts.URL+"/wails/answer", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	next("mdsh:closed")
	s.mu.Lock()
	n := len(s.windows)
	s.mu.Unlock()
	if n != 0 {
		t.Errorf("%d windows left", n)
	}
}
