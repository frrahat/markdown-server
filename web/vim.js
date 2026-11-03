// Vim-style keys for reading: scrolling, in-document find, and a help dialog.

const lineScrollPixels = 60;
// How long after "g" a second "g" still counts as "gg".
const keySequenceTimeoutMs = 800;
const findHighlightName = "find-match";
const currentFindHighlightName = "find-current";

export function installVimKeys({ contentPane, docArticle, treeRoot, findBar, findInput, helpDialog, showStatus }) {
  const supportsHighlights = typeof CSS !== "undefined" && "highlights" in CSS;
  let pendingG = false;
  let pendingGTimer = 0;

  // Find state. lastQuery survives document changes, like vim's last search
  // pattern, so "n" works again after opening another file.
  let lastQuery = "";
  let matchRanges = [];
  let currentMatchIndex = -1;
  let matchesAreStale = true;

  function scrollContent(deltaPixels) {
    contentPane.scrollBy({ top: deltaPixels });
  }

  const halfPage = () => Math.round(contentPane.clientHeight / 2);

  function isTypingTarget(element) {
    return element instanceof HTMLElement &&
      (element.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(element.tagName));
  }

  // ---- Find ----

  // Collects the document's text nodes into one string with an offset map,
  // so a match can span element boundaries: highlighted code splits
  // "fmt.Println" across several spans.
  function collectText() {
    const textNodes = [];
    const nodeStarts = [];
    let fullText = "";
    const walker = document.createTreeWalker(docArticle, NodeFilter.SHOW_TEXT, {
      acceptNode: (node) => node.parentElement?.closest("svg, style, script")
        ? NodeFilter.FILTER_REJECT
        : NodeFilter.FILTER_ACCEPT,
    });
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      textNodes.push(node);
      nodeStarts.push(fullText.length);
      fullText += node.data;
    }
    return { textNodes, nodeStarts, fullText };
  }

  function positionAt(textIndex, { textNodes, nodeStarts }) {
    // Binary search for the last node starting at or before textIndex.
    let low = 0;
    let high = nodeStarts.length - 1;
    while (low < high) {
      const middle = Math.ceil((low + high) / 2);
      if (nodeStarts[middle] <= textIndex) low = middle;
      else high = middle - 1;
    }
    return { node: textNodes[low], offset: textIndex - nodeStarts[low] };
  }

  // Smartcase: case-insensitive unless the query has an uppercase letter.
  function findMatches(query) {
    if (!query) return [];
    const collected = collectText();
    let haystack = collected.fullText;
    let needle = query;
    if (query === query.toLowerCase()) {
      const lowered = haystack.toLowerCase();
      // Lowercasing can change the length of some characters (e.g. "İ"),
      // which would misalign offsets; fall back to case-sensitive then.
      if (lowered.length === haystack.length) haystack = lowered;
    }
    const ranges = [];
    for (let index = haystack.indexOf(needle); index !== -1; index = haystack.indexOf(needle, index + needle.length)) {
      const start = positionAt(index, collected);
      const end = positionAt(index + needle.length - 1, collected);
      const range = document.createRange();
      range.setStart(start.node, start.offset);
      range.setEnd(end.node, end.offset + 1);
      ranges.push(range);
    }
    return ranges;
  }

  function paintHighlights() {
    if (!supportsHighlights) return;
    CSS.highlights.set(findHighlightName, new Highlight(...matchRanges));
    const current = matchRanges[currentMatchIndex];
    if (current) CSS.highlights.set(currentFindHighlightName, new Highlight(current));
    else CSS.highlights.delete(currentFindHighlightName);
  }

  function clearHighlights() {
    if (!supportsHighlights) return;
    CSS.highlights.delete(findHighlightName);
    CSS.highlights.delete(currentFindHighlightName);
  }

  function revealMatch(range) {
    // Centres vertically in the document and scrolls any horizontally
    // scrolling ancestor (a wide table or code block) to the match.
    range.startContainer.parentElement?.scrollIntoView({ block: "center", inline: "nearest" });
    if (!supportsHighlights) {
      const selection = getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
    }
  }

  function reportMatchPosition() {
    if (matchRanges.length === 0) {
      showStatus(lastQuery ? `Pattern not found: ${lastQuery}` : "", Boolean(lastQuery));
    } else {
      showStatus(`/${lastQuery}  ${currentMatchIndex + 1} of ${matchRanges.length}`);
    }
  }

  // The first match at or below the top of the visible area, so a search
  // starts from where the reader is rather than from the top.
  function firstMatchFromViewport() {
    const viewportTop = contentPane.getBoundingClientRect().top;
    const index = matchRanges.findIndex((range) => range.getBoundingClientRect().bottom >= viewportTop);
    return index === -1 ? 0 : index;
  }

  function runSearch(query) {
    lastQuery = query;
    matchRanges = findMatches(query);
    matchesAreStale = false;
    currentMatchIndex = matchRanges.length > 0 ? firstMatchFromViewport() : -1;
    paintHighlights();
    if (currentMatchIndex >= 0) revealMatch(matchRanges[currentMatchIndex]);
    reportMatchPosition();
  }

  function stepMatch(direction) {
    if (!lastQuery) return;
    if (matchesAreStale) {
      runSearch(lastQuery);
      return;
    }
    if (matchRanges.length === 0) {
      reportMatchPosition();
      return;
    }
    currentMatchIndex = (currentMatchIndex + direction + matchRanges.length) % matchRanges.length;
    paintHighlights();
    revealMatch(matchRanges[currentMatchIndex]);
    reportMatchPosition();
  }

  function openFindBar() {
    findBar.hidden = false;
    findInput.value = "";
    findInput.focus();
  }

  function closeFindBar({ keepMatches }) {
    findBar.hidden = true;
    if (!keepMatches) {
      matchRanges = [];
      currentMatchIndex = -1;
      clearHighlights();
      showStatus("");
    }
    contentPane.focus({ preventScroll: true });
  }

  findInput.addEventListener("input", () => runSearch(findInput.value));
  findInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      closeFindBar({ keepMatches: true });
    } else if (event.key === "Escape") {
      event.preventDefault();
      closeFindBar({ keepMatches: false });
    }
  });

  // ---- Key dispatch ----

  document.addEventListener("keydown", (event) => {
    if (event.defaultPrevented || isTypingTarget(event.target) || helpDialog.open) return;
    if (event.altKey || event.metaKey) return;

    if (event.ctrlKey) {
      // Ctrl-d / Ctrl-u, vim's half-page scrolls.
      if (event.key === "d" || event.key === "u") {
        event.preventDefault();
        scrollContent(event.key === "d" ? halfPage() : -halfPage());
      }
      return;
    }

    // In the tree, j/k/h/l are handled as arrow keys by the tree itself.
    if (treeRoot.contains(event.target) && ["j", "k", "h", "l"].includes(event.key)) return;

    const wasPendingG = pendingG;
    pendingG = false;
    clearTimeout(pendingGTimer);

    switch (event.key) {
      case "j": scrollContent(lineScrollPixels); break;
      case "k": scrollContent(-lineScrollPixels); break;
      case "d": scrollContent(halfPage()); break;
      case "u": scrollContent(-halfPage()); break;
      case "G": contentPane.scrollTop = contentPane.scrollHeight; break;
      case "g":
        if (wasPendingG) {
          contentPane.scrollTop = 0;
        } else {
          pendingG = true;
          pendingGTimer = setTimeout(() => { pendingG = false; }, keySequenceTimeoutMs);
        }
        break;
      case "/": openFindBar(); break;
      case "n": stepMatch(1); break;
      case "N": stepMatch(-1); break;
      case "?": helpDialog.showModal(); break;
      case "Escape":
        if (matchRanges.length > 0) closeFindBar({ keepMatches: false });
        return;
      default:
        return;
    }
    event.preventDefault();
  });

  return {
    // Called after the document content changes; old ranges point at
    // removed nodes. The last query is kept for "n".
    documentChanged() {
      matchRanges = [];
      currentMatchIndex = -1;
      matchesAreStale = true;
      clearHighlights();
    },
  };
}
