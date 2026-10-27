// The browser's own notifications (Notification API). The permission is asked for only when the person
// turns the setting on (NotifyRows.jsx), never on entering the site.

export const notifySupported = () => typeof window !== "undefined" && "Notification" in window;

// "granted" | "denied" | "default" | "unsupported"
export function notifyPermission() {
  return notifySupported() ? window.Notification.permission : "unsupported";
}

// Asks the browser once (it does not ask again after an answer). Call it from a click.
export async function askNotifyPermission() {
  if (!notifySupported()) return "unsupported";
  if (window.Notification.permission !== "default") return window.Notification.permission;
  try {
    return await window.Notification.requestPermission();
  } catch {
    return window.Notification.permission;
  }
}

// Shows a system notice; pressing it brings the tab forward and goes to `href` (a #hash address).
// False when nothing could be shown.
export function showSystemNotice(text, href, title = "Dyanis Chess") {
  if (notifyPermission() !== "granted") return false;
  try {
    const n = new window.Notification(title, { body: text, tag: "dyanis-corr" });
    n.onclick = () => {
      try {
        window.focus();
        if (href) window.location.hash = href;
      } finally {
        n.close();
      }
    };
    return true;
  } catch {
    return false;
  }
}
