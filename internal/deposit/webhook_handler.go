package deposit

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	service *Service
	secret  string
}

func NewWebhookHandler(service *Service, secret string) *WebhookHandler {
	return &WebhookHandler{
		service: service,
		secret:  secret,
	}
}

type paymentWebhookRequest struct {
	DepositID   int64  `json:"deposit_id"`
	Status      string `json:"status"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

func (h *WebhookHandler) Handle(c *gin.Context) {
	// Read the exact request bytes before decoding JSON.
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read webhook body"})
		return
	}

	signature := c.GetHeader("X-Webhook-Signature")
	if !VerifyWebhookSignature(body, signature, h.secret) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook signature"})
		return
	}

	var event paymentWebhookRequest
	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook JSON"})
		return
	}

	if event.DepositID <= 0 ||
		event.AmountMinor <= 0 ||
		event.Status != "succeeded" ||
		len(event.Currency) != 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment event"})
		return
	}

	// Confirmation will be implemented in the next step.
	if err := h.service.ConfirmPayment(
		c.Request.Context(),
		event.DepositID,
		event.AmountMinor,
		event.Currency,
	); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "payment confirmation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// Keep this import out of the handler unless needed elsewhere.
var _ = os.Getenv
