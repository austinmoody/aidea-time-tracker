// Build with: go build -o test_ollama test_ollama.go ollama_api.go
//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_ollama.go \"Your task description here\"")
		os.Exit(1)
	}

	description := os.Args[1]
	TestCategorize(description)
}
