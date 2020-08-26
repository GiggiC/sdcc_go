// main.go
package main

import (
	"database/sql"
	"fmt"
	_ "github.com/gomodule/redigo/redis"
	"github.com/gorilla/sessions"
	_ "github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type Object struct {
	Status string
	Data   interface{}
}

type server struct {
	db *sql.DB
}

var (
	// key must be 16, 24 or 32 bytes long (AES-128, AES-192 or AES-256)
	key   = []byte("super-secret-key")
	store = sessions.NewCookieStore(key)
)

type User struct {
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	Age       int    `json:"age"`
}

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

func (s *server) publish(res http.ResponseWriter, req *http.Request) {

	/*// We can obtain the session token from the requests cookies, which come with every request
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
	res.Write([]byte(fmt.Sprintf("Welcomeeeeeee %s!", response)))*/
}

func (s *server) notifications(res http.ResponseWriter, req *http.Request) {

	session, _ := store.Get(req, "cookie-name")

	// Check if user is authenticated
	if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {

		http.Redirect(res, req, "/", http.StatusForbidden)
		return
	}

	redirecter(res, req, "notifications.html")
}

func main() {

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

	http.HandleFunc("/", loginPage)
	http.HandleFunc("/registration", s.registration)
	http.HandleFunc("/registrationPage", registrationPage)
	http.HandleFunc("/registrationError", registrationError)
	http.HandleFunc("/login", s.login)
	http.HandleFunc("/logout", logout)
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
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
