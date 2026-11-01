package main

import "os"

// envFirst returns the value of the first environment variable in names that is set and non-empty.
func envFirst(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}
