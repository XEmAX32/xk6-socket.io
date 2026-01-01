// Package socket.io contains the xk6-socket.io extension.
package socketio

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"strconv"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/modules"
)

const engineIOVersion = 4

type rootModule struct{}

func (*rootModule) NewModuleInstance(vu modules.VU) modules.Instance {
	return &module{vu}
}

type module struct {
	vu modules.VU
}

func (m *module) Exports() modules.Exports {
	return modules.Exports{
		Named: map[string]any{
			"io":  m.io,
		},
	}
}

var _ modules.Module = (*rootModule)(nil)

type Options struct {
	Path      string         `json:"path"`
	Namespace string         `json:"namespace"`
	Auth      map[string]any `json:"auth"`
	Query     map[string]any `json:"query"`
	Params    map[string]any `json:"params"`
}

func (m *module) io(host string, optionsVal sobek.Value, handler sobek.Value) (sobek.Value, error) {
	runtime := m.vu.Runtime()

	var options Options
	if optionsVal != nil && !sobek.IsUndefined(optionsVal) && !sobek.IsNull(optionsVal) {
		if err := runtime.ExportTo(optionsVal, &options); err != nil {
			return nil, fmt.Errorf("invalid options: %w", err)
		}
	}

	if options.Path == "" { options.Path = "/socket.io/" }
	if options.Namespace == "" { options.Namespace = "/" }
	if options.Params == nil { options.Params = map[string]any{} }

	websocketURL, err := buildSocketIOWSURL(host, options)
	if err != nil { return nil, err }

	var handlerFunction sobek.Callable
	if handler != nil && !sobek.IsUndefined(handler) && !sobek.IsNull(handler) {
		_handlerFunction, ok := sobek.AssertFunction(handler)
		if !ok { return nil, fmt.Errorf("handler must be a function") }
	
		// if _, herr := hfn(sobek.Undefined(), socket); herr != nil { panic(herr) }
		handlerFunction = _handlerFunction
	}

	// require("k6/ws")
	reqValue := runtime.Get("require")
	requireFunction, ok := sobek.AssertFunction(reqValue)
	if !ok { return nil, fmt.Errorf("require() not available") }	
	
	wsModuleValue, err := requireFunction(sobek.Undefined(), runtime.ToValue("k6/ws"))
	if err != nil { return nil, err }
	wsModuleObj := wsModuleValue.ToObject(runtime)
	connectFunction := requireMethod(runtime, wsModuleObj, "connect")

	callback := runtime.ToValue(func(callbackContext sobek.FunctionCall) sobek.Value {
		connected := false
	
		socketValue := callbackContext.Argument(0)
		socketObject := socketValue.ToObject(runtime)

		onCallbackFunction := requireMethod(runtime, socketObject, "on")
		sendFunction := requireMethod(runtime, socketObject, "send")

		jsonObject := runtime.Get("JSON").ToObject(runtime)
		jsonStringifyFunction := requireMethod(runtime, jsonObject, "stringify")
		
		emitFunction := func(event string, data sobek.Value) {
			array := runtime.NewArray()
			_ = array.Push(runtime.ToValue(event))
			_ = array.Push(data)

			stringifiedArray, err := jsonStringifyFunction(sobek.Undefined(), array)
			if err != nil { panic(err) }

			packet := "42" + stringifiedArray.String()

			if _, err := sendFunction(socketObject, runtime.ToValue(packet)); err != nil {
				panic(err)
			}
		}

		msgHandler := runtime.ToValue(func(msgHandlerContext sobek.FunctionCall) sobek.Value {
			msg := msgHandlerContext.Argument(0).String()
			
			// Engine.IO ping -> pong
			if msg == "2" {
				_, _ := sendFunction(socketObject, runtime.ToValue("3"))
				return sobek.Undefined()
			}

			if strings.HasPrefix(msg, "0") {
				if connected { return sobek.Undefined() }
				connected = true

				packet := "40"

				// handle namespace
				namespace := options.Namespace
				if namespace != "" && namespace != "/" {
					if !strings.HasPrefix(namespace, "/") {
						namespace = "/" + namespace
					}
					packet = packet + namespace
				}

				// handle authentication
				if options.Auth != nil {
					bearer, _ := json.Marshal(options.Auth)
					if namespace != "" && namespace != "/" {
						packet = packet + "," + string(bearer)
					} else {
						packet = packet + string(bearer)
					}
				}

				_, _ := sendFunction(socketObject, runtime.ToValue(packet))
				return sobek.Undefined()
			}
		}) 

		if _, err := onCallbackFunction(socketObject, runtime.ToValue("message"), msgHandler); err != nil {
			panic(err)
		}

		// inject emit method
		socketObject.Set("emit", runtime.ToValue(func(emitContext sobek.FunctionCall) sobek.Value {
			if len(emitContext.Arguments) == 0 { panic(runtime.ToValue("emit(event, data): missing event")) }
			event := emitContext.Argument(0).String()

			data := sobek.Undefined()
			if len(emitContext.Arguments) > 1 { data = emitContext.Argument(1) }

			emitFunction(event, data)
			return sobek.Undefined()
		}))

		// inject send method
		socketObject.Set("send", runtime.ToValue(func(sendContext sobek.FunctionCall) sobek.Value {
			if len(sendContext.Arguments) == 0 { panic(runtime.ToValue("send(data): missing data")) }

			emitFunction("message", sendContext.Argument(0))
			return sobek.Undefined()
		}))

		// run handler and exit
		if handlerFunction != nil {
			if _, err := handlerFunction(sobek.Undefined(), socketValue); err != nil {
				panic(err)
			} 
		}
		
		return sobek.Undefined()
	})


	return connectFunction(
		sobek.Undefined(),
		runtime.ToValue(websocketURL),
		runtime.ToValue(options.Params),
		callback,
	)
}

func requireMethod(runtime *sobek.Runtime, obj *sobek.Object, name string) sobek.Callable {
	v := obj.Get(name)
	function, ok := sobek.AssertFunction(v)

	if !ok {
		panic(runtime.ToValue(fmt.Sprintf("method %q not found or not callable", name)))
	}
	return function
}

func buildSocketIOWSURL(host string, opts Options) (string, error) {
	_url, err := url.Parse(host)
	if err != nil { return "", err }

	switch _url.Scheme {
		case "http":
			_url.Scheme = "ws"
		case "https":
			_url.Scheme = "wss"
		case "ws", "wss":
		default:
			return "", fmt.Errorf("unsupported scheme: %s", _url.Scheme)
	}

	path := opts.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	_url.Path = path

	_query := _url.Query()
	_query.Set("EIO", strconv.Itoa(engineIOVersion))
	_query.Set("transport", "websocket")

	for index, value := range opts.Query {
		_query.Set(index, fmt.Sprint(value))
	}

	_url.RawQuery = _query.Encode()

	return _url.String(), nil
}