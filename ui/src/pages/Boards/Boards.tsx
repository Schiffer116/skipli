import { Outlet, redirect } from "react-router";
import Header from "./Header";

export async function boardsLoader() {
  const email = await fetch("/api/auth/email", {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });

  if (email.status === 401) {
    return redirect("/login");
  }

  return await email.json();
}

export default function Boards() {
  return (
    <div className="flex flex-col min-h-screen min-w-full w-fit bg-background">
      <Header />
      <Outlet />
    </div>
  );
}
