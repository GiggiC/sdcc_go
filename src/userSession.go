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

		redirecter(c, "registration.html", "not-logged", nil, false, http.StatusOK, "")

	} else {

		redirecter(c, "notifications.html", "logged", nil, true, http.StatusOK, "")
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

			redirecter(c, "registration.html", "not-logged", nil, false, http.StatusInternalServerError, "")
		}

		sqlStatement := `INSERT INTO users (email, password) VALUES ($1, $2)`

		_, err = s.db.Exec(sqlStatement, email, hashedPassword)

		if err != nil {
			panic(err)
		}

		redirecter(c, "login.html", "not-logged", nil, false, http.StatusOK, "")
		return

	case err != nil:

		redirecter(c, "registrationError.html", "not-logged", nil, false, http.StatusInternalServerError, "")
		return

	default:

		redirecter(c, "registrationError.html", "not-logged", nil, false, http.StatusInternalServerError, "") //TODO general error
		return
	}
}

func loginPage(c *gin.Context) {

	err := TokenValid(c)

	if err != nil {

		redirecter(c, "login.html", "not-logged", nil, false, http.StatusOK, "")

	} else {

		redirecter(c, "notifications.html", "logged", nil, true, http.StatusOK, "")
	}
}

func (s *server) login(c *gin.Context) {

	email := c.Request.FormValue("email")
	password := c.Request.FormValue("password")

	var databaseEmail string
	var databasePassword string

	err := s.db.QueryRow("SELECT email, password FROM users WHERE email=$1", email).Scan(&databaseEmail, &databasePassword)

	if err != nil {

		redirecter(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "")
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(databasePassword), []byte(password))

	if err != nil {

		redirecter(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "")
		return
	}

	ts, err := CreateToken(email)

	if err != nil {

		redirecter(c, "login.html", "not-logged", nil, false, http.StatusUnprocessableEntity, "")
		return
	}

	saveErr := CreateAuth(email, ts)

	if saveErr != nil {
		redirecter(c, "login.html", "not-logged", nil, false, http.StatusUnprocessableEntity, "")
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:    "access_token",
		Value:   ts.AccessToken,
		Expires: time.Now().Add(time.Minute * 15),
	})

	redirecter(c, "notifications.html", "logged", nil, false, http.StatusOK, "")

}

func logout(c *gin.Context) {

	checkSession(c)

	au, _ := ExtractTokenMetadata(c)
	deleted, delErr := DeleteAuth(au.AccessUuid)

	if delErr != nil || deleted == 0 { //if any goes wrong

		redirecter(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "") //TODO error
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:    "access_token",
		Value:   "",
		Expires: time.Now(),
	})

	redirecter(c, "login.html", "not-logged", nil, false, http.StatusOK, "")
}

func checkSession(c *gin.Context) {

	tokenAuth, exErr := ExtractTokenMetadata(c)
	fErr := FetchAuth(tokenAuth)

	if exErr != nil || fErr != nil {

		redirecter(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "")
		c.Abort()
	}
}

func redirecter(c *gin.Context, url string, status string, results interface{}, check bool, code int, title string) {

	if check {
		checkSession(c)
	}

	c.HTML(
		code,
		url,
		gin.H{
			"title":   title,
			"status":  status,
			"results": results,
		},
	)
}
