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

type createDepositRequest struct {
	WalletID    int64  `json:"wallet_id" binding:"required,gt=0"`
	AmountMinor int64  `json:"amount_minor" binding:"required,gt=0"`
	Currency    string `json:"currency" binding:"required,len=3"`
}

func (h *Handler) CreateDeposit(c *gin.Context) {
	var req createDepositRequest

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
