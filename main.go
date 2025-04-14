package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

// TimeEntry represents a single time tracking entry
type TimeEntry struct {
	ID          string
	Timespan    string
	Description string
	Task        string
	Jira        string
	Confidence  string
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

	// Parse form data
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Error parsing form data", http.StatusBadRequest)
		return
	}

	// Get description from form
	description := r.FormValue("description")
	if description == "" {
		http.Error(w, "Description is required", http.StatusBadRequest)
		return
	}

	// Create a new time entry
	entry := TimeEntry{
		ID:          uuid.New().String(),
		Description: description,
		// Other fields are left empty as specified
	}

	// Save to CSV
	err = saveToCSV(entry)
	if err != nil {
		http.Error(w, "Error saving data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "Time entry saved successfully with ID: %s", entry.ID)
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
		headers := []string{"id", "timespan", "description", "task", "jira", "confidence"}
		if err := writer.Write(headers); err != nil {
			return fmt.Errorf("error writing headers: %v", err)
		}
	}

	// Write the entry as a CSV record
	record := []string{
		entry.ID,
		entry.Timespan,
		entry.Description,
		entry.Task,
		entry.Jira,
		entry.Confidence,
	}

	if err := writer.Write(record); err != nil {
		return fmt.Errorf("error writing record: %v", err)
	}

	return nil
}
