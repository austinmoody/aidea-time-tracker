package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

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

func main() {
	// Check if we're running the test command
	if len(os.Args) > 1 && os.Args[0] == "test_ollama" {
		// We're running the test binary
		if len(os.Args) < 2 {
			fmt.Println("Usage: ./test_ollama \"Your task description here\"")
			os.Exit(1)
		}
		TestCategorize(os.Args[1])
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/save_time", saveTimeHandler)
	mux.HandleFunc("/api/v1/categorize", categorizeHandler)

	// Start the server
	fmt.Println("Server starting on :8080...")
	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
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
	// Only allow POST method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate filename based on current date
	currentDate := time.Now().Format("20060102") // Format for YYYYMMDD
	filename := fmt.Sprintf("aidea_time_tracking_%s.csv", currentDate)

	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("No data file found for today (%s)", filename), http.StatusNotFound)
		return
	}

	// Open the CSV file for reading and writing
	file, err := os.OpenFile(filename, os.O_RDWR, 0644)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error opening file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Read all records from the CSV file
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading CSV: %v", err), http.StatusInternalServerError)
		return
	}

	if len(records) <= 1 {
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

	// Check if we found all required columns
	if idIdx == -1 || descIdx == -1 || timespanIdx == -1 || taskIdx == -1 || reasonIdx == -1 ||
		jiraIdx == -1 || confidenceIdx == -1 || categorizedIdx == -1 {
		http.Error(w, "CSV file does not have the required columns", http.StatusInternalServerError)
		return
	}

	// Process uncategorized entries
	uncategorizedCount := 0
	successCount := 0
	errors := []string{}

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

		// Get the description
		description := record[descIdx]
		if description == "" {
			errors = append(errors, fmt.Sprintf("Entry ID %s has no description", record[idIdx]))
			continue
		}

		// Call Ollama to categorize the description
		categoryResp, err := categorizeDescription(description)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Error categorizing entry ID %s: %v", record[idIdx], err))
			continue
		}

		// Update the record with the category information
		record[taskIdx] = categoryResp.Task
		record[reasonIdx] = categoryResp.Reason
		record[jiraIdx] = categoryResp.Jira
		record[timespanIdx] = categoryResp.Timespan
		record[confidenceIdx] = categoryResp.Confidence
		record[categorizedIdx] = "true"

		// Update the record in the records slice
		records[i] = record
		successCount++
	}

	// If no uncategorized entries were found
	if uncategorizedCount == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "No uncategorized entries found",
		})
		return
	}

	// Write the updated records back to the file
	file.Seek(0, 0)
	file.Truncate(0)
	writer := csv.NewWriter(file)
	err = writer.WriteAll(records)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error writing updated CSV: %v", err), http.StatusInternalServerError)
		return
	}
	writer.Flush()

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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
