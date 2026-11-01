# Dependency lines in mdsh-gui

Status: design, Nov 1, 2026. Both parts are built.

`depends=` (on main, #91) says which blocks must have run before a
block. The rendered view draws those as lines in the page margin, painted
onto the background, and lets you make, move and break them by hand.

## What is drawn

- **One line per dependency.** A line runs from the dependent block up
  to the block it depends on. Each card has two anchors on its edge: a
  ring at the top, on the head row, where its own lines leave and are
  dragged from, and a dot at the bottom, where lines from the blocks
  that depend on it arrive (j, Nov 1). A dot sits at the line's
  corner on the dependency's row, in the line's own lane: every line
  into a card arrives at the same spot on its edge, so the corner is
  what tells them apart, and it is the thing to click or drag.
- **Four colors, by position.** A block's first dependency is drawn in
  color 1, its second in color 2, and so on to four, in the order the
  file lists them (`depends=push,migrate,login,build` is purple, pink,
  teal, orange). A fifth and later are drawn too, in a muted gray, so
  nothing in the file goes unseen. The four colors are picked away from
  the colors cards already use (accent blue for selected, green ran, red
  failed) and have dark-mode variants.
- **Painted on.** Lines are an SVG layer behind the cards, at about a
  third opacity, with rounded corners. Where two overlap they add up a
  little darker, like paint. They never cover a card.
- **Lanes.** Each line gets a lane, a vertical offset from the cards.
  Shorter lines sit nearest the cards, and a line takes the first lane
  where no other line runs over the same rows, so two lines rarely merge
  into one. Lanes are 10px apart, 14px out from the card edge.
- **Which side.** Whichever margin has room for the lanes in use (at
  least 40px, plus 10px per lane), the left first. The outline drawer
  and the source pane eat into the left, so split view often puts them
  on the right. When neither margin has room, the lines fall back to a
  single lane in the 28px strip left of the cards, still colored.

## Selecting a block

Selecting a card (click, `j`/`k`, or the cursor in the source) brings its
lines to full color and outlines each block it depends on in that line's
color. Other lines stay faint. The outline follows the same highlight
rules as today's selection ring, one color per dependency.

## Editing (part 2)

All of these write `depends=` into the block's `{…}` and go through the
same change path as moves and folds, so Undo takes them back and the
source pane shows the new attribute at once. Blocks without ids get
them first, as moving a block does.

- **Make one.** Drag a card's top anchor up: a line follows the
  pointer, and a card above lights up as you pass over it, with the line
  snapping to its bottom anchor. The line only goes up; below the block's own row it
  stays put and nothing is a target. Drop to add that card's id to the end of
  `depends=`, in the next color. Dropping below the block, on the block
  itself, or on a block that can't be depended on (an include, a tape, a
  textbox or choice) does nothing and the status line says why.
- **Break one.** Click the dot at a line's corner: that id comes out of
  `depends=`. If it was the last one, the attribute goes. Dragging the
  dot out past the lanes, away from the cards, where the line could no
  longer turn back to them, tears it off the same way when let go.
- **Move one.** Drag the dot to another card above the dependent block:
  the id is replaced in place, so the line keeps its color.
- **Jump.** Double-click a line to scroll to its far end: from near the
  bottom you land on the dependency, from near the top on the dependent
  block. The target is selected, so its own lines light up, and you can
  walk a chain by double-clicking again.

The document's own checks still apply: a drop that would make a cycle is
refused with mdsh's message.

## How it is built

- `notebook.Cell` gains `depends`, the block's `depends=` ids, so the
  frontend has the edges without parsing attributes.
- A new `deps.ts` keeps two SVG layers over the scrolling pane: the
  paint, behind the cards, and a clear one over everything holding the
  dots, the rings and unseen wider copies of the lines to click, since
  the paint can't be reached where the page is over it. It measures
  cards after each draw, fold or resize (a `ResizeObserver` on the pane
  and `#doc`), assigns lanes and draws paths. Edges are `{from, to, k}`
  by block index; a target hidden in a folded section attaches to the
  section's heading instead.
- `Notebook.SetDepends(block int, deps []int)` takes the dependencies
  by block index, since a block without an id can be dragged to; it
  names the blocks first, as a move does, rewrites the info string with
  `doc.SplitInfo` / `Info.Set` and the block's `InfoSpan`, and goes
  through `change` for Undo. Parsing the result runs `checkDepends`, so
  cycles and bad targets come back as errors and change nothing.

## Split

1. Part 1: lines, colors, lanes, side choice, selection highlight,
   double-click to jump. Read-only.
2. Part 2: handle, disconnect, retarget.
