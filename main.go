package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Logger for consistent log formatting with timestamps
var logger = log.New(os.Stdout, "", log.LstdFlags)

// TimeEntry represents a single time tracking entry
type TimeEntry struct {
	ID          string `json:"id,omitempty"`
	Timespan    string `json:"timespan,omitempty"`
	Description string `json:"description"`
	Task        string `json:"task,omitempty"`
	TaskReason  string `json:"task_reason,omitempty"`
	Jira        string `json:"jira,omitempty"`
	Confidence  string `json:"confidence,omitempty"`
	Categorized bool   `json:"categorized,omitempty"`
}

// TimeEntryRequest represents the JSON request for creating a time entry
type TimeEntryRequest struct {
	Description string `json:"description"`
}

// Rule represents a categorization rule
type Rule struct {
	ID        string `json:"id,omitempty"`
	Pattern   string `json:"pattern"`
	Task      string `json:"task"`
	Jira      string `json:"jira,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// RuleRequest represents the JSON request for adding a rule
type RuleRequest struct {
	Pattern string `json:"pattern"`
	Task    string `json:"task"`
	Jira    string `json:"jira,omitempty"`
}

// RuleManager handles the storage and management of categorization rules
type RuleManager struct {
	rules     []Rule
	rulesFile string
	mu        sync.RWMutex
}

// Global rule manager
var ruleManager RuleManager

// Initialize the rule manager
func initRuleManager() error {
	logger.Println("Initializing rule manager...")
	ruleManager = RuleManager{
		rules:     []Rule{},
		rulesFile: "categorization_rules.csv",
	}
	err := ruleManager.loadRules()
	if err != nil {
		logger.Printf("ERROR: Failed to load rules: %v", err)
		return err
	}

	logger.Printf("Rule manager initialized successfully with %d rules", len(ruleManager.rules))
	return nil
}

// loadRules loads rules from the CSV file
func (rm *RuleManager) loadRules() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	logger.Printf("Loading rules from file: %s", rm.rulesFile)

	// Check if file exists
	if _, err := os.Stat(rm.rulesFile); os.IsNotExist(err) {
		logger.Printf("Rules file not found, creating new file: %s", rm.rulesFile)
		// Create empty rules file with headers
		file, err := os.Create(rm.rulesFile)
		if err != nil {
			return fmt.Errorf("error creating rules file: %v", err)
		}
		defer file.Close()

		writer := csv.NewWriter(file)
		defer writer.Flush()

		headers := []string{"id", "pattern", "task", "jira", "created_at"}
		if err := writer.Write(headers); err != nil {
			return fmt.Errorf("error writing headers: %v", err)
		}

		logger.Println("Created new empty rules file with headers")
		return nil
	}

	// Open file for reading
	file, err := os.Open(rm.rulesFile)
	if err != nil {
		return fmt.Errorf("error opening rules file: %v", err)
	}
	defer file.Close()

	// Read all records
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("error reading CSV: %v", err)
	}

	if len(records) <= 1 {
		// Only headers, no rules
		logger.Println("No rules found in file (only headers)")
		return nil
	}

	// Parse headers
	headers := records[0]
	idIdx, patternIdx, taskIdx, jiraIdx, createdAtIdx := -1, -1, -1, -1, -1

	for i, header := range headers {
		switch header {
		case "id":
			idIdx = i
		case "pattern":
			patternIdx = i
		case "task":
			taskIdx = i
		case "jira":
			jiraIdx = i
		case "created_at":
			createdAtIdx = i
		}
	}

	// Check if we found all required columns
	if idIdx == -1 || patternIdx == -1 || taskIdx == -1 {
		return fmt.Errorf("rules CSV file does not have all required columns")
	}

	// Parse rules
	rules := make([]Rule, 0, len(records)-1)
	for i, record := range records {
		if i == 0 {
			// Skip header
			continue
		}

		rule := Rule{
			ID:      record[idIdx],
			Pattern: record[patternIdx],
			Task:    record[taskIdx],
		}

		if jiraIdx >= 0 && jiraIdx < len(record) {
			rule.Jira = record[jiraIdx]
		}

		if createdAtIdx >= 0 && createdAtIdx < len(record) {
			rule.CreatedAt = record[createdAtIdx]
		}

		rules = append(rules, rule)
		logger.Printf("Loaded Rule #%d: ID=%s, Pattern='%s', Task=%s, Jira=%s",
			i, rule.ID, rule.Pattern, rule.Task, rule.Jira)
	}

	rm.rules = rules
	logger.Printf("Successfully loaded %d rules from file", len(rules))
	return nil
}

// saveRules saves the current rules to the CSV file
func (rm *RuleManager) saveRules() error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	logger.Printf("Saving %d rules to file: %s", len(rm.rules), rm.rulesFile)

	file, err := os.Create(rm.rulesFile)
	if err != nil {
		logger.Printf("ERROR: Failed to create rules file: %v", err)
		return fmt.Errorf("error creating rules file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write headers
	headers := []string{"id", "pattern", "task", "jira", "created_at"}
	if err := writer.Write(headers); err != nil {
		logger.Printf("ERROR: Failed to write headers: %v", err)
		return fmt.Errorf("error writing headers: %v", err)
	}

	// Write rules
	for i, rule := range rm.rules {
		record := []string{
			rule.ID,
			rule.Pattern,
			rule.Task,
			rule.Jira,
			rule.CreatedAt,
		}

		if err := writer.Write(record); err != nil {
			logger.Printf("ERROR: Failed to write rule #%d: %v", i, err)
			return fmt.Errorf("error writing rule: %v", err)
		}
	}

	logger.Printf("Successfully saved %d rules to file", len(rm.rules))
	return nil
}

// addRule adds a new rule to the manager and saves the rules
func (rm *RuleManager) addRule(rule Rule) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	logger.Printf("Adding new rule: ID=%s, Pattern='%s', Task=%s, Jira=%s",
		rule.ID, rule.Pattern, rule.Task, rule.Jira)

	// Add the rule
	rm.rules = append(rm.rules, rule)

	logger.Printf("Rule added, now saving rules to file (total rules: %d)", len(rm.rules))

	// Save the updated rules
	err := rm.saveRules()
	if err != nil {
		logger.Printf("ERROR: Failed to save rules after adding new rule: %v", err)
		return err
	}

	logger.Printf("Rule added and saved successfully: ID=%s", rule.ID)
	return nil
}

// getAllRulesAsText returns all rules as a formatted text for LLM context
func (rm *RuleManager) getAllRulesAsText() string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	logger.Printf("Formatting %d rules as text for LLM context", len(rm.rules))

	if len(rm.rules) == 0 {
		logger.Println("No rules defined, returning default message")
		return "No rules defined yet."
	}

	var sb strings.Builder
	sb.WriteString("Categorization Rules:\n")

	for i, rule := range rm.rules {
		sb.WriteString(fmt.Sprintf("%d. When a task matches pattern '%s', categorize it as '%s'",
			i+1, rule.Pattern, rule.Task))

		if rule.Jira != "" {
			sb.WriteString(fmt.Sprintf(" with JIRA '%s'", rule.Jira))
		}

		sb.WriteString("\n")
	}

	rulesText := sb.String()
	logger.Printf("Generated rules text (%d bytes)", len(rulesText))
	return rulesText
}

func main() {
	logger.Println("Starting AIDeA Time Tracker application")

	// Check if we're running the test command
	if len(os.Args) > 1 && os.Args[0] == "test_ollama" {
		// We're running the test binary
		logger.Println("Running in test mode (test_ollama)")
		if len(os.Args) < 2 {
			fmt.Println("Usage: ./test_ollama \"Your task description here\"")
			os.Exit(1)
		}
		TestCategorize(os.Args[1])
		return
	}

	// Initialize the rule manager
	logger.Println("Starting server, initializing components...")
	if err := initRuleManager(); err != nil {
		logger.Printf("WARNING: Failed to initialize rule manager: %v", err)
	}

	// Set up HTTP handlers
	logger.Println("Setting up HTTP handlers")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/save_time", saveTimeHandler)
	mux.HandleFunc("/api/v1/categorize", categorizeHandler)
	mux.HandleFunc("/api/v1/rule/add", addRuleHandler)

	// Start the server
	logger.Println("Server starting on :8080...")
	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		logger.Fatalf("ERROR: Server failed to start: %v", err)
	}
}

// addRuleHandler processes requests to add a new rule
func addRuleHandler(w http.ResponseWriter, r *http.Request) {
	logger.Printf("Received request to %s %s", r.Method, r.URL.Path)

	// Only allow POST method
	if r.Method != http.MethodPost {
		logger.Printf("Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		logger.Printf("Invalid content type: %s", contentType)
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Printf("Error reading request body: %v", err)
		http.Error(w, "Error reading request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	logger.Printf("Received rule request body: %s", string(body))

	// Parse JSON request
	var request RuleRequest
	err = json.Unmarshal(body, &request)
	if err != nil {
		logger.Printf("Error parsing JSON: %v", err)
		http.Error(w, "Error parsing JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if request.Pattern == "" {
		logger.Println("Validation failed: Pattern is required")
		http.Error(w, "Pattern is required", http.StatusBadRequest)
		return
	}
	if request.Task == "" {
		logger.Println("Validation failed: Task is required")
		http.Error(w, "Task is required", http.StatusBadRequest)
		return
	}

	logger.Printf("Creating new rule with pattern: '%s', task: '%s', jira: '%s'",
		request.Pattern, request.Task, request.Jira)

	// Create a new rule
	rule := Rule{
		ID:        uuid.New().String(),
		Pattern:   request.Pattern,
		Task:      request.Task,
		Jira:      request.Jira,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	// Add the rule
	err = ruleManager.addRule(rule)
	if err != nil {
		logger.Printf("Error adding rule: %v", err)
		http.Error(w, "Error adding rule: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create JSON response
	response := map[string]interface{}{
		"id":      rule.ID,
		"message": "Rule added successfully",
		"rule":    rule,
	}

	// Send JSON response
	logger.Printf("Rule added successfully with ID: %s", rule.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func saveTimeHandler(w http.ResponseWriter, r *http.Request) {
	// Only allow POST method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON request
	var request TimeEntryRequest
	err = json.Unmarshal(body, &request)
	if err != nil {
		http.Error(w, "Error parsing JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if request.Description == "" {
		http.Error(w, "Description is required", http.StatusBadRequest)
		return
	}

	// Create a new time entry
	entry := TimeEntry{
		ID:          uuid.New().String(),
		Description: request.Description,
		Categorized: false,
	}

	// Save to CSV
	err = saveToCSV(entry)
	if err != nil {
		http.Error(w, "Error saving data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create JSON response
	response := map[string]string{
		"id":      entry.ID,
		"message": "Time entry saved successfully",
	}

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func saveToCSV(entry TimeEntry) error {
	// Generate filename based on current date
	currentDate := time.Now().Format("20060102") // Format for YYYYMMDD
	filename := fmt.Sprintf("aidea_time_tracking_%s.csv", currentDate)

	// Check if file exists to determine if we need to write headers
	fileExists := false
	if _, err := os.Stat(filename); err == nil {
		fileExists = true
	}

	// Open file in append mode or create if it doesn't exist
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("couldn't open file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write headers if file was just created
	if !fileExists {
		headers := []string{"id", "timespan", "description", "task", "task_reason", "jira", "confidence", "categorized"}
		if err := writer.Write(headers); err != nil {
			return fmt.Errorf("error writing headers: %v", err)
		}
	}

	// Write the entry as a CSV record
	categorizedStr := "false"
	if entry.Categorized {
		categorizedStr = "true"
	}

	record := []string{
		entry.ID,
		entry.Timespan,
		entry.Description,
		entry.Task,
		entry.TaskReason,
		entry.Jira,
		entry.Confidence,
		categorizedStr,
	}

	if err := writer.Write(record); err != nil {
		return fmt.Errorf("error writing record: %v", err)
	}

	return nil
}

func categorizeHandler(w http.ResponseWriter, r *http.Request) {
	logger.Printf("Received request to %s %s", r.Method, r.URL.Path)

	// Only allow POST method
	if r.Method != http.MethodPost {
		logger.Printf("Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate filename based on current date
	currentDate := time.Now().Format("20060102") // Format for YYYYMMDD
	filename := fmt.Sprintf("aidea_time_tracking_%s.csv", currentDate)
	logger.Printf("Looking for time entries file: %s", filename)

	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		logger.Printf("File not found: %s", filename)
		http.Error(w, fmt.Sprintf("No data file found for today (%s)", filename), http.StatusNotFound)
		return
	}

	// Open the CSV file for reading and writing
	file, err := os.OpenFile(filename, os.O_RDWR, 0644)
	if err != nil {
		logger.Printf("Error opening file: %v", err)
		http.Error(w, fmt.Sprintf("Error opening file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	logger.Printf("Reading time entries from: %s", filename)
	// Read all records from the CSV file
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		logger.Printf("Error reading CSV: %v", err)
		http.Error(w, fmt.Sprintf("Error reading CSV: %v", err), http.StatusInternalServerError)
		return
	}

	logger.Printf("Read %d records (including header)", len(records))
	if len(records) <= 1 {
		logger.Println("No time entries found (only header row)")
		http.Error(w, "No time entries found", http.StatusNotFound)
		return
	}

	// Get headers
	headers := records[0]

	// Find index of each column
	idIdx := -1
	descIdx := -1
	timespanIdx := -1
	taskIdx := -1
	reasonIdx := -1
	jiraIdx := -1
	confidenceIdx := -1
	categorizedIdx := -1

	for i, header := range headers {
		switch header {
		case "id":
			idIdx = i
		case "description":
			descIdx = i
		case "timespan":
			timespanIdx = i
		case "task":
			taskIdx = i
		case "task_reason":
			reasonIdx = i
		case "jira":
			jiraIdx = i
		case "confidence":
			confidenceIdx = i
		case "categorized":
			categorizedIdx = i
		}
	}

	logger.Printf("Found column indices: id=%d, desc=%d, timespan=%d, task=%d, reason=%d, jira=%d, confidence=%d, categorized=%d",
		idIdx, descIdx, timespanIdx, taskIdx, reasonIdx, jiraIdx, confidenceIdx, categorizedIdx)

	// Check if we found all required columns
	if idIdx == -1 || descIdx == -1 || timespanIdx == -1 || taskIdx == -1 || reasonIdx == -1 ||
		jiraIdx == -1 || confidenceIdx == -1 || categorizedIdx == -1 {
		logger.Println("CSV file missing required columns")
		http.Error(w, "CSV file does not have the required columns", http.StatusInternalServerError)
		return
	}

	// Process uncategorized entries
	uncategorizedCount := 0
	successCount := 0
	errors := []string{}

	logger.Println("Starting categorization of uncategorized entries...")
	for i, record := range records {
		// Skip header row
		if i == 0 {
			continue
		}

		// Check if entry is already categorized
		if record[categorizedIdx] == "true" {
			continue
		}

		uncategorizedCount++
		entryID := record[idIdx]
		logger.Printf("Processing uncategorized entry ID: %s", entryID)

		// Get the description
		description := record[descIdx]
		if description == "" {
			errorMsg := fmt.Sprintf("Entry ID %s has no description", entryID)
			logger.Printf("Error: %s", errorMsg)
			errors = append(errors, errorMsg)
			continue
		}

		// Call Ollama to categorize the description with rules as context
		logger.Printf("Categorizing entry ID %s with description: %s", entryID, description)
		categoryResp, err := categorizeDescriptionWithRules(description)
		if err != nil {
			errorMsg := fmt.Sprintf("Error categorizing entry ID %s: %v", entryID, err)
			logger.Printf("Error: %s", errorMsg)
			errors = append(errors, errorMsg)
			continue
		}

		// Update the record with the category information
		logger.Printf("Categorization result for entry ID %s: Task=%s, Jira=%s, Timespan=%s, Confidence=%s",
			entryID, categoryResp.Task, categoryResp.Jira, categoryResp.Timespan, categoryResp.Confidence)

		record[taskIdx] = categoryResp.Task
		record[reasonIdx] = categoryResp.Reason
		record[jiraIdx] = categoryResp.Jira
		record[timespanIdx] = categoryResp.Timespan
		record[confidenceIdx] = categoryResp.Confidence
		record[categorizedIdx] = "true"

		// Update the record in the records slice
		records[i] = record
		successCount++
		logger.Printf("Successfully categorized entry ID: %s", entryID)
	}

	// If no uncategorized entries were found
	if uncategorizedCount == 0 {
		logger.Println("No uncategorized entries found")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "No uncategorized entries found",
		})
		return
	}

	// Write the updated records back to the file
	logger.Printf("Writing %d updated records back to file", len(records))
	file.Seek(0, 0)
	file.Truncate(0)
	writer := csv.NewWriter(file)
	err = writer.WriteAll(records)
	if err != nil {
		logger.Printf("Error writing updated CSV: %v", err)
		http.Error(w, fmt.Sprintf("Error writing updated CSV: %v", err), http.StatusInternalServerError)
		return
	}
	writer.Flush()
	logger.Printf("Successfully wrote updated records to %s", filename)

	// Create response
	response := map[string]interface{}{
		"total_uncategorized": uncategorizedCount,
		"success_count":       successCount,
		"error_count":         len(errors),
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}

	// Send JSON response
	logger.Printf("Categorization summary: total=%d, success=%d, errors=%d",
		uncategorizedCount, successCount, len(errors))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// categorizeDescriptionWithRules calls Ollama with rules as context
func categorizeDescriptionWithRules(description string) (*CategoryResponse, error) {
	// Get all rules as formatted text
	rulesText := ruleManager.getAllRulesAsText()

	logger.Printf("Categorizing description: '%s'", description)
	logger.Printf("Using rules context with %d bytes", len(rulesText))

	// Call the Ollama API with rules context
	result, err := categorizeWithRules(description, rulesText)
	if err != nil {
		logger.Printf("Error in categorization: %v", err)
		return nil, err
	}

	logger.Printf("Categorization result: Task=%s, Jira=%s, Timespan=%s, Confidence=%s",
		result.Task, result.Jira, result.Timespan, result.Confidence)
	return result, nil
}
