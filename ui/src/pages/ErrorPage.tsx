import { useEffect } from "react";
import { isRouteErrorResponse, Link, useRouteError } from "react-router";
import { CircleAlert, CloudOff, SearchX, ShieldAlert } from "lucide-react";

import { NOT_OFFLINE } from "./Boards/loaders";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

type ErrorCopy = {
  status?: number;
  title: string;
  message: string;
  Icon: typeof CircleAlert;
  retryable: boolean;
};

// fetch rejects with a TypeError when a request never got an answer; each
// browser words it differently.
const isUnreachable = (error: unknown) =>
  error instanceof TypeError &&
  /fetch|network|load failed/i.test(error.message);

function describe(error: unknown): ErrorCopy {
  if (isRouteErrorResponse(error)) {
    switch (error.status) {
      case 404:
        return {
          status: 404,
          title: "Page not found",
          message:
            "The page or board you're looking for doesn't exist, or it may have been deleted.",
          Icon: SearchX,
          retryable: false,
        };
      case 403:
        return {
          status: 403,
          title: "You don't have access",
          message:
            "You aren't a member of this board. Ask its owner to invite you.",
          Icon: ShieldAlert,
          retryable: false,
        };
      case NOT_OFFLINE:
        return error.data === "board"
          ? {
              title: "Not available offline",
              message:
                "This board hasn't been opened on this device, so it isn't saved here. Boards you open while online stay available offline.",
              Icon: CloudOff,
              retryable: true,
            }
          : {
              title: "You're offline",
              message:
                "Sign in once while online to use Skipli offline on this device.",
              Icon: CloudOff,
              retryable: true,
            };
      default:
        return {
          status: error.status,
          title: "Something went wrong",
          message:
            error.status >= 500
              ? "The server couldn't handle this request. Please try again in a moment."
              : "This request couldn't be completed.",
          Icon: CircleAlert,
          retryable: true,
        };
    }
  }

  if (isUnreachable(error)) {
    return {
      title: "Couldn't reach Skipli",
      message: "Check your connection and try again.",
      Icon: CloudOff,
      retryable: true,
    };
  }

  return {
    title: "Something went wrong",
    message: "An unexpected error occurred. Please try again.",
    Icon: CircleAlert,
    retryable: true,
  };
}

function errorDetails(error: unknown) {
  if (isRouteErrorResponse(error)) {
    return `${error.status} ${error.statusText}\n${
      typeof error.data === "string" ? error.data : JSON.stringify(error.data)
    }`;
  }
  if (error instanceof Error) {
    return error.stack ?? error.message;
  }
  return String(error);
}

// `fullPage` is for the root route, where no layout (and so no top nav) has
// rendered; nested inside the /boards layout the nav is already on screen.
export default function ErrorPage({
  fullPage = false,
}: {
  fullPage?: boolean;
}) {
  const error = useRouteError();
  const { status, title, message, Icon, retryable } = describe(error);

  useEffect(() => {
    document.title = `${title} · Skipli`;
    return () => {
      document.title = "Skipli";
    };
  }, [title]);

  useEffect(() => {
    // Expected HTTP errors (404, 403, not available offline) and a lost
    // connection aren't bugs; anything else is.
    if (isRouteErrorResponse(error)) {
      if (error.status >= 500 && error.status !== NOT_OFFLINE) {
        console.error(error);
      }
    } else if (!isUnreachable(error)) {
      console.error(error);
    }
  }, [error]);

  return (
    <main
      className={`flex flex-1 justify-center items-start px-4 py-16 ${
        fullPage ? "min-h-screen bg-background border-t-[3rem] border-nav" : ""
      }`}
    >
      <Card
        role="alert"
        className="w-full max-w-lg px-8 py-8 gap-4 border-t-4 border-t-destructive"
      >
        <div className="flex items-start gap-4">
          <Icon className="size-8 shrink-0 text-destructive" aria-hidden />
          <div className="flex flex-col gap-1">
            {status && (
              <p className="text-sm font-bold text-muted-foreground">
                Error {status}
              </p>
            )}
            <h1 className="text-2xl font-bold">{title}</h1>
            <p className="text-muted-foreground">{message}</p>
          </div>
        </div>

        <div className="flex gap-2 pl-12">
          <Button asChild>
            <Link to="/boards">Go to your boards</Link>
          </Button>
          {retryable && (
            <Button variant="outline" onClick={() => window.location.reload()}>
              Try again
            </Button>
          )}
        </div>

        {import.meta.env.DEV && (
          <details className="pl-12 text-sm">
            <summary className="cursor-pointer text-link">
              Error details (development only)
            </summary>
            <pre className="mt-2 max-h-64 overflow-auto rounded-md bg-muted p-3 text-xs whitespace-pre-wrap">
              {errorDetails(error)}
            </pre>
          </details>
        )}
      </Card>
    </main>
  );
}
