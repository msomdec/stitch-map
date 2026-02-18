package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/starfederation/datastar-go/datastar"
	"github.com/stitchmap/stitchmap/internal/model"
	"github.com/stitchmap/stitchmap/internal/view"
)

// --- Helper: ownership check ---

func checkPatternOwnership(db *sql.DB, patternID, userID int64) bool {
	p, err := model.FindPatternByID(db, patternID)
	if err != nil {
		return false
	}
	return p.UserID == userID
}

// --- Pattern Handlers ---

// PatternNew renders the new pattern form (normal page).
func PatternNew(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		renderTempl(w, r, http.StatusOK, view.NewPatternPage(user.Email, ""))
	}
}

// PatternCreate handles POST /patterns (normal form submit, not SSE).
func PatternCreate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		name := strings.TrimSpace(r.FormValue("name"))
		description := strings.TrimSpace(r.FormValue("description"))

		if name == "" {
			renderTempl(w, r, http.StatusUnprocessableEntity,
				view.NewPatternPage(user.Email, "Pattern name is required."))
			return
		}

		pattern, err := model.CreatePattern(db, user.ID, name, description)
		if err != nil {
			renderTempl(w, r, http.StatusInternalServerError,
				view.NewPatternPage(user.Email, "Failed to create pattern."))
			return
		}

		http.Redirect(w, r, "/patterns/"+strconv.FormatInt(pattern.ID, 10), http.StatusSeeOther)
	}
}

// PatternShow renders the pattern detail page (normal page).
func PatternShow(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		pattern, sections, err := model.LoadPatternFull(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if pattern.UserID != user.ID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		renderTempl(w, r, http.StatusOK, view.PatternDetailPage(view.PatternDetailData{
			Pattern:  pattern,
			Sections: sections,
			Email:    user.Email,
		}))
	}
}

// PatternEditForm returns the edit form via SSE.
func PatternEditForm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		pattern, err := model.FindPatternByID(db, id)
		if err != nil || pattern.UserID != user.ID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(view.PatternEditForm(pattern), datastar.WithSelectorID("pattern-content"), datastar.WithModeBefore())
	}
}

type patternSignals struct {
	Name string `json:"patternName"`
	Desc string `json:"patternDesc"`
}

// PatternUpdate handles PUT /patterns/{id} via SSE.
func PatternUpdate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		if !checkPatternOwnership(db, id, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		signals := &patternSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(signals.Name)
		desc := strings.TrimSpace(signals.Desc)

		sse := datastar.NewSSE(w, r)

		if name == "" {
			sse.PatchElementTempl(view.PatternError("Pattern name is required."))
			return
		}

		if err := model.UpdatePattern(db, id, user.ID, name, desc); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to update pattern."))
			return
		}

		// Reload by redirecting to the same page.
		sse.Redirectf("/patterns/%d", id)
	}
}

// PatternDelete handles DELETE /patterns/{id} via SSE.
func PatternDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		sse := datastar.NewSSE(w, r)

		if err := model.DeletePattern(db, id, user.ID); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to delete pattern."))
			return
		}

		sse.Redirect("/")
	}
}

// --- Section Handlers ---

type sectionSignals struct {
	Name  string `json:"newSectionName,omitempty"`
	SName string `json:"sectionName,omitempty"`
	Notes string `json:"sectionNotes,omitempty"`
}

// SectionCreate handles POST /patterns/{id}/sections via SSE.
func SectionCreate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		patternID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		signals := &sectionSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(signals.Name)
		sse := datastar.NewSSE(w, r)

		if name == "" {
			sse.PatchElementTempl(view.PatternError("Section name is required."))
			return
		}

		if _, err := model.CreateSection(db, patternID, name); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to create section."))
			return
		}

		refreshPatternSections(sse, db, patternID)
	}
}

// SectionEditForm returns the section edit form via SSE.
func SectionEditForm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		section, err := model.FindSectionByID(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if !checkPatternOwnership(db, section.PatternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(view.SectionEditForm(section))
	}
}

// SectionUpdate handles PUT /sections/{id} via SSE.
func SectionUpdate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		section, err := model.FindSectionByID(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, section.PatternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		signals := &sectionSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(signals.SName)
		notes := strings.TrimSpace(signals.Notes)

		sse := datastar.NewSSE(w, r)

		if name == "" {
			sse.PatchElementTempl(view.PatternError("Section name is required."))
			return
		}

		if err := model.UpdateSection(db, id, name, notes); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to update section."))
			return
		}

		refreshPatternSections(sse, db, section.PatternID)
	}
}

// SectionDelete handles DELETE /sections/{id} via SSE.
func SectionDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForSection(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)

		if err := model.DeleteSection(db, id); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to delete section."))
			return
		}

		refreshPatternSections(sse, db, patternID)
	}
}

// SectionMoveUp handles POST /sections/{id}/move-up via SSE.
func SectionMoveUp(db *sql.DB) http.HandlerFunc {
	return sectionMoveHandler(db, model.MoveSectionUp)
}

// SectionMoveDown handles POST /sections/{id}/move-down via SSE.
func SectionMoveDown(db *sql.DB) http.HandlerFunc {
	return sectionMoveHandler(db, model.MoveSectionDown)
}

func sectionMoveHandler(db *sql.DB, moveFn func(*sql.DB, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForSection(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		moveFn(db, id)
		refreshPatternSections(sse, db, patternID)
	}
}

// SectionsRefresh handles GET /patterns/{id}/sections-refresh via SSE.
func SectionsRefresh(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		patternID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		refreshPatternSections(sse, db, patternID)
	}
}

// --- Row Handlers ---

type rowSignals struct {
	Label              string `json:"rowLabel"`
	Type               string `json:"rowType"`
	StitchCount        string `json:"rowStitchCount"`
	TurningChain       string `json:"rowTurningChain"`
	TurningChainCounts bool   `json:"rowTurningChainCounts"`
	RepeatCount        string `json:"rowRepeatCount"`
	Notes              string `json:"rowNotes"`
}

func (s *rowSignals) parse() (label, rowType string, stitchCount, turningChain int, turningChainCounts bool, repeatCount int, notes string) {
	label = strings.TrimSpace(s.Label)
	rowType = s.Type
	stitchCount, _ = strconv.Atoi(s.StitchCount)
	turningChain, _ = strconv.Atoi(s.TurningChain)
	turningChainCounts = s.TurningChainCounts
	repeatCount, _ = strconv.Atoi(s.RepeatCount)
	if repeatCount < 1 {
		repeatCount = 1
	}
	if stitchCount < 1 {
		stitchCount = 1
	}
	notes = strings.TrimSpace(s.Notes)

	// Validate type.
	switch rowType {
	case "row", "joined_round", "continuous_round":
	default:
		rowType = "continuous_round"
	}
	return
}

// RowNewForm returns the add row form via SSE.
func RowNewForm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		sectionID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForSection(db, sectionID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(view.AddRowForm(sectionID))
	}
}

// RowCreate handles POST /sections/{id}/rows via SSE.
func RowCreate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		sectionID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForSection(db, sectionID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		signals := &rowSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		label, rowType, stitchCount, turningChain, turningChainCounts, repeatCount, notes := signals.parse()

		sse := datastar.NewSSE(w, r)

		if _, err := model.CreateRow(db, sectionID, label, rowType, stitchCount, turningChain, turningChainCounts, repeatCount, notes); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to create row."))
			return
		}

		refreshSectionRows(sse, db, sectionID)
	}
}

// RowEditForm returns the row edit form via SSE.
func RowEditForm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		row, err := model.FindRowByID(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		patternID, _ := model.GetPatternIDForRow(db, id)
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(view.EditRowForm(row))
	}
}

// RowCancelEdit handles GET /rows/{id}/cancel-edit — re-renders the row table row.
func RowCancelEdit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		row, err := model.FindRowByID(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		patternID, _ := model.GetPatternIDForRow(db, id)
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Get total rows for move button visibility.
		rows, _ := model.ListRowsBySection(db, row.SectionID)

		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(view.RowTableRow(*row, len(rows)))
	}
}

// RowUpdate handles PUT /rows/{id} via SSE.
func RowUpdate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		row, err := model.FindRowByID(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		patternID, _ := model.GetPatternIDForRow(db, id)
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		signals := &rowSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		label, rowType, stitchCount, turningChain, turningChainCounts, repeatCount, notes := signals.parse()

		sse := datastar.NewSSE(w, r)

		if err := model.UpdateRow(db, id, label, rowType, stitchCount, turningChain, turningChainCounts, repeatCount, notes); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to update row."))
			return
		}

		refreshSectionRows(sse, db, row.SectionID)
	}
}

// RowDelete handles DELETE /rows/{id} via SSE.
func RowDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		row, err := model.FindRowByID(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		patternID, _ := model.GetPatternIDForRow(db, id)
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)

		if err := model.DeleteRow(db, id); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to delete row."))
			return
		}

		refreshSectionRows(sse, db, row.SectionID)
	}
}

// RowMoveUp handles POST /rows/{id}/move-up via SSE.
func RowMoveUp(db *sql.DB) http.HandlerFunc {
	return rowMoveHandler(db, model.MoveRowUp)
}

// RowMoveDown handles POST /rows/{id}/move-down via SSE.
func RowMoveDown(db *sql.DB) http.HandlerFunc {
	return rowMoveHandler(db, model.MoveRowDown)
}

func rowMoveHandler(db *sql.DB, moveFn func(*sql.DB, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		row, err := model.FindRowByID(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		patternID, _ := model.GetPatternIDForRow(db, id)
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		moveFn(db, id)
		refreshSectionRows(sse, db, row.SectionID)
	}
}

// RowsRefresh handles GET /sections/{id}/rows-refresh via SSE.
func RowsRefresh(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		sectionID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForSection(db, sectionID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		refreshSectionRows(sse, db, sectionID)
	}
}

// --- Helpers ---

func refreshPatternSections(sse *datastar.ServerSentEventGenerator, db *sql.DB, patternID int64) {
	_, sections, _ := model.LoadPatternFull(db, patternID)
	sse.PatchElementTempl(view.PatternSections(patternID, sections), datastar.WithSelectorID("pattern-content"), datastar.WithModeInner())
	sse.PatchElementTempl(view.PatternSummaryBlock(sections), datastar.WithSelectorID("pattern-summary"))
	sse.RemoveElementByID("pattern-error")
}

func refreshSectionRows(sse *datastar.ServerSentEventGenerator, db *sql.DB, sectionID int64) {
	rows, _ := model.ListRowsBySection(db, sectionID)
	for i := range rows {
		instructions, _ := model.ListInstructionsForRow(db, rows[i].ID)
		rows[i].Instructions = instructions
	}
	sse.PatchElementTempl(view.RowList(sectionID, rows), datastar.WithSelectorID("section-"+strconv.FormatInt(sectionID, 10)+"-rows"), datastar.WithModeInner())
	sse.RemoveElementByID("pattern-error")
}
