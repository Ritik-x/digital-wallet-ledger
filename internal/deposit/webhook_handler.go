package deposit

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	service *Service
	secret  string
}
type WebhookResponse struct {
    Status string `json:"status" example:"received"`
}

func NewWebhookHandler(service *Service, secret string) *WebhookHandler {
	return &WebhookHandler{
		service: service,
		secret:  secret,
	}
}

type PaymentWebhookRequest struct {
	DepositID   int64  `json:"deposit_id"`
	Status      string `json:"status"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

// Handle godoc
// @Summary Handle a mock payment webhook
// @Description Verifies the HMAC signature and processes a successful payment event for a deposit.
// @Tags Payment Webhooks
// @Accept json
// @Produce json
// @Param X-Webhook-Signature header string true "HMAC signature of the exact request body"
// @Param request body PaymentWebhookRequest true "Payment event"
// @Success 200 {object} WebhookResponse "Webhook received"
// @Failure 400 {object} map[string]interface{} "Invalid body or payment event"
// @Failure 401 {object} map[string]interface{} "Invalid webhook signature"
// @Failure 422 {object} map[string]interface{} "Payment confirmation failed"
// @Router /api/v1/webhooks/mock-payment [post]
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
	var event PaymentWebhookRequest
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

		log.Printf(
			"payment confirmation failed for deposit_id=%d: %v",
			event.DepositID,
			err,
		)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "payment confirmation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// Keep this import out of the handler unless needed elsewhere.
var _ = os.Getenv
