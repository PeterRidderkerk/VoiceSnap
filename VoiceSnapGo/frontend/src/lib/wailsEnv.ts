/**
 * True when the page runs inside the Wails desktop shell (WebView2 / WKWebView / etc.).
 * In a normal browser there is no Go backend or /wails/runtime bridge.
 */
export function isWailsWebView(): boolean {
  if (typeof window === "undefined") return false;
  try {
    const w = window as Window & {
      chrome?: { webview?: { postMessage?: unknown } };
      webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } };
      wails?: { invoke?: unknown };
    };
    if (w.chrome?.webview?.postMessage) return true;
    if (w.webkit?.messageHandlers?.external?.postMessage) return true;
    if (w.wails?.invoke) return true;
  } catch {
    return false;
  }
  return false;
}
