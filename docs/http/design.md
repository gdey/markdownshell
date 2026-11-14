# HTTP request blocks with checks

Status: design, Nov 13, 2026; j settled questions 1 to 7 the same
day. Building for 1.6: the fence, its checks and captures, masking and
the audit log and the response placeholders are built (see the
spec's HTTP blocks); the views' tabs and *Copy as curl* come next, and `background` on an http block is a parse
error for now. The open questions at the end keep their answers; the
text above them reads as settled.

The idea (j, Nov 13): a runbook often has to make an HTTP request, with
a body and parameters, and then check that the response matches some
criteria. That needs a good way to describe the request, a way to play
it like any other block, and a way to record what came back.

Today that is a `sh` block with `curl`, a pipe to `jq`, and a `test`
or `grep` whose failure makes the block fail. It works, but the
request is buried in quoting, the checks are a shell idiom rather than a
statement, and the recorded output is whatever `curl -i` printed.

## Prior art

| Tool | Request | Checks | Variables | Fits mdsh |
| --- | --- | --- | --- | --- |
| `.http` files (JetBrains HTTP Client, VS Code REST Client) | `GET url`, header lines, blank line, body; `###` separates requests | JetBrains: a JavaScript handler `> {% client.test(...) %}`; VS Code: none | `{{name}}`, from env files | The request syntax is the one most people already know, and `{{…}}` is mdsh's own placeholder shape. The JS checks don't fit a markdown file |
| [Hurl](https://hurl.dev) | The same request shape, plus `[QueryStringParams]`, `[FormParams]`, `[Options]` sections | `HTTP 200`, then `[Asserts]`: `jsonpath "$.id" == 1`, `header "X" contains "y"`, `status`, `body`, `xpath`, `regex`, `duration`; `[Captures]` binds a value for later requests | `{{name}}` | Plain text, one request and its checks in one place, no scripting. The assertion grammar is the best one in this table |
| httpie | `http POST url name=j q==1 X-Hdr:v` on a command line | None | Shell | A shorthand for a terminal, not a document |
| Bruno `.bru` | Its own block language: `post { url: … }`, `body:json { … }` | `assert { res.status: eq 200 }` | `{{name}}` | Readable, but a format of its own that nothing else reads |
| Postman | JSON collections | JavaScript tests | Environments | Not hand-written |
| `curl` in a `sh` block (today) | A command line | `jq -e`, `grep`, `test` | The shell | Works now, with no new syntax; the request and the checks are hard to read |

The pattern across the good ones: the request is written the way it
goes over the wire, checks are one-line statements that name a part of
the response, and a value from one response can be captured for the
next request.

## Goals

- A request reads as a request: method, URL, headers, body, with no
  shell quoting.
- Checks are statements, one per line, each with a pass or fail of its
  own, and the block fails when any check fails. `on_error`, `{{#id/?}}`,
  `depends=` and the rest then work as for any block.
- The block plays like any block in `mdsh`, `mdsh -i` and mdsh-gui, and
  its output is saved under it with `-w`, as an `output` fence.
- Values from a response reach later blocks and prose: a token from a
  login reaches the next request, an id reaches a `sh` block.
- Secrets in a request are never written to the file.

Not goals: a full API test suite (Hurl exists), a browser, WebSockets,
and a scripting language for checks.

## Three ways to write it

### Option A: an `http` fence shaped like Hurl (recommended)

The fence's body is the request as it goes over the wire, then an
optional expected status and sections of checks and captures.

````markdown
```http {#login}
POST {{$API}}/sessions
Content-Type: application/json

{"user": "j", "password": "{{$API_PASSWORD}}"}

HTTP 201
[Asserts]
header Content-Type contains application/json
jsonpath $.token exists
[Captures]
token: jsonpath $.token
```

```output {for=login exit=0}
HTTP/1.1 201 Created
Content-Type: application/json
Content-Length: 61

{"token": "…", "expires": "2026-11-14T19:07:00Z"}
```

```http {#whoami depends=login}
GET {{$API}}/me
Authorization: Bearer {{#login/env(token)}}

HTTP 200
[Asserts]
jsonpath $.user == "j"
```
````

Request: the first line is `METHOD URL`; the lines up to the first
blank line are headers; what follows, up to `HTTP` or a `[Section]` at
the start of a line, is the body. `[QueryStringParams]` (`name: value`
lines) adds parameters to the URL, `[FormParams]` sends a form body,
`[Options]` holds `timeout`, `insecure`, `follow` and `retry` (below).
`@file` as the whole body sends a file beside the document, inside the
document's root as for includes.

Checks: `HTTP 201` alone is the status check. Every `[Asserts]` line
is `WHAT [ARG] OP VALUE`: `status`, `header NAME`, `jsonpath PATH`,
`body`, `regex PATTERN` and `duration` (milliseconds), with `==`, `!=`,
`<`, `<=`, `>`, `>=`, `contains`, `startsWith`, `endsWith`, `matches`,
`exists`, `isEmpty`, `isNumber`, `isString`, `isBoolean`, `isCollection`.
These are Hurl's names, kept so a Hurl user needs nothing new and so a
block can be pasted into a `.hurl` file. Quoting is optional: `$.user`
and `"$.user"` are the same; a value is a JSON literal or a bare word.

Captures: `name: WHAT [ARG]` sets a variable of the block, as a
`sh` block's `token=…` line sets one of its shell, so a later `http`
block, a condition and prose read it as `{{#login/env(token)}}`, and
`-w` stores the last value in the placeholder as it does for a shell
variable. Nothing reaches a shell: a `sh` block can't read a capture,
since placeholders in code are left alone; a request that needs the
value is written as an `http` block. If a shell ever needs one, the
way in is `mdsh ctl`, not a new rule, and that is a later question.
`name: secret jsonpath $.token` marks it secret: mdsh-gui blurs
output that names it, and `-w` stores it bare.

Output: the response as it came, status line, headers and body, in the
block's `output` fence, with `exit=0` when the status and every check
pass and `exit=1` otherwise. A failing check is reported on the
block's stderr fence, one line per failed check, so it shows by the
same rule as a failed `sh` block's stderr:

````markdown
```output {for=whoami stream=stderr}
assert jsonpath $.user == "j": got "guest"
```
````

Headers that carry a credential, `Set-Cookie`, `Authorization`,
`Proxy-Authorization`, `WWW-Authenticate` and `X-API-Key`, are saved
with their value masked (`Set-Cookie: (secret)`), so a saved file holds
no session; the live run still has the real value for checks and
captures. The front matter adds to that list, since an API can put a
token in a header of its own:

```yaml
mdsh:
  http:
    secret_headers: [X-Session-Id, X-Request-Token]
```

Names match case-insensitively, as headers do; the built-in five are
always masked, and the list can't take one off (j, Nov 13). A JSON body is pretty-printed in the output (`raw` on the
fence keeps the bytes). The `out=` kinds don't apply; `show-output=`, `nosave`,
`secret`, `links` and `collapse-output` do.

Variables: placeholders in the body, `{{$VAR}}` from mdsh's exposed
environment, `{{$MDSH_VAR_name}}` from inputs, `ctl set` and captures,
and `{{#block/env(VAR)}}` from an earlier `sh` block's shell or an
earlier `http` block's captures, are filled in as the request is made,
the way a tape's `Type` string is. They are never
stored in the file, so the fence above keeps `{{$API_PASSWORD}}` and
the output never repeats a request header. `{{#login/json($.token)}}`,
`{{#login/header(Content-Type)}}` and `{{#login/status}}` read a
response from prose and conditions, as `env(VAR)` reads a shell, and
`-w` stores the last value in them in the same way.

Why this one: one fence holds the whole step, the way a `sh` block
does. The request reads as the wire format people already write in
`.http` files, and the checks reuse a grammar that exists, is
documented and is small. mdsh does the request itself, in Go, so it
runs the same on every platform, needs no `curl` or `jq`, and has the
response as data for checks and placeholders instead of as text to
parse.

### Option B: an `http` fence with checks in the attributes

The same request body, with the checks on the fence:

````markdown
```http {#whoami status=200 expect="jsonpath $.user == \"j\"" expect="header Content-Type contains json"}
GET {{$API}}/me
Authorization: Bearer {{$MDSH_VAR_token}}
```
````

The request stays pure `.http`, so other tools read it. But the
attributes are one line, each check needs its quotes escaped, and
attributes are a set in mdsh, so repeating `expect=` is new parsing.
Captures have no place at all. Not recommended; `status=` alone could
be kept as a shortcut for the common case.

### Option C: no new fence, checks for every block

Keep `curl` in a `sh` block and add a `check {for=id}` fence, like
`tape {for=id}`, whose lines check the block's stdout and status:

````markdown
```sh {#whoami}
curl -sS -H "Authorization: Bearer $MDSH_VAR_token" "$API/me"
```

```check {for=whoami}
status == 0
jsonpath $.user == "j"
```
````

Nothing new to learn about requests, and checks reach every block,
not only HTTP. But the request is still a `curl` line with its
quoting, headers and the status code are not in the output unless
`-i` is passed and then the body isn't JSON any more, and captures
would have to parse text. The grammar is the same as Option A's
`[Asserts]`, so this can be a second step: build the grammar once for
`http` blocks, then let a `check {for=id}` fence apply it to any
block's stdout. See the last open question.

## How it runs

- `http` is a runnable language, next to `sh` and `zsh`: `Runnable`
  says so, `--only`, `depends=`, `confirm`, `run=`, `skip`,
  `timeout=`, `on_error=`, `background` and `nosave` mean what they
  mean on a `sh` block. `env=` and `share=` are parse errors on it (it
  has no shell); `tty` too.
- The request is made by Go's `net/http`, with redirects not followed
  unless `[Options]` says `follow: true`, a 30s timeout unless
  `timeout=` says otherwise, and the system proxy and certificate
  store. `insecure: true` skips certificate checks, like `curl -k`.
- `retry: 5` and `retry-interval: 2s` in `[Options]` repeat the
  request until the checks pass or the tries run out, for a deploy
  that polls a health check. The output is the last response.
- The audit log records the method, URL, status, duration and the
  checks' results, never headers or bodies.
- A remote document (`mdsh https://…`) with `http` blocks gets the same
  preview and the one question as today; a request is a block that
  runs, and nothing here gives a document a new power.
- mdsh-gui and `mdsh -i` show a status badge and the duration on the
  card, show the body and the headers as two tabs, body first,
  pretty-print the body, and list the checks with a tick or a cross; the GUI's copy button offers *Copy as curl*.
  Those views are for the thread that builds it; nothing in the file
  format depends on them.
- The LSP and the attribute form learn the `[Asserts]` words and the
  `http` attributes.

## Open questions

1. **Which option?** *Settled (j, Nov 13): A.* B is a shortcut, C a
   second step.
2. **Where do a request's variables come from?** Placeholders only
   (`{{$VAR}}`, `{{$MDSH_VAR_x}}`, `{{#block/env(VAR)}}`), or should
   an `http` block also take `env=` and read that shell's variables?
   *Settled (j, Nov 13): placeholders only.* `{{#block/env(VAR)}}`
   already reads any block's shell, no shell is involved, and the rule
   is the one tapes have.
3. **What is saved as output?** *Settled (j, Nov 13): the whole
   response, with `Set-Cookie` and the other credential headers
   masked, and body and headers as two tabs in the interactive views.*
4. **Hurl's words or mdsh's?** *Settled (j, Nov 13): Hurl's words*,
   so blocks stay portable and documented elsewhere.
5. **Bodies.** *Settled (j, Nov 13): JSON and forms only.* Multipart
   uploads, XML with `xpath` and GraphQL are for later.
6. **What a capture sets.** The first draft bound `$MDSH_VAR_name`,
   as `ctl set` does, so a `sh` block could read it. *Settled (j,
   Nov 13): a capture is the block's own variable, read as
   `{{#block/env(name)}}`*, the one rule for every block's variables.
   A `sh` block can't read it; see Captures above.
7. **Polling.** *Settled (j, Nov 13): `retry` is not in the first
   cut.* A `sh` loop around `mdsh ctl` does for now.
8. **Checks for `sh` blocks too.** Should the same `[Asserts]` grammar
   be usable on any block's stdout, as a `check {for=id}` fence
   (Option C)? *Settled (j, Nov 13): not now.*
