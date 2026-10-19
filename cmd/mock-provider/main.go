package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type PaymentEvent struct {
	DepositID   int64  `json:"deposit_id"`
	Status      string `json:"status"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

func main() {
	_ = godotenv.Load()

	secret := os.Getenv("MOCK_PAYMENT_WEBHOOK_SECRET")
	if len(secret) < 32 {
		log.Fatal("MOCK_PAYMENT_WEBHOOK_SECRET must be at least 32 characters")
	}

	if len(os.Args) != 4 {
		log.Fatal("Usage: go run ./cmd/mock-provider <deposit_id> <amount_minor> <currency>")
	}

	var event PaymentEvent

	if _, err := fmt.Sscanf(os.Args[1], "%d", &event.DepositID); err != nil || event.DepositID <= 0 {
		log.Fatal("Invalid deposit ID")
	}

	if _, err := fmt.Sscanf(os.Args[2], "%d", &event.AmountMinor); err != nil || event.AmountMinor <= 0 {
		log.Fatal("Invalid amount_minor")
	}

	event.Currency = strings.ToUpper(strings.TrimSpace(os.Args[3]))
	if len(event.Currency) != 3 {
		log.Fatal("Currency must be a three-letter code")
	}
	for _, ch := range event.Currency {
		if ch < 'A' || ch > 'Z' {
			log.Fatal("Currency must contain only letters")
		}
	}

	event.Status = "succeeded"

	body, err := json.Marshal(event)
	if err != nil {
		log.Fatal("Could not encode payment event:", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(
		http.MethodPost,
		"http://localhost:8080/api/v1/webhooks/mock-payment",
		bytes.NewReader(body),
	)
	if err != nil {
		log.Fatal("Could not create request:", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Could not contact LedgerX API:", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Fatal("Could not read response:", err)
	}

	fmt.Printf("HTTP status: %s\n", resp.Status)
	fmt.Printf("Response: %s\n", strings.TrimSpace(string(responseBody)))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
