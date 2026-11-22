// "Recently opened" and "Recently updated" sidebar sections. Recently opened
// lives in localStorage, so it survives reloads and is shared by every tab
// on this origin. Recently updated comes from the server's last scan, so it
// changes only when the tree is refreshed. Each section is hidden while it
// has nothing to show.

const maxRecentlyOpened = 5;
const recentlyOpenedStorageKey = "mdserver-recently-opened";

export function installRecentFiles({ openedSection, openedList, updatedSection, updatedList, updatedCount, routeFor, fetchJSON }) {
  let currentPath = null;

  function loadOpenedPaths() {
    try {
      const stored = JSON.parse(localStorage.getItem(recentlyOpenedStorageKey));
      return Array.isArray(stored) ? stored.filter((path) => typeof path === "string") : [];
    } catch {
      return [];
    }
  }

  function saveOpenedPaths(paths) {
    try {
      localStorage.setItem(recentlyOpenedStorageKey, JSON.stringify(paths));
    } catch {
      // Not persisting is acceptable.
    }
  }

  // Plain links, so middle-click or Ctrl-click opens the file in a new tab.
  // The hash change opens it in this tab.
  function createFileLink(path, title) {
    const item = document.createElement("li");
    const link = document.createElement("a");
    link.href = routeFor(path);
    link.dataset.path = path;
    if (title) link.title = title;
    const slash = path.lastIndexOf("/");
    const name = document.createElement("span");
    name.className = "result-name";
    name.textContent = path.slice(slash + 1);
    link.append(name);
    if (slash >= 0) {
      const dir = document.createElement("span");
      dir.className = "result-dir";
      dir.textContent = path.slice(0, slash);
      link.append(dir);
    }
    item.append(link);
    return item;
  }

  function markCurrent(list) {
    for (const link of list.querySelectorAll("a")) {
      if (link.dataset.path === currentPath) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    }
  }

  function renderOpened() {
    const paths = loadOpenedPaths();
    openedList.replaceChildren(...paths.map((path) => createFileLink(path)));
    openedSection.hidden = paths.length === 0;
    markCurrent(openedList);
  }

  // Another tab opening a file updates the list here too.
  window.addEventListener("storage", (event) => {
    if (event.key === recentlyOpenedStorageKey || event.key === null) renderOpened();
  });

  renderOpened();

  return {
    docOpened(path) {
      currentPath = path;
      const paths = [path, ...loadOpenedPaths().filter((openedPath) => openedPath !== path)];
      saveOpenedPaths(paths.slice(0, maxRecentlyOpened));
      renderOpened();
      markCurrent(updatedList);
    },

    // Drops a path that failed to open. Otherwise a deleted or renamed file
    // would stay in the list for good.
    docFailed(path) {
      currentPath = null;
      saveOpenedPaths(loadOpenedPaths().filter((openedPath) => openedPath !== path));
      renderOpened();
      markCurrent(updatedList);
    },

    async refreshUpdated() {
      let response;
      try {
        response = await fetchJSON("/api/recent-files");
      } catch {
        // Keep showing the previous list. The section is a convenience, so a
        // failure here isn't worth a status message.
        return;
      }
      updatedList.replaceChildren(...response.files.map((file) =>
        createFileLink(file.path, `Modified ${new Date(file.modifiedAt).toLocaleTimeString()}`)));
      updatedCount.textContent = response.total > response.files.length
        ? `(${response.files.length} of ${response.total})`
        : `(${response.total})`;
      updatedSection.hidden = response.files.length === 0;
      markCurrent(updatedList);
    },
  };
}
