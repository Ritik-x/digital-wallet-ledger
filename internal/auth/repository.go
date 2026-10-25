package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

var ErrEmailAlreadyExists = errors.New("email already registered")

type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserRepository interface {
	Create(ctx context.Context, email, passwordHash string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
}
type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) Create(
	ctx context.Context,
	email string,
	passwordHash string,
) (*User, error) {
	var user User

	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO users (email, password_hash)
		 VALUES ($1, $2)
		 RETURNING id, email, password_hash ,role ,created_at `,
		email,
		passwordHash,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role,
		&user.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "users_email_key" {
			return nil, ErrEmailAlreadyExists
		}

		return nil, fmt.Errorf("create user: %w", err)
	}

	return &user, nil
}

// func (r *PostgresUserRepository) GetByEmail(
// 	ctx context.Context,
// 	email string,
// ) (*User, error) {
// 	var user User

// 	err := r.db.QueryRowContext(
// 		ctx,
// 		`SELECT id, email, password_hash , role, created_at
// 		 FROM users
// 		 WHERE email = $1`,
// 		email,
// 	).Scan(&user.ID, &user.Email, &user.PasswordHash)

// 	if errors.Is(err, sql.ErrNoRows) {
// 		return nil, sql.ErrNoRows
// 	}
// 	if err != nil {
// 		return nil, fmt.Errorf("get user by email: %w", err)
// 	}

// 	return &user, nil
// }

func (r *PostgresUserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (*User, error) {
	var user User

	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, email, password_hash, role, created_at
         FROM users
         WHERE email = $1`,
		email,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}

	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	return &user, nil
}
