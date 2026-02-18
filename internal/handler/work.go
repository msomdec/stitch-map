package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/starfederation/datastar-go/datastar"
	"github.com/stitchmap/stitchmap/internal/model"
	"github.com/stitchmap/stitchmap/internal/view"
)

// WorkStart handles GET /patterns/{id}/work.
// Finds or creates an active session and renders the work mode page.
func WorkStart(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		patternID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		pattern, err := model.FindPatternByID(db, patternID)
		if err != nil || pattern.UserID != user.ID {
			http.NotFound(w, r)
			return
		}

		// Load full pattern with all sections, rows, instructions.
		_, sections, err := model.LoadPatternFull(db, patternID)
		if err != nil {
			http.Error(w, "Failed to load pattern", http.StatusInternalServerError)
			return
		}

		// Find or create active session.
		session, err := model.FindActiveSession(db, user.ID, patternID)
		if err != nil {
			http.Error(w, "Failed to look up session", http.StatusInternalServerError)
			return
		}

		if session == nil {
			// Create a new session.
			session, err = model.CreateWorkSession(db, user.ID, patternID)
			if err != nil {
				http.Error(w, "Failed to create session", http.StatusInternalServerError)
				return
			}
			// Initialize progress to first stitch.
			if err := model.InitProgress(db, session.ID, sections); err != nil {
				// Pattern has no instructions yet.
				renderTempl(w, r, http.StatusOK, view.WorkNoInstructionsPage(pattern, user.Email))
				return
			}
		}

		state, err := loadWorkState(db, session, sections, pattern.Name)
		if err != nil {
			http.Error(w, "Failed to load work state", http.StatusInternalServerError)
			return
		}

		renderTempl(w, r, http.StatusOK, view.WorkPage(state, user.Email))
	}
}

// WorkAdvance handles POST /sessions/{id}/advance.
func WorkAdvance(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		sessionID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		session, err := model.FindSessionByID(db, sessionID)
		if err != nil || session.UserID != user.ID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		completed, err := model.AdvanceProgress(db, sessionID)
		if err != nil {
			http.Error(w, "Failed to advance", http.StatusInternalServerError)
			return
		}

		// Reload session (completed_at may have been set).
		session, _ = model.FindSessionByID(db, sessionID)

		_, sections, _ := model.LoadPatternFull(db, session.PatternID)
		pattern, _ := model.FindPatternByID(db, session.PatternID)

		var state model.WorkDisplayState
		if completed {
			state = model.WorkDisplayState{
				SessionID:   sessionID,
				PatternID:   session.PatternID,
				PatternName: pattern.Name,
				Completed:   true,
			}
		} else {
			state, _ = loadWorkState(db, session, sections, pattern.Name)
		}

		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(
			view.WorkDisplay(state),
			datastar.WithSelectorID("work-display"),
		)
	}
}

// WorkUndo handles POST /sessions/{id}/undo.
func WorkUndo(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		sessionID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		session, err := model.FindSessionByID(db, sessionID)
		if err != nil || session.UserID != user.ID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if err := model.UndoProgress(db, sessionID); err != nil {
			http.Error(w, "Failed to undo", http.StatusInternalServerError)
			return
		}

		// Reload session (completed_at may have been cleared by undo).
		session, _ = model.FindSessionByID(db, sessionID)

		_, sections, _ := model.LoadPatternFull(db, session.PatternID)
		pattern, _ := model.FindPatternByID(db, session.PatternID)
		state, _ := loadWorkState(db, session, sections, pattern.Name)

		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(
			view.WorkDisplay(state),
			datastar.WithSelectorID("work-display"),
		)
	}
}

// loadWorkState builds a WorkDisplayState from the current session + pattern data.
func loadWorkState(db *sql.DB, session *model.WorkSession, sections []model.PatternSection, patternName string) (model.WorkDisplayState, error) {
	progress, err := model.GetProgress(db, session.ID)
	if err != nil {
		return model.WorkDisplayState{}, err
	}
	return model.BuildWorkDisplayState(session, progress, sections, patternName), nil
}
