export function createSidebarController({
  isControllerPage,
  isChatPage,
  onLayoutChange,
  approvalDetails,
}) {
  const sidebarToggleBtn = document.getElementById("sidebarToggleBtn");
  const sidebarBackdrop = document.getElementById("sidebarBackdrop");
  const mobileMedia = window.matchMedia("(max-width: 900px)");
  let mobileKeyboardOpen = false;

  function isMobileViewport() {
    return mobileMedia.matches;
  }

  function toggleSidebar(open) {
    if (!sidebarToggleBtn || !sidebarBackdrop) return;
    const nextOpen = typeof open === "boolean" ? open : !document.body.classList.contains("sidebar-open");
    document.body.classList.toggle("sidebar-open", nextOpen);
    sidebarToggleBtn.setAttribute("aria-expanded", String(nextOpen));
    sidebarBackdrop.hidden = !nextOpen;
    setTimeout(() => onLayoutChange(), 0);
  }

  function closeSidebarOnMobile() {
    if (isMobileViewport()) toggleSidebar(false);
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
    toggleSidebar(false);
    if (sidebarToggleBtn) sidebarToggleBtn.addEventListener("click", () => toggleSidebar());
    if (sidebarBackdrop) sidebarBackdrop.addEventListener("click", () => toggleSidebar(false));

    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && document.body.classList.contains("sidebar-open")) {
        toggleSidebar(false);
      }
    });

    mobileMedia.addEventListener("change", () => {
      if (!isMobileViewport()) toggleSidebar(false);
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
  }

  return {
    mount,
    isMobileViewport,
    toggleSidebar,
    closeSidebarOnMobile,
  };
}
