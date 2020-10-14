package main

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"time"
)

type AccessDetails struct {
	AccessUuid string
	Email      string
}

func registrationPage(c *gin.Context) {

	err := TokenValid(c)

	if err != nil {

		redirect(c, "registration.html", "not-logged", nil, false, http.StatusOK, "Registration Page")

	} else {

		redirect(c, "notifications.html", "logged", nil, true, http.StatusOK, "Notifications")
	}
}

func (s *server) registration(c *gin.Context) {

	password := c.Request.FormValue("password")
	email := c.Request.FormValue("email")

	var user string

	err := s.db.QueryRow("SELECT email FROM users WHERE email=$1", email).Scan(&user)

	switch {

	case err == sql.ErrNoRows:

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		if err != nil {

			redirect(c, "registration.html", "not-logged", nil, false, http.StatusInternalServerError, "Registration Page")
		}

		sqlStatement := `INSERT INTO users (email, password) VALUES ($1, $2)`

		_, err = s.db.Exec(sqlStatement, email, hashedPassword)

		if err != nil {
			panic(err)
		}

		redirect(c, "login.html", "not-logged", nil, false, http.StatusOK, "Login Page")
		return

	case err != nil:

		redirect(c, "registrationError.html", "not-logged", nil, false, http.StatusInternalServerError, "Registration Error")
		return

	default:

		redirect(c, "registrationError.html", "not-logged", nil, false, http.StatusInternalServerError, "Registration Error") //TODO general error
		return
	}
}

func loginPage(c *gin.Context) {

	err := TokenValid(c)

	if err != nil {

		redirect(c, "login.html", "not-logged", nil, false, http.StatusOK, "Login Page")

	} else {

		redirect(c, "notifications.html", "logged", nil, true, http.StatusOK, "Notifications")
	}
}

func (s *server) login(c *gin.Context) {

	email := c.Request.FormValue("email")
	password := c.Request.FormValue("password")

	var databaseEmail string
	var databasePassword string

	err := s.db.QueryRow("SELECT email, password FROM users WHERE email=$1", email).Scan(&databaseEmail, &databasePassword)

	if err != nil {

		redirect(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "Login Page")
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(databasePassword), []byte(password))

	if err != nil {

		redirect(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "Login Page")
		return
	}

	ts, err := CreateToken(email)

	if err != nil {

		redirect(c, "login.html", "not-logged", nil, false, http.StatusUnprocessableEntity, "Login Page")
		return
	}

	saveErr := CreateAuth(email, ts)

	if saveErr != nil {
		redirect(c, "login.html", "not-logged", nil, false, http.StatusUnprocessableEntity, "Login Page")
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:    "access_token",
		Value:   ts.AccessToken,
		Expires: time.Now().Local().Add(time.Minute * 15),
	})

	redirect(c, "notifications.html", "logged", nil, false, http.StatusOK, "Notifications")

}

func logout(c *gin.Context) {

	checkSession(c)

	au, _ := ExtractTokenMetadata(c)
	deleted, delErr := DeleteAuth(au.AccessUuid)

	if delErr != nil || deleted == 0 { //if any goes wrong

		redirect(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "Login Page") //TODO error
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:    "access_token",
		Value:   "",
		Expires: time.Now().Local(),
	})

	redirect(c, "login.html", "not-logged", nil, false, http.StatusOK, "Login Page")
}

func checkSession(c *gin.Context) {

	tokenAuth, exErr := ExtractTokenMetadata(c)
	fErr := FetchAuth(tokenAuth)

	if exErr != nil || fErr != nil {

		redirect(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "Login Page")
		c.Abort()
	}
}

func redirect(c *gin.Context, url string, status string, results interface{}, check bool, code int, title string) {

	var email string

	if check {
		checkSession(c)
		ad, _ := ExtractTokenMetadata(c)
		email = ad.Email
	}

	c.HTML(
		code,
		url,
		gin.H{
			"title":            title,
			"status":           status,
			"results":          results,
			"email":            email,
			"deliverySemantic": deliverySemantic,
		},
	)
}
