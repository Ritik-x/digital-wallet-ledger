package history

import (
	"errors"
	"net/http"
	"strconv"

	"digital-wallet/internal/auth"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// List godoc
// @Summary List wallet transactions
// @Description Retrieves paginated transaction history for a wallet belonging to the authenticated user.
// @Tags Transaction History
// @Security BearerAuth
// @Produce json
// @Param id path int true "Wallet ID"
// @Param limit query int false "Maximum number of transactions to return" default(20)
// @Param offset query int false "Number of transactions to skip" default(0)
// @Success 200 {object} ListResult "Transaction history"
// @Failure 400 {object} map[string]interface{} "Invalid wallet ID, limit, or offset"
// @Failure 401 {object} map[string]interface{} "Authentication required"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/wallets/{id}/transactions [get]
func (h *Handler) List(c *gin.Context) {

	userID, ok := auth.UserIDFromContext(c)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	walletID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || walletID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid wallet ID",
		})
		return
	}

	limit := 20
	if rawLimit := c.Query("limit"); rawLimit != "" {
		limit, err = strconv.Atoi(rawLimit)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid limit",
			})
			return
		}
	}

	offset := 0
	if rawOffset := c.Query("offset"); rawOffset != "" {
		offset, err = strconv.Atoi(rawOffset)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid offset",
			})
			return
		}
	}

	result, err := h.service.List(
		c.Request.Context(),
		ListInput{
			UserID:   userID,
			WalletID: walletID,
			Limit:    limit,
			Offset:   offset,
		},
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidUserID):
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
			})
		case errors.Is(err, ErrInvalidWalletID),
			errors.Is(err, ErrInvalidLimit),
			errors.Is(err, ErrInvalidOffset):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "could not retrieve transaction history",
			})
		}
		return
	}

	c.JSON(http.StatusOK, result)
}
