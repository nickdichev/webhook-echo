package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
)

var debug bool

type WebhookParams struct {
	EventType string         `json:"event"`
	Payload   map[string]any `json:"data"`
	Version   string         `json:"version"`
}

type RingBuffer struct {
	items []WebhookParams
	head  int
	count int
	size  int
	mu    sync.RWMutex
}

func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		items: make([]WebhookParams, size),
		size:  size,
	}
}

func (rb *RingBuffer) Push(item WebhookParams) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.items[rb.head] = item
	rb.head = (rb.head + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
}

func (rb *RingBuffer) Query(eventType string, filters map[string]string) []WebhookParams {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	var results []WebhookParams

	// Iterate through items newest to oldest
	for i := 0; i < rb.count; i++ {
		idx := (rb.head - 1 - i + rb.size) % rb.size
		item := rb.items[idx]

		// Filter by event type
		if item.EventType != eventType {
			continue
		}

		// Filter by payload fields
		match := true
		for key, value := range filters {
			if payloadVal, ok := item.Payload[key]; ok {
				// Convert payload value to string for comparison
				var payloadStr string
				switch v := payloadVal.(type) {
				case string:
					payloadStr = v
				case float64:
					payloadStr = strconv.FormatFloat(v, 'f', -1, 64)
				case bool:
					payloadStr = strconv.FormatBool(v)
				default:
					payloadStr = fmt.Sprintf("%v", v)
				}
				if payloadStr != value {
					match = false
					break
				}
			} else {
				match = false
				break
			}
		}

		if match {
			results = append(results, item)
		}
	}

	return results
}

func recordWebhookHandler(buffer *RingBuffer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		buffer.Push(res)
		if debug {
			fmt.Println("Inserted webhook:", res)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}
}

func queryWebhookHandler(buffer *RingBuffer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract event_type from path
		eventType := r.PathValue("event_type")
		if eventType == "" {
			http.Error(w, "Event type is required", http.StatusBadRequest)
			return
		}

		// Build filters from query parameters
		filters := make(map[string]string)
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				filters[key] = values[0]
			}
		}

		webhooks := buffer.Query(eventType, filters)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(webhooks)
	}
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func main() {
	// Define CLI flags
	port := flag.Int("port", 8080, "Port to listen on (env: PORT)")
	bufferSize := flag.Int("buffer-size", 1000, "Ring buffer size (env: BUFFER_SIZE)")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.Parse()

	// Environment variables override defaults (but not explicit CLI flags)
	if !isFlagSet("port") {
		*port = getEnvInt("PORT", *port)
	}
	if !isFlagSet("buffer-size") {
		*bufferSize = getEnvInt("BUFFER_SIZE", *bufferSize)
	}

	buffer := NewRingBuffer(*bufferSize)

	http.HandleFunc("POST /", recordWebhookHandler(buffer))
	http.HandleFunc("GET /query/{event_type}", queryWebhookHandler(buffer))

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Server starting on %s (buffer size: %d)", addr, *bufferSize)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func isFlagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
