// Light or dark, chosen per device in the account dialog, following the
// OS by default. index.html applies it before the first paint with the
// same key and rules, so keep the two in step.
export type Theme = "system" | "light" | "dark";

const THEME_KEY = "skipli:theme";
const darkQuery = window.matchMedia("(prefers-color-scheme: dark)");

export function readTheme(): Theme {
  try {
    const theme = localStorage.getItem(THEME_KEY);
    return theme === "light" || theme === "dark" ? theme : "system";
  } catch {
    return "system";
  }
}

function applyTheme(theme: Theme) {
  const dark = theme === "dark" || (theme === "system" && darkQuery.matches);
  const root = document.documentElement;
  root.classList.toggle("dark", dark);
  // The browser's toolbar matches the nav bar.
  const nav = getComputedStyle(root).getPropertyValue("--nav").trim();
  if (nav) {
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute("content", nav);
  }
}

export function setTheme(theme: Theme) {
  try {
    if (theme === "system") localStorage.removeItem(THEME_KEY);
    else localStorage.setItem(THEME_KEY, theme);
  } catch {
    // Not remembered, but still applied for now.
  }
  applyTheme(theme);
}

// Keeps up with the OS switching between light and dark, and with the
// choice changing in another tab.
export function watchTheme() {
  applyTheme(readTheme());
  darkQuery.addEventListener("change", () => applyTheme(readTheme()));
  window.addEventListener("storage", (e) => {
    if (e.key === THEME_KEY || e.key === null) applyTheme(readTheme());
  });
}
