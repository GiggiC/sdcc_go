package main

import (
	"fmt"
	"math/rand"
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

type API int

//RPC function to get message from queue
func (a *API) Procedure(arguments *Structure, reply *Structure) error {

	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)

	x := r1.Intn(3)

	if arguments.Semantic == 2 {

		if x == 1 {

			time.Sleep(2*time.Second)
		}

	} else if arguments.Semantic == 1 {

		if x == 2 {

			fmt.Print("CONSUMER: Error occurred\n")
			return rpc.ErrShutdown
		}
	}

	reply.Message = arguments.Message
	fmt.Printf("CONSUMER: Value received from the queue: %s\n", arguments.Message)

	return nil
}

func main() {

	arguments := os.Args
	if len(arguments) == 1 {

		fmt.Println("usage: progName listening port")
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

	fmt.Printf("CONSUMER: Listening on port %d\n", port)

	for {

		c, err := l.Accept()
		if err != nil {
			continue
		}
		rpc.ServeConn(c)
	}
}
