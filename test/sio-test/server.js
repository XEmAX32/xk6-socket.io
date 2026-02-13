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
  const { token } = socket.handshake.auth; // from 40/chat,{"token":...}
console.log('token', socket.handshake.auth, token)
  if (!token) return next(new Error("unauthorized"));

  // TODO: validate token (JWT verify, DB lookup, etc.)
  if (token !== "123") return next(new Error("unauthorized"));

  // you can attach user info
  socket.user = { id: "user1" };

  next();
});

chat.on("connection", (socket) => {
  console.log("connected to /chat:", socket.id);

  socket.on("message", (data) => {
    console.log("message(/chat):", data);
    socket.emit("message", { text: "server got your message in /chat" });
  });

  // emit to everyone in /chat
  // chat.emit("message", { text: "Hello everyone in /chat namespace" });

});

console.log("listening on http://localhost:4000");