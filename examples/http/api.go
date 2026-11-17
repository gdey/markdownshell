// A tiny JSON API for http.md to talk to.
//
//	go run ./http 8765          serve on that port until POST /quit
//	go run ./http -wait 8765    wait until the server there answers
//
// It keeps no state and sends no Date header, so the output http.md
// saves reads the same on every run.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const token = "t0k3n-4f8a2c91"

type item struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}

var items = []item{
	{1, "desk lamp", 30},
	{2, "floor lamp", 75},
	{3, "chair", 120},
}

func main() {
	wait := flag.Bool("wait", false, "wait until the server on PORT answers, then exit")
	flag.Parse()
	port := "8765"
	if flag.NArg() > 0 {
		port = flag.Arg(0)
	}
	addr := "127.0.0.1:" + port
	if *wait {
		for i := 0; i < 100; i++ {
			if r, err := http.Get("http://" + addr + "/health"); err == nil {
				r.Body.Close()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		fmt.Fprintln(os.Stderr, "no server on", addr)
		os.Exit(1)
	}
	srv := &http.Server{Addr: addr, Handler: http.HandlerFunc(handle)}
	fmt.Println("listening on http://" + addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// reply writes v as JSON with status code, without a Date header.
func reply(w http.ResponseWriter, code int, v any, headers ...string) {
	w.Header()["Date"] = nil
	for i := 0; i+1 < len(headers); i += 2 {
		w.Header().Set(headers[i], headers[i+1])
	}
	if v == nil {
		w.WriteHeader(code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func user(r *http.Request) string {
	if r.Header.Get("Authorization") == "Bearer "+token {
		return "j"
	}
	return "guest"
}

var server *http.Server

func handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method + " " + r.URL.Path {
	case "GET /health":
		reply(w, 200, map[string]string{"status": "ok"}, "X-Request-Token", "req-77aa")
	case "GET /redirect":
		reply(w, 302, nil, "Location", "/health")
	case "GET /me":
		reply(w, 200, map[string]string{"user": user(r)})
	case "GET /items":
		q := r.URL.Query().Get("q")
		found := []item{}
		for _, i := range items {
			if strings.Contains(i.Name, q) {
				found = append(found, i)
			}
		}
		reply(w, 200, map[string]any{"total": len(found), "items": found})
	case "POST /sessions":
		var creds struct{ User, Password string }
		json.NewDecoder(r.Body).Decode(&creds)
		if creds.User == "j" && creds.Password == "hunter2" {
			reply(w, 201, map[string]string{"token": token, "expires": "2030-01-01T00:00:00Z"},
				"Set-Cookie", "session="+token+"; HttpOnly")
		} else {
			reply(w, 401, map[string]string{"error": "bad user or password"})
		}
	case "POST /items":
		if user(r) != "j" {
			reply(w, 401, map[string]string{"error": "log in first"})
			return
		}
		var it item
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			r.ParseForm()
			it.Name = r.Form.Get("name")
			it.Price, _ = strconv.Atoi(r.Form.Get("price"))
		} else {
			json.NewDecoder(r.Body).Decode(&it)
		}
		it.ID = len(items) + 1
		reply(w, 201, it, "Location", fmt.Sprintf("/items/%d", it.ID))
	case "POST /quit":
		reply(w, 200, map[string]bool{"bye": true})
		go func() { time.Sleep(100 * time.Millisecond); os.Exit(0) }()
	default:
		reply(w, 404, map[string]string{"error": "no such path"})
	}
}
