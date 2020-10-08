// main.go
package main

import (
	"database/sql"
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
type Topics []string

type EventBus struct {
	topicMessages map[string]DataEventSlice //key: topic - value: messages
	userTopics    map[string]Topics         //key: topic - value: users
	rm            sync.RWMutex
}

type Receivers struct {
	dbServer server
	eb       EventBus
}

func (r *Receivers) topicSubscription(topic string, email string) {

	r.eb.rm.Lock()

	//user not subscribed to any topic yet
	if _, found := r.eb.userTopics[email]; !found {

		topics := []string{topic}
		r.eb.userTopics[email] = topics

	} else { //append new topic subscription

		r.eb.userTopics[email] = append(r.eb.userTopics[email], topic)
	}

	r.eb.rm.Unlock()
}

func remove(s []string, i int) []string {

	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func (r *Receivers) topicUnsubscription(email string, topic string) {

	r.eb.rm.Lock()

	topicList := r.eb.userTopics[email]
	var newList []string

	for i := 0; i < len(topicList); i++ {

		if topicList[i] == topic {

			newList = remove(topicList, i)
		}
	}

	delete(r.eb.userTopics, email)
	r.eb.userTopics[email] = newList

	r.eb.rm.Unlock()
}

func (r *Receivers) publishTo(topic string, message string, radius int, lifeTime int, latitude string, longitude string) bool {

	startTime := time.Now().Nanosecond()
	expirationTime := time.Now().Local().Add(time.Minute * time.Duration(lifeTime))

	r.eb.rm.RLock()

	checked := false

	for i := 0; i < 5; i++ {

		latitudeFloat, _ := strconv.ParseFloat(latitude, 64)
		longitudeFloat, _ := strconv.ParseFloat(longitude, 64)
		dataEvent := DataEvent{Message: message, Topic: topic, Radius: radius, LifeTime: expirationTime,
			Latitude: latitudeFloat, Longitude: longitudeFloat}

		size := len(r.eb.topicMessages[topic])
		r.eb.topicMessages[topic] = append(r.eb.topicMessages[topic], dataEvent)
		size1 := len(r.eb.topicMessages[topic])

		if size1 > size {

			checked = true
		}

		endTime := time.Now().Nanosecond()

		if (endTime-startTime) < 100000000 && checked {

			break
		}
	}

	r.eb.rm.RUnlock()

	return true
}

func (r *Receivers) deleteMessage(topic string) {

	for i := 0; i < len(r.eb.topicMessages[topic]); {

		if time.Now().After(r.eb.topicMessages[topic][i].LifeTime) {

			r.eb.topicMessages[topic][i] = r.eb.topicMessages[topic][len(r.eb.topicMessages[topic])-1]
			r.eb.topicMessages[topic] = r.eb.topicMessages[topic][:len(r.eb.topicMessages[topic])-1]

		} else {

			i++
		}
	}
}

func (r *Receivers) garbageCollection() {

	for {

		for topic := range r.eb.topicMessages {

			go func() {

				r.deleteMessage(topic)

				sqlStatement := `DELETE FROM messages WHERE topic = $1`

				_, err := r.dbServer.db.Exec(sqlStatement, topic)

				if err != nil {
					panic(err)
				}

			}()

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

func (r *Receivers) notifications(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email

	startTime := time.Now().Nanosecond()

	latitudes, _ := c.Request.URL.Query()["latitude"]
	longitudes, _ := c.Request.URL.Query()["longitude"]
	radius, _ := c.Request.URL.Query()["radius"]

	sessionLatitude, _ := strconv.ParseFloat(latitudes[0], 64)
	sessionLongitude, _ := strconv.ParseFloat(longitudes[0], 64)
	sessionRadius, _ := strconv.ParseInt(radius[0], 10, 64)

	data, err := r.dbServer.db.Query("SELECT topic FROM subscriptions "+
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

		for _, topic := range r.eb.userTopics[email] {

			for _, message := range r.eb.topicMessages[topic] {

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

func (r *Receivers) editSubscription(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email

	topics, ok := c.Request.URL.Query()["topic"]

	if !ok || len(topics[0]) < 1 {
		log.Println("Url Param 'session' is missing")
		return
	}

	topic := topics[0]
	err := r.dbServer.db.QueryRow("select topic from subscriptions where subscriber = $1 and topic = $2", email, topic).Scan()

	switch {

	case err != sql.ErrNoRows:

		sqlStatement := `DELETE FROM subscriptions WHERE subscriber = $1 AND topic = $2`

		r.topicUnsubscription(email, topic)
		_, err := r.dbServer.db.Exec(sqlStatement, email, topic)

		if err != nil {
			panic(err)
		}

	case err == sql.ErrNoRows:

		sqlStatement := `INSERT INTO subscriptions (subscriber, topic) VALUES ($1, $2)`

		r.topicSubscription(topic, email)
		_, err := r.dbServer.db.Exec(sqlStatement, email, topic)

		if err != nil {
			panic(err)
		}

	default:

		panic(err)
	}
}

func publishPage(c *gin.Context) {

	redirecter(c, "publish.html", "logged", nil)
}

func (r *Receivers) publish(c *gin.Context) {

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

	sqlStatement := `INSERT INTO messages (payload, topic, radius, latitude, longitude, lifetime) VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.dbServer.db.Exec(sqlStatement, payload, topic, radius, latitude, longitude, lifeTime)

	if err != nil {
		panic(err)
	}

	go r.publishTo(topic, payload, radius, lifeTime, latitude, longitude)

	c.Redirect(301, "/publishPage")
}

func (r *Receivers) initEB() {

	messages, err := r.dbServer.db.Query("SELECT * FROM messages")

	if err != nil {
		panic(err)
	}

	for messages.Next() {
		var payload, topic, latitude, longitude string
		var radius, lifetime, id int
		messages.Scan(&payload, &topic, &id, &radius, &latitude, &longitude, &lifetime)
		r.publishTo(topic, payload, radius, lifetime, latitude, longitude)

	}

	subscriptions, err := r.dbServer.db.Query("SELECT * FROM subscriptions ORDER BY topic")

	if err != nil {
		panic(err)
	}

	for subscriptions.Next() {
		var subscriber, topic string
		subscriptions.Scan(&subscriber, &topic)

		//r.eb.topicMessages[topic] = DataEventSlice{}
		r.topicSubscription(topic, subscriber)
	}

	return
}

func main() {

	initRedis()
	s, db := initDB()
	defer db.Close()

	var eb = &EventBus{
		topicMessages: map[string]DataEventSlice{},
		userTopics:    map[string]Topics{},
	}

	var r = &Receivers{
		dbServer: *s,
		eb:       *eb,
	}

	r.initEB()

	go r.garbageCollection()

	router.StaticFS("/static/", http.Dir("../static"))
	router.LoadHTMLGlob("../templates/*")

	router.GET("/", loginPage)
	router.POST("/registration", s.registration)
	router.GET("/registrationPage", registrationPage)
	router.POST("/login", s.login)
	router.GET("/logout", TokenAuthMiddleware(), logout)
	router.GET("/notificationsPage", TokenAuthMiddleware(), notificationsPage)
	router.GET("/notifications", TokenAuthMiddleware(), r.notifications)
	router.GET("/publishPage", TokenAuthMiddleware(), publishPage)
	router.GET("/subscriptionPage", TokenAuthMiddleware(), s.subscriptionPage)
	router.GET("/editSubscription", TokenAuthMiddleware(), r.editSubscription)
	router.GET("/publish", TokenAuthMiddleware(), r.publish)

	log.Println("Listening on :8080...")
	err := router.Run(":8080")

	if err != nil {

		log.Fatal(err)
	}

}
