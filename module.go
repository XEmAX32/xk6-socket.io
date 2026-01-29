package socketio

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/modules"
)

const engineIOVersion = 4

type rootModule struct{}

func (*rootModule) NewModuleInstance(vu modules.VU) modules.Instance {
	runtime := vu.Runtime()

	// require("k6/ws")
	reqValue := runtime.Get("require")
	requireFunction, ok := sobek.AssertFunction(reqValue)
	if !ok {
		panic(runtime.ToValue(`require() not available in init stage`))
	}

	wsModuleValue, err := requireFunction(sobek.Undefined(), runtime.ToValue("k6/ws"))
	if err != nil {
		panic(err)
	}

	wsModuleObj := wsModuleValue.ToObject(runtime)
	connectFunction := requireMethod(runtime, wsModuleObj, "connect")

	return &module{
		vu:        vu,
		wsConnect: connectFunction,
	}
}

type module struct {
	vu        modules.VU
	wsConnect sobek.Callable
}

func (m *module) Exports() modules.Exports {
	return modules.Exports{
		Named: map[string]any{
			"io": m.io,
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
	Timeout   int            `json:"timeout"`
}

var EngineIOCodes = struct {
	Open    string
	Close   string
	Ping    string
	Pong    string
	Message string
	Upgrade string
	Noop    string
}{
	Open:    "0",
	Close:   "1",
	Ping:    "2",
	Pong:    "3",
	Message: "4",
	Upgrade: "5",
	Noop:    "6",
}

var SocketIOCodes = struct {
	Connect     string
	Disconnect  string
	Event       string
	Ack         string
	Error       string
	BinaryEvent string
	BinaryAck   string
}{
	Connect:     "0",
	Disconnect:  "1",
	Event:       "2",
	Ack:         "3",
	Error:       "4",
	BinaryEvent: "5",
	BinaryAck:   "6",
}

func (m *module) io(host string, optionsVal sobek.Value, handler sobek.Value) (sobek.Value, error) {
	runtime := m.vu.Runtime()

	var options Options
	if optionsVal != nil && !sobek.IsUndefined(optionsVal) && !sobek.IsNull(optionsVal) {
		if err := runtime.ExportTo(optionsVal, &options); err != nil {
			return nil, fmt.Errorf("invalid options: %w", err)
		}
	}

	if options.Path == "" {
		options.Path = "/socket.io/"
	}
	if options.Namespace == "" {
		options.Namespace = "/"
	}
	if options.Params == nil {
		options.Params = map[string]any{}
	}
	if options.Timeout <= 0 {
		options.Timeout = 10
	}

	websocketURL, err := buildSocketIOWSURL(host, options)
	if err != nil {
		return nil, err
	}

	var handlerFunction sobek.Callable
	if handler != nil && !sobek.IsUndefined(handler) && !sobek.IsNull(handler) {
		_handlerFunction, ok := sobek.AssertFunction(handler)
		if !ok {
			return nil, fmt.Errorf("handler must be a function")
		}

		// if _, herr := hfn(sobek.Undefined(), socket); herr != nil { panic(herr) }
		handlerFunction = _handlerFunction
	}

	// // require("k6/ws")
	// reqValue := runtime.Get("require")
	// requireFunction, ok := sobek.AssertFunction(reqValue)
	// if !ok { return nil, fmt.Errorf("require() not available") }

	// wsModuleValue, err := requireFunction(sobek.Undefined(), runtime.ToValue("k6/ws"))
	// if err != nil { return nil, err }
	// wsModuleObj := wsModuleValue.ToObject(runtime)
	// connectFunction := requireMethod(runtime, wsModuleObj, "connect")

	callback := runtime.ToValue(func(callbackContext sobek.FunctionCall) sobek.Value {
		var connected = false
		var connectionEstablished = false
		var pendingEmits []func()
		var callbackHandlers = map[string][]sobek.Callable{}

		socketValue := callbackContext.Argument(0)
		socketObject := socketValue.ToObject(runtime)

		onCallbackFunction := requireMethod(runtime, socketObject, "on")
		sendFunction := requireMethod(runtime, socketObject, "send")

		jsonObject := runtime.Get("JSON").ToObject(runtime)
		jsonStringifyFunction := requireMethod(runtime, jsonObject, "stringify")

		dispatch := func(event string, payload any) {
			if list, ok := callbackHandlers[event]; ok {
				for _, handler := range list {
					_, err = handler(sobek.Undefined(), runtime.ToValue(payload))
					if err != nil {
						fmt.Println("event dispatch error")
					}
				}
			}
		}

		emitFunction := func(event string, args []sobek.Value) {
			send := func() {
				array := runtime.NewArray()
				_ = array.Set("0", runtime.ToValue(event))
				for i, arg := range args {
					_ = array.Set(strconv.Itoa(i+1), arg)
				}

				str, _ := jsonStringifyFunction(sobek.Undefined(), array)
				packet := EngineIOCodes.Message + SocketIOCodes.Event + str.String()

				if _, err := sendFunction(socketValue, runtime.ToValue(packet)); err != nil {
					panic(err)
				}
			}

			if !connected {
				pendingEmits = append(pendingEmits, send)
				return
			}

			send()
		}

		wrapper := runtime.NewObject()
		err = wrapper.SetPrototype(socketObject)
		if err != nil {
			fmt.Println("error while setting up prototype")
		}

		// inject emit method
		err = wrapper.Set("emit", runtime.ToValue(func(emitContext sobek.FunctionCall) sobek.Value {
			if len(emitContext.Arguments) == 0 {
				panic(runtime.ToValue("emit(event, ...args): missing event"))
			}
			event := emitContext.Argument(0).String()

			var args []sobek.Value
			if len(emitContext.Arguments) > 1 {
				args = emitContext.Arguments[1:]
			}

			emitFunction(event, args)
			return sobek.Undefined()
		}))

		if err != nil {
			fmt.Println("error while adding emit to socket prototype")
		}

		// inject send method
		err = wrapper.Set("send", runtime.ToValue(func(sendContext sobek.FunctionCall) sobek.Value {
			if len(sendContext.Arguments) == 0 {
				panic(runtime.ToValue("send(data): missing data"))
			}

			emitFunction("message", []sobek.Value{sendContext.Argument(0)})
			return sobek.Undefined()
		}))

		if err != nil {
			fmt.Println("error while adding send to socket prototype")
		}

		err = wrapper.Set("on", runtime.ToValue(func(eventHandlerContext sobek.FunctionCall) sobek.Value {
			if len(eventHandlerContext.Arguments) == 0 {
				panic(runtime.ToValue("on(event, handler): missing event"))
			}

			eventType := eventHandlerContext.Argument(0).String()
			handlerValue := eventHandlerContext.Argument(1)
			var handlerFunction sobek.Callable
			if handlerValue != nil && !sobek.IsUndefined(handlerValue) && !sobek.IsNull(handlerValue) {
				_handlerFunction, ok := sobek.AssertFunction(handlerValue)
				if !ok {
					panic(runtime.ToValue("on(event, handler): handler must be a function"))
				}

				handlerFunction = _handlerFunction
			}

			if eventType == "connect" {
				callbackHandlers["connect"] = append(callbackHandlers["connect"], handlerFunction)
				return sobek.Undefined()
			}

			if eventType == "disconnect" {
				callbackHandlers["disconnect"] = append(callbackHandlers["disconnect"], handlerFunction)
				return sobek.Undefined()
			}

			if eventType == "error" {
				callbackHandlers["error"] = append(callbackHandlers["error"], handlerFunction)
				return sobek.Undefined()
			}

			if _, err := onCallbackFunction(socketValue, runtime.ToValue("message"), runtime.ToValue(func(msgHandlerContext sobek.FunctionCall) sobek.Value {
				msg := msgHandlerContext.Argument(0).String()

				if strings.HasPrefix(msg, EngineIOCodes.Message+SocketIOCodes.Event) {
					trimmed := strings.TrimPrefix(msg, EngineIOCodes.Message+SocketIOCodes.Event)
					event, data, _ := extractEvent(trimmed)

					if eventType == event {
						if _, err := handlerFunction(sobek.Undefined(), runtime.ToValue(data)); err != nil {
							panic(err)
						}
					}
				}

				return sobek.Undefined()
			})); err != nil {
				panic(err)
			}

			return sobek.Undefined()
		}))

		if err != nil {
			fmt.Println("error while adding on to socket prototype")
		}

		msgHandler := runtime.ToValue(func(msgHandlerContext sobek.FunctionCall) sobek.Value {
			msg := msgHandlerContext.Argument(0).String()

			// Engine.IO ping -> pong
			if msg == EngineIOCodes.Ping {
				if _, err := sendFunction(socketValue, runtime.ToValue(EngineIOCodes.Pong)); err != nil {
					panic(err)
				}

				return sobek.Undefined()
			}

			if strings.HasPrefix(msg, EngineIOCodes.Message+SocketIOCodes.Connect) {
				connected = true
				connectionEstablished = true

				// fmt.Println("pendings", pendingEmits)

				for _, fn := range pendingEmits {
					//fmt.Println("going through pending")
					fn()
				}
				pendingEmits = nil

				dispatch("connect", nil)

				return sobek.Undefined()
			}

			if msg == EngineIOCodes.Close || msg == EngineIOCodes.Message+SocketIOCodes.Disconnect {
				connected = false

				dispatch("disconnect", nil)

				callbackHandlers = map[string][]sobek.Callable{}
				pendingEmits = nil

				closeFn := requireMethod(runtime, socketObject, "close")
				_, err = closeFn(socketObject)

				if err != nil {
					fmt.Println("error while closing ws")
				}

				return sobek.Undefined()
			}

			// Handle Socket.IO error messages
			if strings.HasPrefix(msg, EngineIOCodes.Message+SocketIOCodes.Error) {
				trimmed := strings.TrimPrefix(msg, EngineIOCodes.Message+SocketIOCodes.Error)

				errorObj := runtime.NewObject()
				_ = errorObj.Set("message", runtime.ToValue(trimmed))
				_ = errorObj.Set("type", runtime.ToValue("socketio"))

				dispatch("error", errorObj)

				return sobek.Undefined()
			}

			if strings.HasPrefix(msg, EngineIOCodes.Open) {
				if connected {
					return sobek.Undefined()
				}
				//fmt.Println("going through, ", connected, msg)

				packet := EngineIOCodes.Message + SocketIOCodes.Connect

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
				//fmt.Printf("here %s", packet)

				if _, err := sendFunction(socketValue, runtime.ToValue(packet)); err != nil {
					panic(err)
				}

				return sobek.Undefined()
			}

			return sobek.Undefined()
		})

		if _, err := onCallbackFunction(socketValue, runtime.ToValue("message"), msgHandler); err != nil {
			panic(err)
		}

		// Subscribe to WebSocket error event
		errorHandler := runtime.ToValue(func(errorHandlerContext sobek.FunctionCall) sobek.Value {
			errorMsg := errorHandlerContext.Argument(0)

			// Dispatch error to Socket.IO error handlers
			dispatch("error", errorMsg)

			return sobek.Undefined()
		})

		if _, err := onCallbackFunction(socketValue, runtime.ToValue("error"), errorHandler); err != nil {
			panic(err)
		}

		// Connection timeout mechanism
		go func() {
			timeoutDuration := time.Duration(options.Timeout) * time.Second
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			timeoutTimer := time.After(timeoutDuration)

			for {
				select {
				case <-timeoutTimer:
					// Timeout reached
					if !connectionEstablished {
						errorObj := runtime.NewObject()
						_ = errorObj.Set("message", runtime.ToValue("Connection timeout: failed to establish connection"))
						_ = errorObj.Set("type", runtime.ToValue("timeout"))
						dispatch("error", errorObj)
					}
					return
				case <-ticker.C:
					// Check if connection established
					if connectionEstablished {
						return
					}
				}
			}
		}()

		// run handler
		if handlerFunction != nil {
			if _, err := handlerFunction(sobek.Undefined(), wrapper); err != nil {
				panic(err)
			}
		}

		return sobek.Undefined()
	})

	res, err := m.wsConnect(
		sobek.Undefined(),
		runtime.ToValue(websocketURL),
		runtime.ToValue(options.Params),
		callback,
	)

	if err != nil {
		return nil, err
	}

	resObj := res.ToObject(runtime)
	status := resObj.Get("status")

	if status != nil && status.ToInteger() >= 400 {
		return nil, fmt.Errorf(
			"socket.io connect failed: %d (%s)",
			status.ToInteger(),
			websocketURL,
		)
	}

	return res, nil
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
	if err != nil {
		return "", err
	}

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

func extractEvent(msg string) (string, any, error) {
	var arr []any

	if err := json.Unmarshal([]byte(msg), &arr); err != nil {
		return "", nil, err
	}

	event, ok := arr[0].(string)
	if !ok {
		return "", nil, fmt.Errorf("event is not a string")
	}

	var data any
	if len(arr) > 2 {
		data = arr[1:]
	} else if len(arr) > 1 {
		data = arr[1]
	}

	return event, data, nil
}
