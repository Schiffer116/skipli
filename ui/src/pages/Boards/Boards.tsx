import { Outlet } from "react-router";
import Header from "./Header";

export default function Boards() {
  return (
    <div className="flex flex-col min-h-screen bg-background">
      <Header />
      <Outlet />
    </div>
  );
}
