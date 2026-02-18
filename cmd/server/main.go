package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/stitchmap/stitchmap/internal/database"
	"github.com/stitchmap/stitchmap/internal/handler"
	"github.com/stitchmap/stitchmap/internal/model"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "stitchmap.db", "SQLite database file path")
	flag.Parse()

	// Open database.
	db, err := database.Open(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Run migrations.
	if err := database.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Clean up expired sessions on startup.
	if n, err := model.DeleteExpiredSessions(db); err == nil && n > 0 {
		fmt.Printf("Cleaned up %d expired sessions\n", n)
	}

	// Set up routes.
	mux := http.NewServeMux()

	// Public routes.
	mux.HandleFunc("GET /login", handler.LoginPage(db))
	mux.HandleFunc("POST /login", handler.Login(db))
	mux.HandleFunc("GET /register", handler.RegisterPage(db))
	mux.HandleFunc("POST /register", handler.Register(db))
	mux.HandleFunc("POST /logout", handler.Logout(db))

	// Authenticated routes.
	authed := http.NewServeMux()
	authed.HandleFunc("GET /{$}", handler.Dashboard(db))

	// Stitch routes.
	authed.HandleFunc("GET /stitches", handler.StitchIndex(db))
	authed.HandleFunc("POST /stitches", handler.StitchCreate(db))
	authed.HandleFunc("GET /stitches/new", handler.StitchNewForm(db))
	authed.HandleFunc("GET /stitches/{id}/edit", handler.StitchEdit(db))
	authed.HandleFunc("PUT /stitches/{id}", handler.StitchUpdate(db))
	authed.HandleFunc("DELETE /stitches/{id}", handler.StitchDelete(db))

	// Pattern routes.
	authed.HandleFunc("GET /patterns/new", handler.PatternNew(db))
	authed.HandleFunc("POST /patterns", handler.PatternCreate(db))
	authed.HandleFunc("GET /patterns/{id}", handler.PatternShow(db))
	authed.HandleFunc("GET /patterns/{id}/edit", handler.PatternEditForm(db))
	authed.HandleFunc("PUT /patterns/{id}", handler.PatternUpdate(db))
	authed.HandleFunc("DELETE /patterns/{id}", handler.PatternDelete(db))
	authed.HandleFunc("GET /patterns/{id}/sections-refresh", handler.SectionsRefresh(db))

	// Section routes.
	authed.HandleFunc("POST /patterns/{id}/sections", handler.SectionCreate(db))
	authed.HandleFunc("GET /sections/{id}/edit", handler.SectionEditForm(db))
	authed.HandleFunc("PUT /sections/{id}", handler.SectionUpdate(db))
	authed.HandleFunc("DELETE /sections/{id}", handler.SectionDelete(db))
	authed.HandleFunc("POST /sections/{id}/move-up", handler.SectionMoveUp(db))
	authed.HandleFunc("POST /sections/{id}/move-down", handler.SectionMoveDown(db))

	// Row routes.
	authed.HandleFunc("GET /sections/{id}/rows/new", handler.RowNewForm(db))
	authed.HandleFunc("POST /sections/{id}/rows", handler.RowCreate(db))
	authed.HandleFunc("GET /sections/{id}/rows-refresh", handler.RowsRefresh(db))
	authed.HandleFunc("GET /rows/{id}/edit", handler.RowEditForm(db))
	authed.HandleFunc("GET /rows/{id}/cancel-edit", handler.RowCancelEdit(db))
	authed.HandleFunc("PUT /rows/{id}", handler.RowUpdate(db))
	authed.HandleFunc("DELETE /rows/{id}", handler.RowDelete(db))
	authed.HandleFunc("POST /rows/{id}/move-up", handler.RowMoveUp(db))
	authed.HandleFunc("POST /rows/{id}/move-down", handler.RowMoveDown(db))

	// Instruction routes.
	authed.HandleFunc("GET /rows/{id}/instructions/new", handler.InstructionNewForm(db))
	authed.HandleFunc("GET /rows/{id}/instructions/group/new", handler.InstructionGroupNewForm(db))
	authed.HandleFunc("GET /rows/{id}/instructions-refresh", handler.InstructionsRefresh(db))
	authed.HandleFunc("POST /rows/{id}/instructions", handler.InstructionCreate(db))
	authed.HandleFunc("POST /rows/{id}/instructions/group", handler.InstructionGroupCreate(db))
	authed.HandleFunc("GET /instructions/{id}/edit", handler.InstructionEditForm(db))
	authed.HandleFunc("GET /instructions/{id}/children/new", handler.InstructionChildNewForm(db))
	authed.HandleFunc("POST /instructions/{id}/children", handler.InstructionChildCreate(db))
	authed.HandleFunc("PUT /instructions/{id}", handler.InstructionUpdate(db))
	authed.HandleFunc("DELETE /instructions/{id}", handler.InstructionDelete(db))
	authed.HandleFunc("POST /instructions/{id}/move-up", handler.InstructionMoveUp(db))
	authed.HandleFunc("POST /instructions/{id}/move-down", handler.InstructionMoveDown(db))

	mux.Handle("/", handler.RequireAuth(db, authed))

	fmt.Printf("StitchMap listening on %s\n", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
