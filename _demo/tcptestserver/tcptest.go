package main

import (
	"github.com/balu-/jsonrpc"
	"fmt"
	"log"
	"net"
)

//TODO
// SEND MUTEXE

const (
	RECV_BUF_LEN = 1024
)

type Servable struct {
}

func (t *Servable) Ping(args *string, reply *string) error {
	fmt.Println("REMOTE: Ping")
	//str := "Pong" + *args
	(*reply) = "Pong zu " + *args
	fmt.Println("Ping reply:" + *reply)
	return nil
}

//starts listening as json-rpc server
//todo listen only on defined ip
func StartServer() int {
	service := ":2000"
	//server := rpc.NewServer()
	//localNode := new(Servable)
	//server.Register(localNode)

	//server.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)
	fmt.Println("Prepare listen on port " + service)
	l, e := net.Listen("tcp", service)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}

		go handle(conn)
	}

	fmt.Println("Close " + l.Addr().String())
	return 0
}

//function for handling RPC connection
func handle(conn net.Conn) {
	RPCStruct := new(Servable)
	json := jsonrpc.NewJsonRpc(conn)
	json.Register(RPCStruct, "")
	go json.Serve()
	for {

		fmt.Println("waiting for okay to go")
		var mySStr string
		fmt.Scanln(&mySStr)
		fmt.Println("go")

		var pong string
		go func() {

			err := json.Call("Servable.Ping", "HI TEHERE", &pong) // "hi there"
			if err != nil {
				fmt.Println("FEHLER!")
				//fail
				fmt.Errorf("Could not ping/pong PredReplace", err)
				return
			}

			fmt.Println("Ping pong:" + pong)
		}()

	}
	fmt.Println("Verbindung zu")

}

func main() {
	fmt.Println("boot")
	StartServer()

}
