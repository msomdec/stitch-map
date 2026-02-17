package model

import (
	"database/sql"
	"fmt"
)

type Stitch struct {
	ID           int64
	UserID       *int64 // nil for built-in
	Name         string
	Abbreviation string
	Description  string
	IsBuiltin    bool
}

// ListStitchesForUser returns all built-in stitches plus the user's custom stitches.
func ListStitchesForUser(db *sql.DB, userID int64) ([]Stitch, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, abbreviation, description, is_builtin
		FROM stitches
		WHERE user_id IS NULL OR user_id = ?
		ORDER BY is_builtin DESC, name ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list stitches: %w", err)
	}
	defer rows.Close()

	return scanStitches(rows)
}

// ListBuiltinStitches returns only the built-in stitches.
func ListBuiltinStitches(db *sql.DB) ([]Stitch, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, abbreviation, description, is_builtin
		FROM stitches
		WHERE is_builtin = 1
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list builtin stitches: %w", err)
	}
	defer rows.Close()

	return scanStitches(rows)
}

// ListCustomStitches returns only the user's custom stitches.
func ListCustomStitches(db *sql.DB, userID int64) ([]Stitch, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, abbreviation, description, is_builtin
		FROM stitches
		WHERE user_id = ?
		ORDER BY name ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list custom stitches: %w", err)
	}
	defer rows.Close()

	return scanStitches(rows)
}

func FindStitchByID(db *sql.DB, id int64) (*Stitch, error) {
	s := &Stitch{}
	var userID sql.NullInt64
	err := db.QueryRow(`
		SELECT id, user_id, name, abbreviation, description, is_builtin
		FROM stitches WHERE id = ?
	`, id).Scan(&s.ID, &userID, &s.Name, &s.Abbreviation, &s.Description, &s.IsBuiltin)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		s.UserID = &userID.Int64
	}
	return s, nil
}

func CreateStitch(db *sql.DB, userID int64, name, abbreviation, description string) (*Stitch, error) {
	result, err := db.Exec(`
		INSERT INTO stitches (user_id, name, abbreviation, description, is_builtin)
		VALUES (?, ?, ?, ?, 0)
	`, userID, name, abbreviation, description)
	if err != nil {
		return nil, fmt.Errorf("insert stitch: %w", err)
	}

	id, _ := result.LastInsertId()
	uid := userID
	return &Stitch{
		ID:           id,
		UserID:       &uid,
		Name:         name,
		Abbreviation: abbreviation,
		Description:  description,
		IsBuiltin:    false,
	}, nil
}

func UpdateStitch(db *sql.DB, id, userID int64, name, abbreviation, description string) error {
	result, err := db.Exec(`
		UPDATE stitches SET name = ?, abbreviation = ?, description = ?
		WHERE id = ? AND user_id = ? AND is_builtin = 0
	`, name, abbreviation, description, id, userID)
	if err != nil {
		return fmt.Errorf("update stitch: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stitch not found or not owned by user")
	}
	return nil
}

func DeleteStitch(db *sql.DB, id, userID int64) error {
	result, err := db.Exec(`
		DELETE FROM stitches WHERE id = ? AND user_id = ? AND is_builtin = 0
	`, id, userID)
	if err != nil {
		return fmt.Errorf("delete stitch: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stitch not found, not owned by user, or is built-in")
	}
	return nil
}

func scanStitches(rows *sql.Rows) ([]Stitch, error) {
	var stitches []Stitch
	for rows.Next() {
		var s Stitch
		var userID sql.NullInt64
		if err := rows.Scan(&s.ID, &userID, &s.Name, &s.Abbreviation, &s.Description, &s.IsBuiltin); err != nil {
			return nil, fmt.Errorf("scan stitch: %w", err)
		}
		if userID.Valid {
			s.UserID = &userID.Int64
		}
		stitches = append(stitches, s)
	}
	return stitches, rows.Err()
}
