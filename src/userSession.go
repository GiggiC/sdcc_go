package main

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"time"
)

func registrationPage(c *gin.Context) {

	token := ExtractToken(c)

	if token != "" {

		c.Redirect(http.StatusMovedPermanently, "/notificationsPage")

	} else {

		c.HTML(
			// Set the HTTP status to 200 (OK)
			http.StatusOK,
			// Use the index.html template
			"registration.html",
			// Pass the data that the page uses (in this case, 'title')
			gin.H{
				"title":  "",
				"status": "not-logged",
			},
		)
	}

}

func (s *server) registration(c *gin.Context) {

	username := c.Request.FormValue("username")
	password := c.Request.FormValue("password")
	email := c.Request.FormValue("email")

	var user string

	err := s.db.QueryRow("SELECT email FROM users WHERE email=$1", email).Scan(&user)

	switch {

	case err == sql.ErrNoRows:

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		if err != nil {

			c.HTML(
				// Set the HTTP status to 200 (OK)
				http.StatusInternalServerError,
				// Use the index.html template
				"registration.html",
				// Pass the data that the page uses (in this case, 'title')
				gin.H{
					"title":  "",
					"status": "not-logged",
				},
			)
		}

		sqlStatement := `INSERT INTO users (email, username, password) VALUES ($1, $2, $3)`

		_, err = s.db.Exec(sqlStatement, email, username, hashedPassword)

		if err != nil {
			panic(err)
		}

		c.Redirect(301, "/")
		return

	case err != nil:

		c.HTML(
			// Set the HTTP status to 200 (OK) TODO
			http.StatusInternalServerError,
			// Use the index.html template
			"registrationError.html",
			// Pass the data that the page uses (in this case, 'title')
			gin.H{
				"title":  "",
				"status": "not-logged",
			},
		)

		return

	default:

		c.HTML(
			// Set the HTTP status to 200 (OK) TODO
			http.StatusInternalServerError,
			// Use the index.html template
			"registrationError.html",
			// Pass the data that the page uses (in this case, 'title')
			gin.H{
				"title":  "",
				"status": "not-logged",
			},
		)

		return
	}
}

type AccessDetails struct {
	AccessUuid string
	Email      string
}

func loginPage(c *gin.Context) {

	token := ExtractToken(c)

	if token != "" {

		c.Redirect(http.StatusMovedPermanently, "/notificationsPage")
	}
}

func (s *server) login(c *gin.Context) {

	email := c.Request.FormValue("email")
	password := c.Request.FormValue("password")

	var databaseEmail string
	var databasePassword string

	err := s.db.QueryRow("SELECT email, password FROM users WHERE email=$1", email).Scan(&databaseEmail, &databasePassword)

	if err != nil {
		//res.WriteHeader(http.StatusUnauthorized) TODO
		http.Redirect(c.Writer, c.Request, "/login", 301)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(databasePassword), []byte(password))
	if err != nil {
		//res.WriteHeader(http.StatusUnauthorized) TODO
		http.Redirect(c.Writer, c.Request, "/login", 301)
		return
	}

	ts, err := CreateToken(email)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, err.Error())
		return
	}
	saveErr := CreateAuth(email, ts)
	if saveErr != nil {
		c.JSON(http.StatusUnprocessableEntity, saveErr.Error())
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:    "access_token",
		Value:   ts.AccessToken,
		Expires: time.Now().Add(time.Minute * 15),
	})

	c.Redirect(301, "/notificationsPage")
}

func logout(c *gin.Context) {

	checkSession(c)

	au, _ := ExtractTokenMetadata(c)

	deleted, delErr := DeleteAuth(au.AccessUuid)

	if delErr != nil || deleted == 0 { //if any goes wrong

		c.JSON(http.StatusUnauthorized, "unauthorized")
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:    "access_token",
		Value:   "",
		Expires: time.Now(),
	})

	c.HTML(
		http.StatusOK,
		// Use the index.html template
		"login.html",
		// Pass the data that the page uses (in this case, 'title')
		gin.H{
			"status": "not-logged",
		},
	)

}

func checkSession(c *gin.Context) {

	tokenAuth, err := ExtractTokenMetadata(c)

	if err != nil {
		c.HTML(
			// Set the HTTP status to 200 (OK)
			http.StatusUnauthorized,
			// Use the index.html template
			"login.html",
			// Pass the data that the page uses (in this case, 'title')
			gin.H{
				"status": "not-logged",
			},
		)
		return
	}

	err = FetchAuth(tokenAuth)

	if err != nil {
		c.HTML(
			// Set the HTTP status to 200 (OK)
			http.StatusUnauthorized,
			// Use the index.html template
			"login.html",
			// Pass the data that the page uses (in this case, 'title')
			gin.H{
				"status": "not-logged",
			},
		)
		return
	}

}

func redirecter(c *gin.Context, url string, status string, results interface{}) {

	checkSession(c)

	c.HTML(
		// Set the HTTP status to 200 (OK)
		http.StatusOK,
		// Use the index.html template
		url,
		// Pass the data that the page uses (in this case, 'title')
		gin.H{
			"title":   "",
			"status":  status,
			"results": results,
		},
	)
}
