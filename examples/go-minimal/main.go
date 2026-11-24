// Command go-minimal is a small TPT Pay integration in Go, using only the standard library:
//
//   - POST /checkout creates a payment intent through the API (needs a secret key).
//   - POST /webhooks receives signed events and verifies them before it trusts anything.
//
// Configuration (environment):
//
//	TPT_SECRET_KEY       sk_test_... from `make quickstart`
//	TPT_API_BASE         default http://localhost:8080/v1
//	TPT_WEBHOOK_SECRET   the whsec_... returned when the endpoint was created
//	PORT                 default 4000
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const maxBody = 1 << 20 // 1 MB, the same limit the API applies

func main() {
	secretKey := os.Getenv("TPT_SECRET_KEY")
	apiBase := getenv("TPT_API_BASE", "http://localhost:8080/v1")
	webhookSecret := os.Getenv("TPT_WEBHOOK_SECRET")
	port := getenv("PORT", "4000")

	client := &http.Client{Timeout: 10 * time.Second}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /checkout", func(w http.ResponseWriter, r *http.Request) {
		if secretKey == "" {
			http.Error(w, "TPT_SECRET_KEY is not set", http.StatusInternalServerError)
			return
		}
		checkout(w, r, client, apiBase, secretKey)
	})
	mux.HandleFunc("POST /webhooks", func(w http.ResponseWriter, r *http.Request) {
		if webhookSecret == "" {
			http.Error(w, "TPT_WEBHOOK_SECRET is not set", http.StatusInternalServerError)
			return
		}
		receive(w, r, webhookSecret)
	})

	log.Printf("listening on :%s (API %s)", port, apiBase)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// checkout creates a payment intent. The browser sends {"amount": 1000, "currency": "usd"}; the
// secret key never leaves the server.
func checkout(w http.ResponseWriter, r *http.Request, client *http.Client, apiBase, secretKey string) {
	var in struct {
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&in); err != nil || in.Amount <= 0 || in.Currency == "" {
		http.Error(w, "amount and currency are required", http.StatusBadRequest)
		return
	}
	payload, _ := json.Marshal(map[string]any{"amount": in.Amount, "currency": in.Currency})

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, apiBase+"/payment_intents", bytes.NewReader(payload))
	if err != nil {
		http.Error(w, "could not build request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+secretKey)
	req.Header.Set("Content-Type", "application/json")
	// Passing the caller's key through makes a retried checkout create one intent, not two.
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "payment service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// receive verifies the signature over the exact bytes received, then handles the event.
func receive(w http.ResponseWriter, r *http.Request, secret string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		http.Error(w, "body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	if err := verifySignature(body, r.Header.Get("TPT-Signature"), secret, time.Now(), DefaultTolerance); err != nil {
		log.Printf("rejected webhook: %v", err)
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	// Deliveries are at least once. Store TPT-Event-Id and skip repeats in a real integration.
	log.Printf("event %s (%s, delivery %s)", event.Type, event.ID, r.Header.Get("TPT-Event-Id"))
	w.WriteHeader(http.StatusOK)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
