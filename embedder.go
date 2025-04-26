package main

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type ActivityRule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Jira        string    `json:"jira"`
	Description string    `json:"description"`
	Keywords    []string  `json:"keywords"`
	Embedding   []float64 `json:"embedding,omitempty"`
}

type RuleConfig struct {
	Rules []ActivityRule `json:"rules"`
}

func getEmbedding(description string) ([]float64, error) {
	client := &http.Client{}

	reqBody, _ := json.Marshal(map[string]string{
		"model":  "all-minilm",
		"prompt": description,
	})

	req, err := http.NewRequest("POST", "http://localhost:11434/api/embeddings", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Embedding []float64 `json:"embedding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Embedding, nil
}
