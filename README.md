# URL Shortener REST API

A production-grade REST API service built with Go and PostgreSQL that converts long URLs into short, trackable links.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Database Design](#database-design)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Running with Docker](#running-with-docker)
  - [Running Locally](#running-locally)
- [Environment Variables](#environment-variables)
- [API Reference](#api-reference)
- [End-to-End Demo Walkthrough](#end-to-end-demo-walkthrough)
- [Error Handling](#error-handling)
- [Security Features](#security-features)
- [Advanced Extensions](#advanced-extensions)

---

## Overview

This service shortens URLs into unique Base62 codes, persists mappings in PostgreSQL, and serves HTTP 302 redirects with click counting, expiration validation, and resource ownership protection.

### Key Capabilities

- User registration and authentication using bcrypt password hashing and signed JWT tokens.
- Secure, collision-resistant short code generation with automatic retry.
- HTTP 302 Found redirects with real-time click tracking.
- Optional expiration dates with automated HTTP 410 Gone handling.
- Owner-restricted analytics and deletion operations.
- Built-in in-memory rate limiting, CORS headers, and request body size constraints.

---

## Architecture

### Current Version 1 Architecture

```
Client / Browser
       │
       ▼
 Go REST API (Gin Engine)
   ├── CORS & Body Limit Middleware
   ├── In-Memory Token Bucket Rate Limiter
   ├── JWT Authentication Middleware
   └── Handlers (Auth, URLs, Redirects)
       │
       ▼
 PostgreSQL 16 Database
   ├── users (Credentials & timestamps)
   └── urls  (Mappings, click counts, expiration)
```

---

## Tech Stack

- Language: Go (1.24+)
- Web Framework: Gin
- Database: PostgreSQL 16
- Database Driver: lib/pq
- Authentication: JWT (golang-jwt/jwt) and bcrypt (golang.org/x/crypto/bcrypt)
- Rate Limiting: Token Bucket via golang.org/x/time/rate
- Containerization: Docker and Docker Compose (Multi-stage Alpine image)

---

## Database Design

### Schema

#### users
| Column | Type | Constraints |
| :--- | :--- | :--- |
| id | UUID | PRIMARY KEY |
| name | VARCHAR(100) | NOT NULL, DEFAULT '' |
| email | VARCHAR(255) | UNIQUE, NOT NULL |
| password_hash | TEXT | NOT NULL |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() |

#### urls
| Column | Type | Constraints |
| :--- | :--- | :--- |
| id | UUID | PRIMARY KEY |
| user_id | UUID | REFERENCES users(id) ON DELETE CASCADE |
| short_code | VARCHAR(16) | UNIQUE, NOT NULL |
| original_url | TEXT | NOT NULL |
| click_count | BIGINT | NOT NULL, DEFAULT 0 |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() |
| expires_at | TIMESTAMPTZ | NULLABLE |
| is_active | BOOLEAN | NOT NULL, DEFAULT TRUE |

Indexes:
- UNIQUE index on `urls.short_code`
- Compound index on `urls(user_id, created_at)`
- Optional index on `urls(expires_at)`

---

## Getting Started

### Prerequisites

- Docker and Docker Compose (recommended), or
- Go 1.24+ and a local PostgreSQL 16 server

### Running with Docker

The fastest way to run the complete stack (PostgreSQL + API):

```bash
docker compose up --build -d
```

To stop:

```bash
docker compose down
```

### Running Locally

1. Start PostgreSQL:
   ```bash
   docker compose up -d postgres
   ```

2. Configure environment:
   ```bash
   cp .env.example .env
   ```

3. Run migrations:
   ```bash
   go run ./cmd/migrate up
   ```

4. Start the server:
   ```bash
   go run ./cmd/api
   ```

---

## Environment Variables

| Variable | Description | Default |
| :--- | :--- | :--- |
| `PORT` | Port the HTTP server listens on | `8080` |
| `DATABASE_URL` | PostgreSQL connection string | `postgresql://test_user:test_password@localhost:5433/shortener_url?sslmode=disable` |
| `JWT_SECRET` | Secret key for signing and verifying JWTs | `some-secret-123456` |
| `BASE_URL` | Base URL used to construct short URLs in responses | Dynamic host detection or `http://localhost:8080` |

---

## API Reference

| Method | Endpoint | Auth Required | Description |
| :--- | :--- | :--- | :--- |
| GET | `/health` | No | System health check |
| POST | `/api/v1/auth/register` | No | Register a new user |
| POST | `/api/v1/auth/login` | No | Authenticate user and receive JWT |
| POST | `/api/v1/urls` | Yes | Create a shortened URL |
| GET | `/api/v1/urls` | Yes | List authenticated user's URLs (paginated) |
| GET | `/api/v1/urls/:code` | No | Get public metadata for a short code |
| GET | `/api/v1/urls/:code/stats` | Yes (Owner) | Get private click statistics for an owned URL |
| DELETE | `/api/v1/urls/:code` | Yes (Owner) | Delete an owned URL |
| GET | `/:code` | No | Redirect to the original URL (HTTP 302) |

---

## End-to-End Demo Walkthrough

### 1. Register a New Account

```bash
curl -i -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Jane Doe",
    "email": "jane@example.com",
    "password": "securepassword123"
  }'
```

Response:
```json
{
  "message": "User registered successfully"
}
```

### 2. Log In

```bash
curl -i -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "jane@example.com",
    "password": "securepassword123"
  }'
```

Save the returned JWT token:
```bash
export TOKEN="<jwt-token-string>"
```

### 3. Create a Shortened URL

```bash
curl -i -X POST http://localhost:8080/api/v1/urls \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://github.com/samuel025/url-shortner-go",
    "expires_in_days": 30
  }'
```

Response:
```json
{
  "short_code": "k9B2xM",
  "short_url": "http://localhost:8080/k9B2xM",
  "original_url": "https://github.com/samuel025/url-shortner-go",
  "expires_at": "2026-10-29T00:00:00Z"
}
```

### 4. Visit the Short URL (Redirect)

```bash
curl -i http://localhost:8080/k9B2xM
```

Response:
```http
HTTP/1.1 302 Found
Location: https://github.com/samuel025/url-shortner-go
```

To follow the redirect automatically:
```bash
curl -IL http://localhost:8080/k9B2xM
```

### 5. Inspect URL Statistics (Owner Only)

```bash
curl -i -X GET http://localhost:8080/api/v1/urls/k9B2xM/stats \
  -H "Authorization: Bearer $TOKEN"
```

Response:
```json
{
  "id": "e8913b41-9488-410a-b3e3-7ceca8121650",
  "short_code": "k9B2xM",
  "short_url": "http://localhost:8080/k9B2xM",
  "original_url": "https://github.com/samuel025/url-shortner-go",
  "click_count": 1,
  "created_at": "2026-09-29T00:00:00Z",
  "expires_at": "2026-10-29T00:00:00Z",
  "is_active": true
}
```

### 6. List User URLs with Pagination

```bash
curl -i -X GET "http://localhost:8080/api/v1/urls?page=1&limit=10" \
  -H "Authorization: Bearer $TOKEN"
```

Response:
```json
{
  "urls": [
    {
      "id": "e8913b41-9488-410a-b3e3-7ceca8121650",
      "short_code": "k9B2xM",
      "short_url": "http://localhost:8080/k9B2xM",
      "original_url": "https://github.com/samuel025/url-shortner-go",
      "click_count": 1,
      "created_at": "2026-09-29T00:00:00Z",
      "expires_at": "2026-10-29T00:00:00Z",
      "is_active": true
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 10,
    "total_records": 1,
    "total_pages": 1
  }
}
```

### 7. Delete an Owned URL

```bash
curl -i -X DELETE http://localhost:8080/api/v1/urls/k9B2xM \
  -H "Authorization: Bearer $TOKEN"
```

Response:
```json
{
  "message": "URL deleted successfully"
}
```

Visiting the deleted URL returns `404 Not Found`.

---

## Error Handling

All error responses adhere to a consistent JSON shape:

```json
{
  "error": {
    "code": "INVALID_URL",
    "message": "The supplied URL is invalid. It must begin with http:// or https:// and be well-formed."
  }
}
```

Common status codes and error codes:
- `400 Bad Request`: `INVALID_REQUEST`, `INVALID_URL`, `INVALID_CODE`, `INVALID_EXPIRATION`
- `401 Unauthorized`: Missing or malformed Bearer token
- `403 Forbidden`: `FORBIDDEN` (attempting to access or delete another user's URL)
- `404 Not Found`: `NOT_FOUND`
- `410 Gone`: `EXPIRED_URL` (URL expired or deactivated)
- `429 Too Many Requests`: `RATE_LIMITED`
- `500 Internal Server Error`: `INTERNAL_SERVER_ERROR`

---

## Security Features

- Cryptographic Passwords: Passwords hashed using bcrypt with default cost.
- Secure Short Codes: Generated with cryptographic random bytes against a 62-character alphanumeric alphabet (`[a-zA-Z0-9]`).
- Collision Resolution: Insertion retry mechanism resolves occasional collisions safely.
- In-Memory Rate Limiting: Token bucket algorithm isolates clients by IP with automated background garbage collection.
- Request Body Limit: Global 1 MB ceiling on request payload size prevents memory exhaustion.
- CORS Configuration: Deliberate CORS headers with HTTP OPTIONS preflight handling.
- Ownership Verification: URL statistics and deletion requests check user ownership before mutating state.

---

