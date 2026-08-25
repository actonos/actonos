package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
)

// exitOnListenError terminates the daemon when the HTTP listener fails to bind.
func exitOnListenError(err error, exit func(int)) {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return
	}
	slog.Error("http server error", "error", err)
	if exit == nil {
		os.Exit(1)
	}
	exit(1)
}
