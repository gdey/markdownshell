//go:build !darwin

package main

import (
	"sync"
	"testing"
	"time"
)

// fakeClipboard is a text clipboard for tests.
type fakeClipboard struct {
	mu   sync.Mutex
	text string
}

func (f *fakeClipboard) SetText(s string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.text = s
	return true
}

func (f *fakeClipboard) Text() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.text, true
}

// TestCopySecret checks that a copied secret comes off the clipboard
// after a while, and that something copied since is left alone.
func TestCopySecret(t *testing.T) {
	cb := &fakeClipboard{}
	if err := copySecret("hunter2", cb, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got, _ := cb.Text(); got != "hunter2" {
		t.Fatalf("clipboard = %q right after copying", got)
	}
	time.Sleep(100 * time.Millisecond)
	if got, _ := cb.Text(); got != "" {
		t.Errorf("clipboard = %q after the secret's time, want it cleared", got)
	}

	copySecret("hunter2", cb, 20*time.Millisecond)
	cb.SetText("something else")
	time.Sleep(100 * time.Millisecond)
	if got, _ := cb.Text(); got != "something else" {
		t.Errorf("clipboard = %q, want what was copied after the secret kept", got)
	}
}
