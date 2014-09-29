package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

var errMissingParams = errors.New("jsonrpc: request body missing params")

type serverRequest struct {
	Method string           //`json:"method"`
	Result *json.RawMessage //`json:"result"`
	Params *json.RawMessage //`json:"params"`
	Id     *json.RawMessage //`json:"id"`
	parent *JsonRPC
}

func (r *serverRequest) reset() {
	r.Method = ""
	r.Params = nil
	r.Id = nil
}

/*
Parse Request to retrieve Service and Method
*/
func (r *serverRequest) parseRequestHeader() (service *service, mtype *methodType, err error) {
	//(service *service, mtype *methodType, req *Request, keepReading bool, err error)
	dot := strings.LastIndex(r.Method, ".")

	if dot < 0 {
		err = errors.New("rpc: service/method request ill-formed: " + r.Method)
		return
	}

	serviceName := r.Method[:dot]
	methodName := r.Method[dot+1:]

	// Look up the request.
	r.parent.mu.RLock()
	service = r.parent.serviceMap[serviceName]
	r.parent.mu.RUnlock()
	if service == nil {
		err = errors.New("rpc: can't find service " + r.Method)
		return
	}
	mtype = service.method[methodName]
	if mtype == nil {
		err = errors.New("rpc: can't find method " + r.Method)
	}
	return

}

func (s *serverRequest) parseRequestBody(x interface{}) error {
	if x == nil {
		return nil
	}
	if s.Params == nil {
		return errMissingParams
	}
	// JSON params is array value.
	// RPC params is struct.
	// Unmarshal into array containing struct for now.
	// Should think about making RPC more general.
	var params [1]interface{}
	params[0] = x
	return json.Unmarshal(*s.Params, &params)
}

func (s *serverRequest) parseRequest() (service *service, mtype *methodType, argv, replyv reflect.Value, err error) {
	service, mtype, err = s.parseRequestHeader()

	if err != nil {
		return
	}
	//Decode kram
	// Decode the argument value.
	argIsValue := false // if true, need to indirect before calling.
	if mtype.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(mtype.ArgType.Elem())
	} else {
		argv = reflect.New(mtype.ArgType)
		argIsValue = true
	} // argv guaranteed to be a pointer now.

	//todo
	if err = s.parseRequestBody(argv.Interface()); err != nil {
		return
	}
	if argIsValue {
		argv = argv.Elem()
	}
	replyv = reflect.New(mtype.ReplyType.Elem())
	return
}

func (r *serverRequest) handle() {
	if DEBUG_PRINT { //debug
		fmt.Println("handle Request")
	}
	service, mtype, argv, replyv, err := r.parseRequest()
	//service, mtype, req, argv, replyv, keepReading, err := server.readRequest(codec)
	if err != nil {
		if err != io.EOF {
			log.Println("rpc:", err)
		}
		// send a response if we actually managed to read a header.
		//send error
		if r.Id != nil {
			r.sendResponse(invalidRequest, err.Error())
			//server.freeRequest(req)
		}
		return
	}
	go r.call(service, mtype, argv, replyv)
	//go service.call(server, sending, mtype, req, argv, replyv, codec)
	if DEBUG_PRINT { //debug
		fmt.Println("DO CALL")
	}
}

// A value sent as a placeholder for the server's response value when the server
// receives an invalid request. It is never decoded by the client since the Response
// contains an error when it is used.
var invalidRequest = struct{}{}

//todo add mutex for sending
func (r *serverRequest) sendResponse(reply interface{}, errmsg string) error {
	if DEBUG_PRINT { //debug
		fmt.Println("Send Response")
	}
	resp := new(serverResponse)

	if errmsg != "" {
		resp.Error = errmsg
		reply = nil //invalidRequest
	}

	resp.Id = r.Id

	if errmsg == "" {
		resp.Error = nil
		resp.Result = reply
	} else {
		resp.Error = errmsg
	}
	return r.parent.enc.Encode(resp)

}

type serverResponse struct {
	Id     *json.RawMessage `json:"id"`
	Result interface{}      `json:"result"`
	Error  interface{}      `json:"error"`
}

func (r *serverRequest) call(service *service, mtype *methodType, argv, replyv reflect.Value) {
	//	mtype.Lock()
	//	mtype.numCalls++
	//	mtype.Unlock()
	function := mtype.method.Func
	// Invoke the method, providing a new value for the reply.
	returnValues := function.Call([]reflect.Value{service.rcvr, argv, replyv})
	// The return value for the method is an error.
	errInter := returnValues[0].Interface()
	errmsg := ""
	if errInter != nil {
		errmsg = errInter.(error).Error()
	}
	r.sendResponse(replyv.Interface(), errmsg)
	//server.freeRequest(req)
}

// suitableMethods returns suitable Rpc methods of typ, it will report
// error using log if reportErr is true.
func suitableMethods(typ reflect.Type, reportErr bool) map[string]*methodType {
	methods := make(map[string]*methodType)
	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		mtype := method.Type
		mname := method.Name
		// Method must be exported.
		if method.PkgPath != "" {
			continue
		}
		// Method needs three ins: receiver, *args, *reply.
		if mtype.NumIn() != 3 {
			if reportErr {
				log.Println("method", mname, "has wrong number of ins:", mtype.NumIn())
			}
			continue
		}
		// First arg need not be a pointer.
		argType := mtype.In(1)
		if !isExportedOrBuiltinType(argType) {
			if reportErr {
				log.Println(mname, "argument type not exported:", argType)
			}
			continue
		}
		// Second arg must be a pointer.
		replyType := mtype.In(2)
		if replyType.Kind() != reflect.Ptr {
			if reportErr {
				log.Println("method", mname, "reply type not a pointer:", replyType)
			}
			continue
		}
		// Reply type must be exported.
		if !isExportedOrBuiltinType(replyType) {
			if reportErr {
				log.Println("method", mname, "reply type not exported:", replyType)
			}
			continue
		}
		// Method needs one out.
		if mtype.NumOut() != 1 {
			if reportErr {
				log.Println("method", mname, "has wrong number of outs:", mtype.NumOut())
			}
			continue
		}
		// The return type of the method must be error.
		if returnType := mtype.Out(0); returnType != typeOfError {
			if reportErr {
				log.Println("method", mname, "returns", returnType.String(), "not error")
			}
			continue
		}
		methods[mname] = &methodType{method: method, ArgType: argType, ReplyType: replyType}
	}
	return methods
}

// Is this type exported or a builtin?
func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// PkgPath will be non-empty even for an exported type,
	// so we need to check the type name as well.
	return isExported(t.Name()) || t.PkgPath() == ""
}

// Is this an exported - upper case - name?
func isExported(name string) bool {
	rune, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(rune)
}
