import { type Socket } from "socket.io";

export default function registerSocket(socket: Socket) {
  socket.on("create card", (card) => {
    socket.broadcast.emit("create card", card);
  });

  socket.on("update card", (card) => {
    socket.broadcast.emit("update card", card);
  });

  socket.on("delete card", (id) => {
    socket.broadcast.emit("delete card", id);
  });

  socket.on("move card", (activeCardIndex, overCardIndex) => {
    socket.broadcast.emit("move card", activeCardIndex, overCardIndex);
  });

  socket.on("create task", (task) => {
    socket.broadcast.emit("create task", task);
  });

  socket.on("update task", (task) => {
    socket.broadcast.emit("update task", task);
  });

  socket.on("delete task", (id) => {
    socket.broadcast.emit("delete task", id);
  });

  socket.on("move task", (task, oldCardId, newTaskIndex) => {
    socket.broadcast.emit("move task", task, oldCardId, newTaskIndex);
  });
}
