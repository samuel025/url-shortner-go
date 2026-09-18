package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"uuid"

	"github.com/lib/pq"
)

type UserModel struct {
	DB *sql.DB
}

type User struct {
	ID           uuid.UUID `db:"id" json:"id"`
	Name         string    `db:"name" json:"name"`
	Email        string    `db:"email" json:"email"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

var (
	ErrDuplicateEmail = errors.New("duplicate email")
	ErrNotFound       = errors.New("resource not found")
)

func (m *UserModel) Insert(user *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := "INSERT INTO users (id, name, email, password_hash) VALUES ($1, $2, $3, $4) RETURNING name"

	err := m.DB.QueryRowContext(ctx, query, user.ID, user.Name, user.Email, user.PasswordHash).Scan(&user.Name)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return ErrDuplicateEmail
		}
		return err
	}

	return nil
}

func (m *UserModel) GetByEmail(email string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `
		SELECT id, name, email, password_hash, created_at
		FROM users
		WHERE email = $1`

	var user User
	err := m.DB.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &user, nil
}

func (m *UserModel) GetById(id string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `
		SELECT id, name, email, password_hash, created_at
		FROM users
		WHERE id = $1`

	var user User
	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &user, nil
}