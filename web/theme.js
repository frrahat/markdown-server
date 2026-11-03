// Theme handling shared by the blocking <head> script run and app.js.
// A choice is "auto" (follow the OS), "light", or "dark".

const themeStorageKey = "mdserver-theme";

function savedThemeChoice() {
  try {
    const saved = localStorage.getItem(themeStorageKey);
    return saved === "light" || saved === "dark" ? saved : "auto";
  } catch {
    // Storage can be unavailable (e.g. blocked by browser settings).
    return "auto";
  }
}

function saveThemeChoice(choice) {
  try {
    if (choice === "auto") {
      localStorage.removeItem(themeStorageKey);
    } else {
      localStorage.setItem(themeStorageKey, choice);
    }
  } catch {
    // Not persisting is acceptable; the choice still applies to this page.
  }
}

// data-theme drives the page colors in style.css; the highlight stylesheets
// are switched through their media attribute because their rules aren't
// scoped to a theme selector.
function applyTheme(choice) {
  const root = document.documentElement;
  if (choice === "auto") {
    delete root.dataset.theme;
  } else {
    root.dataset.theme = choice;
  }
  const mediaFor = (sheetTheme) =>
    choice === "auto" ? `(prefers-color-scheme: ${sheetTheme})` : choice === sheetTheme ? "all" : "not all";
  document.getElementById("chroma-light").media = mediaFor("light");
  document.getElementById("chroma-dark").media = mediaFor("dark");
}

function isDarkThemeActive() {
  const explicitTheme = document.documentElement.dataset.theme;
  return explicitTheme ? explicitTheme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
}

applyTheme(savedThemeChoice());
