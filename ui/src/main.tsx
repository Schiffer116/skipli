import { createRoot } from "react-dom/client";
import "./index.css";
import { createBrowserRouter, data, Navigate } from "react-router";
import { RouterProvider } from "react-router/dom";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import Boards from "@/pages/Boards";
import BoardList from "@/pages/Boards/BoardList";
import Login from "@/pages/Login";
import Verify from "@/pages/Verify";
import AcceptInvite from "@/pages/AcceptInvite";
import ErrorPage from "@/pages/ErrorPage";
import BoardView from "@/pages/Boards/BoardView";
import {
  boardListLoader,
  boardsLoader,
  boardsShouldRevalidate,
  boardViewLoader,
} from "@/pages/Boards/loaders";
import UpdatePrompt from "@/components/UpdatePrompt";
import { Toaster } from "@/components/ui/sonner";
import { forgetBoard } from "@/offline/copy";
import { activeUser } from "@/offline/session";
import { socket } from "@/utils/socket";
import { toast } from "sonner";
import { keepFocusVisible } from "@/utils/keepFocusVisible";
import { watchTheme } from "@/utils/theme";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: Infinity,
      refetchOnWindowFocus: false,
    },
  },
});

declare global {
  interface Window {
    __TANSTACK_QUERY_CLIENT__: import("@tanstack/query-core").QueryClient;
  }
}

keepFocusVisible();
watchTheme();

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
        shouldRevalidate: boardsShouldRevalidate,
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
// A teammate renamed the open board: load its data again.
socket.onUpdated(() => void router.revalidate());

// A teammate deleted the open board: leave it, and forget the device's copy.
socket.onDeleted((boardId) => {
  const userId = activeUser();
  if (userId) void forgetBoard(userId, boardId);
  toast("This board was deleted");
  void router.navigate("/boards");
});

createRoot(document.getElementById("root")!).render(
  <QueryClientProvider client={queryClient}>
    <RouterProvider router={router} />
    <UpdatePrompt />
    <Toaster />
  </QueryClientProvider>,
);
