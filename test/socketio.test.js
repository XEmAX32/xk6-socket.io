import { check } from "k6";
import { io } from "k6/x/socketio";
import { sleep } from "k6";

export const options = {
  thresholds: {
    checks: ["rate==1"],
  },
};

export default function () {

  io("http://localhost:4000", { namespace: "chat", auth: { token: "3123" }, params: { headers: { token: "123"}} }, (socket) => {
    let connected = false;

    socket.on("connect", () => {
      console.log('yo')
      socket.emit("message", { test: "test" })
    })

    socket.on("disconnect", () => {
      console.log('closed')
    })

    socket.on("error", (err) => {
      console.log('error', err)
    })

    socket.on("message", (msg) => {
      console.log('getting msg ', msg)
    })

  });


}
