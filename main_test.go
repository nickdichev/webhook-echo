package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer() *http.ServeMux {
	buffer := NewRingBuffer(100)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", recordWebhookHandler(buffer))
	mux.HandleFunc("GET /query/{event_type}", queryWebhookHandler(buffer))
	return mux
}

func postWebhook(t *testing.T, mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func queryWebhooks(t *testing.T, mux *http.ServeMux, path string) []WebhookParams {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("query failed with status %d: %s", rec.Code, rec.Body.String())
	}

	var results []WebhookParams
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	return results
}

func TestPostAndQueryWebhook(t *testing.T) {
	mux := newTestServer()

	rec := postWebhook(t, mux, `{"event":"user_created","data":{"name":"alice"},"version":"1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST failed with status %d", rec.Code)
	}

	results := queryWebhooks(t, mux, "/query/user_created")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Payload["name"] != "alice" {
		t.Errorf("expected name=alice, got %v", results[0].Payload["name"])
	}
}

func TestQueryByEventType(t *testing.T) {
	mux := newTestServer()

	postWebhook(t, mux, `{"event":"order","data":{},"version":"1"}`)
	postWebhook(t, mux, `{"event":"user","data":{},"version":"1"}`)
	postWebhook(t, mux, `{"event":"order","data":{},"version":"1"}`)

	orders := queryWebhooks(t, mux, "/query/order")
	if len(orders) != 2 {
		t.Errorf("expected 2 orders, got %d", len(orders))
	}

	users := queryWebhooks(t, mux, "/query/user")
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}

	empty := queryWebhooks(t, mux, "/query/nonexistent")
	if len(empty) != 0 {
		t.Errorf("expected 0 results, got %d", len(empty))
	}
}

func TestQueryWithFilters(t *testing.T) {
	mux := newTestServer()

	postWebhook(t, mux, `{"event":"user","data":{"status":"active","role":"admin"},"version":"1"}`)
	postWebhook(t, mux, `{"event":"user","data":{"status":"inactive","role":"admin"},"version":"1"}`)
	postWebhook(t, mux, `{"event":"user","data":{"status":"active","role":"user"},"version":"1"}`)

	// Filter by single field
	active := queryWebhooks(t, mux, "/query/user?status=active")
	if len(active) != 2 {
		t.Errorf("expected 2 active users, got %d", len(active))
	}

	// Filter by multiple fields
	activeAdmins := queryWebhooks(t, mux, "/query/user?status=active&role=admin")
	if len(activeAdmins) != 1 {
		t.Errorf("expected 1 active admin, got %d", len(activeAdmins))
	}

	// No matches
	noMatch := queryWebhooks(t, mux, "/query/user?status=pending")
	if len(noMatch) != 0 {
		t.Errorf("expected 0 results, got %d", len(noMatch))
	}
}

func TestQueryWithNumericFilter(t *testing.T) {
	mux := newTestServer()

	postWebhook(t, mux, `{"event":"order","data":{"amount":100},"version":"1"}`)
	postWebhook(t, mux, `{"event":"order","data":{"amount":200},"version":"1"}`)

	results := queryWebhooks(t, mux, "/query/order?amount=100")
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestQueryReturnsNewestFirst(t *testing.T) {
	mux := newTestServer()

	postWebhook(t, mux, `{"event":"log","data":{"seq":1},"version":"1"}`)
	postWebhook(t, mux, `{"event":"log","data":{"seq":2},"version":"1"}`)
	postWebhook(t, mux, `{"event":"log","data":{"seq":3},"version":"1"}`)

	results := queryWebhooks(t, mux, "/query/log")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Newest first
	if results[0].Payload["seq"] != float64(3) {
		t.Errorf("first result should be seq=3, got %v", results[0].Payload["seq"])
	}
	if results[2].Payload["seq"] != float64(1) {
		t.Errorf("last result should be seq=1, got %v", results[2].Payload["seq"])
	}
}

func TestPostInvalidJSON(t *testing.T) {
	mux := newTestServer()

	rec := postWebhook(t, mux, "not json")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestPostEchoesBody(t *testing.T) {
	mux := newTestServer()

	body := `{"event":"test","data":{"foo":"bar"},"version":"1"}`
	rec := postWebhook(t, mux, body)

	if rec.Body.String() != body {
		t.Errorf("expected response to echo body, got %s", rec.Body.String())
	}
}
