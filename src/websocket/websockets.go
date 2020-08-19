// websockets.go
package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Object struct {
	Message string `json:"message"`
	Topic   string `json:"topic"`
}

func main() {
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity

		for {

			var object Object

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

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "websockets.html")
	})

	http.ListenAndServe(":8080", nil)
}
