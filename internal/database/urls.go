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

func (m *URLModel) GetByShortCode(shortCode string) (*URL, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `
		SELECT id, user_id, short_code, original_url, click_count, created_at, expires_at, is_active
		FROM urls
		WHERE short_code = $1`

	var u URL
	err := m.DB.QueryRowContext(ctx, query, shortCode).Scan(
		&u.ID,
		&u.UserID,
		&u.ShortCode,
		&u.OriginalURL,
		&u.ClickCount,
		&u.CreatedAt,
		&u.ExpiresAt,
		&u.IsActive,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &u, nil
}

func (m *URLModel) IncrementClickCount(id uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `
		UPDATE urls
		SET click_count = click_count + 1
		WHERE id = $1`

	_, err := m.DB.ExecContext(ctx, query, id)
	return err
}

func (m *URLModel) GetByUserID(userID uuid.UUID, limit, offset int) ([]URL, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var totalRecords int
	countQuery := `SELECT COUNT(*) FROM urls WHERE user_id = $1`
	err := m.DB.QueryRowContext(ctx, countQuery, userID).Scan(&totalRecords)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, user_id, short_code, original_url, click_count, created_at, expires_at, is_active
		FROM urls
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := m.DB.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	urls := make([]URL, 0)
	for rows.Next() {
		var u URL
		err := rows.Scan(
			&u.ID,
			&u.UserID,
			&u.ShortCode,
			&u.OriginalURL,
			&u.ClickCount,
			&u.CreatedAt,
			&u.ExpiresAt,
			&u.IsActive,
		)
		if err != nil {
			return nil, 0, err
		}
		urls = append(urls, u)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, err
	}

	return urls, totalRecords, nil
}


