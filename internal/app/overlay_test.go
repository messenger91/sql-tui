package app

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestOverlayReplacesCenteredSpan(t *testing.T) {
	background := "abcdef\nghijkl"
	got := ansi.Strip(overlay(background, "XY", 6))
	want := "abXYef\nghijkl"
	if got != want {
		t.Fatalf("overlay = %q, want %q", got, want)
	}
}

func TestOverlayCentersVertically(t *testing.T) {
	background := "aaaa\nbbbb\ncccc"
	got := ansi.Strip(overlay(background, "XX", 4))
	want := "aaaa\nbXXb\ncccc"
	if got != want {
		t.Fatalf("overlay = %q, want %q", got, want)
	}
}

func TestSpliceLinePadsShortBackground(t *testing.T) {
	got := ansi.Strip(spliceLine("ab", "XY", 4))
	if got != "ab  XY" {
		t.Fatalf("splice = %q", got)
	}
}

func TestOverlayPreservesStyledBackground(t *testing.T) {
	background := "\x1b[31mabcdef\x1b[0m"
	got := ansi.Strip(overlay(background, "XY", 6))
	if got != "abXYef" {
		t.Fatalf("overlay = %q", got)
	}
}

func TestOverlayClampsWhenForegroundIsWider(t *testing.T) {
	got := ansi.Strip(overlay("ab", "LONGER", 2))
	if got == "" {
		t.Fatal("overlay returned empty output")
	}
}
