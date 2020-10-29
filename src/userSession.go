package main

import (
	"database/sql"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
	"time"
)

type AccessDetails struct {
	AccessUuid string
	Email      string
}

type LoginDetails struct {
	Email    string `json:"Email"`
	Password string `json:"Password"`
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

	var user LoginDetails
	err := json.NewDecoder(c.Request.Body).Decode(&user)

	if err != nil {
		panic(err)
	}

	err = s.db.QueryRow("SELECT email FROM users WHERE email=$1", user.Email).Scan(&user)

	httpCode := http.StatusOK

	if err == sql.ErrNoRows {

		hashedPassword, errPass := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)

		if errPass != nil {
			httpCode = http.StatusInternalServerError
			panic(errPass)
		}

		sqlStatement := `INSERT INTO users (email, password) VALUES ($1, $2)`

		_, errDB := s.db.Exec(sqlStatement, user.Email, hashedPassword)

		if errDB != nil {
			httpCode = http.StatusInternalServerError
			panic(errDB)
		}

	} else {

		httpCode = http.StatusConflict
	}

	userAgent := c.Request.Header.Get("User-Agent")

	if strings.Contains(userAgent, "curl") {

		c.Writer.WriteHeader(httpCode)

	} else {

		result, _ := json.Marshal(httpCode)
		c.Writer.Header().Set("Content-Type", "application/json")
		_, err = c.Writer.Write(result)

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

	var user LoginDetails
	err := json.NewDecoder(c.Request.Body).Decode(&user)

	if err != nil {
		panic(err)
	}

	var databaseEmail string
	var databasePassword string
	httpCode := http.StatusOK

	err = s.db.QueryRow("SELECT email, password FROM users WHERE email=$1", user.Email).Scan(&databaseEmail, &databasePassword)

	userAgent := c.Request.Header.Get("User-Agent")

	if err != nil {

		httpCode = http.StatusUnauthorized
	}

	err = bcrypt.CompareHashAndPassword([]byte(databasePassword), []byte(user.Password))

	if err != nil {

		httpCode = http.StatusUnauthorized
	}

	ts, err := CreateToken(user.Email)

	if err != nil {

		httpCode = http.StatusUnprocessableEntity
	}

	saveErr := CreateAuth(user.Email, ts)

	if saveErr != nil {

		httpCode = http.StatusUnprocessableEntity
	}

	if httpCode != http.StatusOK {

		if strings.Contains(userAgent, "curl") {

			c.Writer.WriteHeader(httpCode)

		} else {

			redirect(c, "login.html", "not-logged", nil, false, httpCode, "Login Page")

		}

	} else {

		http.SetCookie(c.Writer, &http.Cookie{
			Name:    "access_token",
			Value:   ts.AccessToken,
			Expires: time.Now().Local().Add(time.Minute * 15),
		})

		if strings.Contains(userAgent, "curl") {

			c.Writer.WriteHeader(http.StatusOK)

		}

	}

}

func logout(c *gin.Context) {

	checkSession(c)

	au, _ := ExtractTokenMetadata(c)
	deleted, delErr := DeleteAuth(au.AccessUuid)

	userAgent := c.Request.Header.Get("User-Agent")

	httpCode := http.StatusOK

	if delErr != nil || deleted == 0 { //if any goes wrong

		httpCode = http.StatusUnauthorized
		//redirect(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "Login Page")
	}

	if httpCode != http.StatusOK {

		if strings.Contains(userAgent, "curl") {

			c.Writer.WriteHeader(httpCode)

		} else {

			redirect(c, "login.html", "not-logged", nil, false, httpCode, "Login Page")

		}

	} else {

		http.SetCookie(c.Writer, &http.Cookie{
			Name:    "access_token",
			Value:   "",
			Expires: time.Now().Local(),
		})

		if strings.Contains(userAgent, "curl") {

			c.Writer.WriteHeader(httpCode)

		} else {

			redirect(c, "login.html", "not-logged", nil, false, http.StatusOK, "Login Page")

		}

	}

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
