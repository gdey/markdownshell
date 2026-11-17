#!/usr/bin/env mdsh
---
title: Talking to web services
description: "http blocks: a request as it goes over the wire, checks on the response, and values captured for the next request."
mdsh:
  http:
    secret_headers: [X-Request-Token]
---
# Talking to web services

A runbook that calls an API used to be a `sh` block with `curl`, a
pipe to `jq` and a `test` whose failure made the block fail. An `http`
block says the same thing plainly: the request as it goes over the
wire, then the checks, one per line, in [Hurl](https://hurl.dev)'s
words. mdsh makes the request itself, so nothing else needs to be
installed, and the response is saved under the block with any
credential in it masked.

This file talks to a small API of its own, [http/api.go](http/api.go),
run with the Go toolchain the project already needs. The first block
starts it in the background, the next waits for it to answer, and the
last one asks it to stop.

```sh {#serve background timeout=2m}
go run ./http 8765
```

```output {for=serve exit=0 at=2026-11-17T08:09:57Z}
listening on http://127.0.0.1:8765
```

```sh {#wait show-output=false}
go run ./http -wait 8765
```

## Log in and capture the token

The first line is `METHOD URL`, the lines under it are headers, and
after a blank line comes the body. `HTTP 201` is the status to expect,
each `[Asserts]` line is a check, and `[Captures]` keeps a value from
the response as a variable of the block. `secret` on the capture
masks the token wherever the saved response holds it, and the
`Set-Cookie` header is masked always.

```http {#login}
POST http://127.0.0.1:8765/sessions
Content-Type: application/json

{"user": "j", "password": "hunter2"}

HTTP 201
[Asserts]
header Content-Type contains application/json
jsonpath $.token exists
jsonpath $.expires isString
[Captures]
token: secret jsonpath $.token
```

```output {for=login exit=0 at=2026-11-17T08:09:57Z}
HTTP/1.1 201 Created
Content-Length: 60
Content-Type: application/json
Set-Cookie: (secret)

{
  "expires": "2030-01-01T00:00:00Z",
  "token": "(secret)"
}
```

The login answered {{#login/status:"201"}} and the token is good until
{{#login/json($.expires):"2030-01-01T00:00:00Z"}}. Both placeholders read the response the
block saved, so this line is right after `mdsh -w` without a run.

## Send the token back

`{{#login/env(token)}}` is the capture above, filled in as this
request goes out and never written to the file. `depends=login` runs
the login first when it hasn't run yet.

```http {#whoami depends=login}
GET http://127.0.0.1:8765/me
Authorization: Bearer {{#login/env(token)}}

HTTP 200
[Asserts]
jsonpath $.user == "j"
```

```output {for=whoami exit=0 at=2026-11-17T08:09:57Z}
HTTP/1.1 200 OK
Content-Length: 13
Content-Type: application/json

{
  "user": "j"
}
```

## Query parameters and checks on a list

`[QueryStringParams]` adds parameters to the URL, encoded for you. A
`jsonpath` with `[*]` or `..` reads a list, `count` turns a list into
its length, and `not` turns a predicate around. `regex` reads the
first group of its match in the body, and `duration` is the request's
time in milliseconds.

```http {#search}
GET http://127.0.0.1:8765/items
[QueryStringParams]
q: lamp

HTTP 200
[Asserts]
jsonpath $.total == 2
jsonpath $.items count == 2
jsonpath $.items[0].name contains lamp
jsonpath $.items[*].price not isEmpty
jsonpath $.items[-1].price > 50
regex "\"total\": *([0-9]+)" == 2
duration < 5000
```

```output {for=search exit=0 at=2026-11-17T08:09:57Z}
HTTP/1.1 200 OK
Content-Length: 101
Content-Type: application/json

{
  "items": [
    {
      "id": 1,
      "name": "desk lamp",
      "price": 30
    },
    {
      "id": 2,
      "name": "floor lamp",
      "price": 75
    }
  ],
  "total": 2
}
```

The search returned {{#search/header(Content-Type):"application/json"}} and
{{#search/json($.total):"2"}} items.

## A form, and a body from a file

`[FormParams]` sends a form body instead of a JSON one. `@file` as the
whole body sends a file beside the document, here
[http/item.json](http/item.json).

```http {#create-form depends=login}
POST http://127.0.0.1:8765/items
Authorization: Bearer {{#login/env(token)}}
[FormParams]
name: clip lamp
price: 15

HTTP 201
[Asserts]
header Location startsWith /items/
jsonpath $.name == "clip lamp"
jsonpath $.price == 15
```

```output {for=create-form exit=0 at=2026-11-17T08:09:57Z}
HTTP/1.1 201 Created
Content-Length: 39
Content-Type: application/json
Location: /items/4

{
  "id": 4,
  "name": "clip lamp",
  "price": 15
}
```

```http {#create-file depends=login}
POST http://127.0.0.1:8765/items
Authorization: Bearer {{#login/env(token)}}

@http/item.json

HTTP 201
[Asserts]
jsonpath $.name == "reading lamp"
```

```output {for=create-file exit=0 at=2026-11-17T08:09:57Z}
HTTP/1.1 201 Created
Content-Length: 42
Content-Type: application/json
Location: /items/4

{
  "id": 4,
  "name": "reading lamp",
  "price": 45
}
```

## Options and masked headers

`[Options]` holds `follow: true` to follow redirects, which aren't
followed otherwise, `timeout:` and `insecure: true`. The health check
answers with an `X-Request-Token` header, which the front matter's
`mdsh.http.secret_headers` adds to the headers saved as `(secret)`;
the check still sees the real value.

```http {#health}
GET http://127.0.0.1:8765/redirect
[Options]
follow: true

HTTP 200
[Asserts]
jsonpath $.status == "ok"
header X-Request-Token exists
```

```output {for=health exit=0 at=2026-11-17T08:09:57Z}
HTTP/1.1 200 OK
Content-Length: 16
Content-Type: application/json
X-Request-Token: (secret)

{
  "status": "ok"
}
```

## A check that fails

Without the token, the API says the caller is a guest, so this check
fails on purpose. A failed check fails the block, with one line on
stderr saying what the response had, and `on_error=continue` lets the
run go on past it.

```http {#anon on_error=continue}
GET http://127.0.0.1:8765/me

HTTP 200
[Asserts]
jsonpath $.user == "j"
```

```output {for=anon exit=1 at=2026-11-17T08:09:57Z}
HTTP/1.1 200 OK
Content-Length: 17
Content-Type: application/json

{
  "user": "guest"
}
```

```output {for=anon stream=stderr}
assert jsonpath $.user == "j": got "guest"
```

<!-- mdsh:if {{#anon/?}}!=0 -->
> The anonymous request failed its check, as it should: without a
> token the API answered {{#anon/json($.user):"guest"}}.
<!-- mdsh:end -->

## Stop the server

```http {#stop}
POST http://127.0.0.1:8765/quit

HTTP 200
```

```output {for=stop exit=0 at=2026-11-17T08:09:57Z}
HTTP/1.1 200 OK
Content-Length: 13
Content-Type: application/json

{
  "bye": true
}
```

In mdsh-gui each `http` block's card shows the status beside its exit
status, the body and the headers as two tabs and the checks with a
tick or a cross, and its terminal button copies the request as a
`curl` command. In `mdsh -i`, `H` switches between the body and the
headers and `Y` copies the `curl` command. The
[documentation](https://gdey.github.io/markdownshell/docs/http/) has
every query, predicate and option.
