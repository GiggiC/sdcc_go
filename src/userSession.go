package main

import (
	"database/sql"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"net/http"
	"path/filepath"
)

var (
	// key must be 16, 24 or 32 bytes long (AES-128, AES-192 or AES-256)
	key   = []byte("super-secret-key")
	store = sessions.NewCookieStore(key)
)

func initSession() {
	store = sessions.NewCookieStore(key)
	store.Options = &sessions.Options{
		Path:   "/",
		MaxAge: 10000,
	}
}

func registrationPage(res http.ResponseWriter, req *http.Request) {

	data := Object{
		Status: "not-logged",
	}

	lp := filepath.Join("../templates", "layout.html")
	fp := filepath.Join("../templates", "registration.html")

	tmpl, _ := template.ParseFiles(lp, fp)
	tmpl.ExecuteTemplate(res, "layout", data)
}

func (s *server) registration(res http.ResponseWriter, req *http.Request) {

	username := req.FormValue("username")
	password := req.FormValue("password")
	email := req.FormValue("email")

	var user string

	err := s.db.QueryRow("SELECT email FROM users WHERE email=$1", email).Scan(&user)

	switch {

	case err == sql.ErrNoRows:

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		if err != nil {

			http.Error(res, "Server error, unable to create your account.", 500)
			return
		}

		sqlStatement := `
			INSERT INTO users (email, username, password)
			VALUES ($1, $2, $3)`

		_, err = s.db.Exec(sqlStatement, email, username, hashedPassword)

		if err != nil {
			panic(err)
		}

		http.Redirect(res, req, "/loginPage", 301)
		return

	case err != nil:

		http.Redirect(res, req, "/registrationError", 500)
		return

	default:

		http.Redirect(res, req, "/registrationError", 301)
		return
	}
}

func registrationError(w http.ResponseWriter, r *http.Request) {

	lp := filepath.Join("../templates", "layout.html")
	fp := filepath.Join("../templates", "registrationError.html")

	tmpl, _ := template.ParseFiles(lp, fp)
	tmpl.ExecuteTemplate(w, "layout", nil)
}

func loginPage(res http.ResponseWriter, req *http.Request) {

	data := Object{
		Status: "not-logged",
	}

	lp := filepath.Join("../templates", "layout.html")
	fp := filepath.Join("../templates", "login.html")

	tmpl, _ := template.ParseFiles(lp, fp)
	tmpl.ExecuteTemplate(res, "layout", data)
}

func (s *server) login(res http.ResponseWriter, req *http.Request) {

	session, _ := store.New(req, "session")

	email := req.FormValue("email")
	password := req.FormValue("password")

	var databaseEmail string
	var databasePassword string

	err := s.db.QueryRow("SELECT email, password FROM users WHERE email=$1", email).Scan(&databaseEmail, &databasePassword)

	if err != nil {
		http.Redirect(res, req, "/login", 301)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(databasePassword), []byte(password))
	if err != nil {
		http.Redirect(res, req, "/login", 301)
		return
	}

	session.Values["authenticated"] = true
	session.Values["user"] = databaseEmail
	session.Save(req, res)

	//TODO 301
	http.Redirect(res, req, "/notificationsPage", 301)
}

func logout(res http.ResponseWriter, req *http.Request) {

	session, _ := store.Get(req, "session")

	// Revoke users authentication
	session.Values["authenticated"] = false

	//delete(session.Values, "authenticated")
	//session.Options.MaxAge = -1

	_ = session.Save(req, res)

	http.Redirect(res, req, "/", 301)
}

func checkSession(res http.ResponseWriter, req *http.Request) {

	session, _ := store.Get(req, "session")

	// Check if user is authenticated
	if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {

		http.Redirect(res, req, "/", http.StatusForbidden)
		return
	}
}

func redirecter(res http.ResponseWriter, req *http.Request, url string, results interface{}) {

	data := Object{
		Status: "logged",
		Data:   results,
	}

	lp := filepath.Join("../templates", "layout.html")
	fp := filepath.Join("../templates", url)

	tmpl, _ := template.ParseFiles(lp, fp)
	tmpl.ExecuteTemplate(res, "layout", data)
}
