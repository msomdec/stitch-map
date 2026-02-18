package model

import (
	"database/sql"
	"fmt"
	"time"
)

// WorkSession represents an active or completed work session on a pattern.
type WorkSession struct {
	ID           int64
	UserID       int64
	PatternID    int64
	StartedAt    time.Time
	LastActiveAt time.Time
	CompletedAt  *time.Time // nil if still active
}

// WorkProgress tracks the current position within a work session.
type WorkProgress struct {
	ID                     int64
	SessionID              int64
	SectionID              int64
	RowID                  int64
	RowRepeatIndex         int
	InstructionID          int64
	StitchIndex            int
	GroupRepeatIndex       int
	StitchesCompletedInRow int
	UpdatedAt              time.Time
}

// StitchPos is one position in the flattened instruction sequence for a row.
type StitchPos struct {
	InstructionID    int64
	StitchIndex      int
	GroupRepeatIndex int
}

// FlattenInstructions produces an ordered flat list of every stitch position for a row.
// Groups are expanded across all repeats.
func FlattenInstructions(instructions []RowInstruction) []StitchPos {
	var out []StitchPos
	for _, ri := range instructions {
		if ri.IsGroup {
			for grp := 0; grp < ri.GroupRepeat; grp++ {
				for _, child := range ri.Children {
					for si := 0; si < child.Count; si++ {
						out = append(out, StitchPos{child.ID, si, grp})
					}
				}
			}
		} else {
			for si := 0; si < ri.Count; si++ {
				out = append(out, StitchPos{ri.ID, si, 0})
			}
		}
	}
	return out
}

// FindFlatIndex returns the index of the progress position in the flat list, or -1 if not found.
func FindFlatIndex(flat []StitchPos, p *WorkProgress) int {
	for i, pos := range flat {
		if pos.InstructionID == p.InstructionID &&
			pos.StitchIndex == p.StitchIndex &&
			pos.GroupRepeatIndex == p.GroupRepeatIndex {
			return i
		}
	}
	return -1
}

// --- Session CRUD ---

func scanSession(row interface{ Scan(...any) error }) (*WorkSession, error) {
	s := &WorkSession{}
	var startedAt, lastActiveAt string
	var completedAt sql.NullString
	err := row.Scan(&s.ID, &s.UserID, &s.PatternID, &startedAt, &lastActiveAt, &completedAt)
	if err != nil {
		return nil, err
	}
	s.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	s.LastActiveAt, _ = time.Parse(time.RFC3339, lastActiveAt)
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		s.CompletedAt = &t
	}
	return s, nil
}

// FindActiveSession returns the active (non-completed) session for a user+pattern, or nil.
func FindActiveSession(db *sql.DB, userID, patternID int64) (*WorkSession, error) {
	row := db.QueryRow(`
		SELECT id, user_id, pattern_id, started_at, last_active_at, completed_at
		FROM work_sessions
		WHERE user_id = ? AND pattern_id = ? AND completed_at IS NULL
	`, userID, patternID)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// FindSessionByID loads a session by ID.
func FindSessionByID(db *sql.DB, id int64) (*WorkSession, error) {
	row := db.QueryRow(`
		SELECT id, user_id, pattern_id, started_at, last_active_at, completed_at
		FROM work_sessions WHERE id = ?
	`, id)
	return scanSession(row)
}

// CreateWorkSession creates a new work session.
func CreateWorkSession(db *sql.DB, userID, patternID int64) (*WorkSession, error) {
	result, err := db.Exec(`
		INSERT INTO work_sessions (user_id, pattern_id) VALUES (?, ?)
	`, userID, patternID)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	id, _ := result.LastInsertId()
	now := time.Now().UTC()
	return &WorkSession{
		ID:           id,
		UserID:       userID,
		PatternID:    patternID,
		StartedAt:    now,
		LastActiveAt: now,
	}, nil
}

// MarkSessionCompleted marks a session as complete.
func MarkSessionCompleted(db *sql.DB, id int64) error {
	_, err := db.Exec(`
		UPDATE work_sessions
		SET completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
		    last_active_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?
	`, id)
	return err
}

// TouchSessionActivity updates last_active_at.
func TouchSessionActivity(db *sql.DB, id int64) {
	db.Exec(`UPDATE work_sessions SET last_active_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, id)
}

// --- Progress CRUD ---

// GetProgress loads the work progress for a session.
func GetProgress(db *sql.DB, sessionID int64) (*WorkProgress, error) {
	p := &WorkProgress{}
	var updatedAt string
	err := db.QueryRow(`
		SELECT id, session_id, section_id, row_id, row_repeat_index,
		       instruction_id, stitch_index, group_repeat_index,
		       stitches_completed_in_row, updated_at
		FROM work_progress WHERE session_id = ?
	`, sessionID).Scan(
		&p.ID, &p.SessionID, &p.SectionID, &p.RowID, &p.RowRepeatIndex,
		&p.InstructionID, &p.StitchIndex, &p.GroupRepeatIndex,
		&p.StitchesCompletedInRow, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return p, nil
}

// InitProgress creates the initial work_progress row pointing to the first stitch of the pattern.
// Returns an error if the pattern has no sections, no rows, or no instructions.
func InitProgress(db *sql.DB, sessionID int64, sections []PatternSection) error {
	for _, section := range sections {
		for _, row := range section.Rows {
			flat := FlattenInstructions(row.Instructions)
			if len(flat) == 0 {
				continue
			}
			first := flat[0]
			_, err := db.Exec(`
				INSERT INTO work_progress
				  (session_id, section_id, row_id, row_repeat_index,
				   instruction_id, stitch_index, group_repeat_index, stitches_completed_in_row)
				VALUES (?, ?, ?, 0, ?, ?, ?, 0)
			`, sessionID, section.ID, row.ID, first.InstructionID, first.StitchIndex, first.GroupRepeatIndex)
			return err
		}
	}
	return fmt.Errorf("pattern has no instructions to track")
}

// saveProgress updates the work_progress row for a session.
func saveProgress(db *sql.DB, p *WorkProgress) error {
	_, err := db.Exec(`
		UPDATE work_progress
		SET section_id = ?, row_id = ?, row_repeat_index = ?,
		    instruction_id = ?, stitch_index = ?, group_repeat_index = ?,
		    stitches_completed_in_row = ?,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE session_id = ?
	`, p.SectionID, p.RowID, p.RowRepeatIndex,
		p.InstructionID, p.StitchIndex, p.GroupRepeatIndex,
		p.StitchesCompletedInRow, p.SessionID)
	return err
}

// --- Advance / Undo ---

// AdvanceProgress moves progress forward by one stitch.
// Returns true if the pattern is now complete.
func AdvanceProgress(db *sql.DB, sessionID int64) (completed bool, err error) {
	session, err := FindSessionByID(db, sessionID)
	if err != nil {
		return false, err
	}
	if session.CompletedAt != nil {
		return true, nil // already done
	}

	progress, err := GetProgress(db, sessionID)
	if err != nil {
		return false, err
	}

	_, sections, err := LoadPatternFull(db, session.PatternID)
	if err != nil {
		return false, err
	}

	section, sIdx := findSectionByID(sections, progress.SectionID)
	if section == nil {
		return false, fmt.Errorf("section %d not found", progress.SectionID)
	}
	row, rIdx := findRowByID(section.Rows, progress.RowID)
	if row == nil {
		return false, fmt.Errorf("row %d not found", progress.RowID)
	}

	flat := FlattenInstructions(row.Instructions)
	curIdx := FindFlatIndex(flat, progress)

	newProg := *progress

	if curIdx+1 < len(flat) {
		// Advance within same row repeat.
		next := flat[curIdx+1]
		newProg.InstructionID = next.InstructionID
		newProg.StitchIndex = next.StitchIndex
		newProg.GroupRepeatIndex = next.GroupRepeatIndex
		newProg.StitchesCompletedInRow = curIdx + 1
		if err := saveProgress(db, &newProg); err != nil {
			return false, err
		}
		TouchSessionActivity(db, sessionID)
		return false, nil
	}

	// Current row repeat exhausted — try next repeat or next row.
	newProg.StitchesCompletedInRow = 0

	if progress.RowRepeatIndex+1 < row.RepeatCount {
		// Next repeat of the same row.
		newProg.RowRepeatIndex = progress.RowRepeatIndex + 1
		setToFirstStitch(&newProg, flat)
		if err := saveProgress(db, &newProg); err != nil {
			return false, err
		}
		TouchSessionActivity(db, sessionID)
		return false, nil
	}

	// All row repeats done — find next row in section.
	newProg.RowRepeatIndex = 0
	if rIdx+1 < len(section.Rows) {
		nextRow := &section.Rows[rIdx+1]
		nextFlat := FlattenInstructions(nextRow.Instructions)
		if len(nextFlat) > 0 {
			newProg.RowID = nextRow.ID
			setToFirstStitch(&newProg, nextFlat)
			if err := saveProgress(db, &newProg); err != nil {
				return false, err
			}
			TouchSessionActivity(db, sessionID)
			return false, nil
		}
	}

	// Section exhausted — find next section.
	if sIdx+1 < len(sections) {
		for si := sIdx + 1; si < len(sections); si++ {
			nextSection := &sections[si]
			for ri := range nextSection.Rows {
				nextRow := &nextSection.Rows[ri]
				nextFlat := FlattenInstructions(nextRow.Instructions)
				if len(nextFlat) > 0 {
					newProg.SectionID = nextSection.ID
					newProg.RowID = nextRow.ID
					setToFirstStitch(&newProg, nextFlat)
					if err := saveProgress(db, &newProg); err != nil {
						return false, err
					}
					TouchSessionActivity(db, sessionID)
					return false, nil
				}
			}
		}
	}

	// Pattern complete!
	if err := MarkSessionCompleted(db, sessionID); err != nil {
		return false, err
	}
	return true, nil
}

// UndoProgress moves progress backward by one stitch. No-op at the very beginning.
func UndoProgress(db *sql.DB, sessionID int64) error {
	session, err := FindSessionByID(db, sessionID)
	if err != nil {
		return err
	}
	// Allow undo on a completed session (un-complete it).
	progress, err := GetProgress(db, sessionID)
	if err != nil {
		return err
	}

	_, sections, err := LoadPatternFull(db, session.PatternID)
	if err != nil {
		return err
	}

	section, sIdx := findSectionByID(sections, progress.SectionID)
	if section == nil {
		return fmt.Errorf("section %d not found", progress.SectionID)
	}
	row, rIdx := findRowByID(section.Rows, progress.RowID)
	if row == nil {
		return fmt.Errorf("row %d not found", progress.RowID)
	}

	flat := FlattenInstructions(row.Instructions)
	curIdx := FindFlatIndex(flat, progress)

	newProg := *progress

	if curIdx > 0 {
		// Step back within same row repeat.
		prev := flat[curIdx-1]
		newProg.InstructionID = prev.InstructionID
		newProg.StitchIndex = prev.StitchIndex
		newProg.GroupRepeatIndex = prev.GroupRepeatIndex
		newProg.StitchesCompletedInRow = curIdx - 1
		if err := saveProgress(db, &newProg); err != nil {
			return err
		}
		// Un-complete session if it was marked done.
		if session.CompletedAt != nil {
			db.Exec(`UPDATE work_sessions SET completed_at = NULL WHERE id = ?`, sessionID)
		}
		TouchSessionActivity(db, sessionID)
		return nil
	}

	// At first position in this row repeat.
	if progress.RowRepeatIndex > 0 {
		// Go to last stitch of previous repeat.
		newProg.RowRepeatIndex = progress.RowRepeatIndex - 1
		setToLastStitch(&newProg, flat)
		if err := saveProgress(db, &newProg); err != nil {
			return err
		}
		if session.CompletedAt != nil {
			db.Exec(`UPDATE work_sessions SET completed_at = NULL WHERE id = ?`, sessionID)
		}
		TouchSessionActivity(db, sessionID)
		return nil
	}

	// At first repeat of this row — go to previous row in section.
	if rIdx > 0 {
		prevRow := &section.Rows[rIdx-1]
		prevFlat := FlattenInstructions(prevRow.Instructions)
		if len(prevFlat) > 0 {
			newProg.RowID = prevRow.ID
			newProg.RowRepeatIndex = prevRow.RepeatCount - 1
			setToLastStitch(&newProg, prevFlat)
			if err := saveProgress(db, &newProg); err != nil {
				return err
			}
			if session.CompletedAt != nil {
				db.Exec(`UPDATE work_sessions SET completed_at = NULL WHERE id = ?`, sessionID)
			}
			TouchSessionActivity(db, sessionID)
			return nil
		}
	}

	// At first row of section — go to previous section.
	if sIdx > 0 {
		for si := sIdx - 1; si >= 0; si-- {
			prevSection := &sections[si]
			for ri := len(prevSection.Rows) - 1; ri >= 0; ri-- {
				prevRow := &prevSection.Rows[ri]
				prevFlat := FlattenInstructions(prevRow.Instructions)
				if len(prevFlat) > 0 {
					newProg.SectionID = prevSection.ID
					newProg.RowID = prevRow.ID
					newProg.RowRepeatIndex = prevRow.RepeatCount - 1
					setToLastStitch(&newProg, prevFlat)
					if err := saveProgress(db, &newProg); err != nil {
						return err
					}
					if session.CompletedAt != nil {
						db.Exec(`UPDATE work_sessions SET completed_at = NULL WHERE id = ?`, sessionID)
					}
					TouchSessionActivity(db, sessionID)
					return nil
				}
			}
		}
	}

	// At the very beginning — no-op.
	return nil
}

// --- Session summary for dashboard ---

// SessionSummary holds display data for an active session on the dashboard.
type SessionSummary struct {
	SessionID   int64
	PatternID   int64
	PatternName string
	// Progress text
	SectionName         string
	RowLabel            string
	RowNumberInSection  int
	TotalRowsInSection  int
	StitchesCompleted   int
	ExpectedStitches    int
}

// ListSessionSummaries returns summaries of all active sessions for a user.
func ListSessionSummaries(db *sql.DB, userID int64) ([]SessionSummary, error) {
	rows, err := db.Query(`
		SELECT ws.id, ws.pattern_id, p.name
		FROM work_sessions ws
		JOIN patterns p ON ws.pattern_id = p.id
		WHERE ws.user_id = ? AND ws.completed_at IS NULL
		ORDER BY ws.last_active_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type stub struct {
		sessionID   int64
		patternID   int64
		patternName string
	}
	var stubs []stub
	for rows.Next() {
		var s stub
		if err := rows.Scan(&s.sessionID, &s.patternID, &s.patternName); err != nil {
			return nil, err
		}
		stubs = append(stubs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var summaries []SessionSummary
	for _, s := range stubs {
		progress, err := GetProgress(db, s.sessionID)
		if err != nil {
			continue
		}
		_, sections, err := LoadPatternFull(db, s.patternID)
		if err != nil {
			continue
		}

		section, _ := findSectionByID(sections, progress.SectionID)
		var sectionName, rowLabel string
		var rowNum, totalRows, stitchesDone, expectedStitches int

		if section != nil {
			sectionName = section.Name
			row, _ := findRowByID(section.Rows, progress.RowID)
			rowLabel = computeRowLabel(section.Rows, progress.RowID)
			// Count rows in section (counting repeats)
			totalRows, rowNum = countRowsInSection(section.Rows, progress.RowID, progress.RowRepeatIndex)
			if row != nil {
				expectedStitches = row.ExpectedStitchCount
			}
		}
		stitchesDone = progress.StitchesCompletedInRow

		summaries = append(summaries, SessionSummary{
			SessionID:          s.sessionID,
			PatternID:          s.patternID,
			PatternName:        s.patternName,
			SectionName:        sectionName,
			RowLabel:           rowLabel,
			RowNumberInSection: rowNum,
			TotalRowsInSection: totalRows,
			StitchesCompleted:  stitchesDone,
			ExpectedStitches:   expectedStitches,
		})
	}
	return summaries, nil
}

// --- Work display state (used by work mode view) ---

// WorkDisplayState holds all computed data needed to render the work mode UI.
type WorkDisplayState struct {
	SessionID   int64
	PatternID   int64
	PatternName string
	Completed   bool

	SectionName        string
	RowLabel           string
	RowRepeatIndex     int
	RowRepeatCount     int
	RowNumberInSection int
	TotalRowsInSection int

	Instructions          []RowInstruction
	CurrentInstrID        int64
	CurrentStitchIndex    int
	CurrentGroupRepeatIdx int
	CurrentStitchAbbr     string
	CurrentStitchCount    int

	StitchesCompleted   int
	ExpectedStitchCount int
}

// BuildWorkDisplayState computes the display state from a session + progress.
func BuildWorkDisplayState(session *WorkSession, progress *WorkProgress, sections []PatternSection, patternName string) WorkDisplayState {
	state := WorkDisplayState{
		SessionID:   session.ID,
		PatternID:   session.PatternID,
		PatternName: patternName,
		Completed:   session.CompletedAt != nil,
	}
	if state.Completed {
		return state
	}

	section, _ := findSectionByID(sections, progress.SectionID)
	if section != nil {
		state.SectionName = section.Name
		row, _ := findRowByID(section.Rows, progress.RowID)
		if row != nil {
			state.RowLabel = computeRowLabel(section.Rows, progress.RowID)
			state.RowRepeatCount = row.RepeatCount
			state.RowRepeatIndex = progress.RowRepeatIndex
			state.TotalRowsInSection, state.RowNumberInSection = countRowsInSection(section.Rows, progress.RowID, progress.RowRepeatIndex)
			state.Instructions = row.Instructions
			state.ExpectedStitchCount = row.ExpectedStitchCount
		}
	}

	state.CurrentInstrID = progress.InstructionID
	state.CurrentStitchIndex = progress.StitchIndex
	state.CurrentGroupRepeatIdx = progress.GroupRepeatIndex
	state.StitchesCompleted = progress.StitchesCompletedInRow

	// Resolve stitch abbreviation and count for current instruction.
	if instr := findInstructionInTree(state.Instructions, progress.InstructionID); instr != nil {
		state.CurrentStitchAbbr = instr.StitchAbbr
		state.CurrentStitchCount = instr.Count
	}

	return state
}

// --- Internal helpers ---

func findSectionByID(sections []PatternSection, id int64) (*PatternSection, int) {
	for i := range sections {
		if sections[i].ID == id {
			return &sections[i], i
		}
	}
	return nil, -1
}

func findRowByID(rows []Row, id int64) (*Row, int) {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i], i
		}
	}
	return nil, -1
}

func findInstructionInTree(instructions []RowInstruction, id int64) *RowInstruction {
	for i := range instructions {
		if instructions[i].ID == id {
			return &instructions[i]
		}
		for j := range instructions[i].Children {
			if instructions[i].Children[j].ID == id {
				return &instructions[i].Children[j]
			}
		}
	}
	return nil
}

func setToFirstStitch(p *WorkProgress, flat []StitchPos) {
	if len(flat) > 0 {
		p.InstructionID = flat[0].InstructionID
		p.StitchIndex = flat[0].StitchIndex
		p.GroupRepeatIndex = flat[0].GroupRepeatIndex
	}
	p.StitchesCompletedInRow = 0
}

func setToLastStitch(p *WorkProgress, flat []StitchPos) {
	if len(flat) > 0 {
		last := flat[len(flat)-1]
		p.InstructionID = last.InstructionID
		p.StitchIndex = last.StitchIndex
		p.GroupRepeatIndex = last.GroupRepeatIndex
		p.StitchesCompletedInRow = len(flat) - 1
	}
}

// computeRowLabel returns the display label for a row (auto-generated if Label is empty).
func computeRowLabel(rows []Row, rowID int64) string {
	n := 1
	for _, r := range rows {
		if r.ID == rowID {
			label := r.Label
			if label == "" {
				prefix := "Row"
				if r.Type == "joined_round" || r.Type == "continuous_round" {
					prefix = "Rnd"
				}
				if r.RepeatCount > 1 {
					return fmt.Sprintf("%ss %d-%d", prefix, n, n+r.RepeatCount-1)
				}
				return fmt.Sprintf("%s %d", prefix, n)
			}
			return label
		}
		n += r.RepeatCount
	}
	return "?"
}

// countRowsInSection returns (totalRowsInSection, rowNumberOfCurrent) where
// row numbers count each repeat as one row, 1-based.
func countRowsInSection(rows []Row, rowID int64, rowRepeatIndex int) (total int, current int) {
	n := 0
	cur := 0
	for _, r := range rows {
		for rep := 0; rep < r.RepeatCount; rep++ {
			n++
			if r.ID == rowID && rep == rowRepeatIndex {
				cur = n
			}
		}
	}
	return n, cur
}
