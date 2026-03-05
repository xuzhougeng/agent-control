(() => {
  class Terminal {
    constructor() {
      this.cols = 120;
      this.rows = 30;
      this._dataHandlers = [];
      this._container = null;
      this.unicode = { activeVersion: "" };
    }

    loadAddon(addon) {
      this._addon = addon;
    }

    open(container) {
      this._container = container;
      if (this._container) {
        this._container.textContent = "";
      }
    }

    onData(handler) {
      this._dataHandlers.push(handler);
      return {
        dispose() {},
      };
    }

    write(data, callback) {
      const text = typeof data === "string" ? data : new TextDecoder().decode(data);
      if (this._container) {
        this._container.textContent += text;
      }
      if (typeof callback === "function") {
        callback();
      }
    }

    reset() {
      if (this._container) {
        this._container.textContent = "";
      }
    }

    scrollToBottom() {}
    scrollPages() {}
  }

  class FitAddon {
    fit() {}
  }

  window.Terminal = Terminal;
  window.FitAddon = { FitAddon };
})();
