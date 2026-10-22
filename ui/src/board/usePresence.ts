import { useEffect, useState } from "react";

import { useOffline } from "@/offline/network";
import { useMe } from "@/pages/Boards/favorites";
import { socket, type Person } from "@/utils/socket";

// The other people who have the board open right now: not yourself, and
// nobody while the board's socket is down (offline included).
export default function usePresence(boardId: string): Person[] {
  const myId = useMe().id;
  // A socket can linger a while after the connection is lost.
  const offline = useOffline();
  // The socket may have heard before this was listening.
  const [people, setPeople] = useState(() => socket.presentAt(boardId));
  useEffect(() => {
    setPeople(socket.presentAt(boardId));
    return socket.onPresence((id, people) => {
      if (id === boardId) setPeople(people);
    });
  }, [boardId]);
  if (offline) return [];
  return people.filter((person) => person.id !== myId);
}
