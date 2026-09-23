import { data, redirect } from "react-router";

// For loaders: turn a failed API response into something the router handles
// (a login redirect, or a status the route's ErrorPage can describe) instead
// of letting `res.json()` choke on an error body.
export function throwIfNotOk(res: Response) {
  if (res.status === 401) {
    throw redirect("/login");
  }
  if (!res.ok) {
    throw data(null, { status: res.status, statusText: res.statusText });
  }
}
