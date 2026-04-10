import { writable } from "svelte/store";

export type EngineStatus =
  | "loading"
  | "ready"
  | "need_model"
  | "error"
  | "browser_preview";

export const engineStatus = writable<EngineStatus>("loading");
export const engineHardwareInfo = writable<string>("");
export const deviceName = writable<string>("Default");
export const currentPage = writable<string>("general");
