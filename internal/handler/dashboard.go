package handler

import (
	"database/sql"
	"net/http"

	"github.com/stitchmap/stitchmap/internal/model"
	"github.com/stitchmap/stitchmap/internal/view"
)

// Dashboard renders the main dashboard for authenticated users.
func Dashboard(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		patterns, _ := model.ListPatternsByUser(db, user.ID)
		renderTempl(w, r, http.StatusOK, view.DashboardPage(view.DashboardData{
			Email:    user.Email,
			Patterns: patterns,
		}))
	}
}
