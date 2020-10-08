// main.go
package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
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

func notificationsPage(c *gin.Context) {

	redirecter(c, "notifications.html", "logged", nil)
}

func (s *server) notifications(c *gin.Context) {

	checkSession(c)

	fmt.Print()

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email

	startTime := time.Now().Nanosecond()

	latitudes, _ := c.Request.URL.Query()["latitude"]
	longitudes, _ := c.Request.URL.Query()["longitude"]
	radius, _ := c.Request.URL.Query()["radius"]

	sessionLatitude, _ := strconv.ParseFloat(latitudes[0], 64)
	sessionLongitude, _ := strconv.ParseFloat(longitudes[0], 64)
	sessionRadius, _ := strconv.ParseInt(radius[0], 10, 64)

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

				if checkDistance(sessionLatitude, message.Latitude, sessionLongitude, message.Longitude, int(sessionRadius), message.Radius) {

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

	c.Writer.Header().Set("Content-Type", "application/json")

	_, err = c.Writer.Write(result)

	if err != nil {
		fmt.Println(err)
	}
}

func (s *server) subscriptionPage(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email

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

	redirecter(c, "subscribe.html", "logged", results)
}

func (s *server) editSubscription(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email

	topics, ok := c.Request.URL.Query()["topic"]

	if !ok || len(topics[0]) < 1 {
		log.Println("Url Param 'session' is missing")
		return
	}

	topic := topics[0]
	data, err := s.db.Query("select topic from subscriptions where subscriber = $1 and topic = $2", email, topic)

	if err != nil {
		panic(err)
	}

	if data.Next() {

		sqlStatement := `DELETE FROM subscriptions WHERE subscriber = $1 AND topic = $2`

		_, err := s.db.Exec(sqlStatement, email, topic)

		if err != nil {
			fmt.Println("BBBBBB")
			panic(err)
		}

		eb.topicUnsubscription(email)

	} else {

		sqlStatement := `INSERT INTO subscriptions (subscriber, topic) VALUES ($1, $2)`

		_, err := s.db.Exec(sqlStatement, email, topic)

		if err != nil {
			fmt.Println("CCCCCC")
			panic(err)
		}

		eb.topicSubscription(topic, email)

	}

	//c.Redirect(301, "/subscriptionPage")
}

func publishPage(c *gin.Context) {

	redirecter(c, "publish.html", "logged", nil)
}

func (s *server) publish(c *gin.Context) {

	checkSession(c)

	payloads, _ := c.Request.URL.Query()["payload"]
	topics, _ := c.Request.URL.Query()["topic"]
	radiuss, _ := c.Request.URL.Query()["radius"]
	lifeTimes, _ := c.Request.URL.Query()["lifeTime"]
	latitudes, _ := c.Request.URL.Query()["latitude"]
	longitudes, _ := c.Request.URL.Query()["longitude"]

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
	c.Redirect(301, "/publishPage")
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

	initRedis()
	s, db := initDB()
	defer db.Close()
	s.initEB()
	go eb.garbageCollection()

	//fs := http.FileServer(http.Dir("../static"))
	//http.Handle("/static/", http.StripPrefix("/static/", fs))

	router.StaticFS("/static/", http.Dir("../static"))

	router.LoadHTMLGlob("../templates/*")

	router.GET("/", loginPage)
	router.POST("/registration", s.registration)
	router.GET("/registrationPage", registrationPage)
	router.POST("/login", s.login)
	router.GET("/logout", TokenAuthMiddleware(), logout)
	router.GET("/notificationsPage", TokenAuthMiddleware(), notificationsPage)
	router.GET("/notifications", TokenAuthMiddleware(), s.notifications)
	router.GET("/publishPage", TokenAuthMiddleware(), publishPage)
	router.GET("/subscriptionPage", TokenAuthMiddleware(), s.subscriptionPage)
	router.GET("/editSubscription", TokenAuthMiddleware(), s.editSubscription)
	router.GET("/publish", TokenAuthMiddleware(), s.publish)

	log.Println("Listening on :8080...")
	err := router.Run(":8080")

	if err != nil {

		log.Fatal(err)
	}

}
