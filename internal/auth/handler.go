package auth

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

type CredentialsRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Register godoc
// @Summary Register a new user
// @Description Creates a user account.
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body CredentialsRequest true "Login credentials"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var req CredentialsRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "email and password are required",
		})
		return
	}

	user, err := h.service.Register(
		c.Request.Context(),
		req.Email,
		req.Password,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidEmail),
			errors.Is(err, ErrInvalidPassword):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
		case errors.Is(err, ErrEmailAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{
				"error": "email already registered",
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "could not register user",
			})
		}
		return
	}

	c.JSON(http.StatusCreated, user)
}

// Login godoc
// @Summary      Log in
// @Description  Authenticates a user and returns a JWT access token.
// @Tags         Authentication
// @Accept       json
// @Produce      json
// @Param request body CredentialsRequest true "Login credentials"
// @Success      200 {object} map[string]interface{} "Access token"
// @Failure      400 {object} map[string]interface{} "Invalid request"
// @Failure      401 {object} map[string]interface{} "Invalid credentials"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req CredentialsRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "email and password are required",
		})
		return
	}

	token, err := h.service.Login(
		c.Request.Context(),
		req.Email,
		req.Password,
	)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid email or password",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "could not log in",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   900,
	})
}
