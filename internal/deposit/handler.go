package deposit

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type CreateDepositRequest struct {
	WalletID    int64  `json:"wallet_id" binding:"required,gt=0" minimum:"1"`
	AmountMinor int64  `json:"amount_minor" binding:"required,gt=0" minimum:"1"`
	Currency    string `json:"currency" binding:"required,len=3" minLength:"3" maxLength:"3"`
}

// CreateDeposit godoc
// @Summary Create a deposit
// @Description Credits an eligible wallet through the deposit service. Amount is expressed in minor currency units (paise for INR).
// @Tags Deposits
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "Unique key for this deposit request"
// @Param request body CreateDepositRequest true "Deposit details"
// @Success 201 {object} Deposit "Deposit created"
// @Failure 400 {object} map[string]interface{} "Invalid request or missing idempotency key"
// @Failure 404 {object} map[string]interface{} "Wallet not found"
// @Failure 409 {object} map[string]interface{} "Wallet unavailable or idempotency conflict"
// @Failure 422 {object} map[string]interface{} "Currency mismatch"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/deposits [post]
func (h *Handler) CreateDeposit(c *gin.Context) {

	var req CreateDepositRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid deposit request",
		})
		return
	}

	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Idempotency-Key header is required",
		})
		return
	}

	result, err := h.service.Create(
		c.Request.Context(),
		CreateDepositInput{
			WalletID:       req.WalletID,
			AmountMinor:    req.AmountMinor,
			Currency:       req.Currency,
			IdempotencyKey: key,
		},
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidWalletID),
			errors.Is(err, ErrInvalidAmount),
			errors.Is(err, ErrInvalidCurrency),
			errors.Is(err, ErrInvalidIdempotency):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})

		case errors.Is(err, ErrWalletNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})

		case errors.Is(err, ErrWalletUnavailable),
			errors.Is(err, ErrIdempotencyConflict):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})

		case errors.Is(err, ErrCurrencyMismatch):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "could not create deposit",
			})
		}
		return
	}

	c.JSON(http.StatusCreated, result)
}
