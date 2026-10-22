package wallet

import (
	"digital-wallet/internal/auth"
	"errors"
	"log"
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

type CreateWalletRequest struct {
	Currency string `json:"currency" binding:"required,len=3"`
}

// CreateWallet godoc
// @Summary Create a wallet
// @Description Creates a wallet for the authenticated user.
// @Tags Wallets
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body CreateWalletRequest true "Wallet details"
// @Success 201 {object} Wallet "Wallet created"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Authentication required"
// @Failure 404 {object} map[string]interface{} "User not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/wallets [post]
func (h *Handler) CreateWallet(c *gin.Context) {
	var req CreateWalletRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	input := CreateWalletInput{
		UserID:   userID,
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
			log.Printf("CreateWallet failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "could not create wallet",
			})
		}
		return
	}

	c.JSON(http.StatusCreated, wallet)
}

// GetBalance godoc
// @Summary Get wallet balance
// @Description Retrieves the ledger-derived balance for a wallet.
// @Tags Wallets
// @Security BearerAuth
// @Produce json
// @Param id path int true "Wallet ID"
// @Success 200 {object} Balance "Wallet balance"
// @Failure 400 {object} map[string]interface{} "Invalid wallet ID"
// @Failure 401 {object} map[string]interface{} "Authentication required"
// @Failure 404 {object} map[string]interface{} "Wallet not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/wallets/{id}/balance [get]
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
			log.Printf("GetWalletBalance failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "could not retrieve wallet balance",
			})
		}
		return
	}

	c.JSON(http.StatusOK, balance)
}

var _ = time.Time{}
