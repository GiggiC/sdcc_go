// main.go
package main

import (
	"database/sql"
	"fmt"
	_ "github.com/gomodule/redigo/redis"
	_ "github.com/gorilla/sessions"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"strconv"
	"sync"
)

type Object struct {
	Status string
	Data   interface{}
}

type Topic struct {
	Name string
	Flag bool
}

type server struct {
	db *sql.DB
}

const (
	host     = "localhost"
	port     = 5432
	user     = "postgres"
	password = "password"
	dbname   = "sdcc"
)

type DataEvent struct {
	Message   string
	Topic     string
	Latitude  float64
	Longitude float64
}

type DataEventSlice []DataEvent
type Subscribers []string

// EventBus stores the information about subscribers interested for a particular topic
type EventBus struct {
	topicMessages map[string]DataEventSlice
	subscribers   map[string]Subscribers
	rm            sync.RWMutex
}

func (eb *EventBus) Publish(topic string, message string, latitude string, longitude string) {

	eb.rm.RLock()

	if _, found := eb.topicMessages[topic]; found {

		go func() {

			latitudeFloat, _ := strconv.ParseFloat(latitude, 64)
			longitudeFloat, _ := strconv.ParseFloat(longitude, 64)
			dataEvent := DataEvent{Message: message, Topic: topic, Latitude: latitudeFloat, Longitude: longitudeFloat}
			eb.topicMessages[topic] = append(eb.topicMessages[topic], dataEvent)
		}()
	}

	eb.rm.RUnlock()
}

func (eb *EventBus) Subscribe(topic string, email string) {

	eb.rm.Lock()

	if _, found := eb.subscribers[email]; !found {

		topics := []string{topic}
		eb.subscribers[email] = topics

	} else {

		eb.subscribers[email] = append(eb.subscribers[email], topic)
	}

	eb.rm.Unlock()
}

var eb = &EventBus{
	topicMessages: map[string]DataEventSlice{},
	subscribers:   map[string]Subscribers{},
}

func publishTo(topic string, data string, latitude string, longitude string) {

	eb.Publish(topic, data, latitude, longitude)
}

func Find(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func (s *server) notifications(res http.ResponseWriter, req *http.Request) {

	checkSession(res, req)

	session, _ := store.Get(req, "session")
	email := fmt.Sprintf("%v", session.Values["user"])

	data, err := s.db.Query("SELECT topic FROM subscriptions "+
		"WHERE subscriber = $1", email)

	if err != nil {
		panic(err)
	}

	var results []string

	for data.Next() {
		var topic string
		data.Scan(&topic)
		results = append(results, topic)
	}

	var notifications []DataEvent

	for _, item := range eb.subscribers[email] {

		for _, message := range eb.topicMessages[item] {

			notifications = append(notifications, message)
		}

	}

	redirecter(res, req, "notifications.html", notifications)
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

	topics, ok := req.URL.Query()["topic"]
	session, _ := store.Get(req, "session")
	subscriber := fmt.Sprintf("%v", session.Values["user"])

	if !ok || len(topics[0]) < 1 {
		log.Println("Url Param 'session' is missing")
		return
	}

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

	topics, ok := req.URL.Query()["topic"]
	session, _ := store.Get(req, "session")
	subscriber := fmt.Sprintf("%v", session.Values["user"])

	if !ok || len(topics[0]) < 1 {
		log.Println("Url Param 'session' is missing")
		return
	}

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

	//session, _ := store.Get(req, "session")
	//publisher := fmt.Sprintf("%v", session.Values["user"])

	payloads, _ := req.URL.Query()["payload"]
	topics, _ := req.URL.Query()["topic"]
	latitudes, _ := req.URL.Query()["latitude"]
	longitudes, _ := req.URL.Query()["longitude"]

	payload := payloads[0]
	topic := topics[0]
	latitude := latitudes[0]
	longitude := longitudes[0]

	/*sqlStatement := `
			INSERT INTO messages (payload, publisher,topic)
			VALUES ($1, $2, $3)`

	_, err := s.db.Exec(sqlStatement, payload, publisher, topic)

	if err != nil {
		panic(err)
	}*/

	go publishTo(topic, payload, latitude, longitude)

	//res.WriteHeader(301)

	//TODO 301
	http.Redirect(res, req, "/publishPage", 301)
}

func (s *server) getAllSubscriptions() {

	data, err := s.db.Query("SELECT * FROM subscriptions ORDER BY topic")

	if err != nil {
		panic(err)
	}

	for data.Next() {
		var subscriber, topic string
		data.Scan(&subscriber, &topic)
		eb.Subscribe(topic, subscriber)
	}

	return
}

func (s *server) geo(res http.ResponseWriter, req *http.Request) {

	redirecter(res, req, "geo.html", nil)
}

func main() {

	initSession()

	eb.topicMessages["Topic 1"] = DataEventSlice{}
	eb.topicMessages["Topic 2"] = DataEventSlice{}
	eb.topicMessages["Topic 3"] = DataEventSlice{}
	eb.topicMessages["Topic 4"] = DataEventSlice{}

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
	s.getAllSubscriptions()

	//go listener()

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
	http.HandleFunc("/publish", s.publish)

	log.Println("Listening on :8080...")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}

}
