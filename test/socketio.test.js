import { check } from "k6";
import { io } from "k6/x/socketio";

export const options = {
  thresholds: {
    checks: ["rate==1"],
  },
};

export default function () {
  console.log('TEST')
  io("http://localhost:4000", {}, (socket) => {
    console.log('TEST2')
    console.log('socket', socket)
    socket.on("message", (m) => console.log("RAW:", m));

    socket.send("hi from k6");
    socket.emit("hello", { from: "k6", t: Date.now() });
  });

}
