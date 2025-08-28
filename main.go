package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

type WebhookParams struct {
	RequestID int
	EventType string
	Payload   map[string]any
	Version   string
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
			request_id INTEGER PRIMARY KEY,
			event_type TEXT NOT NULL,
			payload JSONB,
			version TEXT NOT NULL
		)
	`)
	if err != nil {
		log.Fatal("Failed to create table:", err)
	}

	sampleWebhooks := []WebhookParams{
		{
			RequestID: 1,
			EventType: "user.created",
			Payload: map[string]any{
				"user_id": "usr_123",
				"email":   "alice@example.com",
				"name":    "Alice Smith",
			},
			Version: "v1.0",
		},
		{
			RequestID: 2,
			EventType: "payment.processed",
			Payload: map[string]any{
				"payment_id": "pay_456",
				"amount":     99.99,
				"currency":   "USD",
				"status":     "completed",
			},
			Version: "v1.1",
		},
		{
			RequestID: 3,
			EventType: "order.shipped",
			Payload: map[string]any{
				"order_id":     "ord_789",
				"tracking_num": "1Z999AA10123456784",
				"carrier":      "UPS",
				"items":        3,
			},
			Version: "v2.0",
		},
	}

	for _, webhook := range sampleWebhooks {
		payloadJSON, err := json.Marshal(webhook.Payload)
		if err != nil {
			log.Printf("Failed to marshal payload for RequestID %d: %v", webhook.RequestID, err)
			continue
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO webhook_params (request_id, event_type, payload, version)
			VALUES (?, ?, ?, ?)
		`, webhook.RequestID, webhook.EventType, string(payloadJSON), webhook.Version)

		if err != nil {
			log.Printf("Failed to insert webhook %d: %v", webhook.RequestID, err)
		} else {
			fmt.Printf("Inserted webhook: RequestID=%d, EventType=%s\n", webhook.RequestID, webhook.EventType)
		}
	}

	fmt.Println("\n--- All Webhook Params in Database ---")

	rows, err := db.QueryContext(ctx, `
		SELECT request_id, event_type, payload, version 
		FROM webhook_params 
		ORDER BY request_id
	`)
	if err != nil {
		log.Fatal("Failed to query webhooks:", err)
	}
	defer rows.Close()

	for rows.Next() {
		var requestID int
		var eventType, payloadStr, version string

		err := rows.Scan(&requestID, &eventType, &payloadStr, &version)
		if err != nil {
			log.Printf("Failed to scan row: %v", err)
			continue
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
			log.Printf("Failed to unmarshal payload for RequestID %d: %v", requestID, err)
			payload = make(map[string]any)
		}

		fmt.Printf("\nRequestID: %d\n", requestID)
		fmt.Printf("EventType: %s\n", eventType)
		fmt.Printf("Version: %s\n", version)
		fmt.Printf("Payload: %+v\n", payload)
	}

	if err = rows.Err(); err != nil {
		log.Printf("Row iteration error: %v", err)
	}

	fmt.Println("\n--- JSONB Query Examples ---")

	fmt.Println("\n1. Query webhooks where payload contains 'status' = 'completed':")
	rows, err = db.QueryContext(ctx, `
		SELECT request_id, event_type, json_extract(payload, '$.status') as status
		FROM webhook_params 
		WHERE json_extract(payload, '$.status') = 'completed'
	`)
	if err != nil {
		log.Printf("Failed to query by status: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var requestID int
			var eventType, status string
			if err := rows.Scan(&requestID, &eventType, &status); err == nil {
				fmt.Printf("   RequestID: %d, EventType: %s, Status: %s\n", requestID, eventType, status)
			}
		}
	}

	fmt.Println("\n2. Query webhooks with amount > 50:")
	rows, err = db.QueryContext(ctx, `
		SELECT request_id, event_type, json_extract(payload, '$.amount') as amount
		FROM webhook_params 
		WHERE CAST(json_extract(payload, '$.amount') AS REAL) > 50
	`)
	if err != nil {
		log.Printf("Failed to query by amount: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var requestID int
			var eventType string
			var amount float64
			if err := rows.Scan(&requestID, &eventType, &amount); err == nil {
				fmt.Printf("   RequestID: %d, EventType: %s, Amount: %.2f\n", requestID, eventType, amount)
			}
		}
	}

	fmt.Println("\n3. Extract specific fields from payload using json_extract:")
	rows, err = db.QueryContext(ctx, `
		SELECT 
			request_id,
			event_type,
			json_extract(payload, '$.user_id') as user_id,
			json_extract(payload, '$.email') as email,
			json_extract(payload, '$.payment_id') as payment_id,
			json_extract(payload, '$.order_id') as order_id
		FROM webhook_params
		ORDER BY request_id
	`)
	if err != nil {
		log.Printf("Failed to extract fields: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var requestID int
			var eventType string
			var userID, email, paymentID, orderID sql.NullString

			if err := rows.Scan(&requestID, &eventType, &userID, &email, &paymentID, &orderID); err == nil {
				fmt.Printf("   RequestID: %d, EventType: %s", requestID, eventType)
				if userID.Valid {
					fmt.Printf(", UserID: %s", userID.String)
				}
				if email.Valid {
					fmt.Printf(", Email: %s", email.String)
				}
				if paymentID.Valid {
					fmt.Printf(", PaymentID: %s", paymentID.String)
				}
				if orderID.Valid {
					fmt.Printf(", OrderID: %s", orderID.String)
				}
				fmt.Println()
			}
		}
	}

	fmt.Println("\n4. Query all keys in payload using json_each:")
	rows, err = db.QueryContext(ctx, `
		SELECT 
			wp.request_id,
			wp.event_type,
			je.key,
			je.value
		FROM webhook_params wp, json_each(wp.payload) je
		WHERE wp.request_id = 2
	`)
	if err != nil {
		log.Printf("Failed to query json_each: %v", err)
	} else {
		defer rows.Close()
		fmt.Println("   Keys and values for RequestID 2:")
		for rows.Next() {
			var requestID int
			var eventType, key, value string
			if err := rows.Scan(&requestID, &eventType, &key, &value); err == nil {
				fmt.Printf("     %s: %s\n", key, value)
			}
		}
	}
}
