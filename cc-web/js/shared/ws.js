export function createWSClient({
  getToken,
  shouldConnect,
  onOpen,
  onMessage,
  onError,
  setStatus,
}) {
  let ws = null;

  function send(msg) {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      console.warn("[ws] not connected, dropping", msg.type);
      return false;
    }
    ws.send(JSON.stringify(msg));
    return true;
  }

  function connect() {
    if (!shouldConnect()) return;
    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    const url = `${scheme}://${window.location.host}/ws/client?token=${encodeURIComponent(getToken() || "")}`;
    ws = new WebSocket(url);

    ws.onopen = () => {
      setStatus(true);
      onOpen({ send });
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        onMessage(msg, { send });
      } catch (err) {
        console.error("[ws] parse error", err);
      }
    };

    ws.onerror = (err) => {
      setStatus(false);
      if (onError) onError(err);
    };

    ws.onclose = () => {
      setStatus(false);
      setTimeout(reconnect, 1000);
    };
  }

  function reconnect() {
    if (!shouldConnect()) return;
    if (ws) {
      ws.onclose = null;
      ws.close();
    }
    connect();
  }

  return {
    connect,
    reconnect,
    send,
    getSocket: () => ws,
    isOpen: () => Boolean(ws && ws.readyState === WebSocket.OPEN),
  };
}

export function setWSBadge(connected) {
  const el = document.getElementById("wsStatus");
  if (!el) return;
  if (connected) {
    el.textContent = "WS: connected";
    el.className = "badge badge-online";
  } else {
    el.textContent = "WS: disconnected";
    el.className = "badge badge-offline";
  }
}
