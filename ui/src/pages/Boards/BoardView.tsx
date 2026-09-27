import Board from "./Board/Board";
import Sidebar from "./Board/Sidebar";

export default function BoardView() {
  return (
    <div className="flex h-[calc(100vh-3rem)] min-w-0">
      <Sidebar />
      <Board />
    </div>
  );
}
