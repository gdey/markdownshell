# mdsh website

The project site, built with [Hugo](https://gohugo.io) and published to
GitHub Pages by [`.github/workflows/pages.yml`](../.github/workflows/pages.yml)
on every push to `main` that touches `site/`.

## Work on it locally

```sh
go install github.com/gohugoio/hugo@v0.147.9   # or brew install hugo
cd site
hugo server                                     # http://localhost:1313
```

Or run [`docs/site.md`](../docs/site.md) with mdsh: it checks for Hugo
(and offers to install the workflow's version), builds the site the way
CI does, then serves it.

There is no theme to install: the layouts in `layouts/` and the CSS in
`static/css/` are the whole design. Pages live in `content/`; docs pages
are ordered by `weight` in their front matter, and the landing pages
(Desktop app, Examples, Install, Security, Roadmap) use
`layout: landing`.

## Two skins

Every page is built once and painted by one of two skins: the teletype
(`static/css/tty.css`, fan-fold paper in light mode and green phosphor
in dark) by default, and the plain one (`static/css/site.css`, which
also holds the layout). `<html data-theme="tty">` or `"plain"` picks
it; the `[tty] [plain]` switch in the header saves the choice in
`localStorage`, and a few lines in `<head>` read it before the first
paint. The teletype palette is the app's Teletype look, from
`web/looks/teletype/look.css`; keep the two in step.

## The app's looks

The home page's looks band and the `{{< looks >}}` shortcode ("Plain,
Teletype, … or Witchcraft") read `data/looks.toml`, which `go generate`
in `cmd/mdsh-gui` makes from the app's looks; don't edit it by hand. A
look's band picture is the `[hero]` in its `look.css` front matter; see
[`docs/looks.md`](../docs/looks.md).

## The changelog page and the cheat sheet

`/changelog/` is the repository's `CHANGELOG.md`, mounted into the
site's assets in `hugo.toml` and rendered by
`layouts/_default/changelog.html`, so the site never carries a copy.
`/docs/cheatsheet/` is `docs/cheatsheet.md` the same way, mounted into
`content/docs/` beside the other docs pages, so its front matter is a
docs page's (`title`, `description`, `weight`). The Website workflow
runs when either file changes, too.

## Screenshots

The app and terminal pictures in `static/images/app/` were taken from a
real build of `main` for 1.3: `mdsh-gui` built on Linux (GTK 4,
WebKitGTK 6) and run under Xvfb at 2x scale, driven with `xdotool`, on a
copy of `examples/`, with the GTK menu bar cropped off; and `mdsh -i` in
an xterm. The 1.4 additions (`secrets-*`, `front-matter-*`,
`hero-bubblegum*`) were taken the same way, from a build of `main` for
1.4, with GTK animations off (reduced motion). Each picture has a light and, where it matters, a dark file
(`-dark`, or `paper` and `phosphor` for the Teletype look); pages show
them with the `shot` shortcode or partial, which puts them in a window
frame and swaps to the dark one with the OS. Give it `name=` instead of
`src=` and it shows the app in the look that matches the site's skin:
`name-light` and `name-dark` (the app's Plain look) under the plain
skin, `name-paper` and `name-phosphor` (its Teletype look) under the
teletype one, so a new picture needs all four. Squeeze new ones with
`pngquant --quality 75-95`.

The sample reports on the Use cases pages, in `static/samples/`, and
their previews come from the runbooks in `examples/runbooks`. Regenerate
them with `./samples.sh` (needs `mdsh` on your `PATH` and `pdftoppm`).

`static/css/chroma.css` holds the code highlighting colors. Regenerate it
with:

```sh
( hugo gen chromastyles --style=github
  echo '@media (prefers-color-scheme: dark) {'
  hugo gen chromastyles --style=github-dark
  echo '}' ) > static/css/chroma.css
```

## First deploy

1. In the repo, open **Settings → Pages** and set **Source** to
   **GitHub Actions**.
2. Merge to `main` (or run the **Website** workflow by hand). The site
   appears at `https://gdey.github.io/markdownshell/`.

## Adding a custom domain later

The workflow passes the Pages URL to Hugo as `--baseURL`, so no file in
this repo has to change.

1. At your DNS provider, point the domain at GitHub Pages:
   - an apex domain (`mdsh.example`): `A` records for `185.199.108.153`,
     `185.199.109.153`, `185.199.110.153` and `185.199.111.153` (and
     optionally `AAAA` records for `2606:50c0:8000::153` through
     `2606:50c0:8003::153`);
   - a subdomain (`www.mdsh.example` or `mdsh.example.dev`): a `CNAME`
     record to `gdey.github.io`.
2. In **Settings → Pages → Custom domain**, enter the domain and save.
   GitHub checks DNS; that can take a while.
3. Once the check passes, tick **Enforce HTTPS**.
4. Re-run the **Website** workflow so the site is rebuilt with the new
   base URL.

A `CNAME` file is not needed: sites deployed by a workflow take the custom
domain from the Pages settings, not from a file. It is also worth
[verifying the domain](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site/verifying-your-custom-domain-for-github-pages)
for your account, so nobody else can claim it.
