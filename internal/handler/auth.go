package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/stitchmap/stitchmap/internal/model"
	"github.com/stitchmap/stitchmap/internal/view"
)

const sessionCookieName = "session"
const sessionMaxAge = 30 * 24 * 60 * 60 // 30 days in seconds

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   sessionMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func renderTempl(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	component.Render(r.Context(), w)
}

// LoginPage renders the login form.
func LoginPage(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderTempl(w, r, http.StatusOK, view.LoginPage(view.AuthPageData{}))
	}
}

// Login handles POST /login.
func Login(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")

		if email == "" || password == "" {
			renderTempl(w, r, http.StatusUnprocessableEntity, view.LoginPage(view.AuthPageData{
				Error: "Email and password are required.",
				Email: email,
			}))
			return
		}

		user, err := model.FindUserByEmail(db, email)
		if err != nil {
			renderTempl(w, r, http.StatusUnprocessableEntity, view.LoginPage(view.AuthPageData{
				Error: "Invalid email or password.",
				Email: email,
			}))
			return
		}

		if !model.CheckPassword(user, password) {
			renderTempl(w, r, http.StatusUnprocessableEntity, view.LoginPage(view.AuthPageData{
				Error: "Invalid email or password.",
				Email: email,
			}))
			return
		}

		session, err := model.CreateSession(db, user.ID)
		if err != nil {
			renderTempl(w, r, http.StatusInternalServerError, view.LoginPage(view.AuthPageData{
				Error: "Something went wrong. Please try again.",
				Email: email,
			}))
			return
		}

		setSessionCookie(w, session.ID)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// RegisterPage renders the registration form.
func RegisterPage(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderTempl(w, r, http.StatusOK, view.RegisterPage(view.AuthPageData{}))
	}
}

// Register handles POST /register.
func Register(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")
		passwordConfirm := r.FormValue("password_confirm")

		if email == "" || password == "" {
			renderTempl(w, r, http.StatusUnprocessableEntity, view.RegisterPage(view.AuthPageData{
				Error: "Email and password are required.",
				Email: email,
			}))
			return
		}

		if len(password) < 8 {
			renderTempl(w, r, http.StatusUnprocessableEntity, view.RegisterPage(view.AuthPageData{
				Error: "Password must be at least 8 characters.",
				Email: email,
			}))
			return
		}

		if password != passwordConfirm {
			renderTempl(w, r, http.StatusUnprocessableEntity, view.RegisterPage(view.AuthPageData{
				Error: "Passwords do not match.",
				Email: email,
			}))
			return
		}

		// Check if email is already taken.
		if _, err := model.FindUserByEmail(db, email); err == nil {
			renderTempl(w, r, http.StatusUnprocessableEntity, view.RegisterPage(view.AuthPageData{
				Error: "An account with that email already exists.",
				Email: email,
			}))
			return
		}

		user, err := model.CreateUser(db, email, password)
		if err != nil {
			renderTempl(w, r, http.StatusInternalServerError, view.RegisterPage(view.AuthPageData{
				Error: "Something went wrong. Please try again.",
				Email: email,
			}))
			return
		}

		session, err := model.CreateSession(db, user.ID)
		if err != nil {
			renderTempl(w, r, http.StatusInternalServerError, view.RegisterPage(view.AuthPageData{
				Error: "Account created but could not log in. Please try logging in.",
				Email: email,
			}))
			return
		}

		setSessionCookie(w, session.ID)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// Logout handles POST /logout.
func Logout(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			model.DeleteSession(db, cookie.Value)
		}
		clearSessionCookie(w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}
