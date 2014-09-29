package main

import (
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
//can be done with  "github.com/balu-/jsonrpc"
	"time"
)

const (
	RECV_BUF_LEN = 1024
)

type UdevStr struct {
	Test string
	Blub string
}

type Servable struct {
}

func (t *Servable) Ping(args *string, reply *string) error {
	fmt.Println("REMOTE: Ping")
	(*reply) = "Pong Cli:" + *args
	fmt.Println("Ping reply:" + *reply)
	return nil
}

func main() {
	fmt.Println("hi")

	service := "127.0.0.1:2000"
	tcpAddr, err := net.ResolveTCPAddr("tcp", service)
	if err != nil {
		fmt.Println("äh tot")
		return
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)

	if err != nil {
		fmt.Println("could not connect to remote host " + err.Error())
		return
	}

	conn.SetKeepAlive(true)
	conn.SetKeepAlivePeriod(5 * time.Second)

	// conn.Close()
	// time.Sleep(2 * time.Second)

	// if conn != nil {
	// 	fmt.Println("Con != nil")
	// 	fmt.Println(conn)
	// }
	// return

	server := rpc.NewServer()
	localNode := new(Servable)
	server.Register(localNode)

	handle(server, conn)

}

//function for handling RPC connection
func handle(server *rpc.Server, conn net.Conn) {
	fmt.Println("start handle")
	//defer conn.Close() //make sure connection gets closed
	remote := conn.RemoteAddr().String() + " --> " + conn.LocalAddr().String()
	fmt.Println("==conn " + remote)
	//requests
	//doRequests(conn)
	// time.Sleep(3 * time.Second)

	fmt.Println("==conn " + remote)
	server.ServeCodec(jsonrpc.NewServerCodec(conn))
	fmt.Println("==discon " + remote)
	fmt.Println("end handle")
}

func doRequests(conn net.Conn) {

	fmt.Println("start doRequests")

	jsonRpcClient := jsonrpc.NewClient(conn)

	for {
		fmt.Println("waiting for okay to go")
		var mySStr string
		fmt.Scanln(&mySStr)
		fmt.Println("go")

		var pong string
		go func() {
			var test *UdevStr
			test = new(UdevStr)
			test.Blub = "hallo"
			test.Test = "test"
			err := jsonRpcClient.Call("Servable.Ping", "HI TEHERE", &pong) // "hi there"
			if err != nil {
				//fail
				fmt.Errorf("Could not ping/pong PredReplace", err)
				return
			}

			fmt.Println("Ping pong:" + pong)
		}()

	}

	fmt.Println("end doRequests")

}
