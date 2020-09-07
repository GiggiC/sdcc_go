// main.go
package main

import (
	"encoding/json"
	_ "encoding/json"
	"fmt"
	_ "github.com/gomodule/redigo/redis"
	_ "github.com/gorilla/sessions"
	_ "github.com/lib/pq"
	"log"
	"math"
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

type DataEvent struct {
	Message   string  `json:"Message"`
	Topic     string  `json:"Topic"`
	Latitude  float64 `json:"Latitude"`
	Longitude float64 `json:"Longitude"`
	Radius    int     `json:"Radius"`
}

type DataEventSlice []DataEvent
type Subscribers []string

type EventBus struct {
	topicMessages map[string]DataEventSlice
	subscribers   map[string]Subscribers
	rm            sync.RWMutex
}

func (eb *EventBus) Publish(topic string, message string, radius int, latitude string, longitude string) {

	eb.rm.RLock()

	if _, found := eb.topicMessages[topic]; found {

		go func() {

			latitudeFloat, _ := strconv.ParseFloat(latitude, 64)
			longitudeFloat, _ := strconv.ParseFloat(longitude, 64)
			dataEvent := DataEvent{Message: message, Topic: topic, Radius: radius, Latitude: latitudeFloat, Longitude: longitudeFloat}
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

func publishTo(topic string, data string, radius int, latitude string, longitude string) {

	eb.Publish(topic, data, radius, latitude, longitude)
}

func Find(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func checkDistance(x1 float64, x2 float64, y1 float64, y2 float64, r1 int, r2 int) bool {

	distance := math.Sqrt(math.Pow(x1-x2, 2) - math.Pow(y1-y2, 2))

	if distance > float64(r1+r2) {

		return false
	}

	return true
}

func notificationsPage(res http.ResponseWriter, req *http.Request) {

	redirecter(res, req, "notifications.html", nil)
}

func (s *server) notifications(res http.ResponseWriter, req *http.Request) {

	checkSession(res, req)

	latitudes, _ := req.URL.Query()["latitude"]
	longitudes, _ := req.URL.Query()["longitude"]

	sessionLatitude, _ := strconv.ParseFloat(latitudes[0], 64)
	sessionLongitude, _ := strconv.ParseFloat(longitudes[0], 64)

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

			if checkDistance(sessionLatitude, message.Latitude, sessionLongitude, message.Longitude, 5, message.Radius) {

				notifications = append(notifications, message)
			}
		}
	}

	b, _ := json.Marshal(notifications)

	fmt.Println(b)
	res.Header().Set("Content-Type", "application/json")

	_, err = res.Write(b)

	if err != nil {
		fmt.Println(err)
	}

	//redirecter(res, req, "notifications.html", notifications)
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

	payloads, _ := req.URL.Query()["payload"]
	topics, _ := req.URL.Query()["topic"]
	radiuses, _ := req.URL.Query()["radius"]
	latitudes, _ := req.URL.Query()["latitude"]
	longitudes, _ := req.URL.Query()["longitude"]

	payload := payloads[0]
	topic := topics[0]
	radius, _ := strconv.Atoi(radiuses[0])
	latitude := latitudes[0]
	longitude := longitudes[0]

	/*sqlStatement := `
			INSERT INTO messages (payload, publisher,topic)
			VALUES ($1, $2, $3)`

	_, err := s.db.Exec(sqlStatement, payload, publisher, topic)

	if err != nil {
		panic(err)
	}*/

	go publishTo(topic, payload, radius, latitude, longitude)

	//TODO 301
	http.Redirect(res, req, "/publishPage", 301)
}

func (s *server) initEB() {

	data, err := s.db.Query("SELECT * FROM subscriptions ORDER BY topic")

	if err != nil {
		panic(err)
	}

	for data.Next() {
		var subscriber, topic string
		data.Scan(&subscriber, &topic)
		eb.topicMessages[topic] = DataEventSlice{}
		eb.Subscribe(topic, subscriber)
	}

	return
}

func main() {

	initSession()
	s, db := initDB()
	defer db.Close()
	s.initEB()

	fs := http.FileServer(http.Dir("../static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	http.HandleFunc("/", loginPage)
	http.HandleFunc("/registration", s.registration)
	http.HandleFunc("/registrationPage", registrationPage)
	http.HandleFunc("/registrationError", registrationError)
	http.HandleFunc("/login", s.login)
	http.HandleFunc("/logout", logout)
	http.HandleFunc("/notificationsPage", notificationsPage)
	http.HandleFunc("/notifications", s.notifications)
	http.HandleFunc("/publishPage", publishPage)
	http.HandleFunc("/subscriptionPage", s.subscriptionPage)
	http.HandleFunc("/subscribe", s.subscribe)
	http.HandleFunc("/unsubscribe", s.unsubscribe)
	http.HandleFunc("/publish", s.publish)

	log.Println("Listening on :8080...")
	err := http.ListenAndServe(":8080", nil)

	if err != nil {

		log.Fatal(err)
	}

}
