package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-redis/redis"
	"github.com/twinj/uuid"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	rtClaims["user_id"] = email
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
	bearToken := r.Header.Get("Authorization")
	//normally Authorization the_token_xxx
	strArr := strings.Split(bearToken, " ")
	if len(strArr) == 2 {
		return strArr[1]
	}
	return ""
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

func FetchAuth(authD *AccessDetails) (uint64, error) {
	userid, err := client.Get(ctx, authD.AccessUuid).Result()
	if err != nil {
		return 0, err
	}
	userID, _ := strconv.ParseUint(userid, 10, 64)
	return userID, nil
}

func (s *server) login(res http.ResponseWriter, req *http.Request) {

	email := req.FormValue("email")
	password := req.FormValue("password")

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

	token, err := CreateToken(email)

	if err != nil {

		res.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	saveErr := CreateAuth(email, token)
	if saveErr != nil {
		res.WriteHeader(http.StatusUnprocessableEntity)
	}

	http.SetCookie(res, &http.Cookie{
		Name:  "access_token",
		Value: token.AccessToken,
	})

	http.SetCookie(res, &http.Cookie{
		Name:  "refresh_token",
		Value: token.RefreshToken,
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

	var td *Todo
	if err := c.ShouldBindJSON(&td); err != nil {
		c.JSON(http.StatusUnprocessableEntity, "invalid json")
		return
	}
	tokenAuth, err := ExtractTokenMetadata(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, "unauthorized")
		return
	}
	userId, err = FetchAuth(tokenAuth)
	if err != nil {
		c.JSON(http.StatusUnauthorized, "unauthorized")
		return
	}
	td.UserID = userId

	//you can proceed to save the Todo to a database
	//but we will just return it to the caller here:
	c.JSON(http.StatusCreated, td)
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
