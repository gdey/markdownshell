// Package greet says hello.
package greet

import "fmt"

// Greeter greets people by name.
type Greeter struct {
	Greeting string
}

// Greet returns the greeting for name.
func (g Greeter) Greet(name string) string {
	return fmt.Sprintf("%s, %s", g.Greeting, name)
}
