/**
 * When the UI is served as a static site (e.g. Netlify), @wailsio/runtime would
 * POST to /wails/runtime (404) before any Svelte code runs. Install a no-network
 * transport so the bundle loads without fetch errors; desktop WebView is unchanged.
 */
import { setTransport, objectNames } from "@wailsio/runtime/runtime.js";
import { isWailsWebView } from "./wailsEnv";

function bindingResult(args: unknown): unknown {
  if (!args || typeof args !== "object") return undefined;
  const a = args as Record<string, unknown>;
  const methodName = a.methodName as string | undefined;
  if (!methodName) return undefined;

  switch (methodName) {
    case "voicesnap/services.AudioService.GetDeviceName":
      return "Default";
    case "voicesnap/services.AudioService.ListInputDevices":
      return [];
    case "voicesnap/services.AudioService.SetDevice":
      return undefined;
    case "voicesnap/services.ConfigService.IsStartupEnabled":
      return false;
    case "voicesnap/services.ConfigService.GetAutoHide":
      return true;
    case "voicesnap/services.ConfigService.GetSoundFeedback":
      return true;
    case "voicesnap/services.ConfigService.SetAutoHide":
    case "voicesnap/services.ConfigService.SetSoundFeedback":
    case "voicesnap/services.ConfigService.SetStartupEnabled":
      return undefined;
    case "voicesnap/services.ConfigService.GetConfig":
      return null;
    case "voicesnap/services.EngineService.ModelExists":
      return true;
    case "voicesnap/services.EngineService.DownloadModel":
      return undefined;
    case "voicesnap/services.HotkeyService.GetHotkeyName":
      return "Ctrl";
    case "voicesnap/services.HotkeyService.GetKeyName":
      return "Ctrl";
    case "voicesnap/services.HotkeyService.StartRecordingHotkey":
    case "voicesnap/services.HotkeyService.StopRecordingHotkey":
    case "voicesnap/services.HotkeyService.SetHotkey":
      return undefined;
    case "voicesnap/services.AppService.GetVersion":
      return "2.1.1";
    case "voicesnap/services.AppService.OpenURL": {
      const rest = a.args as unknown[] | undefined;
      const url = rest?.[0];
      if (typeof url === "string") {
        try {
          window.open(url, "_blank", "noopener,noreferrer");
        } catch {
          /* ignore */
        }
      }
      return undefined;
    }
    case "voicesnap/services.UpdaterService.CheckForUpdate":
      return null;
    case "voicesnap/services.UpdaterService.PerformUpdate":
      return undefined;
    case "voicesnap/services.HistoryService.GetAll":
      return [];
    case "voicesnap/services.HistoryService.GetRetentionDays":
      return 30;
    case "voicesnap/services.HistoryService.SetRetentionDays":
    case "voicesnap/services.HistoryService.Delete":
    case "voicesnap/services.HistoryService.ClearAll":
      return undefined;
    default:
      return undefined;
  }
}

function systemResult(method: number): unknown {
  switch (method) {
    case 0:
      return false;
    case 1:
      return { OS: "web", Arch: "js", Debug: false };
    case 2:
      return {};
    default:
      return undefined;
  }
}

function browserResult(
  objectID: number,
  method: number,
  _windowName: string,
  args: unknown
): Promise<unknown> {
  if (objectID === objectNames.Call && method === 0) {
    return Promise.resolve(bindingResult(args));
  }
  if (objectID === objectNames.CancelCall && method === 0) {
    return Promise.resolve(undefined);
  }
  if (objectID === objectNames.Events && method === 0) {
    return Promise.resolve(undefined);
  }
  if (objectID === objectNames.System) {
    return Promise.resolve(systemResult(method));
  }
  if (objectID === objectNames.Browser && method === 0 && args && typeof args === "object") {
    const url = (args as { url?: string }).url;
    if (typeof url === "string") {
      try {
        window.open(url, "_blank", "noopener,noreferrer");
      } catch {
        /* ignore */
      }
    }
    return Promise.resolve(undefined);
  }
  return Promise.resolve(undefined);
}

export function installBrowserWailsStub(): void {
  if (isWailsWebView()) return;
  setTransport({
    call: (objectID, method, windowName, args) =>
      browserResult(objectID, method, windowName, args),
  });
}
