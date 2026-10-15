package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidPassword    = errors.New("password must be 8-72 bytes")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

type Service struct {
	repo      UserRepository
	jwtSecret string
}

func NewService(repo UserRepository, jwtSecret string) *Service {
	return &Service{
		repo:      repo,
		jwtSecret: jwtSecret,
	}
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", ErrInvalidEmail
	}

	return email, nil
}

func (s *Service) Register(
	ctx context.Context,
	email string,
	password string,
) (*User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}

	if len(password) < 8 || len(password) > 72 {
		return nil, ErrInvalidPassword
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.Create(ctx, email, string(passwordHash))
	if err != nil {
		return nil, err
	}

	// Never return the password hash from registration.
	user.PasswordHash = ""
	return user, nil
}

func (s *Service) Login(
	ctx context.Context,
	email string,
	password string,
) (string, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return "", ErrInvalidCredentials
	}

	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(password),
	); err != nil {
		return "", ErrInvalidCredentials
	}

	token, err := GenerateToken(
		user.ID,
		s.jwtSecret,
		15*time.Minute,
	)
	if err != nil {
		return "", err
	}

	return token, nil
}
