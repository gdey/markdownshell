#!/bin/sh
# A tiny interactive prompt: name a fruit, or q to quit.
while printf 'fruit> ' && read -r f; do
	case $f in
	q) echo 'bye'; break ;;
	'') ;;
	*) echo "one $f, coming up" ;;
	esac
done
