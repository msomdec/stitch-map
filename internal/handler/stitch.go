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

type stitchSignals struct {
	Name string `json:"stitchName"`
	Abbr string `json:"stitchAbbr"`
	Desc string `json:"stitchDesc"`
}

// StitchIndex renders the full stitches management page (normal GET, no SSE).
func StitchIndex(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		stitches, err := model.ListStitchesForUser(db, user.ID)
		if err != nil {
			renderTempl(w, r, http.StatusInternalServerError, view.StitchPage(view.StitchPageData{
				Error: "Failed to load stitches.",
			}, user.Email))
			return
		}

		renderTempl(w, r, http.StatusOK, view.StitchPage(view.StitchPageData{
			Stitches: stitches,
		}, user.Email))
	}
}

// StitchCreate handles POST /stitches via Datastar SSE.
func StitchCreate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		signals := &stitchSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(signals.Name)
		abbr := strings.TrimSpace(signals.Abbr)
		desc := strings.TrimSpace(signals.Desc)

		sse := datastar.NewSSE(w, r)

		if name == "" || abbr == "" {
			sse.PatchElementTempl(view.StitchError("Name and abbreviation are required."))
			return
		}

		_, err := model.CreateStitch(db, user.ID, name, abbr, desc)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				sse.PatchElementTempl(view.StitchError("A stitch with that abbreviation already exists."))
			} else {
				sse.PatchElementTempl(view.StitchError("Failed to create stitch."))
			}
			return
		}

		// Re-render the full stitch list and a blank form.
		stitches, _ := model.ListStitchesForUser(db, user.ID)
		sse.PatchElementTempl(view.StitchList(stitches), datastar.WithSelectorID("stitch-list"))
		sse.PatchElementTempl(view.StitchForm(nil))
		sse.RemoveElementByID("stitch-error")
	}
}

// StitchEdit handles GET /stitches/{id}/edit via Datastar SSE — returns the edit form.
func StitchEdit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		stitch, err := model.FindStitchByID(db, id)
		if err != nil {
			http.Error(w, "Stitch not found", http.StatusNotFound)
			return
		}

		if stitch.IsBuiltin || stitch.UserID == nil || *stitch.UserID != user.ID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(view.StitchForm(stitch))
	}
}

// StitchNewForm handles GET /stitches/new via Datastar SSE — returns a blank form.
func StitchNewForm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r)
		sse.PatchElementTempl(view.StitchForm(nil))
	}
}

// StitchUpdate handles PUT /stitches/{id} via Datastar SSE.
func StitchUpdate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		signals := &stitchSignals{}
		if err := datastar.ReadSignals(r, signals); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(signals.Name)
		abbr := strings.TrimSpace(signals.Abbr)
		desc := strings.TrimSpace(signals.Desc)

		sse := datastar.NewSSE(w, r)

		if name == "" || abbr == "" {
			sse.PatchElementTempl(view.StitchError("Name and abbreviation are required."))
			return
		}

		if err := model.UpdateStitch(db, id, user.ID, name, abbr, desc); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				sse.PatchElementTempl(view.StitchError("A stitch with that abbreviation already exists."))
			} else {
				sse.PatchElementTempl(view.StitchError("Failed to update stitch."))
			}
			return
		}

		stitches, _ := model.ListStitchesForUser(db, user.ID)
		sse.PatchElementTempl(view.StitchList(stitches), datastar.WithSelectorID("stitch-list"))
		sse.PatchElementTempl(view.StitchForm(nil))
		sse.RemoveElementByID("stitch-error")
	}
}

// StitchDelete handles DELETE /stitches/{id} via Datastar SSE.
func StitchDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		sse := datastar.NewSSE(w, r)

		if err := model.DeleteStitch(db, id, user.ID); err != nil {
			sse.PatchElementTempl(view.StitchError("Cannot delete this stitch. It may be built-in or not yours."))
			return
		}

		sse.RemoveElementf("#stitch-%d", id)
		sse.RemoveElementByID("stitch-error")
	}
}
