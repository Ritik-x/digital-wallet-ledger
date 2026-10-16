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
