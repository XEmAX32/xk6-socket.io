const { Server } = require("socket.io");

const io = new Server(4000, { cors: { origin: "*" } });

io.on("connection", (socket) => {
  console.log("connected:", socket.id);

  socket.on("message", (data) => {
    console.log("message:", data);
    socket.send("server got your message");
  });

  socket.on("hello", (data) => {
    console.log("hello:", data);
    socket.emit("hello_back", { ok: true, got: data });
  });
});

console.log("listening on http://localhost:4000");