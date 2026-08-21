//go:build !cgo || !ORT

package main

import "fmt"

func main() {
	fmt.Println("embeddingd requires CGO_ENABLED=1 and -tags ORT")
}
