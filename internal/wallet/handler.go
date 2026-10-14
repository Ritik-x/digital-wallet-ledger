package wallet

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createWalletRequest struct {
	UserID   int64  `json:"user_id" binding:"required,gt=0"`
	Currency string `json:"currency" binding:"required,len=3"`
}

func (h *Handler) CreateWallet(c *gin.Context) {
	var req createWalletRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	input := CreateWalletInput{
		UserID:   req.UserID,
		Currency: req.Currency,
	}

	wallet, err := h.service.CreateWallet(
		c.Request.Context(),
		input,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidUserID),
			errors.Is(err, ErrInvalidCurrency):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})

		case errors.Is(err, ErrUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": "user not found",
			})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "could not create wallet",
			})
		}
		return
	}

	c.JSON(http.StatusCreated, wallet)
}

func (h *Handler) GetBalance(c *gin.Context) {
	walletID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || walletID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid wallet ID",
		})
		return
	}

	balance, err := h.service.GetBalance(
		c.Request.Context(),
		walletID,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidWalletID):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})

		case errors.Is(err, ErrWalletNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": "wallet not found",
			})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "could not retrieve wallet balance",
			})
		}
		return
	}

	c.JSON(http.StatusOK, balance)
}

var _ = time.Time{}