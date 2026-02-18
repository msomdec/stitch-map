package model

import (
	"database/sql"
	"fmt"
)

// RowInstruction is a single instruction step within a row/round.
type RowInstruction struct {
	ID          int64
	RowID       int64
	Position    int
	StitchID    *int64  // nullable — nil for non-stitch instructions or group headers
	StitchName  string  // populated via JOIN
	StitchAbbr  string  // populated via JOIN
	Count       int
	Into        string
	IsGroup     bool
	ParentID    *int64 // nullable — nil for top-level instructions
	GroupRepeat int
	Note        string
	Children    []RowInstruction // populated for group headers
}

const instructionSelectCols = `
	ri.id, ri.row_id, ri.position, ri.stitch_id,
	COALESCE(s.name, ''), COALESCE(s.abbreviation, ''),
	ri.count, ri."into", ri.is_group, ri.parent_id, ri.group_repeat, ri.note
`

func scanInstruction(row interface{ Scan(...any) error }) (*RowInstruction, error) {
	ri := &RowInstruction{}
	var stitchID sql.NullInt64
	var parentID sql.NullInt64
	var isGroup int
	err := row.Scan(
		&ri.ID, &ri.RowID, &ri.Position, &stitchID,
		&ri.StitchName, &ri.StitchAbbr,
		&ri.Count, &ri.Into, &isGroup, &parentID, &ri.GroupRepeat, &ri.Note,
	)
	if err != nil {
		return nil, err
	}
	if stitchID.Valid {
		v := stitchID.Int64
		ri.StitchID = &v
	}
	if parentID.Valid {
		v := parentID.Int64
		ri.ParentID = &v
	}
	ri.IsGroup = isGroup != 0
	return ri, nil
}

// ListTopLevelInstructions returns top-level instructions (parent_id IS NULL) for a row.
func ListTopLevelInstructions(db *sql.DB, rowID int64) ([]RowInstruction, error) {
	rows, err := db.Query(`
		SELECT `+instructionSelectCols+`
		FROM row_instructions ri
		LEFT JOIN stitches s ON ri.stitch_id = s.id
		WHERE ri.row_id = ? AND ri.parent_id IS NULL
		ORDER BY ri.position ASC
	`, rowID)
	if err != nil {
		return nil, fmt.Errorf("list top-level instructions: %w", err)
	}
	defer rows.Close()

	var result []RowInstruction
	for rows.Next() {
		ri, err := scanInstruction(rows)
		if err != nil {
			return nil, fmt.Errorf("scan instruction: %w", err)
		}
		result = append(result, *ri)
	}
	return result, rows.Err()
}

// ListChildInstructions returns child instructions for a group.
func ListChildInstructions(db *sql.DB, parentID int64) ([]RowInstruction, error) {
	rows, err := db.Query(`
		SELECT `+instructionSelectCols+`
		FROM row_instructions ri
		LEFT JOIN stitches s ON ri.stitch_id = s.id
		WHERE ri.parent_id = ?
		ORDER BY ri.position ASC
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("list child instructions: %w", err)
	}
	defer rows.Close()

	var result []RowInstruction
	for rows.Next() {
		ri, err := scanInstruction(rows)
		if err != nil {
			return nil, fmt.Errorf("scan child instruction: %w", err)
		}
		result = append(result, *ri)
	}
	return result, rows.Err()
}

// ListInstructionsForRow returns all instructions for a row, with children nested inside groups.
func ListInstructionsForRow(db *sql.DB, rowID int64) ([]RowInstruction, error) {
	top, err := ListTopLevelInstructions(db, rowID)
	if err != nil {
		return nil, err
	}
	for i := range top {
		if top[i].IsGroup {
			children, err := ListChildInstructions(db, top[i].ID)
			if err != nil {
				return nil, err
			}
			top[i].Children = children
		}
	}
	return top, nil
}

// FindInstructionByID fetches a single instruction by ID.
func FindInstructionByID(db *sql.DB, id int64) (*RowInstruction, error) {
	row := db.QueryRow(`
		SELECT `+instructionSelectCols+`
		FROM row_instructions ri
		LEFT JOIN stitches s ON ri.stitch_id = s.id
		WHERE ri.id = ?
	`, id)
	return scanInstruction(row)
}

// CreateInstruction inserts a new top-level instruction in a row.
func CreateInstruction(db *sql.DB, rowID int64, stitchID *int64, count int, into string, note string) (*RowInstruction, error) {
	return insertInstruction(db, rowID, nil, stitchID, count, into, false, 1, note)
}

// CreateGroupInstruction inserts a new group header (is_group=true) in a row.
func CreateGroupInstruction(db *sql.DB, rowID int64, groupRepeat int, note string) (*RowInstruction, error) {
	return insertInstruction(db, rowID, nil, nil, 1, "", true, groupRepeat, note)
}

// CreateChildInstruction inserts a child instruction inside a group.
func CreateChildInstruction(db *sql.DB, parentID int64, stitchID *int64, count int, into string, note string) (*RowInstruction, error) {
	parent, err := FindInstructionByID(db, parentID)
	if err != nil {
		return nil, fmt.Errorf("parent instruction not found: %w", err)
	}
	return insertInstruction(db, parent.RowID, &parentID, stitchID, count, into, false, 1, note)
}

func insertInstruction(db *sql.DB, rowID int64, parentID *int64, stitchID *int64, count int, into string, isGroup bool, groupRepeat int, note string) (*RowInstruction, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get next position within (row_id, parent_id) scope.
	var maxPos sql.NullInt64
	if parentID == nil {
		tx.QueryRow(
			"SELECT MAX(position) FROM row_instructions WHERE row_id = ? AND parent_id IS NULL",
			rowID,
		).Scan(&maxPos)
	} else {
		tx.QueryRow(
			"SELECT MAX(position) FROM row_instructions WHERE row_id = ? AND parent_id = ?",
			rowID, *parentID,
		).Scan(&maxPos)
	}
	nextPos := 1
	if maxPos.Valid {
		nextPos = int(maxPos.Int64) + 1
	}

	isGroupInt := 0
	if isGroup {
		isGroupInt = 1
	}

	result, err := tx.Exec(`
		INSERT INTO row_instructions (row_id, position, stitch_id, count, "into", is_group, parent_id, group_repeat, note)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, rowID, nextPos, stitchID, count, into, isGroupInt, parentID, groupRepeat, note)
	if err != nil {
		return nil, fmt.Errorf("insert instruction: %w", err)
	}

	// Touch parent pattern.
	touchPatternUpdatedAtForRow(tx, rowID)

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	ri := &RowInstruction{
		ID:          id,
		RowID:       rowID,
		Position:    nextPos,
		StitchID:    stitchID,
		Count:       count,
		Into:        into,
		IsGroup:     isGroup,
		ParentID:    parentID,
		GroupRepeat: groupRepeat,
		Note:        note,
	}
	return ri, nil
}

// UpdateInstruction updates a non-group instruction's fields.
func UpdateInstruction(db *sql.DB, id int64, stitchID *int64, count int, into, note string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var rowID int64
	if err := tx.QueryRow("SELECT row_id FROM row_instructions WHERE id = ?", id).Scan(&rowID); err != nil {
		return fmt.Errorf("instruction not found: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE row_instructions SET stitch_id = ?, count = ?, "into" = ?, note = ?
		WHERE id = ?
	`, stitchID, count, into, note, id); err != nil {
		return fmt.Errorf("update instruction: %w", err)
	}

	touchPatternUpdatedAtForRow(tx, rowID)
	return tx.Commit()
}

// UpdateGroupInstruction updates a group header's repeat and note fields.
func UpdateGroupInstruction(db *sql.DB, id int64, groupRepeat int, note string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var rowID int64
	if err := tx.QueryRow("SELECT row_id FROM row_instructions WHERE id = ?", id).Scan(&rowID); err != nil {
		return fmt.Errorf("instruction not found: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE row_instructions SET group_repeat = ?, note = ?
		WHERE id = ? AND is_group = 1
	`, groupRepeat, note, id); err != nil {
		return fmt.Errorf("update group instruction: %w", err)
	}

	touchPatternUpdatedAtForRow(tx, rowID)
	return tx.Commit()
}

// DeleteInstruction deletes an instruction (children cascade via FK).
func DeleteInstruction(db *sql.DB, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var rowID int64
	var position int
	var parentID sql.NullInt64
	if err := tx.QueryRow(
		"SELECT row_id, position, parent_id FROM row_instructions WHERE id = ?", id,
	).Scan(&rowID, &position, &parentID); err != nil {
		return fmt.Errorf("instruction not found: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM row_instructions WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete instruction: %w", err)
	}

	// Reorder siblings.
	if parentID.Valid {
		tx.Exec(
			"UPDATE row_instructions SET position = position - 1 WHERE row_id = ? AND parent_id = ? AND position > ?",
			rowID, parentID.Int64, position,
		)
	} else {
		tx.Exec(
			"UPDATE row_instructions SET position = position - 1 WHERE row_id = ? AND parent_id IS NULL AND position > ?",
			rowID, position,
		)
	}

	touchPatternUpdatedAtForRow(tx, rowID)
	return tx.Commit()
}

// MoveInstructionUp moves an instruction one position up among its siblings.
func MoveInstructionUp(db *sql.DB, id int64) error {
	return swapInstructionPosition(db, id, -1)
}

// MoveInstructionDown moves an instruction one position down among its siblings.
func MoveInstructionDown(db *sql.DB, id int64) error {
	return swapInstructionPosition(db, id, +1)
}

func swapInstructionPosition(db *sql.DB, id int64, delta int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var rowID int64
	var position int
	var parentID sql.NullInt64
	if err := tx.QueryRow(
		"SELECT row_id, position, parent_id FROM row_instructions WHERE id = ?", id,
	).Scan(&rowID, &position, &parentID); err != nil {
		return fmt.Errorf("instruction not found: %w", err)
	}

	targetPos := position + delta
	if targetPos < 1 {
		return nil
	}

	// Find sibling at target position.
	var otherID int64
	var scanErr error
	if parentID.Valid {
		scanErr = tx.QueryRow(
			"SELECT id FROM row_instructions WHERE row_id = ? AND parent_id = ? AND position = ?",
			rowID, parentID.Int64, targetPos,
		).Scan(&otherID)
	} else {
		scanErr = tx.QueryRow(
			"SELECT id FROM row_instructions WHERE row_id = ? AND parent_id IS NULL AND position = ?",
			rowID, targetPos,
		).Scan(&otherID)
	}
	if scanErr != nil {
		return nil // no neighbor
	}

	// Swap via temp position.
	if _, err := tx.Exec("UPDATE row_instructions SET position = -1 WHERE id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE row_instructions SET position = ? WHERE id = ?", position, otherID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE row_instructions SET position = ? WHERE id = ?", targetPos, id); err != nil {
		return err
	}

	touchPatternUpdatedAtForRow(tx, rowID)
	return tx.Commit()
}

// GetRowIDForInstruction returns the row_id for an instruction, for ownership checks.
func GetRowIDForInstruction(db *sql.DB, instructionID int64) (int64, error) {
	var rowID int64
	err := db.QueryRow("SELECT row_id FROM row_instructions WHERE id = ?", instructionID).Scan(&rowID)
	return rowID, err
}

// touchPatternUpdatedAtForRow touches the parent pattern's updated_at via a row's chain.
func touchPatternUpdatedAtForRow(tx *sql.Tx, rowID int64) {
	var patternID int64
	tx.QueryRow(`
		SELECT ps.pattern_id FROM rows r
		JOIN pattern_sections ps ON r.section_id = ps.id
		WHERE r.id = ?
	`, rowID).Scan(&patternID)
	if patternID > 0 {
		touchPatternUpdatedAt(tx, patternID)
	}
}
