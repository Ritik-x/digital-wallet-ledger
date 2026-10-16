package transfer

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"digital-wallet/internal/auth"

	"github.com/gin-gonic/gin"
)

type ReversalHandler struct {
	service *ReversalService
}

func NewReversalHandler(service *ReversalService) *ReversalHandler {
	return &ReversalHandler{service: service}
}

func (h *ReversalHandler) Reverse(c *gin.Context) {
	userID, ok := auth.UserIDFromContext(c)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	transferID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || transferID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid transfer ID",
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

	result, err := h.service.Reverse(
		c.Request.Context(),
		ReverseTransferInput{
			UserID:         userID,
			TransferID:     transferID,
			IdempotencyKey: key,
		},
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidReversalUserID):
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
			})

		case errors.Is(err, ErrInvalidReversalTransferID),
			errors.Is(err, ErrInvalidReversalKey):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})

		case errors.Is(err, ErrTransferNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": "transfer not found",
			})

		case errors.Is(err, ErrTransferAlreadyReversed),
			errors.Is(err, ErrTransferNotReversible),
			errors.Is(err, ErrReversalIdempotencyConflict),
			errors.Is(err, ErrWalletUnavailable):
			c.JSON(http.StatusConflict, gin.H{
				"error": "transfer cannot be reversed",
			})

		case errors.Is(err, ErrReversalInsufficientFunds):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "receiver has insufficient funds for reversal",
			})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "could not reverse transfer",
			})
		}
		return
	}

	c.JSON(http.StatusOK, result)
}
