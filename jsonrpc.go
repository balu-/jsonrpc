package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"sync"
)

const (
	DEBUG_PRINT = false
)

var null = json.RawMessage([]byte("null"))

// Precompute the reflect type for error.  Can't use error directly
// because Typeof takes an empty interface value.  This is annoying.
var typeOfError = reflect.TypeOf((*error)(nil)).Elem()

type jsonMessage struct {
	Method string           `json:"method"`
	Result *json.RawMessage `json:"result"`
	Error  *json.RawMessage `json:"error"`
	Params *json.RawMessage `json:"params"`
	Id     *json.RawMessage `json:"id"`
}

func (j *jsonMessage) reset() {
	j.Method = ""
	j.Result = nil
	j.Params = nil
	j.Id = nil
}

func (j *jsonMessage) isRequest() bool {
	return (j.Method != "" && j.Params != nil && j.Id != nil && j.Result == nil && j.Error == nil)
}

func (j *jsonMessage) getRequest(parent *JsonRPC) *serverRequest {
	//for declaration of serverRequest see server.go
	req := new(serverRequest)
	req.Id = j.Id
	req.Method = j.Method
	req.Params = j.Params
	req.parent = parent
	return req
}

func (j *jsonMessage) isResponse() bool {
	return ((j.Result != nil || j.Error != nil) && j.Id != nil && j.Method == "" && j.Params == nil)
}

func (j *jsonMessage) getResponse(parent *JsonRPC) *clientResponse {
	resp := new(clientResponse)
	resp.Error = j.Error
	err := json.Unmarshal(*j.Id, &resp.Id)
	if DEBUG_PRINT { //debug?
		if err != nil {
			fmt.Println("fataler fehler id parsing: " + err.Error())
		} else {
			fmt.Printf("Response ID : %d", resp.Id)
		}
	}
	resp.Result = j.Result
	resp.parent = parent

	return resp
}

//structs for reflection
type methodType struct {
	//	sync.Mutex // protects counters
	method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
	//	numCalls   uint
}

type service struct {
	name   string                 // name of service
	rcvr   reflect.Value          // receiver of methods for the service
	typ    reflect.Type           // type of the receiver
	method map[string]*methodType // registered methods
}

//top object
type JsonRPC struct {
	dec *json.Decoder // for reading JSON values
	enc *json.Encoder // for writing JSON values
	c   io.Closer

	//reflection data
	mu         sync.RWMutex // protects the serviceMap
	serviceMap map[string]*service

	//client
	clmu             sync.RWMutex
	pendingCalls     map[uint64]*Call //waiting calls
	clientSequenzNum uint64
}

func (j *JsonRPC) Close() {
	j.c.Close()

}

func NewJsonRpc(conn io.ReadWriteCloser) *JsonRPC {
	str := new(JsonRPC)
	str.c = conn
	str.dec = json.NewDecoder(conn)
	str.enc = json.NewEncoder(conn)
	str.serviceMap = make(map[string]*service) //reflection map
	str.clientSequenzNum = 0
	str.pendingCalls = make(map[uint64]*Call)

	return str
}

/*
	register methods of rcvr as rpc callable
	name is used as name for it or original name if name string is ""
*/
func (j *JsonRPC) Register(rcvr interface{}, name string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	s := new(service)
	s.typ = reflect.TypeOf(rcvr)
	s.rcvr = reflect.ValueOf(rcvr)
	sname := reflect.Indirect(s.rcvr).Type().Name()

	//check if struct is exported
	if !isExported(sname) {
		s := "rpc.Register: type " + sname + " is not exported"
		log.Print(s)
		return errors.New(s)
	}

	//check if struct should be renamed
	if name != "" {
		sname = name
	}

	//check if allready registerd
	if _, present := j.serviceMap[sname]; present {
		return errors.New("rpc: service already defined: " + sname)
	}
	s.name = sname

	//install methods
	s.method = suitableMethods(s.typ, true)

	if len(s.method) == 0 {
		str := ""
		// To help the user, see if a pointer receiver would work.
		method := suitableMethods(reflect.PtrTo(s.typ), false)
		if len(method) != 0 {
			str = "rpc.Register: type " + sname + " has no exported methods of suitable type (hint: pass a pointer to value of that type)"
		} else {
			str = "rpc.Register: type " + sname + " has no exported methods of suitable type"
		}
		log.Print(str)
		return errors.New(str)
	}

	//finaly realy register
	j.serviceMap[s.name] = s

	//debug print
	if DEBUG_PRINT {
		for k, v := range j.serviceMap {
			fmt.Println("Registriert " + k + " | " + v.name + " | ")
			for k2, v2 := range v.method {
				fmt.Println("Methode " + k2 + " | " + v2.method.Name + " | " + v2.method.PkgPath)
			}
		}
	}

	return nil
}

//handle incoming data
//needed for server and client use
func (j *JsonRPC) Serve() {

	msg := new(jsonMessage)

	for {
		err := j.dec.Decode(msg)
		if err == nil {
			if DEBUG_PRINT { //debug
				fmt.Printf("MSG: Method: %s | Res: %s | Param: %s | ID: %s\n", msg.Method, msg.Result, msg.Params, msg.Id)
			}
			if msg.isResponse() {
				if DEBUG_PRINT { //debug
					fmt.Println("Client Response")
				}
				//build response + handle
				resp := msg.getResponse(j)
				resp.handle()

			} else if msg.isRequest() {
				if DEBUG_PRINT { //debug
					fmt.Println("Server Request")
				}
				//build request + handle
				req := msg.getRequest(j)
				req.handle()

			} else {
				if DEBUG_PRINT { //debug
					fmt.Println("WTF wrong format")
				}
			}

		} else {
			fmt.Errorf("Ups")
			break
		}
		msg.reset()
	}
}
