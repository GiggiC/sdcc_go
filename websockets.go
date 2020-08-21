// websockets.go
package main

import (
	"database/sql"
	"fmt"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

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

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/", serveTemplate)

	http.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
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
	})

	log.Println("Listening on :8080...")
	err3 := http.ListenAndServe(":8080", nil)
	if err3 != nil {
		log.Fatal(err)
	}

	http.ListenAndServe(":8080", nil)

}

func serveTemplate(w http.ResponseWriter, r *http.Request) {
	lp := filepath.Join("templates", "layout.html")
	fp := filepath.Join("templates", filepath.Clean(r.URL.Path))

	tmpl, _ := template.ParseFiles(lp, fp)
	tmpl.ExecuteTemplate(w, "layout", nil)
}
