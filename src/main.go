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
	Message   string  `json:"Message"`
	Topic     string  `json:"Topic"`
	Radius    int     `json:"Radius,string"`
	LifeTime  int     `json:"LifeTime,string"`
	Latitude  float64 `json:"Latitude"`
	Longitude float64 `json:"Longitude"`
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

var router = gin.Default()

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

	r.eb.rm.RLock()

	checked := false

	for i := 0; i < 5; i++ {

		latitudeFloat, _ := strconv.ParseFloat(latitude, 64)
		longitudeFloat, _ := strconv.ParseFloat(longitude, 64)
		dataEvent := DataEvent{Message: message, Topic: topic, Radius: radius, LifeTime: lifeTime,
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

		expirationTime := time.Now().Local().Add(time.Minute * time.Duration(r.eb.topicMessages[topic][i].LifeTime))

		if time.Now().After(expirationTime) {

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

	redirecter(c, "notifications.html", "logged", nil, true, http.StatusOK, "")
}

func (r *Receivers) notifications(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email

	startTime := time.Now().Nanosecond()

	var d DataEvent
	err := json.NewDecoder(c.Request.Body).Decode(&d)

	if err != nil {
		panic(err)
	}

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

				if checkDistance(d.Latitude, message.Latitude, d.Longitude, message.Longitude, d.Radius, message.Radius) {

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

	redirecter(c, "subscribe.html", "logged", results, true, http.StatusOK, "")
}

func (r *Receivers) editSubscription(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email

	var d DataEvent
	err := json.NewDecoder(c.Request.Body).Decode(&d)

	if err != nil {
		panic(err)
	}

	err = r.dbServer.db.QueryRow("select topic from subscriptions where subscriber = $1 and topic = $2", email, d.Topic).Scan()

	switch {

	case err != sql.ErrNoRows:

		sqlStatement := `DELETE FROM subscriptions WHERE subscriber = $1 AND topic = $2`

		r.topicUnsubscription(email, d.Topic)
		_, err := r.dbServer.db.Exec(sqlStatement, email, d.Topic)

		if err != nil {
			panic(err)
		}

	case err == sql.ErrNoRows:

		sqlStatement := `INSERT INTO subscriptions (subscriber, topic) VALUES ($1, $2)`

		r.topicSubscription(d.Topic, email)
		_, err := r.dbServer.db.Exec(sqlStatement, email, d.Topic)

		if err != nil {
			panic(err)
		}

	default:

		panic(err)
	}
}

func publishPage(c *gin.Context) {

	redirecter(c, "publish.html", "logged", nil, true, http.StatusOK, "")
}

func (r *Receivers) publish(c *gin.Context) {

	checkSession(c)

	var d DataEvent
	err := json.NewDecoder(c.Request.Body).Decode(&d)

	if err != nil {
		panic(err)
	}

	sqlStatement := `INSERT INTO messages (payload, topic, radius, latitude, longitude, lifetime) VALUES ($1, $2, $3, $4, $5, $6)`

	_, err = r.dbServer.db.Exec(sqlStatement, d.Message, d.Topic, d.Radius, d.Latitude, d.Longitude, d.LifeTime)

	if err != nil {
		panic(err)
	}

	go r.publishTo(d.Topic, d.Message, d.Radius, d.LifeTime, fmt.Sprintf("%f", d.Latitude), fmt.Sprintf("%f", d.Longitude))
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

	// go r.garbageCollection()

	router.StaticFS("/static/", http.Dir("../static"))
	router.LoadHTMLGlob("../templates/*")

	router.GET("/", loginPage)
	router.GET("/registrationPage", registrationPage)
	router.GET("/logout", TokenAuthMiddleware(), logout)
	router.GET("/publishPage", TokenAuthMiddleware(), publishPage)
	router.GET("/subscriptionPage", TokenAuthMiddleware(), s.subscriptionPage)
	router.GET("/notificationsPage", TokenAuthMiddleware(), notificationsPage)

	router.POST("/login", s.login)
	router.POST("/registration", s.registration)
	router.POST("/publish", TokenAuthMiddleware(), r.publish)
	router.POST("/editSubscription", TokenAuthMiddleware(), r.editSubscription)
	router.POST("/notifications", TokenAuthMiddleware(), r.notifications)

	log.Println("Listening on :8080...")
	err := router.Run(":8080")

	if err != nil {

		log.Fatal(err)
	}

}
