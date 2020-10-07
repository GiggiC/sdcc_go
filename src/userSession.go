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
	"html/template"
	"net/http"
	"os"
	"path/filepath"
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
	AccessToken  string
	RefreshToken string
	AccessUuid   string
	RefreshUuid  string
	AtExpires    int64
	RtExpires    int64
}

func registrationPage(c *gin.Context) {

	// Check if user is authenticated
	if checkSession(c) == "" {

		data := Object{
			Status: "not-logged",
		}

		lp := filepath.Join("../templates", "layout.html")
		fp := filepath.Join("../templates", "registration.html")

		tmpl, _ := template.ParseFiles(lp, fp)
		tmpl.ExecuteTemplate(c.Writer, "layout", data)

		return
	}

	http.Redirect(c.Writer, c.Request, "/notificationsPage", 301)
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

			http.Error(c.Writer, "Server error, unable to create your account.", 500)
			return
		}

		sqlStatement := `INSERT INTO users (email, username, password) VALUES ($1, $2, $3)`

		_, err = s.db.Exec(sqlStatement, email, username, hashedPassword)

		if err != nil {
			panic(err)
		}

		http.Redirect(c.Writer, c.Request, "/loginPage", 301)
		return

	case err != nil:

		http.Redirect(c.Writer, c.Request, "/registrationError", 500)
		return

	default:

		http.Redirect(c.Writer, c.Request, "/registrationError", 301)
		return
	}
}

func registrationError(c *gin.Context) {

	lp := filepath.Join("../templates", "layout.html")
	fp := filepath.Join("../templates", "registrationError.html")

	tmpl, _ := template.ParseFiles(lp, fp)
	tmpl.ExecuteTemplate(c.Writer, "layout", nil)
}

func loginPage(c *gin.Context) {

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

func CreateToken(email string) (*TokenDetails, error) {

	td := &TokenDetails{}
	td.AtExpires = time.Now().Add(time.Minute * 15).Unix()
	td.AccessUuid = uuid.NewV4().String()

	td.RtExpires = time.Now().Add(time.Hour * 24 * 7).Unix()
	td.RefreshUuid = uuid.NewV4().String()

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

	//Creating Refresh Token
	os.Setenv("REFRESH_SECRET", "mcmvmkmsdnfsdmfdsjf") //this should be in an env file
	rtClaims := jwt.MapClaims{}
	rtClaims["refresh_uuid"] = td.RefreshUuid
	rtClaims["email"] = email
	rtClaims["exp"] = td.RtExpires
	rt := jwt.NewWithClaims(jwt.SigningMethodHS256, rtClaims)
	td.RefreshToken, err = rt.SignedString([]byte(os.Getenv("REFRESH_SECRET")))
	if err != nil {
		return nil, err
	}
	return td, nil
}

func CreateAuth(email string, td *TokenDetails) error {

	at := time.Unix(td.AtExpires, 0) //converting Unix to UTC(to Time object)
	rt := time.Unix(td.RtExpires, 0)
	now := time.Now()

	errAccess := client.Set(ctx, td.AccessUuid, email, at.Sub(now)).Err()

	if errAccess != nil {
		return errAccess
	}

	errRefresh := client.Set(ctx, td.RefreshUuid, email, rt.Sub(now)).Err()

	if errRefresh != nil {
		return errRefresh
	}

	return nil
}

func VerifyToken(r *http.Request) (*jwt.Token, error) {

	tokenString := ExtractToken(r)

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

func TokenValid(r *http.Request) error {

	token, err := VerifyToken(r)
	if err != nil {
		return err
	}
	if _, ok := token.Claims.(jwt.Claims); !ok && !token.Valid {
		return err
	}
	return nil
}

func ExtractToken(r *http.Request) string {

	accessToken, _ := r.Cookie("access_token")
	//refreshToken, _ := r.Cookie("refresh_token")

	return accessToken.Value
}

type AccessDetails struct {
	AccessUuid string
	Email      string
}

func ExtractTokenMetadata(r *http.Request) (*AccessDetails, error) {

	token, err := VerifyToken(r)

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

func FetchAuth(authD *AccessDetails) (string, error) {

	email, err := client.Get(ctx, authD.AccessUuid).Result()

	if err != nil {
		return "", err
	}

	return email, nil
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
		err := TokenValid(c.Request)

		if err != nil {
			c.JSON(http.StatusUnauthorized, err.Error())
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

	http.SetCookie(c.Writer, &http.Cookie{
		Name:    "refresh_token",
		Value:   ts.RefreshToken,
		Expires: time.Now().Add(time.Hour * 24 * 7),
	})

	c.Redirect(301, "/notificationsPage")
}

func logout(c *gin.Context) {

	fmt.Println("AAAAAA")

	au, err := ExtractTokenMetadata(c.Request)

	if err != nil {

		c.JSON(http.StatusUnauthorized, "unauthorized")
		return
	}

	fmt.Println("BBBBBB")

	deleted, delErr := DeleteAuth(au.AccessUuid)

	if delErr != nil || deleted == 0 { //if any goes wrong

		c.JSON(http.StatusUnauthorized, "unauthorized")
		return
	}

	fmt.Println("CCCCC")

	accessToken, _ := c.Request.Cookie("access_token")

	http.SetCookie(c.Writer, &http.Cookie{
		Name:    "access_token",
		Value:   accessToken.Value,
		Expires: time.Now(),
	})

	c.Redirect(301, "/")
}

func checkSession(c *gin.Context) string {

	tokenAuth, err := ExtractTokenMetadata(c.Request)

	if err != nil {
		c.JSON(http.StatusUnauthorized, "unauthorized")
		return ""
	}

	email, err := FetchAuth(tokenAuth)

	if err != nil {
		c.JSON(http.StatusUnauthorized, "unauthorized")
		return ""
	}

	return email
}

func redirecter(c *gin.Context, url string, status string, results interface{}) {

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
