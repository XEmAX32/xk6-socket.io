# xk6-socket.io

Socket.IO client for k6, implemented as an xk6 extension. Use it to drive Socket.IO WebSocket traffic from k6 scripts, including custom events, connect/disconnect hooks, and basic auth/query configuration.

## Features

- Socket.IO over Engine.IO v4 WebSocket transport
- Simple `io()` API that mirrors common Socket.IO usage
- Event handlers for `connect`, `disconnect`, and custom events
- `emit()` and `send()` helpers
- Pass-through options for `k6/ws.connect`

## Requirements

- Go 1.24+
- k6 and xk6

## Install / Build

Build a custom k6 binary with this extension:

```shell
xk6 build --with github.com/xemax32/xk6-socket.io@latest
```

If you are working locally in this repo:

```shell
xk6 build --with github.com/xemax32/xk6-socket.io=.
```

## Quick start

Start a Socket.IO test server (a simple one is provided):

```shell
node test/sio-test/server.js
```

Run a k6 script using the custom binary:

```shell
./k6 run script.js
```

Example script:

```javascript
import { io } from "k6/x/socketio";
import { sleep } from "k6";

export default function () {
  const options = {
    path: "/socket.io/",
    namespace: "/",
    auth: { token: "demo-token" },
    query: { env: "local", user: "vu-1" },
    params: {
      headers: { "x-client": "k6" },
      tags: { scenario: "socketio" },
    },
  };

  io("http://localhost:4000", options, (socket) => {
    socket.on("connect", () => {
      console.log("connected");
      socket.emit("hello", { payload: "hi from k6" });
      socket.send({ type: "data", ts: Date.now() });
    });

    socket.on("message", (msg) => {
      console.log("message", msg);
    });

    socket.on("disconnect", () => {
      console.log("disconnected");
    });
  });
}
```

## API

### `io(host, options?, handler?)`

Connects to a Socket.IO server and returns the underlying `k6/ws.connect` result.

- `host` (string): Base URL such as `http://localhost:4000` or `wss://example.com`.
- `options` (object, optional): Configuration described below.
- `handler` (function, optional): Called with a Socket.IO-like `socket` wrapper.

### Socket wrapper

Inside the handler you can use:

- `socket.on(event, handler)`
- `socket.emit(event, data)`
- `socket.send(data)` (alias for `emit("message", data)`)
- All other `k6/ws` socket methods are available as-is via the wrapper (it preserves the original WebSocket prototype).

Supported built-in events:

- `connect`
- `disconnect`

Custom events are dispatched from `socket.on("your_event", handler)` when messages arrive.

## Options

All options are optional.

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `path` | string | `/socket.io/` | Socket.IO path. |
| `namespace` | string | `/` | Namespace to connect to (e.g. `/chat`). |
| `auth` | object | `null` | Auth payload sent in the connect packet. |
| `query` | object | `{}` | Query parameters appended to the URL. |
| `params` | object | `{}` | Passed directly to `k6/ws.connect` (e.g. headers, tags). |

<!-- ## Development

Run Go tests:

```shell
go test ./...
```

Run the Socket.IO JS test (requires a running server):

```shell
node test/sio-test/server.js
./k6 run test/socketio.test.js
``` -->

## Compatibility and limitations

- WebSocket transport only (no HTTP long-polling).
- Engine.IO protocol v4.
- No reconnection logic.
- ACKs and binary payloads are not implemented.

## TODO

- [ ] Tests
- [ ] Namespaces support (including dynamic namespace handling).
- [ ] Connection state recovery / session ID handling.
- [ ] Authentication middleware / auth payload support.
- [ ] Client/server-side connect_error and error packet handling.
- [ ] ACKs support.
- [ ] Room support
- [ ] Binary attachments framing.

## License

See `LICENSE`.
