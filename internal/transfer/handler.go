package transfer

import (
	"digital-wallet/internal/auth"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createTransferRequest struct {
	SenderWalletID   int64  `json:"sender_wallet_id" binding:"required,gt=0"`
	ReceiverWalletID int64  `json:"receiver_wallet_id" binding:"required,gt=0"`
	AmountMinor      int64  `json:"amount_minor" binding:"required,gt=0"`
	Currency         string `json:"currency" binding:"required,len=3"`
}

func (h *Handler) CreateTransfer(c *gin.Context) {
	var req createTransferRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))

	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Idempotency-Key header is required",
		})
		return
	}

	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}
	input := CreateTransferInput{
		UserID:           userID,
		SenderWalletID:   req.SenderWalletID,
		ReceiverWalletID: req.ReceiverWalletID,
		AmountMinor:      req.AmountMinor,
		Currency:         req.Currency,
	}

	result, err := h.service.CreateTransfer(
		c.Request.Context(),
		input,
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidWalletID),
			errors.Is(err, ErrSameWallet),
			errors.Is(err, ErrInvalidAmount),
			errors.Is(err, ErrInvalidCurrency):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})

		case errors.Is(err, ErrWalletNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": "wallet not found",
			})

		case errors.Is(err, ErrWalletUnavailable):
			c.JSON(http.StatusConflict, gin.H{
				"error": "one or both wallets are unavailable",
			})

		case errors.Is(err, ErrCurrencyMismatch):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "currency mismatch",
			})

		case errors.Is(err, ErrInsufficientFunds):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "insufficient funds",
			})
		case errors.Is(err, ErrIdempotencyConflict):
			c.JSON(http.StatusConflict, gin.H{
				"error": "Idempotency-Key was already used for a different transfer",
			})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "could not complete transfer",
			})
		}
		return
	}

	c.JSON(http.StatusCreated, result)
}
