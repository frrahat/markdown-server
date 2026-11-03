// Fuzzy file filter in the sidebar. Matching runs on the server against the
// in-memory index (GET /api/files); this module renders the ranked results
// and handles the keyboard. While the filter has text, the results list
// replaces the tree.

const resultLimit = 100;
// Short enough to feel instant, long enough to skip requests for keystrokes
// typed in a burst.
const debounceMs = 60;

export function installFileFilter({ filterInput, resultsList, summary, treeRoot, fetchJSON, openFile }) {
  let debounceTimer = 0;
  let inFlight = null;
  let results = [];
  let activeIndex = -1;

  const isActive = () => filterInput.value.trim() !== "";

  function showTree(showIt) {
    treeRoot.hidden = !showIt;
    resultsList.hidden = showIt;
    summary.hidden = showIt;
    filterInput.setAttribute("aria-expanded", String(!showIt));
  }

  async function search() {
    const query = filterInput.value;
    if (!isActive()) {
      inFlight?.abort();
      results = [];
      activeIndex = -1;
      resultsList.replaceChildren();
      filterInput.removeAttribute("aria-activedescendant");
      showTree(true);
      return;
    }
    inFlight?.abort();
    const controller = new AbortController();
    inFlight = controller;
    let response;
    try {
      response = await fetchJSON(`/api/files?q=${encodeURIComponent(query)}&limit=${resultLimit}`, { signal: controller.signal });
    } catch (error) {
      if (error.name === "AbortError") return;
      summary.textContent = `Filter failed: ${error.message}`;
      showTree(false);
      return;
    }
    if (controller !== inFlight) return;
    results = response.files;
    activeIndex = results.length > 0 ? 0 : -1;
    renderResults(response.total);
    showTree(false);
  }

  function scheduleSearch() {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      debounceTimer = 0;
      search();
    }, debounceMs);
  }

  // Returns `text` as a fragment with matched characters wrapped in <mark>.
  // matchedPositions holds code-point offsets into the full path, and offset
  // is where `text` starts within it.
  function highlighted(text, offset, matchedPositions) {
    const fragment = document.createDocumentFragment();
    let plainRun = "";
    Array.from(text).forEach((character, index) => {
      if (matchedPositions.has(offset + index)) {
        if (plainRun) fragment.append(plainRun);
        plainRun = "";
        const mark = document.createElement("mark");
        mark.textContent = character;
        fragment.append(mark);
      } else {
        plainRun += character;
      }
    });
    if (plainRun) fragment.append(plainRun);
    return fragment;
  }

  function renderResults(total) {
    summary.textContent = results.length === 0
      ? "No matching files"
      : total > results.length ? `Top ${results.length} of ${total} matches` : `${total} ${total === 1 ? "match" : "matches"}`;
    resultsList.replaceChildren(...results.map((result, index) => {
      const pathCharacters = Array.from(result.path);
      const nameStart = pathCharacters.lastIndexOf("/") + 1;
      const matchedPositions = new Set(result.positions);

      const option = document.createElement("li");
      option.id = `filter-option-${index}`;
      option.setAttribute("role", "option");
      option.dataset.path = result.path;
      const name = document.createElement("span");
      name.className = "result-name";
      name.append(highlighted(pathCharacters.slice(nameStart).join(""), nameStart, matchedPositions));
      option.append(name);
      if (nameStart > 0) {
        const dir = document.createElement("span");
        dir.className = "result-dir";
        dir.append(highlighted(pathCharacters.slice(0, nameStart - 1).join(""), 0, matchedPositions));
        option.append(dir);
      }
      return option;
    }));
    updateActiveOption();
  }

  function updateActiveOption() {
    resultsList.querySelectorAll('[role="option"]').forEach((option, index) => {
      option.setAttribute("aria-selected", String(index === activeIndex));
    });
    const activeOption = resultsList.children[activeIndex];
    if (activeOption) {
      filterInput.setAttribute("aria-activedescendant", activeOption.id);
      activeOption.scrollIntoView({ block: "nearest" });
    } else {
      filterInput.removeAttribute("aria-activedescendant");
    }
  }

  function moveActive(step) {
    if (results.length === 0) return;
    activeIndex = (activeIndex + step + results.length) % results.length;
    updateActiveOption();
  }

  filterInput.addEventListener("input", scheduleSearch);

  filterInput.addEventListener("keydown", (event) => {
    const down = event.key === "ArrowDown" || (event.ctrlKey && event.key === "n");
    const up = event.key === "ArrowUp" || (event.ctrlKey && event.key === "p");
    if (down || up) {
      event.preventDefault();
      moveActive(down ? 1 : -1);
    } else if (event.key === "Enter") {
      event.preventDefault();
      // Run a pending debounced search first. Otherwise Enter opens the top
      // result for an older, shorter query.
      if (debounceTimer) {
        clearTimeout(debounceTimer);
        debounceTimer = 0;
        search().then(() => results[activeIndex] && openFile(results[activeIndex].path));
      } else if (results[activeIndex]) {
        openFile(results[activeIndex].path);
      }
    } else if (event.key === "Escape") {
      event.preventDefault();
      if (isActive()) {
        filterInput.value = "";
        search();
      } else {
        filterInput.blur();
      }
    }
  });

  resultsList.addEventListener("click", (event) => {
    const option = event.target.closest('[role="option"]');
    if (option) openFile(option.dataset.path);
  });

  document.addEventListener("keydown", (event) => {
    if (!event.ctrlKey || event.key !== "p" || event.altKey || event.metaKey) return;
    if (event.target === filterInput) return;
    const target = event.target;
    if (target instanceof HTMLElement && (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))) return;
    event.preventDefault();
    filterInput.focus();
    filterInput.select();
  });

  return {
    // Re-runs the current filter, e.g. after the index was refreshed.
    refresh() {
      if (isActive()) search();
    },
  };
}
