export function createMessageBinder(el) {
  return (message, isError = false) => {
    if (!el) return;
    el.textContent = message || "";
    el.classList.toggle("error", Boolean(isError));
  };
}
