package main

import "testing"

func TestEnvFirstPrefersTheEarlierNameAndSkipsEmptyValues(t *testing.T) {
	t.Setenv("ACTON_X", "")
	t.Setenv("X", "legacy")
	if got := envFirst("ACTON_X", "X"); got != "legacy" {
		t.Fatalf("an empty canonical name must fall through to the legacy one, got %q", got)
	}
	t.Setenv("ACTON_X", "canonical")
	if got := envFirst("ACTON_X", "X"); got != "canonical" {
		t.Fatalf("the canonical name must win, got %q", got)
	}
	if got := envFirst("ACTON_NOT_SET_ANYWHERE"); got != "" {
		t.Fatalf("nothing set must give the empty string, got %q", got)
	}
}
