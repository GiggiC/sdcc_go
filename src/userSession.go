package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"github.com/twinj/uuid"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"os"
	"time"
)

var (
	router = gin.Default()
)

var client *redis.Client
var ctx = context.Background()

func initRedis() {
	//Initializing redis
	dsn := os.Getenv("REDIS_DSN")
	if len(dsn) == 0 {
		dsn = "localhost:6379"
	}
	client = redis.NewClient(&redis.Options{
		Addr: dsn, //redis port
	})

	_, err := client.Ping(ctx).Result()
	if err != nil {
		panic(err)
	}
}

type TokenDetails struct {
	AccessToken string
	AccessUuid  string
	AtExpires   int64
}

func registrationPage(c *gin.Context) {

	_, err := c.Request.Cookie("access_token")

	if err == nil {
		redirecter(c, "notifications.html", "logged", nil)

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

func loginPage(c *gin.Context) {

	_, err := c.Request.Cookie("access_token")

	if err == nil {
		redirecter(c, "notifications.html", "logged", nil)

	} else {

		c.HTML(
			// Set the HTTP status to 200 (OK)
			http.StatusOK,
			// Use the index.html template
			"login.html",
			// Pass the data that the page uses (in this case, 'title')
			gin.H{
				"title":  "",
				"status": "not-logged",
			},
		)
	}

}

func CreateToken(email string) (*TokenDetails, error) {

	td := &TokenDetails{}
	td.AtExpires = time.Now().Add(time.Minute * 15).Unix()
	td.AccessUuid = uuid.NewV4().String()

	var err error
	//Creating Access Token
	os.Setenv("ACCESS_SECRET", "jdnfksdmfksd") //this should be in an env file
	atClaims := jwt.MapClaims{}
	atClaims["authorized"] = true
	atClaims["access_uuid"] = td.AccessUuid
	atClaims["email"] = email
	atClaims["exp"] = td.AtExpires
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	td.AccessToken, err = at.SignedString([]byte(os.Getenv("ACCESS_SECRET")))

	if err != nil {
		return nil, err
	}

	return td, nil
}

func CreateAuth(email string, td *TokenDetails) error {

	at := time.Unix(td.AtExpires, 0) //converting Unix to UTC(to Time object)
	now := time.Now()

	errAccess := client.Set(ctx, td.AccessUuid, email, at.Sub(now)).Err()

	if errAccess != nil {
		return errAccess
	}

	return nil
}

func VerifyToken(c *gin.Context) (*jwt.Token, error) {

	tokenString := ExtractToken(c)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {

		//Make sure that the token method conform to "SigningMethodHMAC"
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("ACCESS_SECRET")), nil
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

func TokenValid(c *gin.Context) error {

	token, err := VerifyToken(c)
	if err != nil {
		return err
	}
	if _, ok := token.Claims.(jwt.Claims); !ok && !token.Valid {
		return err
	}
	return nil
}

func ExtractToken(c *gin.Context) string {

	accessToken, err := c.Request.Cookie("access_token")
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
		return ""

	}

	return accessToken.Value
}

type AccessDetails struct {
	AccessUuid string
	Email      string
}

func ExtractTokenMetadata(c *gin.Context) (*AccessDetails, error) {

	token, err := VerifyToken(c)

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)

	if ok && token.Valid {

		accessUuid, ok := claims["access_uuid"].(string)

		if !ok {
			return nil, err
		}

		email := fmt.Sprint(claims["email"])

		return &AccessDetails{
			AccessUuid: accessUuid,
			Email:      email,
		}, nil
	}

	return nil, err
}

func FetchAuth(authD *AccessDetails) error {

	_, err := client.Get(ctx, authD.AccessUuid).Result()

	if err != nil {
		return err
	}

	return nil
}

func DeleteAuth(givenUuid string) (int64, error) {

	deleted, err := client.Del(ctx, givenUuid).Result()

	if err != nil {
		return 0, err
	}

	return deleted, nil
}

func TokenAuthMiddleware() gin.HandlerFunc {

	return func(c *gin.Context) {
		err := TokenValid(c)

		if err != nil {

			c.Abort()
			return
		}
		c.Next()
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

	accessToken, _ := c.Request.Cookie("access_token")

	http.SetCookie(c.Writer, &http.Cookie{
		Name:    "access_token",
		Value:   accessToken.Value,
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
