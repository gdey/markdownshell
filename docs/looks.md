# Adding a look

A look is the desktop app's paint: Plain, Teletype, Blueprint, Bubble
Gum, Witchcraft, and one more each minor release. Each look is one
file, `look.css`, in its own folder in
[`cmd/mdsh-gui/frontend/looks`](../cmd/mdsh-gui/frontend/looks). Its
first comment opens with the look's **front matter**, TOML between
`+++` lines that says what the look is; the rules after it paint it:

```css
/* +++
id = "witch"
name = "Witchcraft"
order = 40
since = "1.5"
description = "Witchcraft writes the document on a scroll …"
tagline = "for a purple grimoire under the moon"
tokens = "--witch-"

[texture]
label = "Grimoire"
rules = "Stars and moons"
shadows = "Glow around cards"
+++

   The Witchcraft look (Settings › Look): … */

:root[data-look="witch"],
…
```

TOML has comments, so the front matter can explain itself, and its
types are strict: `order = "40"` is an error, not a number. Plain's
`look.css` has only its front matter, since its colors are
`style.css`'s.

Everything else that names the looks is made from those folders, so a
new look needs nothing outside its folder but pictures and a CHANGELOG
entry:

| Made from the folders | By |
| --- | --- |
| Settings › Look's menu, its description, the look's own text size and texture switches | `frontend/src/looks.gen.ts`, made by `go generate` |
| The stylesheet every page loads | `frontend/bundle/looks.css`, made by `go generate` |
| The looks the app accepts in its settings | `looks.Load`, at start |
| The tests every look must pass | `allLooks`, in the tests |
| The website's looks band and its lists of the looks | `site/data/looks.toml`, made by `go generate` |
| The GUI README's pictures of each look | its looks section, made by `go generate` |

## Steps

From `cmd/mdsh-gui`:

1. **Start it.**

   ```
   go run ./internal/newlook moss "Moss"
   ```

   This makes `frontend/looks/moss/look.css`: front matter with every
   key, the ones it can't guess marked TODO and the optional ones
   commented out, then the plain look's colors under
   `[data-look="moss"]`. The id
   (the setting's value and the `data-look` attribute) is the folder's
   name without dashes, or `-id`; the release is the one after the
   newest look's, or `-since`.

2. **Paint it** in `look.css`. Change the palettes, light and dark; add
   the look's own tokens (`--moss-*`, set in both palettes); then the
   texture rules. Paint, not layout: colors, borders, backgrounds,
   shadows and faces. Every rule selects `[data-look="moss"]`.

3. **Describe it** in the front matter: the `description` shown in
   Settings › Look, the website's `tagline`, and a `texture` if it has
   switches. The switches are the settings `noSprockets`, `noRules` and
   `noShadows`; a look names the ones it draws, under its own names:

   ```toml
   [texture]
   label = "Grimoire"
   rules = "Stars and moons"
   shadows = "Glow around cards"
   ```

   The CSS turns each off under `:root[data-no-rules]` and so on. A
   look that sets `--text-scale` other than 1 gives `textSize` as a
   percent.

4. **Make the files** and test:

   ```
   go generate     # or: go run ./internal/build
   go test ./...
   ```

   The tests fail until the look is done. They check that:

   - every TODO `newlook` left is gone;
   - every token the plain look sets is set in both palettes, for the
     window and the History pane, and the look's own tokens are set in
     the dark as well as the light;
   - text, code, links and buttons are at least 4.5:1 and the accent
     3:1, in both themes;
   - every rule selects the look, and nothing moves the layout;
   - `textSize` is what the CSS sets;
   - the generated files are up to date, and the pictures exist.

5. **Pictures.** Take the light, dark and Settings › Look pictures (see
   [`site/README.md`](../site/README.md)), put them in
   `site/static/images/gui/`, and list them in the front matter's
   `[[screenshots]]`. The website's `hero` pictures come from a release
   build; leave `hero` out until then.

6. **CHANGELOG.** An entry under Unreleased, with a picture committed in
   `docs/changelog/`.

## The front matter

| Field | |
| --- | --- |
| `id` | The setting's value and `data-look`: lowercase letters and digits. Plain's is empty. |
| `name` | What Settings and the docs call it. |
| `order` | Its place in the menu and galleries, lowest first. |
| `since` | The release that brought it, such as `1.6`. |
| `description` | A sentence or two for Settings › Look. |
| `tagline` | Ends "Moss for …" on the website's looks band. |
| `textSize` | Its text size, in percent, if not 100. |
| `tokens` | The prefix of its own CSS tokens, such as `--moss-`. |
| `contrastGaps` | Only for Teletype and Blueprint, whose gaps the accessibility audit lists. A new look passes. |
| `[texture]` | Its texture switches, if any: `label`, and `sprockets`, `rules` and `shadows` for those it draws. |
| `[hero]` | The website's picture: `light`, `dark` and `alt`, from `site/static/images/app/`. |
| `[[screenshots]]` | The GUI README's pictures, each a `file` in `site/static/images/gui/` and its `alt`. |

An unknown key is an error, so a misspelling can't pass silently.
