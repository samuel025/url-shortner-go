package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/samuel025/url-shortner-go/internal/database"
)

var ErrCacheMiss = errors.New("cache miss")

type Client struct {
	rdb *redis.Client
}

func New(addr string) (*Client, error) {
	if addr == "" {
		return nil, nil
	}

	opt, err := redis.ParseURL(addr)
	if err != nil {
		opt = &redis.Options{
			Addr: addr,
		}
	}

	rdb := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &Client{rdb: rdb}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

func (c *Client) SetURL(ctx context.Context, u *database.URL, ttl time.Duration) error {
	if c == nil || c.rdb == nil || u == nil {
		return nil
	}

	data, err := json.Marshal(u)
	if err != nil {
		return err
	}

	return c.rdb.Set(ctx, "url:"+u.ShortCode, data, ttl).Err()
}

func (c *Client) GetURL(ctx context.Context, shortCode string) (*database.URL, error) {
	if c == nil || c.rdb == nil {
		return nil, ErrCacheMiss
	}

	data, err := c.rdb.Get(ctx, "url:"+shortCode).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}

	var u database.URL
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}

	return &u, nil
}

func (c *Client) DeleteURL(ctx context.Context, shortCode string) error {
	if c == nil || c.rdb == nil {
		return nil
	}

	return c.rdb.Del(ctx, "url:"+shortCode).Err()
}
