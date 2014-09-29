// Package jsonrpc implements a JSON-RPC ClientCodec and ServerCodec
// for the rpc package.
package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
)

type clientRequest struct {
	Method string         `json:"method"`
	Params [1]interface{} `json:"params"`
	Id     uint64         `json:"id"`
}

type clientResponse struct {
	Id     uint64           //`json:"id"`
	Result *json.RawMessage //`json:"result"`
	Error  interface{}      //`json:"error"`
	parent *JsonRPC
}

func (r *clientResponse) reset() {
	r.Id = 0
	r.Result = nil
	r.Error = nil
}

func (r *clientResponse) handle() (err error) {
	if DEBUG_PRINT { //debug
		fmt.Println("handling")
	}
	r.parent.clmu.RLock()
	call, ok := r.parent.pendingCalls[r.Id]
	r.parent.clmu.RUnlock()
	if !ok {
		if DEBUG_PRINT { //debug
			fmt.Printf("Id not found %s\n", r.Id)
			fmt.Println(r.parent.pendingCalls)
		}
		return fmt.Errorf("Response not recognized: %n\n", r.Id)
	}
	if r.Result == nil {
		if DEBUG_PRINT { //debug
			fmt.Println("err r.Result = nil")
		}
		return errors.New("nil exit")
	}

	if DEBUG_PRINT { //debug
		fmt.Printf("r.result: %s\n", r.Result)
		fmt.Printf("call.Reply %s\n", call.Reply)
	}
	err = json.Unmarshal(*r.Result, call.Reply)
	//set request as done
	call.Done <- call
	return
	//call.Reply = json.Unmarshal(r.Result, call.Reply)
}

type Call struct {
	ServiceMethod string      // The name of the service and method to call.
	Args          interface{} // The argument to the function (*struct).
	Reply         interface{} // The reply from the function (*struct).
	Error         error       // After completion, the error status.
	Done          chan *Call  // Strobes when call is complete.
	Seq           uint64
	parrent       *JsonRPC
}

func (j *JsonRPC) Call(serviceMethod string, params interface{}, result interface{}) error {
	call := new(Call)
	call.ServiceMethod = serviceMethod
	call.Args = params
	call.Reply = result
	call.parrent = j

	var done chan *Call

	if done == nil {
		done = make(chan *Call, 10) // buffered.
	} else {
		// If caller passes done != nil, it must arrange that
		// done has enough buffer for the number of simultaneous
		// RPCs that will be using that channel.  If the channel
		// is totally unbuffered, it's best not to run at all.
		if cap(done) == 0 {
			log.Panic("rpc: done channel is unbuffered")
		}
	}
	call.Done = done

	j.clmu.Lock()

	j.clientSequenzNum++
	call.Seq = j.clientSequenzNum
	j.pendingCalls[call.Seq] = call

	j.clmu.Unlock()

	err := call.send()
	//wenn fehlerfrei dann warten
	if err == nil {
		<-done
		result = call.Reply
	} else {
		fmt.Println("FEHLER error " + err.Error())
	}
	//clear the call
	j.clmu.Lock()
	delete(j.pendingCalls, call.Seq)
	j.clmu.Unlock()

	return err
}

func (c *Call) send() error {
	req := new(clientRequest)
	req.Id = c.Seq
	req.Method = c.ServiceMethod
	//req.Params
	req.Params[0] = c.Args
	// Encode and send the request.
	//check for nil
	if c == nil {
		fmt.Println("error c = nil")
		return errors.New("c = nil")
	} else if c.parrent == nil {
		fmt.Println("error c.parrent = nil")
		return errors.New("c.parrent = nil")
	} else if c.parrent.enc == nil {
		fmt.Println("error c.parrent.enc = nil")
		return errors.New("c.parrent.enc = nil")
	} else {
		if DEBUG_PRINT {
			fmt.Println("Non of it is null")
		}
	}

	err := c.parrent.enc.Encode(&req)
	return err
}
