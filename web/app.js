const treeRoot = document.getElementById("tree");
const contentPane = document.getElementById("content");
const docArticle = document.getElementById("doc");
const docPathLabel = document.getElementById("doc-path");
const statusLabel = document.getElementById("status");
const reloadButton = document.getElementById("reload-doc");
const refreshButton = document.getElementById("refresh-tree");

// dirPath -> Promise<entries>. Promises rather than results so two quick
// expands of the same folder share one request.
const childrenCache = new Map();
// path -> <li>, for every tree item currently rendered.
const treeItemsByPath = new Map();

let currentDocPath = null;
// Incremented per doc load so a slow response can't overwrite a newer one.
let latestDocRequest = 0;

if (window.mermaid) {
  window.mermaid.initialize({ startOnLoad: false, securityLevel: "strict" });
}

async function fetchJSON(url, options) {
  const response = await fetch(url, options);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error || `${response.status} ${response.statusText}`);
  }
  return body;
}

function loadChildren(dirPath) {
  if (!childrenCache.has(dirPath)) {
    const request = fetchJSON(`/api/tree?path=${encodeURIComponent(dirPath)}`)
      .then((body) => body.children);
    request.catch(() => childrenCache.delete(dirPath));
    childrenCache.set(dirPath, request);
  }
  return childrenCache.get(dirPath);
}

// ---- Tree ----

function createTreeItem(entry, level) {
  const item = document.createElement("li");
  item.setAttribute("role", "treeitem");
  item.setAttribute("aria-level", String(level));
  item.setAttribute("aria-selected", "false");
  item.tabIndex = -1;
  item.dataset.path = entry.path;
  item.dataset.type = entry.type;

  const row = document.createElement("div");
  row.className = "tree-row";
  row.style.setProperty("--level", String(level));
  const twisty = document.createElement("span");
  twisty.className = "twisty";
  twisty.setAttribute("aria-hidden", "true");
  const label = document.createElement("span");
  label.className = "tree-label";
  label.textContent = entry.name;
  row.append(twisty, label);
  item.append(row);

  if (entry.type === "dir") {
    item.setAttribute("aria-expanded", "false");
  }
  treeItemsByPath.set(entry.path, item);
  return item;
}

function renderChildren(container, entries, level) {
  container.replaceChildren(...entries.map((entry) => createTreeItem(entry, level)));
  ensureTabbableItem();
}

// item -> Promise for its in-progress or finished expand, so callers that
// find a folder already expanding can wait for its children to arrive.
const expandPromises = new WeakMap();

function expandDir(item) {
  if (item.getAttribute("aria-expanded") === "true") {
    return expandPromises.get(item) ?? Promise.resolve();
  }
  item.setAttribute("aria-expanded", "true");
  const expansion = showChildren(item);
  expandPromises.set(item, expansion);
  return expansion;
}

async function showChildren(item) {
  let group = item.querySelector(":scope > ul");
  if (!group) {
    group = document.createElement("ul");
    group.setAttribute("role", "group");
    item.append(group);
    try {
      const entries = await loadChildren(item.dataset.path);
      renderChildren(group, entries, Number(item.getAttribute("aria-level")) + 1);
    } catch (error) {
      group.remove();
      item.setAttribute("aria-expanded", "false");
      showStatus(`Could not load ${item.dataset.path}: ${error.message}`, true);
      return;
    }
  }
  group.hidden = false;
}

function collapseDir(item) {
  item.setAttribute("aria-expanded", "false");
  const group = item.querySelector(":scope > ul");
  if (!group) return;
  // Move focus out first, or it would stay on an item that is now hidden.
  if (group.contains(document.activeElement)) item.focus();
  group.hidden = true;
  for (const hiddenItem of group.querySelectorAll('[tabindex="0"]')) hiddenItem.tabIndex = -1;
  ensureTabbableItem();
}

function toggleDir(item) {
  if (item.getAttribute("aria-expanded") === "true") {
    collapseDir(item);
  } else {
    expandDir(item);
  }
}

function selectTreeItem(item) {
  for (const selected of treeRoot.querySelectorAll('[aria-selected="true"]')) {
    selected.setAttribute("aria-selected", "false");
  }
  if (item) {
    item.setAttribute("aria-selected", "true");
    item.scrollIntoView({ block: "nearest" });
  }
}

// Expands each ancestor folder of filePath (fetching only those folders) and
// selects the file's tree item.
async function revealInTree(filePath) {
  const segments = filePath.split("/");
  for (let depth = 1; depth < segments.length; depth++) {
    const dirItem = treeItemsByPath.get(segments.slice(0, depth).join("/"));
    if (!dirItem) break;
    await expandDir(dirItem);
  }
  selectTreeItem(treeItemsByPath.get(filePath));
}

treeRoot.addEventListener("click", (event) => {
  const item = event.target.closest('[role="treeitem"]');
  if (!item) return;
  if (item.dataset.type === "dir") {
    toggleDir(item);
  } else {
    navigateTo(item.dataset.path);
  }
});

// ---- Keyboard navigation (WAI-ARIA tree pattern) ----
// Roving tabindex: exactly one item is in the tab order, so Tab moves past
// the tree in one step and arrow keys move within it.

function visibleTreeItems() {
  return [...treeRoot.querySelectorAll('[role="treeitem"]')]
    .filter((item) => !item.parentElement.closest("ul[hidden]") && !item.parentElement.hidden);
}

function ensureTabbableItem() {
  if (!treeRoot.querySelector('[role="treeitem"][tabindex="0"]')) {
    const firstItem = treeRoot.querySelector('[role="treeitem"]');
    if (firstItem) firstItem.tabIndex = 0;
  }
}

treeRoot.addEventListener("focusin", (event) => {
  const focusedItem = event.target.closest('[role="treeitem"]');
  if (!focusedItem) return;
  for (const item of treeRoot.querySelectorAll('[role="treeitem"][tabindex="0"]')) {
    if (item !== focusedItem) item.tabIndex = -1;
  }
  focusedItem.tabIndex = 0;
});

function focusTreeItem(item) {
  if (item) item.focus();
}

const treeKeys = new Set(["ArrowDown", "ArrowUp", "Home", "End", "ArrowRight", "ArrowLeft", "Enter", " "]);

treeRoot.addEventListener("keydown", async (event) => {
  const item = event.target.closest('[role="treeitem"]');
  if (!item || event.altKey || event.ctrlKey || event.metaKey || !treeKeys.has(event.key)) return;
  // Before any await: once the handler yields, the browser has already
  // applied the default action (scrolling the sidebar).
  event.preventDefault();
  const isDir = item.dataset.type === "dir";
  const isExpanded = item.getAttribute("aria-expanded") === "true";
  const visibleItems = visibleTreeItems();
  const position = visibleItems.indexOf(item);

  switch (event.key) {
    case "ArrowDown":
      focusTreeItem(visibleItems[position + 1]);
      break;
    case "ArrowUp":
      focusTreeItem(visibleItems[position - 1]);
      break;
    case "Home":
      focusTreeItem(visibleItems[0]);
      break;
    case "End":
      focusTreeItem(visibleItems[visibleItems.length - 1]);
      break;
    case "ArrowRight":
      if (isDir && !isExpanded) {
        expandDir(item);
      } else if (isDir) {
        await expandDir(item);
        focusTreeItem(item.querySelector(':scope > ul > [role="treeitem"]'));
      }
      break;
    case "ArrowLeft":
      if (isDir && isExpanded) {
        collapseDir(item);
      } else {
        focusTreeItem(item.parentElement.closest('[role="treeitem"]'));
      }
      break;
    case "Enter":
    case " ":
      if (isDir) {
        toggleDir(item);
      } else {
        navigateTo(item.dataset.path);
      }
      break;
  }
});

async function loadRoot() {
  try {
    const entries = await loadChildren("");
    renderChildren(treeRoot, entries, 1);
    if (entries.length === 0) {
      showStatus("No markdown files found.");
    }
  } catch (error) {
    showStatus(`Could not load the tree: ${error.message}`, true);
  }
}

// Rescans on the server, then rebuilds the tree, re-expanding every folder
// that was open and still exists. Folders are restored one depth level at a
// time (in parallel within a level), and only if they still appear in their
// parent's new listing, so deleted folders are never requested.
async function refreshTree() {
  refreshButton.disabled = true;
  showStatus("Refreshing…");
  const expandedPaths = [...treeRoot.querySelectorAll('[aria-expanded="true"]')]
    .map((item) => item.dataset.path);
  const sidebarScroll = treeRoot.parentElement.scrollTop;
  try {
    const result = await fetchJSON("/api/refresh", { method: "POST" });
    childrenCache.clear();
    treeItemsByPath.clear();
    renderChildren(treeRoot, await loadChildren(""), 1);

    const pathsByDepth = new Map();
    for (const dirPath of expandedPaths) {
      const depth = dirPath.split("/").length;
      pathsByDepth.set(depth, [...(pathsByDepth.get(depth) ?? []), dirPath]);
    }
    for (const depth of [...pathsByDepth.keys()].sort((a, b) => a - b)) {
      const stillPresent = pathsByDepth.get(depth)
        .map((dirPath) => treeItemsByPath.get(dirPath))
        .filter(Boolean);
      await Promise.all(stillPresent.map(expandDir));
    }

    if (currentDocPath) selectTreeItem(treeItemsByPath.get(currentDocPath));
    treeRoot.parentElement.scrollTop = sidebarScroll;
    showStatus(`Indexed ${result.files} files in ${result.durationMs} ms`);
  } catch (error) {
    showStatus(`Refresh failed: ${error.message}`, true);
  } finally {
    refreshButton.disabled = false;
  }
}

refreshButton.addEventListener("click", refreshTree);
reloadButton.addEventListener("click", () => {
  if (currentDocPath) openDoc(currentDocPath, null, { keepScroll: true });
});

// ---- Routing ----
// Routes look like "#/docs/my%20file.md?anchor=heading-id". Path segments are
// percent-encoded, so a "?" or "#" in a file name can't be mistaken for a
// separator; the server's link rewriter produces the same format.

function routeFor(path, anchor) {
  const encodedPath = path.split("/").map(encodeURIComponent).join("/");
  return `#/${encodedPath}${anchor ? `?anchor=${encodeURIComponent(anchor)}` : ""}`;
}

function parseRoute(hash) {
  if (!hash.startsWith("#/")) return null;
  const routeText = hash.slice(2);
  const queryStart = routeText.indexOf("?");
  const encodedPath = queryStart < 0 ? routeText : routeText.slice(0, queryStart);
  const query = queryStart < 0 ? "" : routeText.slice(queryStart + 1);
  try {
    const path = encodedPath.split("/").map(decodeURIComponent).join("/");
    return { path, anchor: new URLSearchParams(query).get("anchor") };
  } catch {
    return null;
  }
}

function navigateTo(path, anchor) {
  const route = routeFor(path, anchor);
  if (location.hash === route) {
    openDoc(path, anchor);
  } else {
    location.hash = route;
  }
}

function handleRoute() {
  const route = parseRoute(location.hash);
  if (route) {
    openDoc(route.path, route.anchor);
  }
}

window.addEventListener("hashchange", handleRoute);

// ---- Documents ----

async function openDoc(path, anchor, { keepScroll = false } = {}) {
  const requestId = ++latestDocRequest;
  const savedScroll = contentPane.scrollTop;
  showStatus("Loading…");
  let doc;
  try {
    doc = await fetchJSON(`/api/doc?path=${encodeURIComponent(path)}`);
  } catch (error) {
    if (requestId !== latestDocRequest) return;
    currentDocPath = null;
    reloadButton.disabled = true;
    docPathLabel.textContent = path;
    const message = document.createElement("p");
    message.className = "error";
    message.textContent = `Could not open ${path}: ${error.message}`;
    docArticle.replaceChildren(message);
    showStatus("");
    return;
  }
  if (requestId !== latestDocRequest) return;

  docArticle.replaceChildren(prepareDocContent(doc.html, path));
  currentDocPath = path;
  reloadButton.disabled = false;
  docPathLabel.textContent = path;
  document.title = `${path.split("/").pop()} — mdserver`;
  showStatus("");

  await renderDiagrams();
  if (keepScroll) {
    contentPane.scrollTop = savedScroll;
  } else if (anchor) {
    scrollToAnchor(anchor);
  } else {
    contentPane.scrollTop = 0;
  }
  revealInTree(path);
}

// Diagrams are rendered one by one with ids from our own counter: mermaid's
// default ids come from Date.now(), so two diagrams rendered in the same
// millisecond share an id and each other's CSS, and draw on top of each other.
let diagramCounter = 0;

async function renderDiagrams() {
  if (!window.mermaid) return;
  for (const diagramNode of docArticle.querySelectorAll("pre.mermaid")) {
    const diagramSource = diagramNode.textContent;
    try {
      const { svg, bindFunctions } = await window.mermaid.render(`mdserver-diagram-${++diagramCounter}`, diagramSource);
      // mermaid sanitizes its own SVG output in strict security mode.
      diagramNode.innerHTML = svg;
      bindFunctions?.(diagramNode);
    } catch (error) {
      diagramNode.classList.add("diagram-error");
      diagramNode.textContent = `Diagram error: ${error.message}\n\n${diagramSource}`;
    }
  }
}

// Builds the document's DOM from server HTML. The HTML is parsed into an
// inert <template> first, so no image starts loading before relative URLs are
// fixed. The server already rewrote markdown links and images; what is left
// relative here came from raw HTML (e.g. <img src="docs/logo.png"> in a
// README), which the server doesn't rewrite.
function prepareDocContent(html, docPath) {
  const template = document.createElement("template");
  // The one deliberate innerHTML: this HTML was sanitized by the server.
  template.innerHTML = html;
  const docDir = docPath.includes("/") ? docPath.slice(0, docPath.lastIndexOf("/")) : "";
  for (const element of template.content.querySelectorAll("img[src], source[src], a[href]")) {
    const attribute = element.tagName === "A" ? "href" : "src";
    const resolvedPath = resolveRelativeURL(element.getAttribute(attribute), docDir);
    if (resolvedPath === null) continue;
    const encodedPath = resolvedPath.split("/").map(encodeURIComponent).join("/");
    const isMarkdownLink = element.tagName === "A" && /\.(md|markdown)$/i.test(resolvedPath);
    element.setAttribute(attribute, isMarkdownLink ? `#/${encodedPath}` : `/raw/${encodedPath}`);
  }
  return template.content;
}

// Returns the root-relative path a relative URL points to, or null when the
// URL is absolute, root-relative, an anchor, or escapes the root.
function resolveRelativeURL(url, docDir) {
  if (!url || /^([a-z][a-z0-9+.-]*:|\/|#)/i.test(url)) return null;
  const pathPart = url.split(/[?#]/, 1)[0];
  let decodedPath;
  try {
    decodedPath = decodeURIComponent(pathPart);
  } catch {
    return null;
  }
  const segments = docDir ? docDir.split("/") : [];
  for (const segment of decodedPath.split("/")) {
    if (segment === "" || segment === ".") continue;
    if (segment === "..") {
      if (segments.length === 0) return null;
      segments.pop();
    } else {
      segments.push(segment);
    }
  }
  return segments.length > 0 ? segments.join("/") : null;
}

// Looks the id up inside the document only: a heading such as "# Tree" gets
// id="tree", which would otherwise collide with the app's own elements.
function scrollToAnchor(anchorId) {
  const target = docArticle.querySelector(`[id="${CSS.escape(anchorId)}"]`);
  if (target) target.scrollIntoView();
}

// In-document anchors (#heading, footnotes) scroll without touching the hash,
// which is reserved for the route.
docArticle.addEventListener("click", (event) => {
  const link = event.target.closest("a[href]");
  if (!link) return;
  const href = link.getAttribute("href");
  if (href.startsWith("#") && !href.startsWith("#/")) {
    event.preventDefault();
    try {
      scrollToAnchor(decodeURIComponent(href.slice(1)));
    } catch {
      // A malformed escape in a hand-written link: nothing to scroll to.
    }
  }
});

function showStatus(message, isError = false) {
  statusLabel.textContent = message;
  statusLabel.classList.toggle("error", isError);
}

await loadRoot();
handleRoute();
