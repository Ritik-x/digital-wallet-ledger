package razorpay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const ordersURL = "https://api.razorpay.com/v1/orders"

type Client struct {
	keyID     string
	keySecret string
	http      *http.Client
}

type CreateOrderRequest struct {
	Amount   int64             `json:"amount"`
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt"`
	Notes    map[string]string `json:"notes,omitempty"`
}

type Order struct {
	ID       string `json:"id"`
	Entity   string `json:"entity"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Receipt  string `json:"receipt"`
	Status   string `json:"status"`
}

func NewClient(keyID, keySecret string) (*Client, error) {
	keyID = strings.TrimSpace(keyID)
	keySecret = strings.TrimSpace(keySecret)

	if keyID == "" || keySecret == "" {
		return nil, fmt.Errorf("razorpay credentials are required")
	}

	return &Client{
		keyID:     keyID,
		keySecret: keySecret,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}
func (c *Client) KeyID() string {
	return c.keyID
}

func (c *Client) CreateOrder(
	ctx context.Context,
	input CreateOrderRequest,
) (*Order, error) {
	if input.Amount <= 0 {
		return nil, fmt.Errorf("order amount must be positive")
	}

	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))

	if len(input.Currency) != 3 {
		return nil, fmt.Errorf("invalid order currency")
	}

	input.Receipt = strings.TrimSpace(input.Receipt)

	if input.Receipt == "" || len(input.Receipt) > 40 {
		return nil, fmt.Errorf("receipt must contain 1 to 40 characters")
	}

	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode razorpay order request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		ordersURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create razorpay request: %w", err)
	}

	req.SetBasicAuth(c.keyID, c.keySecret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send razorpay order request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read razorpay response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not expose Razorpay's response body or credentials to clients.
		return nil, fmt.Errorf(
			"razorpay order creation failed with HTTP status %d",
			resp.StatusCode,
		)
	}

	var order Order

	if err := json.Unmarshal(responseBody, &order); err != nil {
		return nil, fmt.Errorf("decode razorpay order response: %w", err)
	}

	if order.ID == "" ||
		order.Amount != input.Amount ||
		order.Currency != input.Currency {
		return nil, fmt.Errorf("razorpay returned an unexpected order response")
	}

	return &order, nil
}
