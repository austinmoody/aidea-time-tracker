// Build with: go build -o test_ollama test_ollama.go ollama_api.go main.go
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
		fmt.Println("       This will test the categorization using rules defined in categorization_rules.csv")
		os.Exit(1)
	}

	description := os.Args[1]
	TestCategorize(description)
}
