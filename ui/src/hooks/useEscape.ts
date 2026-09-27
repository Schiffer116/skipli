import { useEffect, useRef } from "react";

// Calls onEscape when Escape is pressed anywhere on the page while the
// calling component is mounted (e.g. to cancel an open "create" form even
// after focus has left it). Escape inside a dialog is left to the dialog.
export default function useEscape(onEscape: () => void) {
  const callback = useRef(onEscape);

  useEffect(() => {
    callback.current = onEscape;
  });

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented) return;
      if (
        e.target instanceof Element &&
        e.target.closest('[role="dialog"], [role="alertdialog"]')
      ) {
        return;
      }
      callback.current();
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);
}
