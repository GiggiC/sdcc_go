package main

import (
	"database/sql"
	"github.com/dgrijalva/jwt-go"
	_ "github.com/dgrijalva/jwt-go"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"net/http"
	"path/filepath"
	"time"
)

var jwtKey = []byte("my_secret_key")

type Credentials struct {
	Password string `json:"password"`
	Username string `json:"username"`
}

type Claims struct {
	Username string `json:"username"`
	jwt.StandardClaims
}

func registrationPage(res http.ResponseWriter, req *http.Request) {

	// Check if user is authenticated
	if checkSession(res, req) == "" {

		data := Object{
			Status: "not-logged",
		}

		lp := filepath.Join("../templates", "layout.html")
		fp := filepath.Join("../templates", "registration.html")

		tmpl, _ := template.ParseFiles(lp, fp)
		tmpl.ExecuteTemplate(res, "layout", data)

		return
	}

	http.Redirect(res, req, "/notificationsPage", 301)
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

		sqlStatement := `INSERT INTO users (email, username, password) VALUES ($1, $2, $3)`

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

	// Check if user is authenticated
	if checkSession(res, req) == "" {

		data := Object{
			Status: "not-logged",
		}

		lp := filepath.Join("../templates", "layout.html")
		fp := filepath.Join("../templates", "login.html")

		tmpl, _ := template.ParseFiles(lp, fp)
		tmpl.ExecuteTemplate(res, "layout", data)

		return
	}

	http.Redirect(res, req, "/notificationsPage", 301)
}

func (s *server) login(res http.ResponseWriter, req *http.Request) {

	email := req.FormValue("email")
	password := req.FormValue("password")

	var creds Credentials

	creds.Username = email

	var databaseEmail string
	var databasePassword string

	err := s.db.QueryRow("SELECT email, password FROM users WHERE email=$1", email).Scan(&databaseEmail, &databasePassword)

	if err != nil {
		//res.WriteHeader(http.StatusUnauthorized) TODO
		http.Redirect(res, req, "/login", 301)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(databasePassword), []byte(password))
	if err != nil {
		//res.WriteHeader(http.StatusUnauthorized) TODO
		http.Redirect(res, req, "/login", 301)
		return
	}

	// Declare the expiration time of the token
	// here, we have kept it as 5 minutes
	expirationTime := time.Now().Add(5 * time.Minute)
	// Create the JWT claims, which includes the username and expiry time
	claims := &Claims{
		Username: creds.Username,
		StandardClaims: jwt.StandardClaims{
			// In JWT, the expiry time is expressed as unix milliseconds
			ExpiresAt: expirationTime.Unix(),
		},
	}

	// Declare the token with the algorithm used for signing, and the claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Create the JWT string
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		// If there is an error in creating the JWT return an internal server error
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Finally, we set the client cookie for "token" as the JWT we just generated
	// we also set an expiry time which is the same as the token itself
	http.SetCookie(res, &http.Cookie{
		Name:    "token",
		Value:   tokenString,
		Expires: expirationTime,
	})

	//TODO 301
	http.Redirect(res, req, "/notificationsPage", 301)
}

func logout(res http.ResponseWriter, req *http.Request) { //TODO BLACKLIST

	// We can obtain the session token from the requests cookies, which come with every request
	c, _ := req.Cookie("token")

	// Get the JWT string from the cookie
	tknStr := c.Value
	http.SetCookie(res, &http.Cookie{
		Name:    "token",
		Value:   tknStr,
		Expires: time.Now(),
	})

	http.Redirect(res, req, "/", 301)
}

func checkSession(res http.ResponseWriter, req *http.Request) string {

	// We can obtain the session token from the requests cookies, which come with every request
	c, err := req.Cookie("token")
	if err != nil {
		if err == http.ErrNoCookie {
			// If the cookie is not set, return an unauthorized status
			res.WriteHeader(http.StatusUnauthorized)
			return ""
		}
		// For any other type of error, return a bad request status
		res.WriteHeader(http.StatusBadRequest)
		return ""
	}

	// Get the JWT string from the cookie
	tknStr := c.Value

	// Initialize a new instance of `Claims`
	claims := &Claims{}

	// Parse the JWT string and store the result in `claims`.
	// Note that we are passing the key in this method as well. This method will return an error
	// if the token is invalid (if it has expired according to the expiry time we set on sign in),
	// or if the signature does not match
	tkn, err := jwt.ParseWithClaims(tknStr, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil {
		if err == jwt.ErrSignatureInvalid {
			res.WriteHeader(http.StatusUnauthorized)
			return ""
		}
		res.WriteHeader(http.StatusBadRequest)
		return ""
	}
	if !tkn.Valid {
		res.WriteHeader(http.StatusUnauthorized)
		return ""
	}

	// Finally, return the welcome message to the user, along with their
	// username given in the token
	return claims.Username
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
