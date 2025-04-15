package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type OllamaRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	System      string  `json:"system"`
	Stream      bool    `json:"stream"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type OllamaResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

type CategoryResponse struct {
	Task       string `json:"task"`
	Jira       string `json:"jira"`
	Time       string `json:"time"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

func categorizeDescription(description string) (*CategoryResponse, error) {
	ollamaURL := "http://localhost:11434/api/generate"
	modelName := "gemma3"

	systemPrompt, err := readSystemPrompt()
	if err != nil {
		return nil, fmt.Errorf("error reading system prompt: %w", err)
	}

	request := OllamaRequest{
		Model:       modelName,
		Prompt:      description,
		System:      systemPrompt,
		Stream:      false,
		MaxTokens:   2000,
		Temperature: 0.7,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request: %w", err)
	}

	req, err := http.NewRequest("POST", ollamaURL, bytes.NewBuffer(requestData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama API returned error: %s - %s", resp.Status, string(responseBody))
	}

	// Read the complete response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	// Log the raw response for debugging
	fmt.Println("Raw Ollama response:", string(responseBody))

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(responseBody, &ollamaResp); err != nil {
		return nil, fmt.Errorf("error decoding Ollama response: %w", err)
	}

	// Log the parsed response for debugging
	fmt.Println("Parsed Ollama response text:", ollamaResp.Response)

	// Try to validate if the response is valid JSON
	if !json.Valid([]byte(ollamaResp.Response)) {
		// If not valid JSON, try to extract JSON content
		// Sometimes LLMs might wrap the JSON in markdown code blocks or add text before/after
		jsonStart := strings.Index(ollamaResp.Response, "{")
		jsonEnd := strings.LastIndex(ollamaResp.Response, "}")

		if jsonStart >= 0 && jsonEnd > jsonStart {
			extractedJSON := ollamaResp.Response[jsonStart : jsonEnd+1]
			fmt.Println("Extracted JSON:", extractedJSON)

			// Check if extracted content is valid JSON
			if json.Valid([]byte(extractedJSON)) {
				ollamaResp.Response = extractedJSON
			} else {
				return nil, fmt.Errorf("could not extract valid JSON from response")
			}
		} else {
			return nil, fmt.Errorf("response doesn't contain valid JSON: %s", ollamaResp.Response)
		}
	}

	var categoryResp CategoryResponse
	if err := json.Unmarshal([]byte(ollamaResp.Response), &categoryResp); err != nil {
		return nil, fmt.Errorf("error parsing category JSON: %w, raw response: %s", err, ollamaResp.Response)
	}

	return &categoryResp, nil
}

func readSystemPrompt() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("error getting executable path: %w", err)
	}

	execDir := filepath.Dir(execPath)
	promptFilePath := filepath.Join(execDir, "system_prompt.txt")

	if _, err := os.Stat(promptFilePath); os.IsNotExist(err) {
		currentDir, _ := os.Getwd()
		promptFilePath = filepath.Join(currentDir, "system_prompt.txt")
	}

	promptData, err := os.ReadFile(promptFilePath)
	if err != nil {
		return "", fmt.Errorf("error reading system prompt file: %w", err)
	}

	return string(promptData), nil
}

// TestCategorize is a utility function to test the Ollama categorization
func TestCategorize(description string) {
	fmt.Println("Testing categorization with description:", description)

	result, err := categorizeDescription(description)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("\nSuccessfully categorized:")
	fmt.Printf("Task: %s\n", result.Task)
	fmt.Printf("Jira: %s\n", result.Jira)
	fmt.Printf("Time: %s\n", result.Time)
	fmt.Printf("Confidence: %s\n", result.Confidence)
	fmt.Printf("Reason: %s\n", result.Reason)
}
