const { Server } = require("socket.io");

const io = new Server(4000, { cors: { origin: "*" } });


io.on("connection", (socket) => {
  console.log("connected:", socket.id);

  // socket.emit("hello_back", { ok: true, got: "ciao" });

  socket.on("message", (data) => {
    console.log("message:", data);
    socket.send("server got your message");
  });

  socket.on("hello", (data) => {
    console.log("hello:", data);
    socket.emit("hello_back", { ok: true, got: data });
  });

  // setTimeout(() => {
  //   console.log('disc')
  //   socket.disconnect(true);
  // }, 100000);
});

const chat = io.of("/chat");


chat.use((socket, next) => {
  const { token } = socket.handshake.auth; 

  if (!token) return next(new Error("unauthorized"));

  if (token !== "123") return next(new Error("unauthorized"));

  // you can attach user info
  socket.user = { id: "user1" };

  next();
});

chat.on("connection", (socket) => {
  console.log("connected to /chat:", socket.id);

  socket.on("chat", (data, ack) => {
    console.log("chat(/chat):", data);
    // socket.emit("message", { text: "server got your message in /chat" });
    if (typeof ack === "function") {
      ack({ ok: true });
    }
  });

  // emit to everyone in /chat
  // chat.emit("message", { text: "Hello everyone in /chat namespace" });

});

console.log("listening on http://localhost:4000");