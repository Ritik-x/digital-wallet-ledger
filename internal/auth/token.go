package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken  = errors.New("invalid or expired token")
	ErrInvalidUserID = errors.New("user ID must be positive")
)

type Claims struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func GenerateToken(
	userID int64,
	role string,
	secret string,
	ttl time.Duration,
) (string, error) {
	if userID <= 0 {
		return "", errors.New("user ID must be positive")
	}

	if role != "customer" && role != "support" && role != "admin" {
		return "", errors.New("invalid user role")
	}

	if len(secret) < 32 {
		return "", errors.New("JWT secret must be at least 32 characters")
	}

	if ttl <= 0 {
		return "", errors.New("token TTL must be positive")
	}

	now := time.Now()

	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "digital-wallet",
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

func ParseToken(
	tokenString string,
	secret string,
) (*Claims, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT secret must be at least 32 bytes")
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidToken
			}

			return []byte(secret), nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer("digital-wallet"),
		jwt.WithExpirationRequired(),
	)
	if err != nil || token == nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	if claims.UserID <= 0 ||
		claims.Subject != strconv.FormatInt(claims.UserID, 10) {
		return nil, ErrInvalidToken
	}

	if claims.Role != "customer" &&
		claims.Role != "support" &&
		claims.Role != "admin" {
		return nil, errors.New("invalid user role")
	}

	return claims, nil
}
