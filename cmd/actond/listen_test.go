package main

import (
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestExitOnListenErrorIgnoresServerClosed(t *testing.T) {
	called := 0
	exitOnListenError(http.ErrServerClosed, func(int) { called++ })
	if called != 0 {
		t.Fatal("shutdown should not exit")
	}
}

func TestExitOnListenErrorExitsOnBindFailure(t *testing.T) {
	called := 0
	exitOnListenError(&net.OpError{Op: "listen", Err: errors.New("address already in use")}, func(code int) {
		called = code
	})
	if called != 1 {
		t.Fatalf("expected exit 1, got %d", called)
	}
}
