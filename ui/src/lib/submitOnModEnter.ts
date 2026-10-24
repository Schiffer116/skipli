import type { KeyboardEvent } from "react";

// Ctrl/Cmd+Enter saves the form the textarea is in; plain Enter keeps
// inserting a line break.
export function submitOnModEnter(e: KeyboardEvent<HTMLTextAreaElement>) {
  if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
    e.preventDefault();
    e.currentTarget.form?.requestSubmit();
  }
}
