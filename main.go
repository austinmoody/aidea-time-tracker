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
	Jira        string `json:"jira,omitempty"`
	Confidence  string `json:"confidence,omitempty"`
	Categorized bool   `json:"categorized,omitempty"`
}

// TimeEntryRequest represents the JSON request for creating a time entry
type TimeEntryRequest struct {
	Description string `json:"description"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/save_time", saveTimeHandler)

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
		Categorized: false, // Default to false as requested
		// Other fields are left empty as specified
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
		headers := []string{"id", "timespan", "description", "task", "jira", "confidence", "categorized"}
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
		entry.Jira,
		entry.Confidence,
		categorizedStr,
	}

	if err := writer.Write(record); err != nil {
		return fmt.Errorf("error writing record: %v", err)
	}

	return nil
}
