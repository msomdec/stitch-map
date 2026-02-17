package model

import (
	"database/sql"
	"fmt"
	"time"
)

type Pattern struct {
	ID           int64
	UserID       int64
	Name         string
	Description  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	SectionCount int // populated by list queries
	RowCount     int // populated by list queries
}

type PatternSection struct {
	ID        int64
	PatternID int64
	Position  int
	Name      string
	Notes     string
	Rows      []Row // populated when loading full pattern
}

type Row struct {
	ID                        int64
	SectionID                 int64
	Position                  int
	Label                     string
	Type                      string // "row", "joined_round", "continuous_round"
	ExpectedStitchCount       int
	TurningChainCount         int
	TurningChainCountsAsStitch bool
	RepeatCount               int
	Notes                     string
}

// --- Pattern CRUD ---

func CreatePattern(db *sql.DB, userID int64, name, description string) (*Pattern, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		"INSERT INTO patterns (user_id, name, description) VALUES (?, ?, ?)",
		userID, name, description,
	)
	if err != nil {
		return nil, fmt.Errorf("insert pattern: %w", err)
	}

	patternID, _ := result.LastInsertId()

	// Create default "Main" section.
	_, err = tx.Exec(
		"INSERT INTO pattern_sections (pattern_id, position, name) VALUES (?, 1, 'Main')",
		patternID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert default section: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	now := time.Now().UTC()
	return &Pattern{
		ID:          patternID,
		UserID:      userID,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func ListPatternsByUser(db *sql.DB, userID int64) ([]Pattern, error) {
	rows, err := db.Query(`
		SELECT
			p.id, p.user_id, p.name, p.description, p.created_at, p.updated_at,
			(SELECT COUNT(*) FROM pattern_sections WHERE pattern_id = p.id) as section_count,
			(SELECT COUNT(*) FROM rows r JOIN pattern_sections ps ON r.section_id = ps.id WHERE ps.pattern_id = p.id) as row_count
		FROM patterns p
		WHERE p.user_id = ?
		ORDER BY p.updated_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list patterns: %w", err)
	}
	defer rows.Close()

	var patterns []Pattern
	for rows.Next() {
		var p Pattern
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &createdAt, &updatedAt, &p.SectionCount, &p.RowCount); err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

func FindPatternByID(db *sql.DB, id int64) (*Pattern, error) {
	p := &Pattern{}
	var createdAt, updatedAt string
	err := db.QueryRow(`
		SELECT id, user_id, name, description, created_at, updated_at
		FROM patterns WHERE id = ?
	`, id).Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return p, nil
}

func UpdatePattern(db *sql.DB, id, userID int64, name, description string) error {
	result, err := db.Exec(`
		UPDATE patterns SET name = ?, description = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ? AND user_id = ?
	`, name, description, id, userID)
	if err != nil {
		return fmt.Errorf("update pattern: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pattern not found or not owned by user")
	}
	return nil
}

func DeletePattern(db *sql.DB, id, userID int64) error {
	result, err := db.Exec("DELETE FROM patterns WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return fmt.Errorf("delete pattern: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pattern not found or not owned by user")
	}
	return nil
}

func touchPatternUpdatedAt(tx *sql.Tx, patternID int64) {
	tx.Exec("UPDATE patterns SET updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?", patternID)
}

// --- Section CRUD ---

func ListSectionsByPattern(db *sql.DB, patternID int64) ([]PatternSection, error) {
	rows, err := db.Query(`
		SELECT id, pattern_id, position, name, notes
		FROM pattern_sections
		WHERE pattern_id = ?
		ORDER BY position ASC
	`, patternID)
	if err != nil {
		return nil, fmt.Errorf("list sections: %w", err)
	}
	defer rows.Close()

	var sections []PatternSection
	for rows.Next() {
		var s PatternSection
		if err := rows.Scan(&s.ID, &s.PatternID, &s.Position, &s.Name, &s.Notes); err != nil {
			return nil, fmt.Errorf("scan section: %w", err)
		}
		sections = append(sections, s)
	}
	return sections, rows.Err()
}

func FindSectionByID(db *sql.DB, id int64) (*PatternSection, error) {
	s := &PatternSection{}
	err := db.QueryRow(`
		SELECT id, pattern_id, position, name, notes
		FROM pattern_sections WHERE id = ?
	`, id).Scan(&s.ID, &s.PatternID, &s.Position, &s.Name, &s.Notes)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func CreateSection(db *sql.DB, patternID int64, name string) (*PatternSection, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get next position.
	var maxPos sql.NullInt64
	tx.QueryRow("SELECT MAX(position) FROM pattern_sections WHERE pattern_id = ?", patternID).Scan(&maxPos)
	nextPos := 1
	if maxPos.Valid {
		nextPos = int(maxPos.Int64) + 1
	}

	result, err := tx.Exec(
		"INSERT INTO pattern_sections (pattern_id, position, name) VALUES (?, ?, ?)",
		patternID, nextPos, name,
	)
	if err != nil {
		return nil, fmt.Errorf("insert section: %w", err)
	}

	touchPatternUpdatedAt(tx, patternID)

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &PatternSection{
		ID:        id,
		PatternID: patternID,
		Position:  nextPos,
		Name:      name,
	}, nil
}

func UpdateSection(db *sql.DB, id int64, name, notes string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var patternID int64
	if err := tx.QueryRow("SELECT pattern_id FROM pattern_sections WHERE id = ?", id).Scan(&patternID); err != nil {
		return fmt.Errorf("section not found: %w", err)
	}

	if _, err := tx.Exec("UPDATE pattern_sections SET name = ?, notes = ? WHERE id = ?", name, notes, id); err != nil {
		return fmt.Errorf("update section: %w", err)
	}

	touchPatternUpdatedAt(tx, patternID)
	return tx.Commit()
}

func DeleteSection(db *sql.DB, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var patternID int64
	var position int
	if err := tx.QueryRow("SELECT pattern_id, position FROM pattern_sections WHERE id = ?", id).Scan(&patternID, &position); err != nil {
		return fmt.Errorf("section not found: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM pattern_sections WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete section: %w", err)
	}

	// Reorder remaining sections.
	if _, err := tx.Exec(
		"UPDATE pattern_sections SET position = position - 1 WHERE pattern_id = ? AND position > ?",
		patternID, position,
	); err != nil {
		return fmt.Errorf("reorder sections: %w", err)
	}

	touchPatternUpdatedAt(tx, patternID)
	return tx.Commit()
}

func MoveSectionUp(db *sql.DB, id int64) error {
	return swapSectionPosition(db, id, -1)
}

func MoveSectionDown(db *sql.DB, id int64) error {
	return swapSectionPosition(db, id, +1)
}

func swapSectionPosition(db *sql.DB, id int64, delta int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var patternID int64
	var position int
	if err := tx.QueryRow("SELECT pattern_id, position FROM pattern_sections WHERE id = ?", id).Scan(&patternID, &position); err != nil {
		return fmt.Errorf("section not found: %w", err)
	}

	targetPos := position + delta
	if targetPos < 1 {
		return nil // already at top
	}

	// Find the section at the target position.
	var otherID int64
	err = tx.QueryRow(
		"SELECT id FROM pattern_sections WHERE pattern_id = ? AND position = ?",
		patternID, targetPos,
	).Scan(&otherID)
	if err != nil {
		return nil // no neighbor to swap with
	}

	// Use a temporary position to avoid unique constraint violation.
	if _, err := tx.Exec("UPDATE pattern_sections SET position = -1 WHERE id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE pattern_sections SET position = ? WHERE id = ?", position, otherID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE pattern_sections SET position = ? WHERE id = ?", targetPos, id); err != nil {
		return err
	}

	touchPatternUpdatedAt(tx, patternID)
	return tx.Commit()
}

// --- Row CRUD ---

func ListRowsBySection(db *sql.DB, sectionID int64) ([]Row, error) {
	dbRows, err := db.Query(`
		SELECT id, section_id, position, label, type, expected_stitch_count,
		       turning_chain_count, turning_chain_counts_as_stitch, repeat_count, notes
		FROM rows
		WHERE section_id = ?
		ORDER BY position ASC
	`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("list rows: %w", err)
	}
	defer dbRows.Close()

	var result []Row
	for dbRows.Next() {
		var r Row
		if err := dbRows.Scan(&r.ID, &r.SectionID, &r.Position, &r.Label, &r.Type,
			&r.ExpectedStitchCount, &r.TurningChainCount, &r.TurningChainCountsAsStitch,
			&r.RepeatCount, &r.Notes); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		result = append(result, r)
	}
	return result, dbRows.Err()
}

func FindRowByID(db *sql.DB, id int64) (*Row, error) {
	r := &Row{}
	err := db.QueryRow(`
		SELECT id, section_id, position, label, type, expected_stitch_count,
		       turning_chain_count, turning_chain_counts_as_stitch, repeat_count, notes
		FROM rows WHERE id = ?
	`, id).Scan(&r.ID, &r.SectionID, &r.Position, &r.Label, &r.Type,
		&r.ExpectedStitchCount, &r.TurningChainCount, &r.TurningChainCountsAsStitch,
		&r.RepeatCount, &r.Notes)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func CreateRow(db *sql.DB, sectionID int64, label, rowType string, stitchCount, turningChain int, turningChainCounts bool, repeatCount int, notes string) (*Row, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get next position.
	var maxPos sql.NullInt64
	tx.QueryRow("SELECT MAX(position) FROM rows WHERE section_id = ?", sectionID).Scan(&maxPos)
	nextPos := 1
	if maxPos.Valid {
		nextPos = int(maxPos.Int64) + 1
	}

	// Touch parent pattern.
	var patternID int64
	if err := tx.QueryRow("SELECT pattern_id FROM pattern_sections WHERE id = ?", sectionID).Scan(&patternID); err != nil {
		return nil, fmt.Errorf("section not found: %w", err)
	}

	result, err := tx.Exec(`
		INSERT INTO rows (section_id, position, label, type, expected_stitch_count,
		                   turning_chain_count, turning_chain_counts_as_stitch, repeat_count, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sectionID, nextPos, label, rowType, stitchCount, turningChain, turningChainCounts, repeatCount, notes)
	if err != nil {
		return nil, fmt.Errorf("insert row: %w", err)
	}

	touchPatternUpdatedAt(tx, patternID)

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Row{
		ID:                         id,
		SectionID:                  sectionID,
		Position:                   nextPos,
		Label:                      label,
		Type:                       rowType,
		ExpectedStitchCount:        stitchCount,
		TurningChainCount:          turningChain,
		TurningChainCountsAsStitch: turningChainCounts,
		RepeatCount:                repeatCount,
		Notes:                      notes,
	}, nil
}

func UpdateRow(db *sql.DB, id int64, label, rowType string, stitchCount, turningChain int, turningChainCounts bool, repeatCount int, notes string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get parent pattern via section.
	var sectionID int64
	if err := tx.QueryRow("SELECT section_id FROM rows WHERE id = ?", id).Scan(&sectionID); err != nil {
		return fmt.Errorf("row not found: %w", err)
	}
	var patternID int64
	if err := tx.QueryRow("SELECT pattern_id FROM pattern_sections WHERE id = ?", sectionID).Scan(&patternID); err != nil {
		return fmt.Errorf("section not found: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE rows SET label = ?, type = ?, expected_stitch_count = ?,
		               turning_chain_count = ?, turning_chain_counts_as_stitch = ?,
		               repeat_count = ?, notes = ?
		WHERE id = ?
	`, label, rowType, stitchCount, turningChain, turningChainCounts, repeatCount, notes, id); err != nil {
		return fmt.Errorf("update row: %w", err)
	}

	touchPatternUpdatedAt(tx, patternID)
	return tx.Commit()
}

func DeleteRow(db *sql.DB, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sectionID int64
	var position int
	if err := tx.QueryRow("SELECT section_id, position FROM rows WHERE id = ?", id).Scan(&sectionID, &position); err != nil {
		return fmt.Errorf("row not found: %w", err)
	}

	var patternID int64
	if err := tx.QueryRow("SELECT pattern_id FROM pattern_sections WHERE id = ?", sectionID).Scan(&patternID); err != nil {
		return fmt.Errorf("section not found: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM rows WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete row: %w", err)
	}

	// Reorder remaining rows.
	if _, err := tx.Exec(
		"UPDATE rows SET position = position - 1 WHERE section_id = ? AND position > ?",
		sectionID, position,
	); err != nil {
		return fmt.Errorf("reorder rows: %w", err)
	}

	touchPatternUpdatedAt(tx, patternID)
	return tx.Commit()
}

func MoveRowUp(db *sql.DB, id int64) error {
	return swapRowPosition(db, id, -1)
}

func MoveRowDown(db *sql.DB, id int64) error {
	return swapRowPosition(db, id, +1)
}

func swapRowPosition(db *sql.DB, id int64, delta int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sectionID int64
	var position int
	if err := tx.QueryRow("SELECT section_id, position FROM rows WHERE id = ?", id).Scan(&sectionID, &position); err != nil {
		return fmt.Errorf("row not found: %w", err)
	}

	targetPos := position + delta
	if targetPos < 1 {
		return nil
	}

	var otherID int64
	err = tx.QueryRow(
		"SELECT id FROM rows WHERE section_id = ? AND position = ?",
		sectionID, targetPos,
	).Scan(&otherID)
	if err != nil {
		return nil // no neighbor
	}

	// Temporary position to avoid unique constraint.
	if _, err := tx.Exec("UPDATE rows SET position = -1 WHERE id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE rows SET position = ? WHERE id = ?", position, otherID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE rows SET position = ? WHERE id = ?", targetPos, id); err != nil {
		return err
	}

	var patternID int64
	tx.QueryRow("SELECT pattern_id FROM pattern_sections WHERE id = ?", sectionID).Scan(&patternID)
	touchPatternUpdatedAt(tx, patternID)

	return tx.Commit()
}

// LoadPatternFull loads a pattern with all its sections and rows.
func LoadPatternFull(db *sql.DB, patternID int64) (*Pattern, []PatternSection, error) {
	pattern, err := FindPatternByID(db, patternID)
	if err != nil {
		return nil, nil, err
	}

	sections, err := ListSectionsByPattern(db, patternID)
	if err != nil {
		return nil, nil, err
	}

	for i := range sections {
		sectionRows, err := ListRowsBySection(db, sections[i].ID)
		if err != nil {
			return nil, nil, err
		}
		sections[i].Rows = sectionRows
	}

	return pattern, sections, nil
}

// GetPatternIDForSection returns the pattern_id for a section, used for ownership checks.
func GetPatternIDForSection(db *sql.DB, sectionID int64) (int64, error) {
	var patternID int64
	err := db.QueryRow("SELECT pattern_id FROM pattern_sections WHERE id = ?", sectionID).Scan(&patternID)
	return patternID, err
}

// GetPatternIDForRow returns the pattern_id for a row, used for ownership checks.
func GetPatternIDForRow(db *sql.DB, rowID int64) (int64, error) {
	var patternID int64
	err := db.QueryRow(`
		SELECT ps.pattern_id FROM rows r
		JOIN pattern_sections ps ON r.section_id = ps.id
		WHERE r.id = ?
	`, rowID).Scan(&patternID)
	return patternID, err
}
