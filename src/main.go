// main.go
package main

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/magiconair/properties"
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

type MessageData struct {
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

type MessageDataSlice []MessageData

type Topics []string

var GlobalTopics Topics

//Struct for queue implementation
type EventBroker struct {
	topicMessages map[string]MessageDataSlice //key: topic - value: messages
	userTopics    map[string]Topics           //key: user  - value: topics
	rm            sync.RWMutex
}

type Receivers struct {
	dbServer server
	eb       EventBroker
}

//Map for requests filtering mechanism
var requests = make(map[string]MessageData)

//Loading from properties file
var p = properties.MustLoadFile("../conf.properties", properties.UTF8)
var dbPersistence = p.GetBool("db-persistence", true)
var deliverySemantic = p.GetString("delivery-semantic", "at-least-once")
var retryLimit = p.GetInt("retry-limit", 5)
var deliveryTimeout = p.GetInt("delivery-timeout", 100)
var eliminationPeriod = p.GetInt("elimination-period", 1)
var requestLifetime = p.GetInt("request-lifetime", 2)
var garbageCollectorPeriod = p.GetInt("garbage-collector-period", 1)
var listeningPort = p.GetString("app-listening-port", "8080")

var router = gin.Default()

var logFile os.File

//Adding user subscription into EventBroker
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

//Removing user subscription from EventBroker
func (r *Receivers) topicUnsubscription(email string, topic string) {

	r.eb.rm.Lock()

	topicList := r.eb.userTopics[email]
	var newTopicList []string

	for i := 0; i < len(topicList); i++ {

		if topicList[i] == topic {

			newTopicList = remove(topicList, i)
		}
	}

	delete(r.eb.userTopics, email)

	r.eb.userTopics[email] = newTopicList

	r.eb.rm.Unlock()
}

//Inserting message into EventBroker
func (r *Receivers) publishTo(messageData MessageData) bool {

	r.eb.rm.Lock()

	checked := false

	size := len(r.eb.topicMessages[messageData.Topic])
	r.eb.topicMessages[messageData.Topic] = append(r.eb.topicMessages[messageData.Topic], messageData)
	sizeAfter := len(r.eb.topicMessages[messageData.Topic])

	//size is bigger if insertion is completed
	if sizeAfter > size {
		checked = true
	}

	r.eb.rm.Unlock()

	return checked
}

//Deleting message from db
func (r *Receivers) deleteMessageFromDB(topic string) {

	sqlStatement := `DELETE FROM messages WHERE topic = $1 AND lifetime <= $2`
	_, err := r.dbServer.db.Exec(sqlStatement, topic, time.Now().Local())

	if err != nil {

		log.Panic(err)
	}
}

//Deleting message from queue
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

//Removing requests periodically for at-most-once and exactly-once semantics
func requestGarbageCollector() {

	for {

		for item := range requests {

			if time.Now().Local().After(requests[item].InsertionTime.Add(time.Minute * time.Duration(requestLifetime))) {
				delete(requests, item)
			}
		}

		time.Sleep(time.Minute * time.Duration(eliminationPeriod))
	}
}

//Removing request in exactly-once semantic
func removeRequest(c *gin.Context) {

	checkSession(c)

	var message MessageData
	err := json.NewDecoder(c.Request.Body).Decode(&message)

	if err != nil {
		log.Panic(err)
	}

	delete(requests, message.RequestID)
}

//Deleting expired messages from queue and db periodically
func (r *Receivers) messageGarbageCollector() {

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

//Redirecting to notifications page
func notificationsPage(c *gin.Context) {

	redirect(c, "notifications.html", "logged", nil, true, http.StatusOK, "Notifications Page")
}

//Getting all user messages based on radius and subscriptions
func (r *Receivers) notifications(c *gin.Context) {

	email := checkSession(c)

	var d MessageData
	err := json.NewDecoder(c.Request.Body).Decode(&d)

	if err != nil {
		log.Panic(err)
	}

	var notifications []MessageData

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
		log.Panic(err)
	}
}

//Redirecting to subscription page with subscription info
func (r *Receivers) subscriptionPage(c *gin.Context) {

	email := checkSession(c)

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
		log.Panic(err)
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

//Editing user subscription
func (r *Receivers) editSubscription(c *gin.Context) {

	email := checkSession(c)

	var dataEvent MessageData
	err := json.NewDecoder(c.Request.Body).Decode(&dataEvent)

	if err != nil {
		log.Panic(err)
	}

	subscriptions := r.eb.userTopics[email]

	//Deleting subscription if already subscribed
	if stringInSlice(dataEvent.Topic, subscriptions) {

		if dbPersistence {

			sqlStatement := `DELETE FROM subscriptions WHERE subscriber = $1 AND topic = $2`
			_, err := r.dbServer.db.Exec(sqlStatement, email, dataEvent.Topic)

			if err != nil {
				log.Panic(err)
			}
		}

		r.topicUnsubscription(email, dataEvent.Topic)

		userAgent := c.Request.Header.Get("User-Agent")

		if strings.Contains(userAgent, "curl") {

			message := "Unsubscribed from " + dataEvent.Topic
			c.JSON(http.StatusOK, message)
		}

	} else { //Adding subscription if not subscribed yet

		if dbPersistence {

			sqlStatement := `INSERT INTO subscriptions (subscriber, topic) VALUES ($1, $2)`
			_, err := r.dbServer.db.Exec(sqlStatement, email, dataEvent.Topic)

			if err != nil {
				log.Panic(err)
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

//Redirecting to publish page
func (r *Receivers) publishPage(c *gin.Context) {

	email := checkSession(c)

	results := GlobalTopics

	c.HTML(
		http.StatusOK,
		"publish.html",
		gin.H{
			"title":            "Publish Page",
			"status":           "logged",
			"results":          results,
			"email":            email,
			"deliverySemantic": deliverySemantic,
			"deliveryTimeout":  deliveryTimeout,
			"retryLimit":       retryLimit,
		},
	)
}

//Publishing message according to semantic
func (r *Receivers) publish(c *gin.Context) {

	checkSession(c)

	var message MessageData
	err := json.NewDecoder(c.Request.Body).Decode(&message)

	if err != nil {
		log.Panic(err)
	}

	found := true

	if deliverySemantic != "at-least-once" {

		//checking if request is duplicate
		_, found = requests[message.RequestID]
	}

	returnValue := "fail"

	if deliverySemantic == "at-least-once" || !found {

		expirationTime := time.Now().Local().Add(time.Minute * time.Duration(message.LifeTime))
		var msgID int

		ch := make(chan bool)

		go func() {
			ch <- r.publishTo(message)
		}()

		checked := <-ch

		//checking if queue insertion is successful
		if checked {

			//inserting in requests map for at-most-once and exactly-once semantics
			if deliverySemantic != "at-least-once" {
				message.InsertionTime = time.Now().Local()
				requests[message.RequestID] = message
			}

			if dbPersistence {

				err = r.dbServer.db.QueryRow(`INSERT INTO messages (payload, topic, radius, latitude, longitude, lifetime, title) 
					VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, message.Message, message.Topic, message.Radius, message.Latitude,
					message.Longitude, expirationTime, message.Title).Scan(&msgID)

				if err == nil {

					returnValue = "success"

				}

			} else {

				returnValue = "success"

			}

		} else {

			if dbPersistence {

				sqlStatement := `DELETE FROM messages WHERE id = $1`
				_, err := r.dbServer.db.Exec(sqlStatement, msgID)

				if err != nil {
					log.Panic(err)
				}
			}

			returnValue = "fail"
		}

	} else {

		returnValue = "success"
	}

	/*rnd := rand.Intn(4)
	if rnd > 0 {
		fmt.Println("RAND: ", rnd)
		time.Sleep(6 * time.Second)
	}*/

	result, _ := json.Marshal(returnValue)
	c.Writer.Header().Set("Content-Type", "application/json")
	_, err = c.Writer.Write(result)

	if err != nil {
		log.Panic(err)
	}
}

//Initializing event broker on application start-up
func (r *Receivers) initEB() {

	messages, err := r.dbServer.db.Query("SELECT * FROM messages")

	if err != nil {
		log.Panic(err)
	}

	for messages.Next() {
		var payload, topic, title, id, latitude, longitude string
		var radius int
		var lifetime time.Time
		_ = messages.Scan(&payload, &topic, &id, &radius, &latitude, &longitude, &lifetime, &title)

		latitudeFloat, _ := strconv.ParseFloat(latitude, 64)
		longitudeFloat, _ := strconv.ParseFloat(longitude, 64)

		messageData := MessageData{
			Message:        payload,
			Title:          title,
			Topic:          topic,
			Radius:         radius,
			ExpirationTime: lifetime,
			Latitude:       latitudeFloat,
			Longitude:      longitudeFloat,
		}

		r.publishTo(messageData)

	}

	subscriptions, err := r.dbServer.db.Query("SELECT * FROM subscriptions ORDER BY topic")

	if err != nil {
		log.Panic(err)
	}

	for subscriptions.Next() {

		var subscriber, topic string
		_ = subscriptions.Scan(&subscriber, &topic)
		r.topicSubscription(topic, subscriber)
	}

	data, err := r.dbServer.db.Query("SELECT name FROM topics ")

	if err != nil {
		log.Panic(err)
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

	var eb = &EventBroker{
		topicMessages: map[string]MessageDataSlice{},
		userTopics:    map[string]Topics{},
	}

	var r = &Receivers{
		dbServer: *s,
		eb:       *eb,
	}

	r.initEB()

	go r.messageGarbageCollector() //go routine for message garbage collector
	go requestGarbageCollector()   //go routine for requests garbage collector

	logFile, err := os.OpenFile("../log/server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		log.Fatal(err)
	}

	log.SetOutput(logFile)

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

	log.Println("Listening on :", listeningPort)
	err = router.Run(":" + listeningPort)

	if err != nil {

		log.Fatal(err)
	}
}
