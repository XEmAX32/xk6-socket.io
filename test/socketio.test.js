import { check } from "k6";
import { io } from "k6/x/socketio";
import { sleep } from "k6";

export const options = {
  thresholds: {
    checks: ["rate==1"],
  },
};

export default function () {

  io("http://localhost:4000", {}, (socket) => {
    let connected = false;

    socket.on("message", (m) => {
      console.log("RAW:", m);

      if ((m === "40" || m.startsWith("40"))) {
        connected = true;
        console.log("SIO connected, emitting...");
        socket.emit("hello", { from: "k6", t: Date.now() });
      }
    });
  });

  sleep(5);

}
