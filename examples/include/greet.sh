#!/bin/sh
# greet.sh prints a greeting for each name it is given.

greet() {
	printf 'hello, %s\n' "$1"
}

for name in "$@"; do
	greet "$name"
done
