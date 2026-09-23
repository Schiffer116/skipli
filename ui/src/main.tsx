import { createRoot } from "react-dom/client";
import "./index.css";
import { createBrowserRouter, data, Navigate } from "react-router";
import { RouterProvider } from "react-router/dom";
import io from "socket.io-client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import Boards, { boardsLoader } from "@/pages/Boards";
import BoardList, { boardListLoader } from "@/pages/Boards/BoardList";
import Login from "@/pages/Login";
import Verify from "@/pages/Verify";
import AcceptInvite from "@/pages/AcceptInvite";
import ErrorPage from "@/pages/ErrorPage";
import BoardView, { boardViewLoader } from "@/pages/Boards/BoardView";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: Infinity,
      refetchOnWindowFocus: false,
    },
  },
});

export const socket = io("ws://localhost:3000");

declare global {
  interface Window {
    __TANSTACK_QUERY_CLIENT__: import("@tanstack/query-core").QueryClient;
  }
}

// This code is for all users
window.__TANSTACK_QUERY_CLIENT__ = queryClient;

const router = createBrowserRouter([
  {
    // Pathless root so any error not handled deeper, including ones thrown
    // while rendering pages outside /boards, still gets a real error page.
    errorElement: <ErrorPage fullPage />,
    children: [
      {
        path: "/",
        Component: () => <Navigate to="/boards" replace />,
      },
      {
        path: "/boards",
        Component: Boards,
        loader: boardsLoader,
        children: [
          {
            // Errors here render inside the Boards layout, keeping the nav.
            errorElement: <ErrorPage />,
            children: [
              {
                index: true,
                Component: BoardList,
                loader: boardListLoader,
              },
              {
                path: ":boardId",
                Component: BoardView,
                loader: boardViewLoader,
              },
            ],
          },
        ],
      },
      {
        path: "/login",
        Component: Login,
      },
      {
        path: "/verify",
        Component: Verify,
      },
      {
        path: "/boards/:boardId/invite/accept",
        Component: AcceptInvite,
      },
      {
        path: "*",
        loader: () => {
          throw data(null, { status: 404, statusText: "Not Found" });
        },
      },
    ],
  },
]);
createRoot(document.getElementById("root")!).render(
  <QueryClientProvider client={queryClient}>
    <RouterProvider router={router} />
  </QueryClientProvider>,
);
