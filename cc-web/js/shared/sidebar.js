export function createSidebarController({
  isControllerPage,
  isChatPage,
  onLayoutChange,
  approvalDetails,
}) {
  const sidebarToggleBtn = document.getElementById("sidebarToggleBtn");
  const leftDrawerToggleBtn = document.getElementById("leftDrawerToggleBtn");
  const rightDrawerToggleBtn = document.getElementById("rightDrawerToggleBtn");
  const leftDrawer = document.getElementById("sidebar");
  const rightDrawer = document.getElementById("contextSidebar");
  const sidebarBackdrop = document.getElementById("sidebarBackdrop");
  const mobileMedia = window.matchMedia("(max-width: 900px)");
  const DESKTOP_DRAWER_MARGIN = 18;
  const DESKTOP_DRAWER_MIN_HEIGHT = 260;
  const DESKTOP_DRAWER_DEFAULT_TOP = 18;
  let mobileKeyboardOpen = false;

  function isMobileViewport() {
    return mobileMedia.matches;
  }

  function isDesktopViewport() {
    return !mobileMedia.matches;
  }

  function isLeftDrawerOpen() {
    return document.body.classList.contains("left-drawer-open");
  }

  function isRightDrawerOpen() {
    return document.body.classList.contains("right-drawer-open");
  }

  function syncBackdrop() {
    if (!sidebarBackdrop) return;
    const visible = isLeftDrawerOpen() || isRightDrawerOpen();
    sidebarBackdrop.hidden = !visible;
  }

  function syncButtons() {
    const leftOpen = isLeftDrawerOpen();
    const rightOpen = isRightDrawerOpen();
    if (sidebarToggleBtn) sidebarToggleBtn.setAttribute("aria-expanded", String(leftOpen));
    if (leftDrawerToggleBtn) leftDrawerToggleBtn.setAttribute("aria-expanded", String(leftOpen));
    if (rightDrawerToggleBtn) rightDrawerToggleBtn.setAttribute("aria-expanded", String(rightOpen));
  }

  function persistDesktopState(side, open) {
    if (!isDesktopViewport()) return;
    localStorage.setItem(side === "right" ? "workspace_right_drawer_open" : "workspace_left_drawer_open", open ? "1" : "0");
  }

  function getDesktopDrawerStorageKey(side) {
    return side === "right" ? "workspace_right_drawer_top" : "workspace_left_drawer_top";
  }

  function getDesktopDrawerElements(side) {
    return side === "right"
      ? { drawer: rightDrawer, peek: rightDrawerToggleBtn }
      : { drawer: leftDrawer, peek: leftDrawerToggleBtn };
  }

  function getDrawerTopLimit(drawer) {
    const parent = drawer?.parentElement;
    if (!drawer || !parent) {
      return { min: DESKTOP_DRAWER_DEFAULT_TOP, max: DESKTOP_DRAWER_DEFAULT_TOP, height: 0 };
    }
    const parentRect = parent.getBoundingClientRect();
    const parentHeight = Math.max(
      parent.clientHeight || 0,
      parentRect.height || 0,
      window.innerHeight - parentRect.top - DESKTOP_DRAWER_MARGIN
    );
    const max = Math.max(DESKTOP_DRAWER_DEFAULT_TOP, parentHeight - DESKTOP_DRAWER_MIN_HEIGHT - DESKTOP_DRAWER_MARGIN);
    return { min: DESKTOP_DRAWER_DEFAULT_TOP, max, height: parentHeight };
  }

  function applyDrawerOffset(side, requestedTop) {
    if (!isDesktopViewport()) return;
    const { drawer, peek } = getDesktopDrawerElements(side);
    if (!drawer) return;
    const { min, max, height } = getDrawerTopLimit(drawer);
    const top = Math.min(Math.max(Number.isFinite(requestedTop) ? requestedTop : DESKTOP_DRAWER_DEFAULT_TOP, min), max);
    const nextHeight = Math.max(DESKTOP_DRAWER_MIN_HEIGHT, height - top - DESKTOP_DRAWER_MARGIN);
    drawer.style.top = `${top}px`;
    drawer.style.bottom = "auto";
    drawer.style.height = `${nextHeight}px`;
    if (peek) {
      peek.style.top = `${Math.max(28, top + 10)}px`;
    }
    localStorage.setItem(getDesktopDrawerStorageKey(side), String(Math.round(top)));
  }

  function restoreDrawerOffset(side) {
    const saved = Number(localStorage.getItem(getDesktopDrawerStorageKey(side)));
    applyDrawerOffset(side, Number.isFinite(saved) ? saved : DESKTOP_DRAWER_DEFAULT_TOP);
  }

  function setDrawerState(side, open) {
    const className = side === "right" ? "right-drawer-open" : "left-drawer-open";
    if (isMobileViewport() && open) {
      document.body.classList.remove(side === "right" ? "left-drawer-open" : "right-drawer-open");
    }
    document.body.classList.toggle(className, Boolean(open));
    persistDesktopState(side, Boolean(open));
    syncButtons();
    syncBackdrop();
    setTimeout(() => onLayoutChange(), 0);
  }

  function toggleDrawer(side, open) {
    const current = side === "right" ? isRightDrawerOpen() : isLeftDrawerOpen();
    setDrawerState(side, typeof open === "boolean" ? open : !current);
  }

  function closeAllDrawers() {
    document.body.classList.remove("left-drawer-open", "right-drawer-open", "sidebar-open");
    if (isDesktopViewport()) {
      localStorage.setItem("workspace_left_drawer_open", "0");
      localStorage.setItem("workspace_right_drawer_open", "0");
    }
    syncButtons();
    syncBackdrop();
    setTimeout(() => onLayoutChange(), 0);
  }

  function closeSidebarOnMobile() {
    if (isMobileViewport()) {
      closeAllDrawers();
      return;
    }
    setDrawerState("left", false);
  }

  function initializeApprovalDetails() {
    if (!approvalDetails) return;
    const saved = localStorage.getItem("approval_collapsed");
    if (saved === "1") {
      approvalDetails.removeAttribute("open");
      return;
    }
    if (saved === "0") {
      approvalDetails.setAttribute("open", "");
      return;
    }
    if (isMobileViewport()) {
      approvalDetails.removeAttribute("open");
      return;
    }
    approvalDetails.setAttribute("open", "");
  }

  function applyViewportMode() {
    if (isMobileViewport()) {
      document.body.classList.remove("left-drawer-open", "right-drawer-open", "sidebar-open");
      if (leftDrawer) {
        leftDrawer.style.top = "";
        leftDrawer.style.bottom = "";
        leftDrawer.style.height = "";
      }
      if (rightDrawer) {
        rightDrawer.style.top = "";
        rightDrawer.style.bottom = "";
        rightDrawer.style.height = "";
      }
      if (leftDrawerToggleBtn) leftDrawerToggleBtn.style.top = "";
      if (rightDrawerToggleBtn) rightDrawerToggleBtn.style.top = "";
    } else {
      const leftSaved = localStorage.getItem("workspace_left_drawer_open");
      const rightSaved = localStorage.getItem("workspace_right_drawer_open");
      document.body.classList.toggle("left-drawer-open", leftSaved === "1");
      document.body.classList.toggle("right-drawer-open", rightSaved === "1");
      document.body.classList.remove("sidebar-open");
      restoreDrawerOffset("left");
      restoreDrawerOffset("right");
    }
    syncButtons();
    syncBackdrop();
    setTimeout(() => onLayoutChange(), 0);
  }

  function bindDrawerDrag(side) {
    const { drawer } = getDesktopDrawerElements(side);
    const grip = drawer?.querySelector(`[data-drawer-drag="${side}"]`);
    if (!grip || !drawer) return;
    let dragging = false;

    function startDrag(clientY, pointerId) {
      if (!isDesktopViewport() || dragging) return;
      dragging = true;
      const startTop = drawer.offsetTop;

      const onMove = (moveEvent) => {
        const nextClientY = typeof moveEvent.clientY === "number" ? moveEvent.clientY : clientY;
        const delta = nextClientY - clientY;
        applyDrawerOffset(side, startTop + delta);
        onLayoutChange();
      };

      const onUp = () => {
        dragging = false;
        window.removeEventListener("pointermove", onMove);
        window.removeEventListener("pointerup", onUp);
        window.removeEventListener("pointercancel", onUp);
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
        setTimeout(() => onLayoutChange(), 0);
      };

      if (typeof pointerId === "number") {
        grip.setPointerCapture?.(pointerId);
        window.addEventListener("pointermove", onMove);
        window.addEventListener("pointerup", onUp);
        window.addEventListener("pointercancel", onUp);
      } else {
        window.addEventListener("mousemove", onMove);
        window.addEventListener("mouseup", onUp);
      }
    }

    grip.addEventListener("pointerdown", (event) => {
      if (event.button !== 0) return;
      event.preventDefault();
      startDrag(event.clientY, event.pointerId);
    });

    grip.addEventListener("mousedown", (event) => {
      if (event.button !== 0) return;
      event.preventDefault();
      startDrag(event.clientY);
    });
  }

  function bindPeekDrag(side) {
    const { peek } = getDesktopDrawerElements(side);
    if (!peek) return;

    const MOBILE_STORAGE_KEY = `mobile_peek_${side}_bottom`;
    const DRAG_THRESHOLD = 4;
    const SWIPE_COMMIT = 36;
    let dragging = false;
    let didMove = false;
    let dragAxis = null;
    let startClientY = 0;
    let startClientX = 0;
    let startPos = 0;

    function viewportHeight() {
      return window.visualViewport?.height || window.innerHeight;
    }

    function clampMobileBottom(val) {
      return Math.max(4, Math.min(viewportHeight() - 44, val));
    }

    function getRenderedBottom() {
      const rect = peek.getBoundingClientRect();
      return viewportHeight() - rect.bottom;
    }

    function restoreMobilePosition() {
      if (!isMobileViewport()) return;
      const saved = localStorage.getItem(MOBILE_STORAGE_KEY);
      if (saved !== null) {
        peek.style.bottom = `${clampMobileBottom(Number(saved))}px`;
      }
    }

    peek.addEventListener("pointerdown", (e) => {
      if (e.button !== 0 && e.pointerType === "mouse") return;
      dragging = true;
      didMove = false;
      dragAxis = null;
      startClientY = e.clientY;
      startClientX = e.clientX;
      startPos = isMobileViewport() ? getRenderedBottom() : peek.offsetTop;
      peek.setPointerCapture(e.pointerId);
      e.preventDefault();
    });

    peek.addEventListener("pointermove", (e) => {
      if (!dragging) return;
      const deltaY = startClientY - e.clientY;
      const deltaX = e.clientX - startClientX;

      if (!didMove) {
        if (Math.abs(deltaY) < DRAG_THRESHOLD && Math.abs(deltaX) < DRAG_THRESHOLD) return;
        dragAxis = Math.abs(deltaX) > Math.abs(deltaY) ? "x" : "y";
        didMove = true;
      }

      if (dragAxis === "y") {
        if (isMobileViewport()) {
          peek.style.bottom = `${clampMobileBottom(startPos + deltaY)}px`;
        } else {
          applyDrawerOffset(side, startPos + (e.clientY - startClientY));
          onLayoutChange();
        }
      }
    });

    peek.addEventListener("pointerup", (e) => {
      if (!dragging) return;
      dragging = false;

      if (dragAxis === "y") {
        if (didMove && isMobileViewport()) {
          const bottom = clampMobileBottom(startPos + (startClientY - e.clientY));
          localStorage.setItem(MOBILE_STORAGE_KEY, String(Math.round(bottom)));
        }
      } else if (dragAxis === "x") {
        const deltaX = e.clientX - startClientX;
        if (Math.abs(deltaX) >= SWIPE_COMMIT) {
          const shouldOpen = side === "left" ? deltaX > 0 : deltaX < 0;
          toggleDrawer(side, shouldOpen);
        }
      }
    });

    peek.addEventListener("pointercancel", () => {
      dragging = false;
      didMove = false;
      dragAxis = null;
    });

    peek.addEventListener("click", (e) => {
      if (didMove) {
        didMove = false;
        e.stopImmediatePropagation();
      }
    }, true);

    mobileMedia.addEventListener("change", () => {
      if (isMobileViewport()) restoreMobilePosition();
      else peek.style.bottom = "";
    });

    restoreMobilePosition();
  }

  function handleMobileViewportChange() {
    if (!isControllerPage) return;
    if (!isMobileViewport()) {
      mobileKeyboardOpen = false;
      return;
    }
    const vv = window.visualViewport;
    if (!vv) return;
    const keyboardNowOpen = (window.innerHeight - vv.height) > 120;
    if (keyboardNowOpen && !mobileKeyboardOpen) {
      setTimeout(() => {
        onLayoutChange();
        const terminal = document.getElementById("terminal");
        if (terminal && terminal.scrollIntoView) {
          terminal.scrollIntoView({ behavior: "smooth", block: "end" });
        }
      }, 0);
    } else {
      onLayoutChange();
    }
    mobileKeyboardOpen = keyboardNowOpen;
  }

  function mount() {
    if (!isControllerPage && !isChatPage) return;
    initializeApprovalDetails();
    applyViewportMode();

    if (sidebarToggleBtn) {
      sidebarToggleBtn.addEventListener("click", () => {
        if (isMobileViewport()) {
          toggleDrawer("left");
          return;
        }
        toggleDrawer("left");
      });
    }
    if (leftDrawerToggleBtn) leftDrawerToggleBtn.addEventListener("click", () => toggleDrawer("left"));
    if (rightDrawerToggleBtn) rightDrawerToggleBtn.addEventListener("click", () => toggleDrawer("right"));
    if (sidebarBackdrop) sidebarBackdrop.addEventListener("click", closeAllDrawers);

    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && (isLeftDrawerOpen() || isRightDrawerOpen())) {
        closeAllDrawers();
      }
    });

    mobileMedia.addEventListener("change", () => {
      applyViewportMode();
    });

    window.addEventListener("resize", () => {
      if (!isDesktopViewport()) return;
      restoreDrawerOffset("left");
      restoreDrawerOffset("right");
      onLayoutChange();
    });

    if (window.visualViewport) {
      window.visualViewport.addEventListener("resize", handleMobileViewportChange);
    }

    if (approvalDetails) {
      approvalDetails.addEventListener("toggle", () => {
        localStorage.setItem("approval_collapsed", approvalDetails.open ? "0" : "1");
        setTimeout(() => onLayoutChange(), 0);
      });
    }

    bindDrawerDrag("left");
    bindDrawerDrag("right");
    bindPeekDrag("left");
    bindPeekDrag("right");
  }

  return {
    mount,
    isMobileViewport,
    toggleDrawer,
    closeAllDrawers,
    closeSidebarOnMobile,
  };
}
