package main

import (
	"reflect"
	"testing"
)

func TestRestartArgs(t *testing.T) {
	got := restartArgs([]string{"-headless", "-no-browser", "-updated-from", "v0.1.0", "--config=/x.json", "-updated-from=v0.2.0"}, "v0.5.0")
	want := []string{"-headless", "--config=/x.json", "-no-browser", "-updated-from", "v0.5.0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
