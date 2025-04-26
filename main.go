package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/google/uuid"
)

type ActivityEntry struct {
	Id                   string `json:"id,omitempty"`
	Description          string `json:"description"`
	Category             string `json:"category,omitempty"`
	Jira                 string `json:"jira,omitempty"`
	ConfidenceScore      string `json:"confidence_score,omitempty"`
	ClassificationReason string `json:"classification_reason,omitempty"`
	Categorized          bool   `json:"categorized,omitempty"`
	Duration             string `json:"duration,omitempty"`
}

type MatchResult struct {
	Rule       ActivityRule
	Score      float64
	Confidence string
}

type Server struct {
	ruleConfig RuleConfig
}

func main() {

	// Read Activity Rules & Generate Embeddings
	ruleFile, err := os.ReadFile("activity_rules.json")
	if err != nil {
		fmt.Printf("Error reading rule file: %v\n", err)
		os.Exit(1)
	}

	var config RuleConfig
	if err := json.Unmarshal(ruleFile, &config); err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		os.Exit(1)
	}

	for i, rule := range config.Rules {
		if len(rule.Embedding) == 0 {
			embedding, err := getEmbedding(rule.Description)
			if err != nil {
				fmt.Printf("Error generating embedding for rule %s: %v\n", rule.ID, err)
				os.Exit(1)
			}
			config.Rules[i].Embedding = embedding
		}
	}

	server := &Server{
		config,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/activity", server.activityHandler)
	mux.HandleFunc("/api/v1/categorize", server.categorizeHandler)

	// Start the server
	fmt.Println("Server starting on :8080...")
	err = http.ListenAndServe(":8080", mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func (s *Server) activityHandler(w http.ResponseWriter, r *http.Request) {
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
	var request ActivityEntry
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

	// Set id
	request.Id = uuid.New().String()

	// Save to CSV
	err = saveToCSV(request)
	if err != nil {
		http.Error(w, "Error saving data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create JSON response
	response := map[string]string{
		"id":      request.Id,
		"message": "Time entry saved successfully",
	}

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func saveToCSV(entry ActivityEntry) error {
	// Generate filename based on current date
	currentDate := time.Now().Format("20060102") // Format for YYYYMMDD
	filename := fmt.Sprintf("aidea_time_tracking_%s.csv", currentDate)

	// Check if the file exists to determine if we need to write headers
	fileExists := false
	if _, err := os.Stat(filename); err == nil {
		fileExists = true
	}

	// Open file append mode or create if it doesn't exist
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("couldn't open file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write headers if the file was just created
	if !fileExists {
		headers := []string{"id", "duration", "description", "category", "reason", "jira", "confidence", "categorized"}
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
		entry.Id,
		entry.Duration,
		entry.Description,
		entry.Category,
		entry.ClassificationReason,
		entry.Jira,
		entry.ConfidenceScore,
		categorizedStr,
	}

	if err := writer.Write(record); err != nil {
		return fmt.Errorf("error writing record: %v", err)
	}

	return nil
}

func (s *Server) categorizeHandler(w http.ResponseWriter, r *http.Request) {

	// Only allow POST method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate filename based on current date
	currentDate := time.Now().Format("20060102") // Format for YYYYMMDD
	filename := fmt.Sprintf("aidea_time_tracking_%s.csv", currentDate)

	// Check if the file exists
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
		case "duration":
			timespanIdx = i
		case "category":
			taskIdx = i
		case "reason":
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
	var errors []string

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
		categorizeByEmbedding, err := categorizeByEmbedding(description, s.ruleConfig.Rules)
		log.Printf(categorizeByEmbedding.Jira)
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

		// Update the record in the slice
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

func categorizeByEmbedding(input string, rules []ActivityRule) (*CategoryResponse, error) {
	inputEmbedding, err := getEmbedding(input)
	if err != nil {
		fmt.Printf("Error generating embedding for input: %v\n", err)
		os.Exit(1)
	}

	closestMatch := findCloseMatch(inputEmbedding, rules)

	response := CategoryResponse{
		Task:       closestMatch.Rule.Jira,
		Jira:       closestMatch.Rule.Jira,
		Confidence: closestMatch.Confidence,
	}

	return &response, nil

}

func findCloseMatch(embedding []float64, rules []ActivityRule) MatchResult {
	var results []MatchResult

	for _, rule := range rules {
		score := cosineSimilarity(embedding, rule.Embedding)
		confidence := scoreToConfidence(score)

		results = append(results, MatchResult{
			Rule:       rule,
			Score:      score,
			Confidence: confidence,
		})
	}

	// Sort by score, highest first
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Return the best match
	if len(results) > 0 {
		return results[0]
	}

	// Default ? TODO think about this...
	// Maybe come up with an "unknown" rule to fall back to
	return MatchResult{
		Rule:       ActivityRule{ID: "1", Jira: "FEDS-132"},
		Score:      0,
		Confidence: "F",
	}
}

func cosineSimilarity(a, b []float64) float64 {
	var dotProduct float64
	var normA float64
	var normB float64

	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	normA = math.Sqrt(normA)
	normB = math.Sqrt(normB)

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (normA * normB)
}

func scoreToConfidence(score float64) string {
	if score >= 0.9 {
		return "A"
	} else if score >= 0.8 {
		return "B"
	} else if score >= 0.7 {
		return "C"
	} else if score >= 0.6 {
		return "D"
	} else {
		return "F"
	}
}
