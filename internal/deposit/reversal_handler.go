package deposit

import (
	"errors"
	"net/http"
	"strconv"

	"digital-wallet/internal/auth"

	"github.com/gin-gonic/gin"
)

type ReversalHandler struct {
	service *ReversalService
}

func NewReversalHandler(service *ReversalService) *ReversalHandler {
	return &ReversalHandler{
		service: service,
	}
}

// Reverse godoc
// @Summary Reverse a deposit
// @Description Reverses an eligible deposit, subject to authorization and sufficient wallet funds.
// @Tags Deposits
// @Security BearerAuth
// @Produce json
// @Param id path int true "Deposit ID"
// @Param Idempotency-Key header string true "Unique key for this reversal request"
// @Success 200 {object} DepositReversal "Deposit reversed"
// @Failure 400 {object} map[string]interface{} "Invalid deposit ID or missing idempotency key"
// @Failure 401 {object} map[string]interface{} "Authentication required"
// @Failure 404 {object} map[string]interface{} "Deposit not found"
// @Failure 409 {object} map[string]interface{} "Deposit cannot be reversed or idempotency conflict"
// @Failure 422 {object} map[string]interface{} "Insufficient funds for reversal"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/deposits/{id}/reverse [post]
func (h *ReversalHandler) Reverse(c *gin.Context) {

	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	depositID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || depositID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid deposit ID",
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

	result, err := h.service.Reverse(c.Request.Context(), ReverseDepositInput{
		UserID:         userID,
		DepositID:      depositID,
		IdempotencyKey: key,
	})

	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidReversalUserID):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})

		case errors.Is(err, ErrInvalidReversalDepositID),
			errors.Is(err, ErrInvalidDepositReversalKey):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})

		case errors.Is(err, ErrDepositNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "deposit not found"})

		case errors.Is(err, ErrDepositAlreadyReversed),
			errors.Is(err, ErrDepositNotReversible),
			errors.Is(err, ErrDepositReversalIdempotencyConflict),
			errors.Is(err, ErrDepositWalletUnavailable):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})

		case errors.Is(err, ErrDepositReversalInsufficientFunds):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to reverse deposit",
			})
		}
		return
	}

	c.JSON(http.StatusOK, result)
}
