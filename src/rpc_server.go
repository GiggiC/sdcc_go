package main

import (
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"time"
)

//Message structure
type Structure struct {

	Message    string
	Visibility bool
	Semantic   int
	Timeout    int
}

type ReturnQueue struct{

	A map[int]Structure
}

type Channel struct {
	val int
}

type API int

var queue = make(map[int]Structure)
var keys []Channel
var count int
var index int
var indexConn int
var connectionsNumber int
var connections []*rpc.Client

//Getting minimum key/ next message to be served
func MinIntSlice(v []Channel) (m int) {

	m = v[0].val

	for i := 0; i < len(v); i++ {
		if v[i].val < m {
			m = v[i].val
			index = i
		}
	}
	return
}

//RPC function to insert messages into the queue
func (a *API) QueueInsert(arguments *Structure, reply *Structure) error {

	keys = append(keys, Channel{count})
	arguments.Visibility = true
	queue[count] = *arguments
	count++
	reply.Message = arguments.Message
	fmt.Printf("SERVER: Value received and placed in the queue: %s \n", arguments.Message)

	return nil
}

//RPC function to get queue status
func (a *API) GetQueue(arguments *Structure, reply *ReturnQueue) error {

	reply.A = queue

	return nil
}

//At-least-once semantic
func AtLEastOnceDelivery(min int) {

	reply := new(Structure)
	divCall := connections[indexConn].Go("API.Procedure", queue[min], reply, nil)
	replyCall := <-divCall.Done

	if replyCall.Error == nil {

		delete(queue, min)
		keys = keys[1:]

	} else {

		fmt.Print("Error occurred - retransmission\n")
		time.Sleep(time.Duration(queue[min].Timeout) * 1000000000)
		return
	}

	//Fair dispatching to consumers
	if indexConn == connectionsNumber-1 {

		indexConn = 0
		return

	} else {

		indexConn++
		return
	}
}

//Timeout-based semantic
func TimeOutBasedDelivery(min int) {

	reply := new(Structure)
	divCall := connections[indexConn].Go("API.Procedure", queue[min], reply, nil)

	select {

	case replyCall := <-divCall.Done:

		if replyCall.Error == nil {

			elem := new(Structure)
			elem.Message = queue[min].Message
			elem.Visibility = false
			elem.Semantic = 2
			elem.Timeout = queue[min].Timeout
			queue[min] = *elem

		}

		delete(queue, min)
		keys = keys[1:]

		//Fair dispatching to consumers
		if indexConn == connectionsNumber-1 {

			indexConn = 0
			return

		} else {

			indexConn++
			return
		}

	case <-time.After(time.Duration(queue[min].Timeout) * 1000000000):

		fmt.Print("Timeout occurred - retransmission\n")
		elem := new(Structure)
		elem.Message = queue[min].Message
		elem.Visibility = true
		elem.Semantic = 2
		elem.Timeout = queue[min].Timeout
		queue[min] = *elem
		return
	}
}

func main() {

	arguments := os.Args

	if len(arguments) < 2 {

		fmt.Println("usage: progName listeningPort consumerPort_1 consumerPort_2 ... consumerPort_n")
		return
	}

	PORT := ":" + arguments[1]

	var api = new(API)
	err := rpc.Register(api)
	if err != nil {

		fmt.Println(err)
		return
	}

	t, err := net.ResolveTCPAddr("tcp4", PORT)
	if err != nil {

		fmt.Println(err)
		return
	}

	l, err := net.ListenTCP("tcp4", t)
	if err != nil {

		fmt.Println(err)
		return
	}

	port, _ := strconv.Atoi(arguments[1])
	fmt.Printf("SERVER: Listening on port %d\n", port)

	count = 0
	index = 0
	indexConn = 0
	connectionsNumber = len(arguments) - 2
	var connectionsPort = arguments[2:]
	connections = make([]*rpc.Client, connectionsNumber)

	//accepting incoming requests
	go func() {

		for {

			c, err := l.Accept()
			if err != nil {

				continue
			}

			rpc.ServeConn(c)
		}
	}()

	//Connection to the consumers
	for i := 0; i < connectionsNumber; i++ {

		CONNECT := ":" + connectionsPort[i]

		c, err := rpc.Dial("tcp", CONNECT)
		connections[i] = c
		if err != nil {

			fmt.Println(err)
			return
		}
	}

	for {


		//Eventual waiting time before starting service
		//time.Sleep(20*time.Second)

		if len(queue) != 0 {

			var min = MinIntSlice(keys)

			if queue[min].Visibility == true {

				if queue[min].Semantic == 1 { //at-least-once delivery

					AtLEastOnceDelivery(min)

				} else if queue[min].Semantic == 2 { //timeout-based delivery

					TimeOutBasedDelivery(min)
				}
			}
		}
	}
}
