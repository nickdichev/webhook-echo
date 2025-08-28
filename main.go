package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	_ "modernc.org/sqlite"
	"net/http"
)

type WebhookParams struct {
	EventType string         `json:"event"`
	Payload   map[string]any `json:"data"`
	Version   string         `json:"version"`
}

func recordWebhookHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		res := WebhookParams{}
		if err := json.Unmarshal(body, &res); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		payloadJSON, _ := json.Marshal(res.Payload)
		_, err = db.ExecContext(ctx, `
			INSERT INTO webhook_params (event_type, payload, version)
			VALUES (?, ?, ?)
		`, res.EventType, string(payloadJSON), res.Version)

		if err != nil {
			fmt.Println("Insert error:", err)
			http.Error(w, "Failed to insert webhook", http.StatusInternalServerError)
		} else {
			fmt.Println("Inserted webhook:", res)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}
}

func queryWebhookHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract event_type from path
		eventType := r.PathValue("event_type")
		if eventType == "" {
			http.Error(w, "Event type is required", http.StatusBadRequest)
			return
		}

		// Build SQL query with JSON filters
		query := `
			SELECT id, event_type, payload, version 
			FROM webhook_params 
			WHERE event_type = ?`

		args := []any{eventType}

		// Add JSON filters for query parameters
		queryParams := r.URL.Query()
		for key, values := range queryParams {
			if len(values) > 0 {
				// Use json_extract with CAST to handle both string and numeric comparisons
				query += fmt.Sprintf(" AND CAST(json_extract(payload, '$.%s') AS TEXT) = ?", key)
				args = append(args, values[0]) // Use first value for simplicity
			}
		}

		query += " ORDER BY id"

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			http.Error(w, "Database query failed", http.StatusInternalServerError)
			log.Printf("Query error: %v", err)
			return
		}
		defer rows.Close()

		var webhooks []WebhookParams
		for rows.Next() {
			var requestID int
			var eventType, payloadStr, version string

			if err := rows.Scan(&requestID, &eventType, &payloadStr, &version); err != nil {
				continue
			}

			var payload map[string]any
			if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
				continue
			}

			webhooks = append(webhooks, WebhookParams{
				EventType: eventType,
				Payload:   payload,
				Version:   version,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(webhooks)
	}
}

func main() {
	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS webhook_params (
			id INTEGER PRIMARY KEY,
			event_type TEXT NOT NULL,
			payload TEXT,
			version TEXT NOT NULL
		)
	`)
	if err != nil {
		log.Fatal("Failed to create table:", err)
	}

	http.HandleFunc("POST /", recordWebhookHandler(db))
	http.HandleFunc("GET /query/{event_type}", queryWebhookHandler(db))

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
