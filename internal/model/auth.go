package model

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

type Session struct {
	ID        string
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

const sessionDuration = 30 * 24 * time.Hour // 30 days

func CreateSession(db *sql.DB, userID int64) (*Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	token := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().UTC().Add(sessionDuration)

	_, err := db.Exec(
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		token, userID, expiresAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}

	return &Session{
		ID:        token,
		UserID:    userID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}, nil
}

// FindSession looks up a session by token and returns it if valid (not expired).
func FindSession(db *sql.DB, token string) (*Session, error) {
	s := &Session{}
	var createdAt, expiresAt string
	err := db.QueryRow(
		"SELECT id, user_id, created_at, expires_at FROM sessions WHERE id = ? AND expires_at > ?",
		token, time.Now().UTC().Format(time.RFC3339),
	).Scan(&s.ID, &s.UserID, &createdAt, &expiresAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)

	// Roll the expiry forward on activity.
	newExpiry := time.Now().UTC().Add(sessionDuration).Format(time.RFC3339)
	db.Exec("UPDATE sessions SET expires_at = ? WHERE id = ?", newExpiry, token)

	return s, nil
}

func DeleteSession(db *sql.DB, token string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE id = ?", token)
	return err
}

func DeleteExpiredSessions(db *sql.DB) (int64, error) {
	result, err := db.Exec(
		"DELETE FROM sessions WHERE expires_at <= ?",
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
