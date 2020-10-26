// main.go
package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/magiconair/properties"
	"github.com/umahmood/haversine"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Topic struct {
	Name string
	Flag bool
}

type DataEvent struct {
	Message        string    `json:"Message"`
	Title          string    `json:"Title"`
	Topic          string    `json:"Topic"`
	RequestID      string    `json:"RequestID"`
	Radius         int       `json:"Radius,string"`
	LifeTime       int       `json:"LifeTime,string"`
	InsertionTime  time.Time `json:"InsertionTime,string"`
	ExpirationTime time.Time `json:"ExpirationTime,string"`
	Latitude       float64   `json:"Latitude"`
	Longitude      float64   `json:"Longitude"`
}

type DataEventSlice []DataEvent

type Topics []string

var GlobalTopics Topics

type EventBus struct {
	topicMessages map[string]DataEventSlice //key: topic - value: messages
	userTopics    map[string]Topics         //key: user  - value: topics
	rm            sync.RWMutex
}

type Receivers struct {
	dbServer server
	eb       EventBus
}

var requests = make(map[string]DataEvent)
var p = properties.MustLoadFile("../conf.properties", properties.UTF8)
var dbPersistence = p.GetBool("db-persistence", true)
var deliverySemantic = p.GetString("delivery-semantic", "at-least-once")
var retryLimit = p.GetInt("retry-limit", 5)
var deliveryTimeout = p.GetInt("delivery-timeout", 100)
var eliminationPeriod = p.GetInt("elimination-period", 1)
var requestLifetime = p.GetInt("request-lifetime", 2)
var garbageCollectorPeriod = p.GetInt("garbage-collector-period", 1)
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

func (r *Receivers) publishTo(topic string, title string, message string, radius int, lifeTime time.Time, latitude string, longitude string) bool {

	r.eb.rm.RLock()

	checked := false

	latitudeFloat, _ := strconv.ParseFloat(latitude, 64)
	longitudeFloat, _ := strconv.ParseFloat(longitude, 64)
	dataEvent := DataEvent{Message: message, Topic: topic, Title: title, Radius: radius, ExpirationTime: lifeTime,
		Latitude: latitudeFloat, Longitude: longitudeFloat}

	size := len(r.eb.topicMessages[topic])
	r.eb.topicMessages[topic] = append(r.eb.topicMessages[topic], dataEvent)
	size1 := len(r.eb.topicMessages[topic])

	//size is bigger if insertion is completed
	if size1 > size {
		checked = true
	}

	r.eb.rm.RUnlock()

	return checked
}

func (r *Receivers) deleteMessageFromDB(topic string) {

	sqlStatement := `DELETE FROM messages WHERE topic = $1 AND lifetime <= $2`
	_, err := r.dbServer.db.Exec(sqlStatement, topic, time.Now().Local())

	if err != nil {
		panic(err)
	}
}

func (r *Receivers) deleteMessageFromQueue(topic string) {

	for i := 0; i < len(r.eb.topicMessages[topic]); {

		if time.Now().Local().After(r.eb.topicMessages[topic][i].ExpirationTime) {

			r.eb.topicMessages[topic][i] = r.eb.topicMessages[topic][len(r.eb.topicMessages[topic])-1]
			r.eb.topicMessages[topic] = r.eb.topicMessages[topic][:len(r.eb.topicMessages[topic])-1]

		} else {

			i++
		}
	}
}

func requestsRemoval() {

	for {

		for item := range requests {

			if time.Now().Local().After(requests[item].InsertionTime.Add(time.Minute * time.Duration(requestLifetime))) {
				delete(requests, item)
			}
		}

		time.Sleep(time.Minute * time.Duration(eliminationPeriod))
	}
}

func removeRequest(c *gin.Context) {

	checkSession(c)

	var message DataEvent
	err := json.NewDecoder(c.Request.Body).Decode(&message)

	if err != nil {
		panic(err)
	}

	delete(requests, message.RequestID)
}

func (r *Receivers) garbageCollection() {

	for {

		for topic := range r.eb.topicMessages {

			go func() {

				if dbPersistence {
					r.deleteMessageFromDB(topic)
				}

				r.deleteMessageFromQueue(topic)
			}()
		}

		time.Sleep(time.Minute * time.Duration(garbageCollectorPeriod))
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

func stringInSlice(a string, list []string) bool {

	for _, b := range list {

		if b == a {
			return true
		}
	}

	return false
}

func notificationsPage(c *gin.Context) {

	redirect(c, "notifications.html", "logged", nil, true, http.StatusOK, "Notifications")
}

func (r *Receivers) notifications(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email

	var d DataEvent
	err := json.NewDecoder(c.Request.Body).Decode(&d)

	if err != nil {
		panic(err)
	}

	var notifications []DataEvent

	for _, topic := range r.eb.userTopics[email] {

		for _, message := range r.eb.topicMessages[topic] {

			if checkDistance(d.Latitude, message.Latitude, d.Longitude, message.Longitude, d.Radius, message.Radius) {

				notifications = append(notifications, message)
			}
		}
	}

	result, _ := json.Marshal(notifications)
	c.Writer.Header().Set("Content-Type", "application/json")
	_, err = c.Writer.Write(result)

	if err != nil {
		panic(err)
	}
}

func (r *Receivers) subscriptionPage(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email
	subscribed := r.eb.userTopics[email]

	tRes := Topic{}
	var results []Topic

	for _, topic := range subscribed {
		tRes.Name = topic
		tRes.Flag = true
		results = append(results, tRes)
	}

	noSubscribed, err := r.dbServer.db.Query("select t.name from topics t where t.name "+
		"not in (select s.topic from subscriptions s where s.subscriber = $1)", email)

	if err != nil {
		panic(err)
	}

	for noSubscribed.Next() {
		var name string
		_ = noSubscribed.Scan(&name)
		tRes.Name = name
		tRes.Flag = false
		results = append(results, tRes)
	}

	userAgent := c.Request.Header.Get("User-Agent")

	if strings.Contains(userAgent, "curl") {

		c.JSON(http.StatusOK, results)

	} else {

		redirect(c, "subscribe.html", "logged", results, true, http.StatusOK, "Subscription Page")
	}
}

func (r *Receivers) editSubscription(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email

	var dataEvent DataEvent
	err := json.NewDecoder(c.Request.Body).Decode(&dataEvent)

	if err != nil {
		panic(err)
	}

	subscriptions := r.eb.userTopics[email]

	if stringInSlice(dataEvent.Topic, subscriptions) {

		if dbPersistence {

			sqlStatement := `DELETE FROM subscriptions WHERE subscriber = $1 AND topic = $2`
			_, err := r.dbServer.db.Exec(sqlStatement, email, dataEvent.Topic)

			if err != nil {
				panic(err)
			}
		}

		r.topicUnsubscription(email, dataEvent.Topic)

		userAgent := c.Request.Header.Get("User-Agent")

		if strings.Contains(userAgent, "curl") {

			message := "Unsubscribed from " + dataEvent.Topic
			c.JSON(http.StatusOK, message)
		}

	} else {

		if dbPersistence {

			sqlStatement := `INSERT INTO subscriptions (subscriber, topic) VALUES ($1, $2)`
			_, err := r.dbServer.db.Exec(sqlStatement, email, dataEvent.Topic)

			if err != nil {
				panic(err)
			}
		}

		r.topicSubscription(dataEvent.Topic, email)

		userAgent := c.Request.Header.Get("User-Agent")

		if strings.Contains(userAgent, "curl") {

			message := "Subscribed to " + dataEvent.Topic
			c.JSON(http.StatusOK, message)
		}
	}
}

func (r *Receivers) publishPage(c *gin.Context) {

	checkSession(c)

	ad, _ := ExtractTokenMetadata(c)
	email := ad.Email
	results := GlobalTopics

	c.HTML(
		http.StatusOK,
		"publish.html",
		gin.H{
			"title":            "",
			"status":           "logged",
			"results":          results,
			"email":            email,
			"deliverySemantic": deliverySemantic,
			"deliveryTimeout":  deliveryTimeout,
			"retryLimit":       retryLimit,
		},
	)
}

func (r *Receivers) publish(c *gin.Context) {

	checkSession(c)

	var message DataEvent
	err := json.NewDecoder(c.Request.Body).Decode(&message)

	if err != nil {
		panic(err)
	}

	found := true

	if deliverySemantic != "at-least-once" {

		_, found = requests[message.RequestID]
	}

	variable := "fail"

	if deliverySemantic == "at-least-once" || !found {

		expirationTime := time.Now().Local().Add(time.Minute * time.Duration(message.LifeTime))
		var msgID int

		ch := make(chan bool)

		go func() {
			ch <- r.publishTo(message.Topic, message.Title, message.Message, message.Radius, expirationTime, fmt.Sprintf("%f", message.Latitude),
				fmt.Sprintf("%f", message.Longitude))
		}()

		checked := <-ch

		if checked {

			if deliverySemantic != "at-least-once" {
				message.InsertionTime = time.Now().Local()
				requests[message.RequestID] = message
			}

			if dbPersistence {

				err = r.dbServer.db.QueryRow(`INSERT INTO messages (payload, topic, radius, latitude, longitude, lifetime, title) 
					VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, message.Message, message.Topic, message.Radius, message.Latitude,
					message.Longitude, expirationTime, message.Title).Scan(&msgID)

				if err == nil {

					variable = "success"

				}

			} else {

				variable = "success"

			}

		} else {

			if dbPersistence {

				sqlStatement := `DELETE FROM messages WHERE id = $1`
				_, err := r.dbServer.db.Exec(sqlStatement, msgID)

				if err != nil {
					panic(err)
				}
			}

			variable = "fail"
		}

	} else {

		variable = "success"
	}

	/*rnd := rand.Intn(4)
	if rnd > 0 {
		fmt.Println("RAND: ", rnd)
		time.Sleep(6 * time.Second)
	}*/

	result, _ := json.Marshal(variable)
	c.Writer.Header().Set("Content-Type", "application/json")
	_, err = c.Writer.Write(result)

	if err != nil {
		panic(err)
	}
}

func (r *Receivers) initEB() {

	messages, err := r.dbServer.db.Query("SELECT * FROM messages")

	if err != nil {
		panic(err)
	}

	for messages.Next() {
		var payload, topic, title, latitude, longitude string
		var radius, id int
		var lifetime time.Time
		_ = messages.Scan(&payload, &topic, &id, &radius, &latitude, &longitude, &lifetime, &title)
		r.publishTo(topic, title, payload, radius, lifetime, latitude, longitude)

	}

	subscriptions, err := r.dbServer.db.Query("SELECT * FROM subscriptions ORDER BY topic")

	if err != nil {
		panic(err)
	}

	for subscriptions.Next() {

		var subscriber, topic string
		_ = subscriptions.Scan(&subscriber, &topic)
		r.topicSubscription(topic, subscriber)
	}

	data, err := r.dbServer.db.Query("SELECT name FROM topics ")

	if err != nil {
		panic(err)
	}

	for data.Next() {
		var topic string
		_ = data.Scan(&topic)
		GlobalTopics = append(GlobalTopics, topic)
	}
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
	go requestsRemoval()

	logFile, err := os.OpenFile("../log/server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		log.Fatal(err)
	}

	router.Use(gin.LoggerWithWriter(logFile))

	router.StaticFS("/static/", http.Dir("../static"))
	router.LoadHTMLGlob("../templates/*")

	router.GET("/", loginPage)
	router.GET("/registrationPage", registrationPage)
	router.GET("/logout", TokenAuthMiddleware(), logout)
	router.GET("/publishPage", TokenAuthMiddleware(), r.publishPage)
	router.GET("/subscriptionPage", TokenAuthMiddleware(), r.subscriptionPage)
	router.GET("/notificationsPage", TokenAuthMiddleware(), notificationsPage)

	router.POST("/login", s.login)
	router.POST("/registration", s.registration)
	router.POST("/publish", TokenAuthMiddleware(), r.publish)
	router.POST("/editSubscription", TokenAuthMiddleware(), r.editSubscription)
	router.POST("/notifications", TokenAuthMiddleware(), r.notifications)
	router.POST("/removeRequest", TokenAuthMiddleware(), removeRequest)

	log.Println("Listening on :8080...")
	err = router.Run(":8080")

	if err != nil {

		log.Fatal(err)
	}
}
