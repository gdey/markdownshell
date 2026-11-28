# OSC escape codes: survey and mdsh support

Design. A survey of the OSC (Operating System Command) sequences
terminals speak, how widely each is supported, what mdsh does with each
today, and what it should do next. The site page
[Terminal escape codes](../../site/content/docs/terminal-codes.md) is the
reference version of this; this file keeps the reasoning and the ranked
plan. Written against `main` at aca5fcf (OSC 7501 steps 1 and 2 merged).

## Background

OSC sequences are `ESC ] Ps ; Pt ST`, where `ST` is `ESC \` or BEL. They
are the escape family for everything that is not cursor movement or
text attributes: titles, colours, clipboard, links, notifications, shell
integration, images. There is no registry. xterm's `ctlseqs` defines the
numbers below about 120; vendors picked large numbers (133, 633, 777,
1337, 7501, 5522) to avoid collisions, and ConEmu reused 9 with
sub-commands, which collides with iTerm2's use of 9.

Three places in mdsh meet OSC sequences:

1. **Output rendering.** Saved and live block output drawn by the TUI
   (`termsafe.Styled`), prompts (`termsafe.SGROnly`) and the GUI's `Ansi`
   class in `web/src/highlight.ts`. All three keep SGR colours and drop
   or make visible every OSC, so output in a file can never set the
   clipboard, the title or a hidden link target. This is a deliberate
   security boundary and most "not supported" entries below are correct
   because of it.
2. **tty blocks and the History pane.** In the GUI, `--serve` and
   `mdsh -i` both are drawn by a terminal emulator inside mdsh: xterm.js
   6, or vt10x in `mdsh -i` (`tui/term.go`) and in recordings
   (`termrec`). Plain `mdsh doc.md` is different: `env.Shell.RunTTY`
   hands the block's pty to the user's real terminal, so there every OSC
   the program writes takes effect on it. xterm.js
   implements titles, OSC 8 links and the 4/10/11/12/104/110–112 colours
   and ignores the rest unless a handler is registered; vt10x implements
   titles (kept in a field mdsh doesn't read) and the 4/10/11 colours
   with their queries, and swallows everything else. So a program's OSC
   in a tty block never reaches the user's real terminal or clipboard in
   the GUI or the TUI unless mdsh forwards it, and always does under
   the CLI.
3. **mdsh's own emission.** OSC 133 marks in the shell integration
   (`env/integration.go`), OSC 52 for copy in the TUI
   (`tui/clipboard.go`), OSC 1337 File for images on iTerm2 and WezTerm
   (`termimg/termimg.go`), OSC 7501 program status (`termstatus/`,
   `cmd/mdsh/status.go`, `tui/status.go`), and the OSC 11 background
   query Lip Gloss sends at startup (its reply handled in
   `tui/reply.go`).

## Support matrix

Legend: ● full, ½ partial or behind a setting, ○ no, ? not verified.
Sources: xterm ctlseqs; Ghostty VT reference, WezTerm
`escape-sequences.md` and the Windows Terminal progress-bar doc, all
read 2026-11-27; iTerm2 and kitty docs; the OSC 7501 design in
[`docs/program-status/design.md`](../program-status/design.md). Entries
marked ? are from memory.

| OSC | What | mdsh | xterm | iTerm2 | kitty | WezTerm | Ghostty | foot | Alacritty | WT | VS Code | tmux | VTE | Konsole | Terminal.app |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 0 / 1 / 2 | icon name, title | strips | ● | ● | ● | ● | ● | ● | ● | ● | ● | ● | ● | ● | ● |
| 4 / 104 | palette set/query/reset | strips | ● | ● | ● | ● | ● | ● | ● | ● | ● | ½ | ● | ● | ○ |
| 5 / 105 | special colours (bold, underline, blink) | strips | ● | ○ | ○ | ○ | ● | ½ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 6 | xterm: special-colour mode; iTerm2: tab colour | strips | ½ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 7 | current directory (file:// URL) | strips | ○ | ● | ● | ● | ● | ● | ○ | ½ ? | ○ | ○ | ● | ● | ● |
| 8 | hyperlinks | sends, strips | ½ | ● | ● | ● | ● | ● | ● | ● | ● | ● (3.4+) | ● | ● | ○ |
| 9 (iTerm2 form) | desktop notification | strips | ○ | ● | ○ | ● | ● | ½ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 9;4 (ConEmu) | progress state | strips | ○ | ½ ? | ○ | ○ | ● | ○ | ○ | ● | ● | ○ | ○ | ½ (draft MR) | ○ |
| 9;9 (ConEmu) | current directory | strips | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ● | ○ | ○ | ○ | ○ | ○ |
| 10 / 11 / 12 | fg, bg, cursor colour (set and `?` query) | reads 11 reply, strips | ● | ● | ● | ● | ● | ● | ● | ● | ● | ½ | ● | ● | ○ |
| 13–19 / 113–119 | pointer, Tektronix, highlight colours | strips | ● | ½ | ½ | ○ | ● | ½ | ○ | ○ | ○ | ○ | ½ | ○ | ○ |
| 21 | kitty colour protocol | strips | ○ | ○ | ● | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 22 | pointer shape | strips | ½ | ○ | ● | ○ | ● | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 50 | xterm font; legacy iTerm2 / Konsole settings | strips | ● | ½ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ½ | ○ |
| 51 | Emacs (eat) shell commands | strips | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 52 | clipboard write / read | sends, strips | ½ (allowWindowOps) | ½ (pref) | ½ (write; read asks) | ½ (write only) | ½ (write; read by config) | ● | ● | ● | ½ | ½ (set-clipboard) | ½ (recent) | ● | ○ |
| 60 / 61 | xterm font query | strips | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 66 | kitty text sizing | strips | ○ | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 99 | kitty desktop notification | strips | ○ | ○ | ● | ○ | ½ (parsed, not shown yet) | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 133 | FinalTerm shell integration marks | sends, reads | ○ | ● | ● | ● | ● | ● | ○ | ● | ● | ○ | ○ | ½ | ○ |
| 633 | VS Code shell integration | strips | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ● | ○ | ○ | ○ | ○ |
| 777 | rxvt extensions (notify) | strips | ○ | ○ | ○ | ● | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 1337 | iTerm2 proprietary (File, SetUserVar, …) | sends File, strips | ○ | ● | ○ | ½ (File, SetUserVar) | ○ | ○ | ○ | ○ | ½ (File via image addon) | ○ | ○ | ½ (File) | ○ |
| 5522 | kitty clipboard protocol | strips | ○ | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 7501 | Program Status Protocol | sends, reads, strips | ○ | ○ | ○ | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |

Three tiers of adoption:

- **Universal**: 0/1/2 titles, 4 and 10–12 colours, 8 hyperlinks, 52
  clipboard writes. Safe to emit anywhere.
- **Wide but not universal**: 7 cwd, 133 shell marks, 9 notifications,
  9;4 progress. Emit behind terminal detection, or accept that some
  terminals ignore them (an unknown OSC is swallowed by every modern
  terminal, so the cost of a miss is nothing).
- **Single-vendor**: 21, 22, 66, 99, 633, 777, 1337, 5522, 7501, 9;9.
  Emit only when the terminal is known.

## Code by code

### OSC 0, 1, 2: window title and icon name

`ESC ] 0 ; text ST` sets both, 1 the icon name (tabs in most terminals),
2 the window title. Universal. WezTerm also takes the Sun forms `ESC ] L`
and `ESC ] l`. Overlaps: ConEmu 9;3 sets the tab title; iTerm2 6 sets
tab colour; Konsole 30 (legacy) set tab title.

mdsh today: does not set a title in the TUI (Bubble Tea's
`SetWindowTitle` is unused). Titles in saved output are stripped, in
prompts dropped, in the GUI stripped. A tty block's title set is kept by
its emulator (vt10x `Title()`, xterm.js `onTitleChange`) and shown
nowhere in the GUI and TUI; under the CLI it changes the user's
terminal's title; mdsh puts it back when the block ends (PR #350).

Should mdsh: **yes, cheaply.** Set the title to the file's name while
the TUI is up and restore it on exit with the xterm title stack
(`CSI 22;0 t` push, `CSI 23;0 t` pop) where supported. Optionally show
a tty block's own title in its card header, since both emulators
already have it. Stripping titles from output stays.

### OSC 4, 10–19, 104–119: colours

`OSC 4 ; n ; spec` sets palette entry n, `OSC 4 ; n ; ?` queries it. 10
foreground, 11 background, 12 cursor, 13/14 pointer, 15/16/18 Tektronix,
17/19 highlight. 104–119 reset the matching entry. Supported almost
everywhere for 4/10/11/12; 13–19 are xterm and Ghostty mostly.
Overlaps: kitty's OSC 21 covers the same colours with a push/pop stack;
kitty's older OSC 30001/30101 did the same and are deprecated.

mdsh today: **consumes the OSC 11 reply.** Lip Gloss queries the
background colour at startup to pick light or dark styles;
`tui/reply.go` filters a late reply so it does not arrive as keystrokes,
and `web/src/terminal.ts` serialises key sends so a GUI tty block's own
query cannot interleave with the cursor report. Output colour-setting
sequences are stripped everywhere. In the GUI and TUI a tty block's colour
changes stay in its emulator (vt10x answers 4/10/11 queries itself with
its own palette, which is not the user's: a program asking a TUI tty
block for its background gets vt10x's default). Under the CLI they
reach the user's terminal, and mdsh resets them when the block ends
(PR #350).

Should mdsh: **no new emission.** The reset after CLI tty blocks (OSC
104, 110, 111, 112 and the title, as tmux does for a pane) is done, in
PR #350. Repainting the user's palette is hostile. Looks (coffee, green tea, blueprint) stay inside mdsh's
drawing. One refinement worth a look: have the TUI pane answer a block's
OSC 11 query with the colour the TUI itself learned at startup, so a
program in a tty block picks the right light or dark theme.

### OSC 5, 6, 105: special colours and iTerm2 tab colour

xterm's OSC 5 sets the colours for bold, underline, blink and reverse;
105 resets; xterm's 6 toggles whether 5 is honoured. iTerm2 reuses 6 as
`OSC 6 ; 1 ; bg ; red ; brightness ; N` to colour the tab. Rare outside
xterm, iTerm2 and Ghostty.

mdsh today: stripped; nothing emitted. Should mdsh: **no.**

### OSC 7: current working directory

`OSC 7 ; file://host/path ST`. Terminals open new tabs in the same
directory and show the path in the tab. Terminal.app invented it;
iTerm2, kitty, WezTerm, Ghostty, foot, VTE and Konsole follow. Windows
Terminal added it after years of only 9;9 (verify). Not in xterm,
Alacritty or tmux. Overlaps: ConEmu 9;9, iTerm2 `1337;CurrentDir=`, VS
Code `633;P;Cwd=`.

mdsh today: not emitted, not consumed. mdsh learns each shell's cwd
through its own state fifo in the integration scripts, which is more
reliable than parsing OSC 7.

Should mdsh: **TUI only, low priority.** Emitting OSC 7 for the
document's folder would make "new tab" in the user's terminal land
beside the file. Consuming it from tty blocks has no use. Under ssh the
hostname matters; `url.URL{Scheme: "file", Host: hostname, Path: dir}`
is the usual form.

### OSC 8: hyperlinks

`OSC 8 ; params ; URI ST text OSC 8 ; ; ST`; `params` carries `id=` to
join a link split across lines. Universal in modern terminals; xterm
since recent patches, tmux since 3.4, Terminal.app never.

mdsh today: **emits, and blocks programs' own.** `weblink.OSC8` wraps
plain http, https and mailto addresses in TUI output in OSC 8; it leaves
text alone if it already contains any OSC. Program-written OSC 8 in
saved output is made visible by `termsafe.Styled`, by design: a link
whose target differs from its text is a phishing vector in a shared
file. In the GUI, xterm.js handles OSC 8 in History and tty terminals
and `links.ts` routes clicks through `App.OpenLink` with a scheme
allow-list; block output rendered as HTML has OSC stripped and URLs
found by regex. In the TUI, vt10x has no hyperlink support, so OSC 8
links in a tty block are plain text there.

Should mdsh: **partly.** The current policy is right for saved output.
A gap worth closing: programs that print OSC 8 links with a visible
target (`ls --hyperlink`, `gcc`, `rg`) are common. Rendering those as
"text (uri)" when text and URI differ, and as a link when they match,
keeps safety and recovers the common case. `termrec/text.go` drops OSC
8 from recordings' text transcripts while the cast player would honour
them; minor, acceptable.

### OSC 9: iTerm2 notifications versus ConEmu sub-commands

Two incompatible uses share the number.

- **iTerm2 form**: `OSC 9 ; message ST` shows a desktop notification.
  iTerm2, WezTerm, Ghostty; foot behind a setting.
- **ConEmu form**: `OSC 9 ; n ; args ST`. `9;1` sleep, `9;2` message
  box, `9;3` set tab title, `9;4` progress state, `9;5` wait for key,
  `9;6` execute GUI macro, `9;7` run external, `9;8` set env var, `9;9`
  report cwd, `9;10` xterm keyboard emulation, `9;11` sleep ms, `9;12`
  prompt start (like 133;A). Only `9;4` and `9;9` escaped ConEmu.
  Windows Terminal implements both; Ghostty implements `9;4` and treats
  any other sub-ID as an iTerm2 notification, which is the conflict made
  visible.

**OSC 9;4 progress**: `OSC 9 ; 4 ; state ; percent ST`. States 0 hide,
1 normal, 2 error, 3 indeterminate, 4 warning or paused. Windows
Terminal (taskbar), Ghostty (bar over the split, 15 second auto-clear so
senders should refresh), VS Code (terminal tab), ConEmu. iTerm2 has an
open request; Konsole a draft merge request; kitty declined. It is
becoming the de facto progress signal for build tools and package
managers.

mdsh today: not emitted; stripped in output; swallowed by both emulators
in tty blocks.

Should mdsh:

- **Emit 9;4 from the TUI and CLI next to 7501: yes.** `termstatus`
  already tracks the root record and knows the percentage for `working`
  and the `error` state. Emitting `9;4;1;N`, `9;4;2;N` and `9;4;0` from
  the same `Tracker.write` on terminals that understand it (Ghostty,
  Windows Terminal, VS Code; or unconditionally, since others ignore
  it) is a few lines. The 7501 design lists this under "Not now" for the
  consuming side; the emitting side is cheaper and reaches more
  terminals than 7501 does today.
- **Consume 9;4 from blocks: in the step 2 scanner** (merged in #349),
  as the 7501 design already says: map it to the block's root record.
- **Emit notifications (iTerm2 9, 777, 99): see "Notifications".**
- **9;1–9;3, 9;5–9;12: no.** `9;7` runs programs.

### OSC 21: kitty colour protocol

Sets, queries, pushes and pops any colour in one sequence. kitty and
Ghostty only. mdsh today: nothing. Should mdsh: **no.**

### OSC 22: pointer shape

kitty, Ghostty, foot, xterm partially. Should mdsh: **no.**

### OSC 50, 51, 60, 61: fonts and Emacs

xterm 50 sets the font, 60/61 query fonts; Konsole once used 50 for
profile changes; iTerm2 used 50 before 1337 and still parses it; 51 is
Emacs eat's command channel. Should mdsh: **no.**

### OSC 52: clipboard

`OSC 52 ; c ; base64 ST` writes the clipboard (`c` clipboard, `p`
primary, `s` selection); `OSC 52 ; c ; ? ST` reads it, which most
terminals refuse or gate behind a prompt. Writes work in nearly every
terminal, often behind a setting: xterm `allowWindowOps`, iTerm2 a
preference, tmux `set-clipboard on`; kitty and Ghostty allow writes by
default and ask for reads. Terminal.app and older VTE ignore it.
Overlaps: kitty's 5522; iTerm2 `1337;CopyToClipboard`.

mdsh today: **emits for copy.** `tui/clipboard.go` sends OSC 52, which
works over ssh, and also runs `pbcopy`, `wl-copy`, `xclip` or `xsel`
locally for terminals that drop it. Output and prose cannot carry OSC 52
(`termsafe`). A program's OSC 52 inside a tty block is swallowed by
vt10x in the TUI and ignored by xterm.js in the GUI (no clipboard addon
is loaded), so `vim`, `tmux` and `lazygit` copy into nothing there;
under the CLI it reaches the terminal and works.

Should mdsh: **close the tty-block gap in both UIs.** In the GUI,
register an OSC 52 handler on the tty and History terminals that
forwards writes to the Wails clipboard and ignores reads. In the TUI,
the pane sits between the pty and vt10x; intercept OSC 52 writes there
(the step 2 OSC scanner in `env` is the right place, since it already
has to see every pty's stream) and hand them to `setClipboard`. Also
wrap the TUI's own OSC 52 in tmux passthrough (`ESC P tmux ; ESC … ESC
\`) when `TMUX` is set; today copy over ssh into tmux fails silently.

### OSC 66: kitty text sizing

kitty 0.40+ only. Should mdsh: **no.**

### OSC 99: kitty desktop notifications

`OSC 99 ; i=id:d=0:p=title ; text ST` with keys for icon, urgency,
buttons and activation reports. kitty and foot implement it; Ghostty
parses it (PR 10467) but did not yet show notifications when checked.

### Notifications in general (OSC 9, 99, 777, 1337 RequestAttention)

No notification sequence is standard; three incompatible ones coexist.
mdsh today: `mdsh -i` sends one (package `termnotify`) when a `confirm`
or question waits, or a run of 10 seconds or more ends, once no key has
been pressed for 10 seconds; see the 7501 design's Notifications
section. Before that, the TUI had no way to reach a user whose terminal
is unfocused. A background job finishing still notifies nobody.

Should mdsh: **yes, in the TUI, by detection.** One helper
`notify(title, body)` that picks the sequence from the environment:
`KITTY_WINDOW_ID` or `TERM=xterm-kitty` → OSC 99; `TERM_PROGRAM` of
`iTerm.app`, `WezTerm` or `ghostty` → OSC 9; `foot` → 777 `notify`;
otherwise BEL. The detection already exists in `termimg`. Emit when a
confirm or ask is waiting and when a block that ran longer than about
10 seconds ends, only when the user has not pressed a key since. Wrap in
tmux passthrough when `TMUX` is set. The 7501 design lists "OS
notifications on blocked, as a setting" under "Not now"; this is that
item, and the 7501 `blocked` transition in `tui/status.go` is the place
to hook it. Consuming a block's notifications: forward OSC 9/99/777 from
tty blocks to native notifications in the GUI, through the same handler
as OSC 52; later.

### OSC 133: FinalTerm shell integration marks

`A` prompt start, `B` input start, `C` output start, `D ; status`
command end. Terminals jump between prompts, select output, decorate
exit status and redraw prompts on resize. iTerm2, kitty, WezTerm,
Ghostty, foot, Windows Terminal, VS Code; Konsole experimental; not
xterm, Alacritty or tmux. Overlaps: VS Code 633 A–E; ConEmu 9;12;
iTerm2 `1337;SetMark`.

mdsh today: **fully in use, and the one OSC mdsh depends on.**

- Emits: the bash, zsh and POSIX sh integrations print `A`, `B`, `C`
  and `D;status` with `mdsh` as the last field so mdsh can tell its own
  marks from the user's (`env/integration.go`).
- Consumes: `env/marks.go` scans the pty stream for mdsh's marks to
  split prompt, input and output and read the exit status, holding back
  a mark split across reads and capping unterminated ones at 64 bytes.
  Other programs' marks pass through as text.
- Re-emits: `interactive_unix.go` writes a clean mark (without the
  `mdsh` field) to the view terminal.
- GUI: `history.ts` registers an xterm.js handler for 133 to track where
  the prompt is, which drives "say above the prompt" placement.

Should mdsh: **nothing more.** The 7501 design's step 2 widens this
scanner into a general OSC scanner, which is the right home for every
"consume" item in this document.

### OSC 633: VS Code shell integration

VS Code's own copy of 133 with `E ; commandline` and `P ; key=value`
properties. VS Code injects it into shells it starts; mdsh's hidden
shells are not those. Should mdsh: **no.**

### OSC 777: rxvt extensions

`OSC 777 ; notify ; title ; body ST`. WezTerm and foot. Covered under
Notifications.

### OSC 1337: iTerm2 proprietary

`OSC 1337 ; Key=value ST`. Sub-commands: `File=` (inline images and
downloads), `SetUserVar=`, `CurrentDir=`, `RemoteHost=`,
`ShellIntegrationVersion=`, `SetMark`, `StealFocus`, `ClearScrollback`,
`SetBadgeFormat=`, `CopyToClipboard=`/`EndCopy`, `RequestAttention=`,
`ReportCellSize`, `SetKeyLabel=`, `PushKeyLabels`/`PopKeyLabels`,
`Custom=id=`, `ReportVariable=`, `SetColors=`, `SetProfile=`,
`HighlightCursorLine=`, `UnicodeVersion=`, `AddAnnotation=`. iTerm2
implements all; WezTerm `File` and `SetUserVar`; Konsole, mintty and VS
Code's image addon `File`. Overlaps: `CurrentDir` with 7,
`CopyToClipboard` with 52, `SetMark` with 133, `RequestAttention` with
9/99/777, `SetColors` with 4/10–19.

`CopyToClipboard` differs from OSC 52 in shape, not effect: OSC 52
carries an invisible base64 payload, while `OSC 1337 ; CopyToClipboard=c
ST` says "copy what I print next", the program prints ordinary visible
text, and `OSC 1337 ; EndCopy ST` stops. It brackets a visible region.

mdsh today: **emits `File=inline=1`** for images in the TUI on iTerm2
and WezTerm (`termimg.iterm2`), with kitty graphics (APC) on kitty and
Ghostty and sixel (DCS) elsewhere. Nothing else; stripped in output.

Should mdsh: **one thing: copyable regions.** Read `CopyToClipboard` /
`EndCopy` from block output and tty streams, and OSC 52 writes too, and
instead of writing the clipboard silently, mark the region as copyable
and show a copy button beside it (in the card, in the cast player while
the region is on screen, in the TUI as a key on the block). That keeps
the no-side-effects rule for saved output and gives any program a way to
offer "copy this token". No other 1337 sub-command is worth a code path.

### OSC 5522: kitty clipboard protocol

Chunked, MIME-typed clipboard with permission prompts. kitty only.
Should mdsh: **no.**

### OSC 7501: Program Status Protocol

`OSC 7501 ; key=value:key=value ST`, Mitchell Hashimoto's spec, revision
0.4. Keys: `state` (idle, working, done, blocked, error, clear), `id`,
`kind` for blocked (permission, question, auth), `progress` 0–100,
`app`, `title`, `msg` (base64). `OSC 7501 ; ?` queries support. Ghostty
consumes it; nothing else yet. Overlaps: 9;4 for progress; 133 D for
exit status; `blocked` with `mdsh ctl ask` and `confirm`.

mdsh today: **steps 1 and 2 of 5 built** (PRs #348 and #349): `mdsh
doc.md` and `mdsh -i` report their own state, and `env` scans every pty
mdsh owns for 7501 reports, keeps the records and answers the feature
query. Steps 3 to 5 (TUI chips and forwarding, GUI chips, `ctl status
--state`) are planned in
[`docs/program-status/design.md`](../program-status/design.md). This
document adds to that plan rather than duplicating it: the emit side
gains 9;4 and notifications, and the step 2 scanner is where 9;4, OSC 52
and the copyable-region sequences get consumed.

### Not OSC but adjacent

- **Kitty graphics** `ESC _ G … ESC \` (APC) and **sixel** `ESC P q … ESC
  \` (DCS): mdsh emits both for images. `termsafe.seqLen` and `termrec`
  skip APC, DCS, PM and SOS strings, so they are stripped from output as
  OSCs are.
- **Synchronized output** `CSI ? 2026 h/l`: Bubble Tea does not use it;
  worth considering for flicker, unrelated to OSC.
- **tmux passthrough** `ESC P tmux ; ESC <seq> ESC \`: needed for every
  vendor OSC when `TMUX` is set, including OSC 52 and 7501 today.

## Gaps found while reading the code

- `tui/clipboard.go` does not wrap OSC 52 in tmux passthrough; the
  local tool fallback hides this on a laptop, and copy over ssh into
  tmux fails silently.
- Programs' OSC 52 writes in tty blocks go nowhere in the GUI and TUI.
- vt10x answers a tty block's OSC 10/11 query with its own default
  palette, not the terminal's, so a program in a TUI tty block may pick
  the wrong light or dark theme.
- `termrec/text.go` drops OSC 8, so a recording's text transcript loses
  links that the cast player shows. Cosmetic.

## Ranked recommendation

1. **Emit OSC 9;4 progress and OSC 0/2 title from the TUI and CLI**,
   from the same `termstatus.Tracker` write that emits 7501. Smallest
   code, widest reach. Title set on open, restored on exit.
2. **TUI notifications (OSC 9 / 99 / 777 by terminal detection, with
   tmux passthrough)** on `blocked` and on a long block's end. Reuses
   `termimg`'s detection and the 7501 state transitions.
3. **Forward OSC 52 writes from tty blocks and History to the
   clipboard, in the GUI (xterm.js handler → Wails) and the TUI (pty
   scanner → `setClipboard`); wrap the TUI's own OSC 52 for tmux.**
4. **Render program OSC 8 links in saved output as "text (uri)"**
   instead of making the raw sequence visible.
5. **Copyable regions**: `1337;CopyToClipboard`/`EndCopy` and OSC 52
   payloads become a copy button in the card, the cast player and the
   TUI, never a silent clipboard write.
6. **Consume 9;4 (and 7501) from tty blocks** in the 7501 step 2
   scanner, for the block's card. Depends on that step.
7. **Do not add**: OSC 7 emission (only if asked), 21, 22, 50, 66, 633,
   5522, 1337 beyond `File`, ConEmu 9;1–9;3 and 9;5–9;12.
