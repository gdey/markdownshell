#!/usr/bin/env mdsh
# Including files

A block marked `<include` shows lines from another file, so this document
can talk about [include/greet.sh](include/greet.sh) without a copy of it
that goes stale. mdsh writes the lines into the block, so the file reads
well anywhere markdown does. The path is relative to this file. Without
`from=` or `to=`, the whole file is included:

```sh {<include file=include/greet.sh}
#!/bin/sh
# greet.sh prints a greeting for each name it is given.

greet() {
	printf 'hello, %s\n' "$1"
}

for name in "$@"; do
	greet "$name"
done
```

An include block never runs and has no name, even when its language is
`sh`. `from=` and `to=` pick lines, counted from 1 and inclusive.
`number_on` asks for line numbers when the document is shown, so the
excerpt still lines up with the source. Here is just the function:

```sh {<include file=include/greet.sh from=4 to=6 number_on}
greet() {
	printf 'hello, %s\n' "$1"
}
```

The fence's language is the language the lines are shown in:

```bash {<include file=include/greet.sh from=8}
for name in "$@"; do
	greet "$name"
done
```

Includes sit alongside ordinary blocks. This one runs the script shown
above:

```sh {#run}
sh include/greet.sh world mdsh
```

```output {for=run exit=0}
hello, world
hello, mdsh
```

Run `mdsh include.md` to print this document with line numbers where
they're asked for, or `mdsh -w include.md` to refresh the lines stored in
each block.
