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

// --- Ownership helpers ---

func checkInstructionOwnership(db *sql.DB, instructionID, userID int64) (rowID int64, ok bool) {
	rowID, err := model.GetRowIDForInstruction(db, instructionID)
	if err != nil {
		return 0, false
	}
	patternID, err := model.GetPatternIDForRow(db, rowID)
	if err != nil {
		return 0, false
	}
	return rowID, checkPatternOwnership(db, patternID, userID)
}

// --- Signal types (separate namespaces to avoid conflicts between forms) ---

type addInstrSignals struct {
	StitchID string `json:"addInstrStitchID"`
	Count    string `json:"addInstrCount"`
	Into     string `json:"addInstrInto"`
	Note     string `json:"addInstrNote"`
}

func (s *addInstrSignals) parse() (stitchID *int64, count int, into, note string) {
	if id, err := strconv.ParseInt(strings.TrimSpace(s.StitchID), 10, 64); err == nil && id > 0 {
		stitchID = &id
	}
	count, _ = strconv.Atoi(s.Count)
	if count < 1 {
		count = 1
	}
	into = strings.TrimSpace(s.Into)
	note = strings.TrimSpace(s.Note)
	return
}

type addChildSignals struct {
	StitchID string `json:"childInstrStitchID"`
	Count    string `json:"childInstrCount"`
	Into     string `json:"childInstrInto"`
	Note     string `json:"childInstrNote"`
}

func (s *addChildSignals) parse() (stitchID *int64, count int, into, note string) {
	if id, err := strconv.ParseInt(strings.TrimSpace(s.StitchID), 10, 64); err == nil && id > 0 {
		stitchID = &id
	}
	count, _ = strconv.Atoi(s.Count)
	if count < 1 {
		count = 1
	}
	into = strings.TrimSpace(s.Into)
	note = strings.TrimSpace(s.Note)
	return
}

type editInstrSignals struct {
	StitchID string `json:"editInstrStitchID"`
	Count    string `json:"editInstrCount"`
	Into     string `json:"editInstrInto"`
	Note     string `json:"editInstrNote"`
}

func (s *editInstrSignals) parse() (stitchID *int64, count int, into, note string) {
	if id, err := strconv.ParseInt(strings.TrimSpace(s.StitchID), 10, 64); err == nil && id > 0 {
		stitchID = &id
	}
	count, _ = strconv.Atoi(s.Count)
	if count < 1 {
		count = 1
	}
	into = strings.TrimSpace(s.Into)
	note = strings.TrimSpace(s.Note)
	return
}

type addGroupSignals struct {
	Repeat string `json:"addGrpRepeat"`
	Note   string `json:"addGrpNote"`
}

type editGroupSignals struct {
	Repeat string `json:"editGrpRepeat"`
	Note   string `json:"editGrpNote"`
}

// --- Form display handlers (loaded lazily on click) ---

func InstructionNewForm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		rowID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForRow(db, rowID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		stitches, _ := model.ListStitchesForUser(db, user.ID)
		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(
			view.AddInstructionForm(rowID, stitches),
			datastar.WithSelectorID("row-"+strconv.FormatInt(rowID, 10)+"-add-instr"),
		)
	}
}

func InstructionGroupNewForm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		rowID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForRow(db, rowID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(
			view.AddGroupForm(rowID),
			datastar.WithSelectorID("row-"+strconv.FormatInt(rowID, 10)+"-add-instr"),
		)
	}
}

func InstructionChildNewForm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		parentID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		rowID, ok := checkInstructionOwnership(db, parentID, user.ID)
		if !ok {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		stitches, _ := model.ListStitchesForUser(db, user.ID)
		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(
			view.AddChildInstructionForm(parentID, rowID, stitches),
			datastar.WithSelectorID("group-"+strconv.FormatInt(parentID, 10)+"-add-child"),
		)
	}
}

// --- CRUD handlers ---

func InstructionCreate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		rowID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForRow(db, rowID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		signals := &addInstrSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		stitchID, count, into, note := signals.parse()
		sse := datastar.NewSSE(w, r)

		if _, err := model.CreateInstruction(db, rowID, stitchID, count, into, note); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to add instruction."))
			return
		}

		refreshRowInstructions(sse, db, rowID, patternID)
	}
}

func InstructionGroupCreate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		rowID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForRow(db, rowID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		signals := &addGroupSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		groupRepeat, _ := strconv.Atoi(signals.Repeat)
		if groupRepeat < 1 {
			groupRepeat = 1
		}
		note := strings.TrimSpace(signals.Note)
		sse := datastar.NewSSE(w, r)

		if _, err := model.CreateGroupInstruction(db, rowID, groupRepeat, note); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to add group."))
			return
		}

		refreshRowInstructions(sse, db, rowID, patternID)
	}
}

func InstructionChildCreate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		parentID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		rowID, ok := checkInstructionOwnership(db, parentID, user.ID)
		if !ok {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		patternID, _ := model.GetPatternIDForRow(db, rowID)

		signals := &addChildSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		stitchID, count, into, note := signals.parse()
		sse := datastar.NewSSE(w, r)

		if _, err := model.CreateChildInstruction(db, parentID, stitchID, count, into, note); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to add stitch to group."))
			return
		}

		refreshRowInstructions(sse, db, rowID, patternID)
	}
}

func InstructionEditForm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		_, ok := checkInstructionOwnership(db, id, user.ID)
		if !ok {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		instr, err := model.FindInstructionByID(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		sse := datastar.NewSSE(w, r)

		if instr.IsGroup {
			sse.PatchElementTempl(
				view.EditGroupForm(instr),
				datastar.WithSelectorID("instruction-"+strconv.FormatInt(id, 10)),
			)
		} else {
			stitches, _ := model.ListStitchesForUser(db, user.ID)
			sse.PatchElementTempl(
				view.EditInstructionForm(instr, stitches),
				datastar.WithSelectorID("instruction-"+strconv.FormatInt(id, 10)),
			)
		}
	}
}

func InstructionUpdate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		rowID, ok := checkInstructionOwnership(db, id, user.ID)
		if !ok {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		patternID, _ := model.GetPatternIDForRow(db, rowID)

		instr, err := model.FindInstructionByID(db, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		sse := datastar.NewSSE(w, r)

		if instr.IsGroup {
			signals := &editGroupSignals{}
			if err := datastar.ReadSignals(r, signals); err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			groupRepeat, _ := strconv.Atoi(signals.Repeat)
			if groupRepeat < 1 {
				groupRepeat = 1
			}
			note := strings.TrimSpace(signals.Note)
			if err := model.UpdateGroupInstruction(db, id, groupRepeat, note); err != nil {
				sse.PatchElementTempl(view.PatternError("Failed to update group."))
				return
			}
		} else {
			signals := &editInstrSignals{}
			if err := datastar.ReadSignals(r, signals); err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			stitchID, count, into, note := signals.parse()
			if err := model.UpdateInstruction(db, id, stitchID, count, into, note); err != nil {
				sse.PatchElementTempl(view.PatternError("Failed to update instruction."))
				return
			}
		}

		refreshRowInstructions(sse, db, rowID, patternID)
	}
}

func InstructionDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		rowID, ok := checkInstructionOwnership(db, id, user.ID)
		if !ok {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		patternID, _ := model.GetPatternIDForRow(db, rowID)

		sse := datastar.NewSSE(w, r)

		if err := model.DeleteInstruction(db, id); err != nil {
			sse.PatchElementTempl(view.PatternError("Failed to delete instruction."))
			return
		}

		refreshRowInstructions(sse, db, rowID, patternID)
	}
}

func InstructionMoveUp(db *sql.DB) http.HandlerFunc {
	return instructionMoveHandler(db, model.MoveInstructionUp)
}

func InstructionMoveDown(db *sql.DB) http.HandlerFunc {
	return instructionMoveHandler(db, model.MoveInstructionDown)
}

func instructionMoveHandler(db *sql.DB, moveFn func(*sql.DB, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		rowID, ok := checkInstructionOwnership(db, id, user.ID)
		if !ok {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		patternID, _ := model.GetPatternIDForRow(db, rowID)

		sse := datastar.NewSSE(w, r)
		moveFn(db, id)
		refreshRowInstructions(sse, db, rowID, patternID)
	}
}

func InstructionsRefresh(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		rowID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

		patternID, err := model.GetPatternIDForRow(db, rowID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkPatternOwnership(db, patternID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		refreshRowInstructions(sse, db, rowID, patternID)
	}
}

// refreshRowInstructions patches the instruction list and resets the add-form area, then refreshes the summary.
func refreshRowInstructions(sse *datastar.ServerSentEventGenerator, db *sql.DB, rowID, patternID int64) {
	instructions, _ := model.ListInstructionsForRow(db, rowID)
	sse.PatchElementTempl(
		view.InstructionListContent(rowID, instructions),
		datastar.WithSelectorID("row-"+strconv.FormatInt(rowID, 10)+"-instructions"),
		datastar.WithModeInner(),
	)
	sse.PatchElementTempl(
		view.AddInstructionArea(rowID),
		datastar.WithSelectorID("row-"+strconv.FormatInt(rowID, 10)+"-add-instr"),
	)
	// Refresh pattern summary.
	_, sections, err := model.LoadPatternFull(db, patternID)
	if err == nil {
		sse.PatchElementTempl(
			view.PatternSummaryBlock(sections),
			datastar.WithSelectorID("pattern-summary"),
		)
	}
	sse.RemoveElementByID("pattern-error")
}
