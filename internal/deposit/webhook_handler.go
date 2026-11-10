package deposit

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

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

type RazorpayWebhookEvent struct {
	Event   string `json:"event"`
	Payload struct {
		Payment struct {
			Entity struct {
				ID       string `json:"id"`
				OrderID  string `json:"order_id"`
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
				Status   string `json:"status"`
				Notes    struct {
					DepositID string `json:"deposit_id"`
				} `json:"notes"`
			} `json:"entity"`
		} `json:"payment"`
	} `json:"payload"`
}

// Handle godoc
// @Summary Handle Razorpay payment webhook
// @Description Verifies Razorpay's webhook signature and confirms captured payments.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param X-Razorpay-Signature header string true "Razorpay webhook signature"
// @Success 200 {object} WebhookResponse
// @Router /api/v1/webhooks/razorpay [post]
func (h *WebhookHandler) Handle(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (1<<20)+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read webhook body"})
		return
	}

	if len(body) > 1<<20 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "webhook payload too large"})
		return
	}

	signature := c.GetHeader("X-Razorpay-Signature")

	if !VerifyWebhookSignature(body, signature, h.secret) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook signature"})
		return
	}

	var event RazorpayWebhookEvent

	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook payload"})
		return
	}

	// We only credit wallets after Razorpay confirms capture.
	if event.Event != "payment.captured" {
		c.JSON(http.StatusOK, WebhookResponse{Status: "ignored"})
		return
	}

	payment := event.Payload.Payment.Entity

	if payment.Status != "captured" ||
		payment.ID == "" ||
		payment.OrderID == "" ||
		payment.Amount <= 0 ||
		len(payment.Currency) != 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid captured payment details"})
		return
	}

	depositID, err := strconv.ParseInt(payment.Notes.DepositID, 10, 64)
	if err != nil || depositID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deposit ID"})
		return
	}

	err = h.service.ConfirmPayment(
		c.Request.Context(),
		depositID,
		payment.OrderID,
		payment.ID,
		payment.Amount,
		payment.Currency,
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "payment confirmation failed"})
		return
	}

	c.JSON(http.StatusOK, WebhookResponse{Status: "received"})
}
