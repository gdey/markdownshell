---
title: HTTP requests
description: http blocks, a request written the way it goes over the wire, with checks on the response and values captured for later blocks.
weight: 23
---

A runbook often has to call a web service: log in, create something,
poll a health check, and make sure what came back is what it should
be. An `http` block holds one request, written the way it goes over
the wire, with the checks under it in [Hurl](https://hurl.dev)'s words.
mdsh makes the request itself, so it needs no `curl` or `jq`, and the
response is saved under the block as its output, with the credentials
in it masked.

````markdown
```http {#login}
POST https://api.example.com/sessions
Content-Type: application/json

{"user": "j", "password": "{{$API_PASSWORD}}"}

HTTP 201
[Asserts]
header Content-Type contains application/json
jsonpath $.token exists
[Captures]
token: secret jsonpath $.token
```

```output {for=login exit=0 at=2026-11-16T19:08:00Z}
HTTP/1.1 201 Created
Content-Length: 61
Content-Type: application/json
Set-Cookie: (secret)

{
  "token": "(secret)",
  "expires": "2026-11-17T19:08:00Z"
}
```

```http {#whoami depends=login}
GET https://api.example.com/me
Authorization: Bearer {{#login/env(token)}}

HTTP 200
[Asserts]
jsonpath $.user == "j"
```
````

The first block logs in, checks the status and two things about the
response, and captures the token. The second sends it back, and runs
`login` first if it hasn't run. `{{$API_PASSWORD}}` is filled in as the
request goes out and never written to the file; see
[Secrets](#secrets) for what is masked.

An `http` block runs like a `sh` block: in `mdsh`, `mdsh -i` and the
desktop app, with `-w` saving its output, and with `depends=`,
`confirm`, `run=`, `skip`, `timeout=` and `on_error=` meaning what they
mean on any block. [examples/http.md](https://github.com/gdey/markdownshell/blob/main/examples/http.md)
in the repository runs each part of this page against a small local
server.

## The request

The body of the fence is the request:

1. `METHOD URL` on the first line: `GET`, `HEAD`, `POST`, `PUT`,
   `PATCH`, `DELETE`, `OPTIONS`, `TRACE` or `CONNECT`, and an
   `http://` or `https://` address.
2. Headers, one `Name: value` per line, right under it. A `Host`
   header sets the host the request is sent with. `User-Agent` is
   `mdsh` unless you set one.
3. A blank line, then the body, up to the `HTTP` line or the first
   `[Section]`. A body that is valid JSON is sent as
   `application/json` unless a `Content-Type` header says otherwise.
   `@file` as the whole body sends a file instead, named relative to
   the document and inside its folder, as an include is.

Lines starting with `#` are comments. Three sections can sit between
the headers and the body, each ended by a blank line:

| Section | Lines | Does |
| --- | --- | --- |
| `[QueryStringParams]` (or `[Query]`) | `name: value` | Adds parameters to the URL, encoded for you |
| `[FormParams]` (or `[Form]`) | `name: value` | Sends a form body, `application/x-www-form-urlencoded`; not with a body of its own |
| `[Options]` | `insecure: true`, `follow: true`, `timeout: 10s` | Skip certificate checks, like `curl -k`; follow redirects, which aren't followed otherwise; give up after this long |

The request goes out through the system's proxy and certificate
store, with a 30 second timeout unless `[Options]` or the block's
`timeout=` attribute says otherwise; the attribute wins. Multipart
bodies, `[Cookies]` and `[BasicAuth]` aren't supported: set a `Cookie`
or `Authorization` header instead. A request that doesn't read, such
as an unknown method or section, is a warning when the file opens and
the block's error when it runs.

Placeholders anywhere in the request, in the URL, a header, a
parameter, the body, an `@file` name or a check's value, are filled in
as the request is made: `{{$VAR}}` from the variables you
[exposed]({{< relref "conditions#exposing-variables" >}}),
`{{$MDSH_VAR_name}}` from an [input]({{< relref "inputs" >}}) or
`mdsh ctl set`, and `{{#block/env(VAR)}}` from an earlier `sh` block's
shell or an earlier `http` block's captures. They are never stored in
the file, and one that would fill a URL or a header with a line break
is an error, so a value can't add a header of its own.

## Checks

A line `HTTP 201` after the body checks the status; `HTTP *` checks
nothing and just marks where the response part starts. Under it,
`[Asserts]` holds one check per line, each `QUERY [count] [not]
PREDICATE [VALUE]`:

````markdown
```http {#search}
GET https://api.example.com/items
[QueryStringParams]
q: lamp

HTTP 200
[Asserts]
header Content-Type startsWith application/json
jsonpath $.items count >= 1
jsonpath $.items[0].name contains lamp
jsonpath $.items[*].price not isEmpty
body matches "\"total\": *[0-9]+"
duration < 2000
```
````

| Query | Reads |
| --- | --- |
| `status` | The status code, as a number |
| `header NAME` | The value of that header; a list when it has several |
| `jsonpath PATH` | A value from a JSON body: `$`, `.name`, `['name']`, `[n]` (negative counts from the end), `.*`, `[*]` and `..name`. A path with a wildcard or `..` reads as a list |
| `body` | The whole body, as text |
| `regex PATTERN` | The first group of the first match in the body, or the whole match when the pattern has no group |
| `duration` | How long the request took, in milliseconds |

`count` after a query turns a list or an object into how many items
it holds, and `not` turns the predicate around.

| Predicate | True when |
| --- | --- |
| `==`, `!=` | The value equals, or doesn't equal, `VALUE` |
| `<`, `<=`, `>`, `>=` | Both are numbers and compare so |
| `contains` | A string holds `VALUE`, or a list has an item equal to it |
| `startsWith`, `endsWith` | A string starts or ends with `VALUE` |
| `matches` | A string matches the regular expression `VALUE` |
| `exists` | The query found something: the header is there, the path leads somewhere |
| `isEmpty` | An empty string, list or object |
| `isNumber`, `isString`, `isBoolean`, `isCollection` | The value is of that kind; a list or an object is a collection |

Quotes are optional: `jsonpath $.user == j` and
`jsonpath "$.user" == "j"` are the same, and a value that reads as a
JSON literal is compared as one, so `== 3` compares a number and
`== "3"` a string (a header, which is always text, equals either). The
words are Hurl's, so a block can be pasted into a `.hurl` file.

A check that fails fails the block: its exit status is 1, and the
block's stderr has one line per failed check, with what the response
had:

````markdown
```output {for=whoami stream=stderr}
assert jsonpath $.user == "j": got "guest"
```
````

So `on_error`, `depends=` and `{{#whoami/?}}` work as for a `sh` block
that failed. A request that gets no response at all, because the host
is down or the timeout ran out, is an error on the block.

## Captures

`[Captures]` lines, `name: QUERY`, keep a value from the response as a
variable of the block, the way a `sh` block's `token=…` sets one of its
shell. Prose, conditions and later `http` blocks read it as
`{{#login/env(token)}}`; `-w` stores the last value in the placeholder
as it does for a shell variable. A capture whose query finds nothing
fails the block, like a check.

`name: secret QUERY` (or Hurl's `name: QUERY redact`) marks a capture
secret: its value is masked as `(secret)` wherever the saved response
holds it, and a placeholder that reads it is stored bare.

No shell sees a capture. Placeholders inside code are left alone, so a
`sh` block can't read `{{#login/env(token)}}`; a request that needs the
value is written as an `http` block.

## The response in prose and conditions

Three placeholders read the last response of an `http` block, as
`env(VAR)` reads a shell:

| Placeholder | Reads |
| --- | --- |
| `{{#login/status}}` | The status code |
| `{{#login/json($.expires)}}` | A value from a JSON body; one that isn't a string reads as JSON |
| `{{#login/header(Content-Type)}}` | A header; the first value when it has several |

They work in prose and in conditions, such as
`run={{#login/status}}==201` or a `<!-- mdsh:if -->` section, and `-w`
stores the last value in them, except that one reading a masked header
or a secret capture's path is stored bare. Each is an error on a block
that isn't an `http` block.

```markdown
The session lasts until {{#login/json($.expires)}}.

<!-- mdsh:if {{#whoami/status}}!=200 -->
> The token didn't work. Run `login` again.
<!-- mdsh:end -->
```

## What is saved

The response is the block's output, saved in an `output` fence as any
block's is: the status line, the headers sorted by name, a blank line
and the body. A JSON body is pretty-printed, unless the block is marked
`raw`; a body that isn't text, such as an image, is described rather
than shown (`(2048 bytes of image/png, not shown)`); a body longer
than 8 MB is cut there and says so. `exit=0` means the status and
every check passed and every capture found something.

### Secrets

The saved response never holds a credential:

- The values of `Set-Cookie`, `Authorization`, `Proxy-Authorization`,
  `WWW-Authenticate` and `X-API-Key` are saved as `(secret)`. An API
  that puts a token in a header of its own can add names in the front
  matter; names match in any case, and the built-in five can't be taken
  off:

  ```yaml
  mdsh:
    http:
      secret_headers: [X-Session-Id, X-Request-Token]
  ```

- A `secret` capture's value is masked as `(secret)` wherever the
  response holds it, in the body, a header or a failed check's `got`,
  however the body escaped it.
- Checks and captures see the real values; a failed check on a masked
  header reports `(secret)` as what it got.
- The request's placeholders are filled in on the way out and never
  stored, and the output never repeats a request header, so the fence
  keeps `{{$API_PASSWORD}}` and the file keeps no password.
- The [audit log]({{< relref "control#the-audit-log" >}}) records, in
  `block.end`, the method, the URL as written (placeholders and all),
  the status and each check's result, never a header or a body. A
  request that got no response reports its error with the URL's scheme
  and host only.

A block marked `secret` or `nosave` works as for any block, when the
whole exchange should stay out of the file.

## Attributes

`#id`, `skip`, `run=`, `show=`, `show-output=`, `show-stderr=`,
`collapse`, `collapse-output`, `links`, `nosave`, `secret`, `depends=`,
`always`, `confirm`, `timeout=` and `on_error=` work as on a `sh`
block, and `raw` is the block's own. `env=`, `share=`, `tty`,
`background` and `out=` are errors on it: it has no shell, no terminal,
can't run in the background yet, and its output is the response.

## In the desktop app and the terminal

In mdsh-gui an `http` block's card shows the response's status beside
the block's exit status, the body and the headers as two tabs, body
first, with JSON highlighted, and each check with a tick or a cross and
what the response had for one that failed. The terminal button copies
the request as a `curl` command, placeholders filled in, for a shell.
Insert › http Request in the source view, and the markdown bar's block
button, start a new one.

{{< shot src="http-gui.png" paper="http-gui-paper.png" phosphor="http-gui-phosphor.png" title="api.md" >}}An http block whose check failed: 200 OK and exit 1 in its head, the JSON body under the Body tab, and a cross by `jsonpath $.user == "j"` with `got "guest"`.{{< /shot >}}

{{< shot src="http-gui-headers.png" paper="http-gui-headers-paper.png" phosphor="http-gui-headers-phosphor.png" title="api.md · Headers" >}}The same card's Headers tab, with every check ticked.{{< /shot >}}

The card folds at two levels. The fold button on the output, or
`collapse-output` on the fence, folds the whole response to its status
line and how many more lines there are, and that fold is saved to the
file as any fold is. The chevron beside the Body tab folds just the
body, to its first line and a count, keeping the status and the checks
in view; that fold lasts while the app is open and isn't saved.

{{< shot src="http-fold.png" paper="http-fold-paper.png" phosphor="http-fold-phosphor.png" title="api.md · output folded" >}}The whole response folded: `HTTP/1.1 200 OK … 10 more lines`.{{< /shot >}}

{{< shot src="http-body-fold.png" paper="http-body-fold-paper.png" phosphor="http-body-fold-phosphor.png" title="api.md · body folded" >}}The body folded to `{ … 6 more lines`, with the three checks still showing.{{< /shot >}}

`mdsh -i` shows the same under the block: the status, the body, and
the checks. `H` switches between the body and the headers, and `Y`
copies the request as `curl`.

{{< shot src="http-tui.png" title="mdsh -i api.md" term=true >}}The terminal UI showing a response's headers after `H`, with its checks below, one crossed.{{< /shot >}}

On the command line, `mdsh -w` reports each block as it does for any
other, and a failed check's line goes to stderr.

{{< shot src="http-cli.png" title="mdsh -w api.md" term=true >}}A login that passes, its token saved as `(secret)`, and a request whose check fails with `got "guest"` on stderr.{{< /shot >}}
