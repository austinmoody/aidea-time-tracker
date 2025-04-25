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
	Timespan   string `json:"timespan"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

// categorizeDescription is the original function, kept for backward compatibility
func categorizeDescription(description string) (*CategoryResponse, error) {
	return categorizeWithRules(description, "")
}

// categorizeWithRules calls Ollama with both a description and rules as context
func categorizeWithRules(description string, rules string) (*CategoryResponse, error) {
	ollamaURL := "http://localhost:11434/api/generate"
	//modelName := "aidea-categorizer"
	modelName := "gemma3"

	logger.Printf("Calling Ollama API with model: %s", modelName)

	// Build the system prompt with rules
	systemPrompt := buildSystemPromptWithRules(rules)
	logger.Printf("Built system prompt with rules (%d bytes)", len(systemPrompt))

	// Include the description to categorize
	prompt := description

	request := OllamaRequest{
		Model:       modelName,
		Prompt:      prompt,
		System:      systemPrompt,
		Stream:      false,
		MaxTokens:   2000,
		Temperature: 0.7,
	}

	logger.Printf("Using temperature: %.1f, max tokens: %d", request.Temperature, request.MaxTokens)

	requestData, err := json.Marshal(request)
	if err != nil {
		logger.Printf("ERROR: Failed to marshal request: %v", err)
		return nil, fmt.Errorf("error marshalling request: %w", err)
	}

	logger.Printf("Sending request to Ollama API: %s", ollamaURL)
	req, err := http.NewRequest("POST", ollamaURL, bytes.NewBuffer(requestData))
	if err != nil {
		logger.Printf("ERROR: Failed to create request: %v", err)
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Printf("ERROR: Failed to send request to Ollama: %v", err)
		return nil, fmt.Errorf("error sending request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	logger.Printf("Received response from Ollama: status=%s", resp.Status)
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		logger.Printf("ERROR: Ollama API returned error status: %s - %s", resp.Status, string(responseBody))
		return nil, fmt.Errorf("Ollama API returned error: %s - %s", resp.Status, string(responseBody))
	}

	// Read the complete response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("ERROR: Failed to read response body: %v", err)
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	// Log the raw response for debugging
	logger.Printf("Raw Ollama response (%d bytes)", len(responseBody))

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(responseBody, &ollamaResp); err != nil {
		logger.Printf("ERROR: Failed to decode Ollama response: %v", err)
		return nil, fmt.Errorf("error decoding Ollama response: %w", err)
	}

	// Log the parsed response for debugging
	logger.Printf("Parsed Ollama response text (%d chars)", len(ollamaResp.Response))

	// Try to validate if the response is valid JSON
	if !json.Valid([]byte(ollamaResp.Response)) {
		logger.Println("Response is not valid JSON, attempting to extract JSON content")
		// If not valid JSON, try to extract JSON content
		// Sometimes LLMs might wrap the JSON in markdown code blocks or add text before/after
		jsonStart := strings.Index(ollamaResp.Response, "{")
		jsonEnd := strings.LastIndex(ollamaResp.Response, "}")

		if jsonStart >= 0 && jsonEnd > jsonStart {
			extractedJSON := ollamaResp.Response[jsonStart : jsonEnd+1]
			logger.Printf("Extracted JSON from response (%d chars)", len(extractedJSON))

			// Check if extracted content is valid JSON
			if json.Valid([]byte(extractedJSON)) {
				ollamaResp.Response = extractedJSON
				logger.Println("Successfully extracted valid JSON from response")
			} else {
				logger.Printf("ERROR: Extracted content is not valid JSON: %s", extractedJSON)
				return nil, fmt.Errorf("could not extract valid JSON from response")
			}
		} else {
			logger.Printf("ERROR: Response doesn't contain valid JSON structure: %s", ollamaResp.Response)
			return nil, fmt.Errorf("response doesn't contain valid JSON: %s", ollamaResp.Response)
		}
	}

	var categoryResp CategoryResponse
	if err := json.Unmarshal([]byte(ollamaResp.Response), &categoryResp); err != nil {
		logger.Printf("ERROR: Failed to parse category JSON: %v", err)
		return nil, fmt.Errorf("error parsing category JSON: %w, raw response: %s", err, ollamaResp.Response)
	}

	logger.Printf("Successfully parsed category response: Task=%s, Jira=%s, Timespan=%s, Confidence=%s",
		categoryResp.Task, categoryResp.Jira, categoryResp.Timespan, categoryResp.Confidence)
	return &categoryResp, nil
}

// buildSystemPromptWithRules creates a system prompt that incorporates the rules
func buildSystemPromptWithRules(rules string) string {
	logger.Println("Building system prompt with rules")

	baseSystemPrompt := `You are an AI assistant that categorizes work descriptions. 

Your task is to analyze a work description and determine:
1. What category of task it belongs to
2. Whether it's associated with a JIRA ticket
3. The timespan involved (if mentioned)

For the task categorization, follow the rules below, listed in order of priority.
`

	if rules == "" || rules == "No rules defined yet." {
		logger.Println("No rules provided, using default categorization guidance")
		baseSystemPrompt += `
Since no specific rules are defined yet, use your best judgment to categorize tasks into general categories 
like "Development", "Meetings", "Research", "Documentation", etc.
`
	} else {
		logger.Printf("Adding %d bytes of rules to system prompt", len(rules))
		baseSystemPrompt += "\n" + rules
	}

	baseSystemPrompt += `
Output your analysis in JSON format with these fields:
- task: The category this work belongs to
- jira: The JIRA ticket ID if mentioned (empty string if none)
- timespan: The duration if mentioned (e.g., "1 hour", "30 minutes", empty string if none)
- confidence: How confident you are in this categorization ("high", "medium", "low")
- reason: Brief explanation of why you assigned this category

Example output:
{
  "task": "Development",
  "jira": "ABC-123",
  "timespan": "1 hour",
  "confidence": "high",
  "reason": "The description clearly mentions coding work on a specific feature"
}
`

	logger.Printf("Final system prompt is %d bytes", len(baseSystemPrompt))
	return baseSystemPrompt
}

func readSystemPrompt() (string, error) {
	logger.Println("Attempting to read system prompt file")

	execPath, err := os.Executable()
	if err != nil {
		logger.Printf("ERROR: Failed to get executable path: %v", err)
		return "", fmt.Errorf("error getting executable path: %w", err)
	}

	execDir := filepath.Dir(execPath)
	promptFilePath := filepath.Join(execDir, "system_prompt.txt")
	logger.Printf("Looking for system prompt at: %s", promptFilePath)

	if _, err := os.Stat(promptFilePath); os.IsNotExist(err) {
		currentDir, _ := os.Getwd()
		promptFilePath = filepath.Join(currentDir, "system_prompt.txt")
		logger.Printf("Prompt not found in executable dir, trying: %s", promptFilePath)
	}

	promptData, err := os.ReadFile(promptFilePath)
	if err != nil {
		logger.Printf("ERROR: Failed to read system prompt file: %v", err)
		return "", fmt.Errorf("error reading system prompt file: %w", err)
	}

	logger.Printf("Successfully read system prompt file (%d bytes)", len(promptData))
	return string(promptData), nil
}

// TestCategorize is a utility function to test the Ollama categorization
func TestCategorize(description string) {
	logger.Println("=====================================")
	logger.Println("STARTING TEST CATEGORIZATION")
	logger.Println("=====================================")
	logger.Printf("Testing categorization with description: %s", description)

	// Load rules first
	logger.Println("Initializing rule manager for test")
	err := initRuleManager()
	if err != nil {
		logger.Printf("WARNING: Error initializing rule manager: %v", err)
	}

	rulesText := ruleManager.getAllRulesAsText()
	logger.Printf("Retrieved rules text (%d bytes)", len(rulesText))
	fmt.Println("\nUsing rules:\n" + rulesText)

	// Call categorization with rules
	logger.Println("Calling categorization with rules")
	result, err := categorizeWithRules(description, rulesText)
	if err != nil {
		logger.Printf("ERROR: Categorization failed: %v", err)
		fmt.Printf("Error: %v\n", err)
		return
	}

	logger.Printf("Categorization succeeded: Task=%s, Jira=%s", result.Task, result.Jira)
	fmt.Println("\nSuccessfully categorized:")
	fmt.Printf("Task: %s\n", result.Task)
	fmt.Printf("Jira: %s\n", result.Jira)
	fmt.Printf("Timespan: %s\n", result.Timespan)
	fmt.Printf("Confidence: %s\n", result.Confidence)
	fmt.Printf("Reason: %s\n", result.Reason)

	logger.Println("=====================================")
	logger.Println("TEST CATEGORIZATION COMPLETE")
	logger.Println("=====================================")
}
