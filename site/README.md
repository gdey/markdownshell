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

There is no theme to install: the layouts in `layouts/` and the CSS in
`static/css/` are the whole design. Pages live in `content/`; docs pages
are ordered by `weight` in their front matter.

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
