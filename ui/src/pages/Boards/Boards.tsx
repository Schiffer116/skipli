import { Outlet, redirect } from "react-router";
import Header from "./Header";

export async function boardsLoader() {
  const res = await fetch("/api/auth/email", {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });

  if (res.status === 401) {
    return redirect("/login");
  }

  return await res.json();
}

export default function Boards() {
  return (
    <div className="flex flex-col min-h-screen min-w-full w-fit bg-gradient-to-br from-orange-50 to-orange-100">
      <Header />
      <Outlet />
    </div>
  );
}
