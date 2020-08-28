// main.go
package main

import (
	"database/sql"
	"fmt"
	_ "github.com/gomodule/redigo/redis"
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

type Message struct {
	Payload, Publisher, Topic string
}

type Topic struct {
	Name string
	Flag bool
}

type server struct {
	db *sql.DB
}

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

func publishTo(topic string, data string) {
	for {
		eb.Publish(topic, data)
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
	}
}

func (s *server) notifications(res http.ResponseWriter, req *http.Request) {

	checkSession(res, req)

	session, _ := store.Get(req, "session")
	email := fmt.Sprintf("%v", session.Values["user"])

	/*var (
		payload string
		publisher string
		topic string
	)*/

	data, err := s.db.Query("SELECT m.payload, m.publisher, m.topic FROM messages m, subscriptions s "+
		"WHERE m.topic = s.topic and s.subscriber = $1", email)

	if err != nil {
		panic(err)
	}

	/*for data.Next() {
		err := data.Scan(&payload, &publisher, &topic)
		if err != nil {
			log.Fatal(err)
		}
		log.Println(payload, publisher, topic)
	}*/

	tRes := Message{}
	var results []Message

	for data.Next() {
		var payload, publisher, topic string
		data.Scan(&payload, &publisher, &topic)
		tRes.Payload = payload
		tRes.Publisher = publisher
		tRes.Topic = topic
		results = append(results, tRes)
	}

	redirecter(res, req, "notifications.html", results)
}

func (s *server) subscriptionPage(res http.ResponseWriter, req *http.Request) {

	checkSession(res, req)

	session, _ := store.Get(req, "session")
	email := fmt.Sprintf("%v", session.Values["user"])

	data, err := s.db.Query("SELECT topic FROM subscriptions"+
		" WHERE subscriber = $1", email)

	if err != nil {
		panic(err)
	}

	tRes := Topic{}
	var results []Topic

	for data.Next() {
		var name string
		data.Scan(&name)
		tRes.Name = name
		tRes.Flag = true
		results = append(results, tRes)
	}

	data, err = s.db.Query("select t.name from topics t where t.name "+
		"not in (select s.topic from subscriptions s where s.subscriber = $1)", email)

	if err != nil {
		panic(err)
	}

	for data.Next() {
		var name string
		data.Scan(&name)
		tRes.Name = name
		tRes.Flag = false
		results = append(results, tRes)
	}

	redirecter(res, req, "subscribe.html", results)
}

func (s *server) subscribe(res http.ResponseWriter, req *http.Request) {

	fmt.Print("IN")
	topics, ok := req.URL.Query()["topic"]
	session, _ := store.Get(req, "session")
	subscriber := fmt.Sprintf("%v", session.Values["user"])

	if !ok || len(topics[0]) < 1 {
		log.Println("Url Param 'session' is missing")
		return
	}

	// Query()["key"] will return an array of items,
	// we only want the single item.
	topic := topics[0]

	sqlStatement := `
			INSERT INTO subscriptions (subscriber, topic)
			VALUES ($1, $2)`

	_, err := s.db.Exec(sqlStatement, subscriber, topic)

	if err != nil {
		panic(err)
	}

	http.Redirect(res, req, "/subscriptionPage", 301)
}

func (s *server) unsubscribe(res http.ResponseWriter, req *http.Request) {

	fmt.Print("un")
	topics, ok := req.URL.Query()["topic"]
	session, _ := store.Get(req, "session")
	subscriber := fmt.Sprintf("%v", session.Values["user"])

	if !ok || len(topics[0]) < 1 {
		log.Println("Url Param 'session' is missing")
		return
	}

	// Query()["key"] will return an array of items,
	// we only want the single item.
	topic := topics[0]

	sqlStatement := `
			DELETE FROM subscriptions WHERE subscriber = $1 AND topic = $2`

	_, err := s.db.Exec(sqlStatement, subscriber, topic)

	if err != nil {
		panic(err)
	}

	http.Redirect(res, req, "/subscriptionPage", 301)
}

func publishPage(res http.ResponseWriter, req *http.Request) {

	redirecter(res, req, "publish.html", nil)
}

func (s *server) publish(res http.ResponseWriter, req *http.Request) {

	conn, _ := upgrader.Upgrade(res, req, nil) // error ignored for sake of simplicity

	session, _ := store.Get(req, "session")
	publisher := fmt.Sprintf("%v", session.Values["user"])

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

		fmt.Print(object.Topic)

		sqlStatement := `
			INSERT INTO messages (payload, publisher,topic)
			VALUES ($1, $2, $3)`

		_, err = s.db.Exec(sqlStatement, object.Message, publisher, object.Topic)

		if err != nil {
			panic(err)
		}

		res.WriteHeader(200)

	}

	//TODO 301
	//http.Redirect(res, req, "/notifications", 301)
}

func main() {

	initSession()

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

	fs := http.FileServer(http.Dir("../static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	http.HandleFunc("/", loginPage)
	http.HandleFunc("/registration", s.registration)
	http.HandleFunc("/registrationPage", registrationPage)
	http.HandleFunc("/registrationError", registrationError)
	http.HandleFunc("/login", s.login)
	http.HandleFunc("/logout", logout)
	http.HandleFunc("/notifications", s.notifications)
	http.HandleFunc("/publishPage", publishPage)
	http.HandleFunc("/subscriptionPage", s.subscriptionPage)
	http.HandleFunc("/subscribe", s.subscribe)
	http.HandleFunc("/unsubscribe", s.unsubscribe)

	http.HandleFunc("/websocket", s.publish)

	log.Println("Listening on :8080...")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
