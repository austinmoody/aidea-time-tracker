package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// TODO - read model, system prompt, etc... from env
// TODO - why isn't ALL of the response coming back?

// OllamaRequest defines the structure for Ollama API requests
type OllamaRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	System      string  `json:"system"`
	Stream      bool    `json:"stream"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// OllamaResponse defines the structure for Ollama API responses
type OllamaResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Main function to test Ollama integration
func callOllama() {
	// Configure the Ollama API endpoint
	ollamaURL := "http://localhost:11434/api/generate"

	// Set the model to use
	modelName := "gemma3"

	// User input - this would come from your application
	userInput := "Bi-weekly security scan"

	// Read system prompt from file
	systemPrompt, err := readSystemPrompt()
	if err != nil {
		fmt.Printf("Error reading system prompt: %v\n", err)
		return
	}

	// Configure the request to Ollama
	request := OllamaRequest{
		Model:       modelName,
		Prompt:      userInput,
		System:      systemPrompt,
		Stream:      false,
		MaxTokens:   2000,
		Temperature: 0.7,
	}

	// Convert the request to JSON
	requestData, err := json.Marshal(request)
	if err != nil {
		fmt.Printf("Error marshalling request: %v\n", err)
		return
	}

	// Create a new HTTP request
	req, err := http.NewRequest("POST", ollamaURL, bytes.NewBuffer(requestData))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending request to Ollama: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Ollama API returned error: %s\n", resp.Status)
		responseBody, _ := io.ReadAll(resp.Body)
		fmt.Printf("Response: %s\n", string(responseBody))
		return
	}

	// Parse and print the response
	if !request.Stream {
		// For non-streaming responses
		var response OllamaResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			fmt.Printf("Error decoding response: %v\n", err)
			return
		}

		// Print the model's response
		fmt.Printf("\n--- Ollama Response (Model: %s) ---\n", response.Model)
		fmt.Println(response.Response)
		fmt.Println("--- End Response ---\n")
	} else {
		// For streaming responses, you would read and process the stream differently
		fmt.Println("Streaming responses not implemented in this example")
	}
}

// readSystemPrompt reads the system prompt from system_prompt.txt
func readSystemPrompt() (string, error) {
	// Get the directory of the current file
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("error getting executable path: %w", err)
	}

	execDir := filepath.Dir(execPath)
	promptFilePath := filepath.Join(execDir, "system_prompt.txt")

	// For development, if we're running with 'go run', also check the current directory
	if _, err := os.Stat(promptFilePath); os.IsNotExist(err) {
		currentDir, _ := os.Getwd()
		promptFilePath = filepath.Join(currentDir, "system_prompt.txt")
	}

	// Read the system prompt file
	promptData, err := os.ReadFile(promptFilePath)
	if err != nil {
		return "", fmt.Errorf("error reading system prompt file: %w", err)
	}

	return string(promptData), nil
}

// This allows the file to be run directly for testing
func main() {
	callOllama()
}
