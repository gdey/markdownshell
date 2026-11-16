package main

import "regexp"

// atStamp matches the at= a run stamps on its output.
var atStamp = regexp.MustCompile(` at="?\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ"?`)

// noAt drops the at= stamps from s, for tests that check output whose
// time they can't know.
func noAt[T string | []byte](s T) string { return atStamp.ReplaceAllString(string(s), "") }
