// websockets.go
package main

import (
	"database/sql"
	"fmt"
	"github.com/gomodule/redigo/redis"
	_ "github.com/gomodule/redigo/redis"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

type server struct {
	db *sql.DB
}

// Store the redis connection as a package level variable
var cache redis.Conn

const (
	host     = "localhost"
	port     = 5432
	user     = "postgres"
	password = "password"
	dbname   = "sdcc"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type DataEvent struct {
	Message string `json:"message"`
	Topic   string `json:"topic"`
}

// DataChannel is a channel which can accept an DataEvent
type DataChannel chan DataEvent

// DataChannelSlice is a slice of DataChannels
type DataChannelSlice []DataChannel

// EventBus stores the information about subscribers interested for a particular topic
type EventBus struct {
	subscribers map[string]DataChannelSlice
	rm          sync.RWMutex
}

func (eb *EventBus) Publish(topic string, message string) {
	eb.rm.RLock()
	if chans, found := eb.subscribers[topic]; found {
		// this is done because the slices refer to same array even though they are passed by value
		// thus we are creating a new slice with our elements thus preserve locking correctly.
		// special thanks for /u/freesid who pointed it out
		channels := append(DataChannelSlice{}, chans...)
		go func(data DataEvent, dataChannelSlices DataChannelSlice) {
			for _, ch := range dataChannelSlices {
				ch <- data
			}
		}(DataEvent{Message: message, Topic: topic}, channels)
	}
	eb.rm.RUnlock()
}

func (eb *EventBus) Subscribe(topic string, ch DataChannel) {
	eb.rm.Lock()
	if prev, found := eb.subscribers[topic]; found {
		eb.subscribers[topic] = append(prev, ch)
	} else {
		eb.subscribers[topic] = append([]DataChannel{}, ch)
	}
	eb.rm.Unlock()
}

var eb = &EventBus{
	subscribers: map[string]DataChannelSlice{},
}

func printDataEvent(ch string, data DataEvent) {
	fmt.Printf("Channel: %s; Topic: %s; DataEvent: %v\n", ch, data.Topic, data.Message)
}

func publisTo(topic string, data string) {
	for {
		eb.Publish(topic, data)
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
	}
}

func initCache() {
	// Initialize the redis connection to a redis instance running on your local machine
	conn, err := redis.DialURL("redis://localhost:6379")
	if err != nil {
		panic(err)
	}
	// Assign the connection to the package level `cache` variable
	cache = conn
}

func (s *server) notifications(res http.ResponseWriter, req *http.Request) {

	if req.Method != "POST" {
		lp := filepath.Join("templates", "layout.html")
		fp := filepath.Join("templates", "notifications.html")

		tmpl, _ := template.ParseFiles(lp, fp)
		tmpl.ExecuteTemplate(res, "layout", nil)
		return
	}
}

func (s *server) signupPage(res http.ResponseWriter, req *http.Request) {

	if req.Method != "POST" {
		lp := filepath.Join("templates", "layout.html")
		fp := filepath.Join("templates", "registration.html")

		tmpl, _ := template.ParseFiles(lp, fp)
		tmpl.ExecuteTemplate(res, "layout", nil)
		return
	}

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

		http.Redirect(res, req, "/index", 301)

		return

	case err != nil:

		res.Write([]byte("User error 1!"))
		http.Error(res, "Server error, unable to create your account.", 500)

		return

	default:

		http.Redirect(res, req, "/registrationError", 301)

		return
	}
}

func (s *server) publish(res http.ResponseWriter, req *http.Request) {

	// We can obtain the session token from the requests cookies, which come with every request
	c, err := req.Cookie("session_token")
	if err != nil {
		if err == http.ErrNoCookie {
			// If the cookie is not set, return an unauthorized status
			res.WriteHeader(http.StatusUnauthorized)
			return
		}
		// For any other type of error, return a bad request status
		res.WriteHeader(http.StatusBadRequest)
		return
	}
	sessionToken := c.Value

	// We then get the name of the user from our cache, where we set the session token
	response, err := cache.Do("GET", sessionToken)
	if err != nil {
		// If there is an error fetching from cache, return an internal server error status
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
	if response == nil {
		// If the session token is not present in cache, return an unauthorized error
		res.WriteHeader(http.StatusUnauthorized)
		return
	}
	// Finally, return the welcome message to the user
	res.Write([]byte(fmt.Sprintf("Welcomeeeeeee %s!", response)))
}

func (s *server) index(res http.ResponseWriter, req *http.Request) {

	if req.Method != "POST" {

		lp := filepath.Join("templates", "layout.html")
		fp := filepath.Join("templates", "signin.html")

		tmpl, _ := template.ParseFiles(lp, fp)
		tmpl.ExecuteTemplate(res, "layout", nil)
	}

	email := req.FormValue("email")
	password := req.FormValue("password")

	var databaseEmail string
	var databasePassword string

	err := s.db.QueryRow("SELECT email, password FROM users WHERE email=$1", email).Scan(&databaseEmail, &databasePassword)

	if err != nil {
		http.Redirect(res, req, "/index", 301)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(databasePassword), []byte(password))
	if err != nil {
		http.Redirect(res, req, "/index", 301)
		return
	}

	// Create a new random session token
	sessionToken := uuid.New().String()

	// Set the token in the cache, along with the user whom it represents
	// The token has an expiry time of 120 seconds
	_, err = cache.Do("SETEX", sessionToken, "120", databaseEmail)
	if err != nil {
		// If there is an error in setting the cache, return an internal server error
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Finally, we set the client cookie for "session_token" as the session token we just generated
	// we also set an expiry time of 120 seconds, the same as the cache
	http.SetCookie(res, &http.Cookie{
		Name:    "session_token",
		Value:   sessionToken,
		Expires: time.Now().Add(120 * time.Second),
	})

	http.Redirect(res, req, "/notifications", 301)

	return
}

func Refresh(res http.ResponseWriter, req *http.Request) {
	// (BEGIN) The code uptil this point is the same as the first part of the `Welcome` route
	c, err := req.Cookie("session_token")
	if err != nil {
		if err == http.ErrNoCookie {
			res.WriteHeader(http.StatusUnauthorized)
			return
		}
		res.WriteHeader(http.StatusBadRequest)
		return
	}
	sessionToken := c.Value

	response, err := cache.Do("GET", sessionToken)
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
	if response == nil {
		res.WriteHeader(http.StatusUnauthorized)
		return
	}
	// (END) The code uptil this point is the same as the first part of the `Welcome` route

	// Now, create a new session token for the current user
	newSessionToken := uuid.New().String()
	_, err = cache.Do("SETEX", newSessionToken, "120", fmt.Sprintf("%s", response))
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Delete the older session token
	_, err = cache.Do("DEL", sessionToken)
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Set the new token as the users `session_token` cookie
	http.SetCookie(res, &http.Cookie{
		Name:    "session_token",
		Value:   newSessionToken,
		Expires: time.Now().Add(120 * time.Second),
	})

	http.Redirect(res, req, "/notifications", 301)

}

func logout(res http.ResponseWriter, req *http.Request) {

	// (BEGIN) The code uptil this point is the same as the first part of the `Welcome` route
	c, err := req.Cookie("session_token")
	if err != nil {
		if err == http.ErrNoCookie {
			res.WriteHeader(http.StatusUnauthorized)
			return
		}
		res.WriteHeader(http.StatusBadRequest)
		return
	}
	sessionToken := c.Value

	response, err := cache.Do("GET", sessionToken)
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
	if response == nil {
		res.WriteHeader(http.StatusUnauthorized)
		return
	}
	// (END) The code uptil this point is the same as the first part of the `Welcome` route

	// Set the new token as the users `session_token` cookie
	http.SetCookie(res, &http.Cookie{
		Name:    sessionToken,
		Value:   "",
		Expires: time.Now().Add(0 * time.Second),
	})

	http.Redirect(res, req, "/notifications", 301)
}

func main() {

	initCache()

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)

	if err != nil {
		panic(err)
	}

	defer db.Close()

	err = db.Ping()

	if err != nil {
		panic(err)
	}

	fmt.Println("Successfully connected!")

	s := server{db: db}

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	http.HandleFunc("/", s.index)
	http.HandleFunc("/logout", logout)
	http.HandleFunc("/refresh", Refresh)
	http.HandleFunc("/registration", s.signupPage)
	http.HandleFunc("/registrationError", registrationError)
	http.HandleFunc("/publish", s.publish)
	http.HandleFunc("/notifications", s.notifications)

	/*http.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity

		for {

			var object DataEvent

			err := conn.ReadJSON(&object)
			if err != nil {
				fmt.Println("Error reading json.", err)
			}

			fmt.Printf("Got message: %#v\n", object)

			if err = conn.WriteJSON(object); err != nil {
				fmt.Println(err)
			}
		}
	})*/

	log.Println("Listening on :8080...")
	err3 := http.ListenAndServe(":8080", nil)
	if err3 != nil {
		log.Fatal(err)
	}

	http.ListenAndServe(":8080", nil)

}

func registrationError(w http.ResponseWriter, r *http.Request) {

	lp := filepath.Join("templates", "layout.html")
	fp := filepath.Join("templates", "registrationError.html")

	tmpl, _ := template.ParseFiles(lp, fp)
	tmpl.ExecuteTemplate(w, "layout", nil)
}
