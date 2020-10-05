// main.go
package main

import (
	"encoding/json"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/umahmood/haversine"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
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
	Message   string    `json:"Message"`
	Topic     string    `json:"Topic"`
	Radius    int       `json:"Radius"`
	LifeTime  time.Time `json:"LifeTime"`
	Latitude  float64   `json:"Latitude"`
	Longitude float64   `json:"Longitude"`
}

type DataEventSlice []DataEvent
type Subscribers []string

type EventBus struct {
	topicMessages map[string]DataEventSlice //key: topic - value: messages
	subscribers   map[string]Subscribers    //key: topic - value: users
	rm            sync.RWMutex
}

func (eb *EventBus) topicSubscription(topic string, email string) {

	eb.rm.Lock()

	if _, found := eb.subscribers[email]; !found {

		topics := []string{topic}
		eb.subscribers[email] = topics

	} else {

		eb.subscribers[email] = append(eb.subscribers[email], topic)
	}

	eb.rm.Unlock()
}

func (eb *EventBus) topicUnsubscription(email string) {

	eb.rm.Lock()
	delete(eb.subscribers, email)
	eb.rm.Unlock()
}

var eb = &EventBus{
	topicMessages: map[string]DataEventSlice{},
	subscribers:   map[string]Subscribers{},
}

func (eb *EventBus) publishTo(topic string, message string, radius int, lifeTime int, latitude string, longitude string) bool {

	startTime := time.Now().Nanosecond()
	expirationTime := time.Now().Local().Add(time.Minute * time.Duration(lifeTime))

	eb.rm.RLock()

	checked := false

	for i := 0; i < 5; i++ {

		latitudeFloat, _ := strconv.ParseFloat(latitude, 64)
		longitudeFloat, _ := strconv.ParseFloat(longitude, 64)
		dataEvent := DataEvent{Message: message, Topic: topic, Radius: radius, LifeTime: expirationTime,
			Latitude: latitudeFloat, Longitude: longitudeFloat}

		size := len(eb.topicMessages[topic])
		eb.topicMessages[topic] = append(eb.topicMessages[topic], dataEvent)
		size1 := len(eb.topicMessages[topic])

		if size1 > size {

			checked = true
		}

		endTime := time.Now().Nanosecond()

		if (endTime-startTime) < 100000000 && checked {

			break
		}
	}

	eb.rm.RUnlock()

	return true
}

func (eb *EventBus) deleteMessage(topic string) {

	for i := 0; i < len(eb.topicMessages[topic]); {

		if time.Now().After(eb.topicMessages[topic][i].LifeTime) {

			eb.topicMessages[topic][i] = eb.topicMessages[topic][len(eb.topicMessages[topic])-1]
			eb.topicMessages[topic] = eb.topicMessages[topic][:len(eb.topicMessages[topic])-1]

		} else {

			i++
		}
	}
}

func (eb *EventBus) garbageCollection() {

	for {

		for topic := range eb.topicMessages {

			go eb.deleteMessage(topic)
		}

		time.Sleep(time.Minute * time.Duration(1))
	}
}

func checkDistance(x1 float64, x2 float64, y1 float64, y2 float64, r1 int, r2 int) bool {

	sessionLocation := haversine.Coord{Lat: x1, Lon: y1}
	publisherLocation := haversine.Coord{Lat: x2, Lon: y2}
	_, km := haversine.Distance(sessionLocation, publisherLocation)

	if km > float64(r1+r2) {

		return false
	}

	return true
}

func notificationsPage(res http.ResponseWriter, req *http.Request) {

	if checkSession(res, req) != "" {
		redirecter(res, req, "notifications.html", nil)
	}
}

func (s *server) notifications(res http.ResponseWriter, req *http.Request) {

	startTime := time.Now().Nanosecond()

	latitudes, _ := req.URL.Query()["latitude"]
	longitudes, _ := req.URL.Query()["longitude"]

	sessionLatitude, _ := strconv.ParseFloat(latitudes[0], 64)
	sessionLongitude, _ := strconv.ParseFloat(longitudes[0], 64)

	email := checkSession(res, req)

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

	for i := 0; i < 5; i++ {

		for _, item := range eb.subscribers[email] {

			for _, message := range eb.topicMessages[item] {

				if checkDistance(sessionLatitude, message.Latitude, sessionLongitude, message.Longitude, 5, message.Radius) {

					notifications = append(notifications, message)
				}
			}
		}

		endTime := time.Now().Nanosecond()

		if (endTime - startTime) < 100000000 {

			break
		}
	}

	result, _ := json.Marshal(notifications)

	res.Header().Set("Content-Type", "application/json")

	_, err = res.Write(result)

	if err != nil {
		fmt.Println(err)
	}
}

func (s *server) subscriptionPage(res http.ResponseWriter, req *http.Request) {

	if checkSession(res, req) != "" {

		email := checkSession(res, req)

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
}

func (s *server) subscribe(res http.ResponseWriter, req *http.Request) {

	topics, ok := req.URL.Query()["topic"]
	subscriber := checkSession(res, req)

	if !ok || len(topics[0]) < 1 {
		log.Println("Url Param 'session' is missing")
		return
	}

	topic := topics[0]

	sqlStatement := `INSERT INTO subscriptions (subscriber, topic) VALUES ($1, $2)`

	_, err := s.db.Exec(sqlStatement, subscriber, topic)

	if err != nil {
		panic(err)
	}

	eb.topicSubscription(topic, subscriber)

	http.Redirect(res, req, "/subscriptionPage", 301)
}

func (s *server) unsubscribe(res http.ResponseWriter, req *http.Request) {

	topics, ok := req.URL.Query()["topic"]
	subscriber := checkSession(res, req)

	if !ok || len(topics[0]) < 1 {
		log.Println("Url Param 'session' is missing")
		return
	}

	topic := topics[0]

	sqlStatement := `DELETE FROM subscriptions WHERE subscriber = $1 AND topic = $2`

	_, err := s.db.Exec(sqlStatement, subscriber, topic)

	if err != nil {
		panic(err)
	}

	eb.topicUnsubscription(subscriber)

	http.Redirect(res, req, "/subscriptionPage", 301)
}

func publishPage(res http.ResponseWriter, req *http.Request) {

	if checkSession(res, req) != "" {
		redirecter(res, req, "publish.html", nil)
	}
}

func (s *server) publish(res http.ResponseWriter, req *http.Request) {

	payloads, _ := req.URL.Query()["payload"]
	topics, _ := req.URL.Query()["topic"]
	radiuss, _ := req.URL.Query()["radius"]
	lifeTimes, _ := req.URL.Query()["lifeTime"]
	latitudes, _ := req.URL.Query()["latitude"]
	longitudes, _ := req.URL.Query()["longitude"]

	payload := payloads[0]
	topic := topics[0]
	radius, _ := strconv.Atoi(radiuss[0])
	lifeTime, _ := strconv.Atoi(lifeTimes[0])
	latitude := latitudes[0]
	longitude := longitudes[0]

	/*sqlStatement := `
			INSERT INTO messages (payload, publisher,topic)
			VALUES ($1, $2, $3)`

	_, err := s.db.Exec(sqlStatement, payload, publisher, topic)

	if err != nil {
		panic(err)
	}*/

	go eb.publishTo(topic, payload, radius, lifeTime, latitude, longitude)

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
		eb.topicSubscription(topic, subscriber)
	}

	return
}

func main() {

	s, db := initDB()
	defer db.Close()
	s.initEB()
	go eb.garbageCollection()

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

	//log.Println("Listening on :8080...")
	//err := http.ListenAndServe(":8080", nil)
	log.Println("Listening on :80...")
	err := http.ListenAndServe(":80", nil)

	if err != nil {

		log.Fatal(err)
	}

}
