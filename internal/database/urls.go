package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"uuid"

	"github.com/lib/pq"
)

var (
	ErrDuplicateShortCode = errors.New("duplicate short code")
)

type URL struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	UserID      uuid.UUID  `db:"user_id" json:"user_id"`
	ShortCode   string     `db:"short_code" json:"short_code"`
	OriginalURL string     `db:"original_url" json:"original_url"`
	ClickCount  int64      `db:"click_count" json:"click_count"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	ExpiresAt   *time.Time `db:"expires_at" json:"expires_at"`
	IsActive    bool       `db:"is_active" json:"is_active"`
}

type URLModel struct {
	DB *sql.DB
}

func (m *URLModel) Insert(u *URL) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `
		INSERT INTO urls (id, user_id, short_code, original_url, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING click_count, created_at, is_active`

	err := m.DB.QueryRowContext(ctx, query, u.ID, u.UserID, u.ShortCode, u.OriginalURL, u.ExpiresAt).
		Scan(&u.ClickCount, &u.CreatedAt, &u.IsActive)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return ErrDuplicateShortCode
		}
		return err
	}

	return nil
}
