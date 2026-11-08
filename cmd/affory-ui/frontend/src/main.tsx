import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./App";
import { zapisatVZhurnal } from "./most";
import "./tokeny.css";
import "./skin"; // AURUM: оформление поверх Affory (src/skin/, см. README.aurum)

// Ошибка страницы до 1.10.0 жила только в консоли WebView2, которой у
// человека нет: белое окно разбирать было не по чему. Теперь она в журнале
// окна и уходит в выгрузку.
window.addEventListener("error", (e) => {
  const stek = e.error instanceof Error && e.error.stack ? `\n${e.error.stack}` : "";
  zapisatVZhurnal(`ошибка: ${e.message} (${e.filename}:${e.lineno}:${e.colno})${stek}`);
});
window.addEventListener("unhandledrejection", (e) => {
  const p: unknown = e.reason;
  zapisatVZhurnal(`необработанный отказ: ${p instanceof Error ? (p.stack ?? p.message) : String(p)}`);
});

// The root exists in index.html; the assertion is the only thing that would
// ever tell you if someone renamed it.
const koren = document.getElementById("root");
if (!koren) throw new Error("в index.html нет #root");

ReactDOM.createRoot(koren).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
