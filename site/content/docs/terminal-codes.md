---
title: Terminal escape codes
description: The OSC sequences terminals speak, which ones mdsh sends, reads and strips, and how widely each is supported.
weight: 56
---

Terminals take orders from the programs in them through escape
sequences. The ones that aren't cursor movement or colours are **OSC**
(Operating System Command) sequences: `ESC ] number ; text ST`, ended by
`ESC \` or a BEL (`\a`). They set the title, the clipboard and the
colours, make links, send notifications, mark where a prompt starts, and
draw images. There is no registry: xterm defined the numbers under 120,
vendors picked large ones (133, 633, 777, 1337, 7501) to stay out of
each other's way, and two of them picked 9.

mdsh meets them in four places, and treats each differently.

| Where | What mdsh does |
| --- | --- |
| **Block output** saved in the file, and the text of prose and prompts | Colours (SGR) stay. Every OSC is taken out: shown as visible text in output, dropped from prompts. A file you were sent can't set your clipboard, your title or a link whose target isn't its text. |
| **tty blocks and the History pane** in the app and in `mdsh -i`, drawn by a terminal emulator inside mdsh (xterm.js, or vt10x in `mdsh -i`) | The emulator handles what it knows and drops the rest. xterm.js knows titles, links and colours; vt10x knows titles and the foreground and background colours. Neither reaches your real terminal or clipboard unless mdsh forwards it. |
| **tty blocks under plain `mdsh doc.md`**, which hands the block your terminal | Everything the program writes reaches your terminal: its title sets, colours, links, clipboard writes and notifications all take effect, and mdsh records them too. |
| **mdsh itself**, as a program in your terminal | Sends OSC 133 marks in the shells it runs, OSC 52 to copy, OSC 1337 images on iTerm2 and WezTerm, and OSC 7501 to say what it is doing. Reads the reply to the OSC 11 colour query. |

## Support by terminal

● full · ½ partial, or behind a setting · ○ none · ? not verified against
current docs. The **mdsh** column says what mdsh does with the code as
of this page: *sends* it to your terminal, *reads* it from the programs
in blocks, *strips* it from output, or nothing (—). Ghostty, WezTerm,
Windows Terminal and the OSC 7501 spec were checked against their
current documentation; the rest is from xterm's `ctlseqs`, iTerm2's and
kitty's docs and memory.

| OSC | What it does | mdsh | xterm | iTerm2 | kitty | WezTerm | Ghostty | foot | Alacritty | Windows Terminal | VS Code | tmux | GNOME (VTE) | Konsole | Terminal.app |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 0, 1, 2 | window title, icon name | strips | ● | ● | ● | ● | ● | ● | ● | ● | ● | ● | ● | ● | ● |
| 4, 104 | palette colours: set, query, reset | strips | ● | ● | ● | ● | ● | ● | ● | ● | ● | ½ | ● | ● | ○ |
| 5, 105 | bold, underline and blink colours | strips | ● | ○ | ○ | ○ | ● | ½ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 6 | xterm: special-colour switch; iTerm2: tab colour | strips | ½ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 7 | current directory, as a `file://` URL | strips | ○ | ● | ● | ● | ● | ● | ○ | ½ | ○ | ○ | ● | ● | ● |
| 8 | hyperlinks | sends, strips | ½ | ● | ● | ● | ● | ● | ● | ● | ● | ● (3.4+) | ● | ● | ○ |
| 9 | iTerm2: desktop notification | strips | ○ | ● | ○ | ● | ● | ½ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 9;4 | ConEmu: progress state | strips | ○ | ½ ? | ○ | ○ | ● | ○ | ○ | ● | ● | ○ | ○ | ½ (draft) | ○ |
| 9;9 | ConEmu: current directory | strips | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ● | ○ | ○ | ○ | ○ | ○ |
| 10, 11, 12 | text, background and cursor colour, with `?` to query | reads the 11 reply, strips | ● | ● | ● | ● | ● | ● | ● | ● | ● | ½ | ● | ● | ○ |
| 13–19, 113–119 | pointer, Tektronix and highlight colours | strips | ● | ½ | ½ | ○ | ● | ½ | ○ | ○ | ○ | ○ | ½ | ○ | ○ |
| 21 | kitty colour protocol | strips | ○ | ○ | ● | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 22 | pointer shape | strips | ½ | ○ | ● | ○ | ● | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 50 | xterm: font; old iTerm2 and Konsole settings | strips | ● | ½ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ½ | ○ |
| 52 | clipboard: write, and read where allowed | sends, reads | ½ | ½ | ½ | ½ (write) | ½ (write) | ● | ● | ● | ½ | ½ | ½ | ● | ○ |
| 66 | kitty text sizing | strips | ○ | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 99 | kitty desktop notification | strips | ○ | ○ | ● | ○ | ½ (parsed) | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 133 | shell integration marks: prompt, input, output, exit status | sends, reads | ○ | ● | ● | ● | ● | ● | ○ | ● | ● | ○ | ○ | ½ | ○ |
| 633 | VS Code shell integration | strips | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ● | ○ | ○ | ○ | ○ |
| 777 | rxvt extensions, `notify` | strips | ○ | ○ | ○ | ● | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 1337 | iTerm2: images, user variables, marks and more | sends `File`, reads `CopyToClipboard` | ○ | ● | ○ | ½ | ○ | ○ | ○ | ○ | ½ (images) | ○ | ○ | ½ (images) | ○ |
| 5522 | kitty clipboard protocol | strips | ○ | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |
| 7501 | Program Status Protocol | sends, reads, strips | ○ | ○ | ○ | ○ | ● | ○ | ○ | ○ | ○ | ○ | ○ | ○ | ○ |

Three tiers fall out of the table. Titles, the common colours, links and
clipboard writes work nearly everywhere. The directory, shell marks,
notifications and progress work in the modern terminals but not in
xterm, Alacritty or tmux. Everything else belongs to one terminal. A
modern terminal swallows an OSC it doesn't know, so sending one costs
nothing where it isn't understood.

## Code by code

### 0, 1, 2: title

Universal. mdsh doesn't set your terminal's title yet. A title a program
sets in a tty block stays inside the block's emulator in the app and in
`mdsh -i`; under plain `mdsh doc.md` it reaches your terminal, and mdsh
puts the title back when the block ends. Overlaps ConEmu `9;3`
and iTerm2's `6`.

### 4, 10–19, 104–119: colours

`OSC 4 ; n ; rgb:rr/gg/bb` sets palette entry n and `OSC 4 ; n ; ?`
asks for it; 10, 11 and 12 are the text, background and cursor colours;
104 to 119 reset. 4 and 10–12 are everywhere, 13–19 are xterm and
Ghostty. mdsh asks for the background colour (OSC 11) when `mdsh -i`
starts, to pick light or dark styles, and filters the reply out of your
typing when it comes late. It never sets your colours, and output can't
either; a program in a tty block under plain `mdsh doc.md` can, and
mdsh resets them when the block ends. Overlaps kitty's 21.

### 7: current directory

`OSC 7 ; file://host/path`. Terminals open new tabs in that directory
and show it in the tab. Terminal.app invented it; iTerm2, kitty,
WezTerm, Ghostty, foot, GNOME Terminal and Konsole follow; xterm,
Alacritty and tmux don't. mdsh learns each shell's directory from its
own hooks, not from this, and doesn't send it. Overlaps ConEmu `9;9`,
iTerm2 `1337;CurrentDir` and VS Code `633;P;Cwd`.

### 8: hyperlinks

`OSC 8 ; ; https://… ST text OSC 8 ; ; ST`. Everywhere except
Terminal.app; tmux since 3.4. mdsh **sends** them in `mdsh -i`: web
addresses in output become links. It **strips** the ones programs
write into saved output, because a link whose text says one thing and
whose target says another is how a shared file misleads you; the
address shows instead. In the app, the History pane and tty blocks
follow OSC 8 links, through the same `http:`, `https:` and `mailto:`
check as typed addresses.

### 9: notifications, or ConEmu

Two meanings share the number. iTerm2's `OSC 9 ; message` shows a
desktop notification, in iTerm2, WezTerm and Ghostty. ConEmu's
`OSC 9 ; n ; …` is a family: `9;4 ; state ; percent` reports progress
(0 clear, 1 normal, 2 error, 3 indeterminate, 4 paused), which Windows
Terminal, Ghostty, VS Code and ConEmu show in the tab or the taskbar;
`9;9` sends the directory on Windows; `9;1` to `9;12` otherwise stay
in ConEmu, `9;7` runs programs and no other terminal should ever honour
it. Ghostty takes an unknown sub-command as a notification, which is
the collision made visible. mdsh sends neither and strips both.

### 21, 22, 66, 5522: kitty's

The colour protocol (21, also Ghostty), pointer shape (22, also Ghostty
and foot), text sizing (66) and the chunked, typed clipboard (5522).
mdsh does nothing with them.

### 50, 51, 60, 61: fonts and Emacs

xterm's font set and query, Konsole's old profile switch, iTerm2's
pre-1337 extensions and Emacs eat's command channel. mdsh strips them.

### 52: clipboard

`OSC 52 ; c ; base64` writes the clipboard, over SSH too; `?` reads it,
which most terminals refuse or ask about. Writes work almost everywhere,
often behind a setting: xterm's `allowWindowOps`, an iTerm2 preference,
tmux's `set-clipboard`. Terminal.app ignores it. mdsh **sends** it when
you copy a block in `mdsh -i`, and runs `pbcopy`, `wl-copy`, `xclip` or
`xsel` as well when it is local, for terminals that drop it. In a
block's output mdsh **reads** it but never writes your clipboard: the
text gets a copy button, or `c` in `mdsh -i` (see
[Text to copy]({{< relref "writing-documents#text-to-copy" >}})). A program that writes OSC 52 inside a tty block is
heard by the block's emulator and goes no further, in the app and in
`mdsh -i`; under plain `mdsh doc.md` it reaches your terminal. Overlaps kitty's 5522 and iTerm2's
`1337;CopyToClipboard`.

### 99 and 777: more notifications

kitty's `OSC 99`, with ids, urgency and buttons, is in kitty and foot,
and parsed but not yet shown by Ghostty. rxvt's `OSC 777 ; notify ;
title ; body` is in WezTerm and foot. No notification sequence is
standard; mdsh sends none.

### 133: shell integration marks

`A` prompt start, `B` input start, `C` output start, `D ; status`
command end, from FinalTerm. Terminals jump between prompts, select a
command's output and colour the exit status with them: iTerm2, kitty,
WezTerm, Ghostty, foot, Windows Terminal, VS Code. This is the one OSC
mdsh depends on. The bash, zsh and sh hooks it installs **send** marks
tagged `mdsh`, and mdsh **reads** them from the shell's stream to tell
prompt from input from output and to learn the exit status of what you
typed in the [History pane]({{< relref "interactive" >}}). Your own
prompt's marks pass through untouched. Overlaps VS Code's 633 and
ConEmu's `9;12`.

### 633: VS Code

VS Code's own copy of 133 with the command line and properties added,
for shells VS Code starts. mdsh's shells aren't those; nothing to do.

### 1337: iTerm2

`OSC 1337 ; Key=value`, a family: `File=` draws an image or downloads a
file, `SetUserVar=`, `CurrentDir=`, `SetMark`, `SetBadgeFormat=`,
`CopyToClipboard`/`EndCopy`, `RequestAttention=` and more. iTerm2 has
all of it; WezTerm takes `File` and `SetUserVar`; Konsole, mintty and
VS Code draw `File` images. mdsh **sends** `File` for images under
iTerm2 and WezTerm (kitty's graphics protocol on kitty and Ghostty,
sixel elsewhere; neither is an OSC). In output it **reads**
`CopyToClipboard` and `EndCopy` and puts a copy button by the text
between them, and strips the rest.

### 7501: program status

`OSC 7501 ; state=working:progress=40:msg=…`, the
[Program Status Protocol](https://mitchellh.com/writing/program-status-osc7501):
a program tells the terminal it is working, blocked on you (a
permission, a question, a credential), done or failed, with a message,
and the terminal lists that in a tab or an inbox. Ghostty shows it;
nothing else does yet. mdsh **sends** it for its own runs, in
`mdsh doc.md` and `mdsh -i` (see [Terminal UI]({{< relref "interactive" >}})),
and strips it from output. It also **reads** it from the programs in
tty blocks and the History pane and keeps their records; showing a
block's state on its card is the planned next step.
Overlaps `9;4` for the percentage and `133 D` for the exit status.

## What's next

The ranked list, from the survey behind this page
([docs/osc-codes/design.md](https://github.com/gdey/markdownshell/blob/main/docs/osc-codes/design.md)
in the repository):

1. Send `9;4` progress and a title from `mdsh -i`, next to the 7501
   reports it already sends.
2. Desktop notifications from `mdsh -i` when a block waits on you or a
   long one ends, by the terminal's own sequence (9, 99 or 777), with
   tmux passthrough.
3. Forward OSC 52 writes out of tty blocks and the History pane to the
   clipboard, in the app and in `mdsh -i`, and wrap mdsh's own copy for
   tmux.
4. Show a program's OSC 8 link in saved output as "text (address)"
   instead of the raw sequence.
5. Read `9;4` and 7501 from the programs in tty blocks, for the block's
   card.
