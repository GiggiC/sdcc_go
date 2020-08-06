package main

import (
	"fmt"
	"net/rpc"
	"os"
	"strconv"
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

func main() {

	arguments := os.Args

	if len(arguments) < 3{

		fmt.Println("usage: progName serverListeningPort message semantic timeout(s)")
		fmt.Println("usage: progName serverListeningPort queue")
		return
	}

	//Connectio to the server
	CONNECT := ":" + arguments[1]

	c, err := rpc.Dial("tcp", CONNECT)
	if err != nil {

		fmt.Println(err)
		return
	}

	//Asynchronus call to get queue status
	if arguments[2] == "queue" {

		var structure = new(Structure)
		var returnQueue = new(ReturnQueue)

		divCall := c.Go("API.GetQueue", structure, returnQueue, nil)
		replyCall := <-divCall.Done
		ret := replyCall.Reply.(*ReturnQueue).A

		for _, value := range ret {

			fmt.Printf("\nMessage: %s\n", value.Message)
			fmt.Printf("Visibility: %t\n", value.Visibility)
			fmt.Printf("Semantic: %d\n", value.Semantic)
			fmt.Printf("Timeout: %d\n", value.Timeout)
		}

		return
	}
	
	request := new(Structure)
	request.Message = arguments[2]
	request.Visibility = true
	request.Semantic, _ = strconv.Atoi(arguments[3])
	request.Timeout, _ = strconv.Atoi(arguments[4])

	reply := new(Structure)

	//Asynchronus call to insert message into the queue
	c.Go("API.QueueInsert", request, reply, nil)
	fmt.Print("PRODUCER: The SERVER received the message and put it in the queue\n")

	return
}
