// Политика безопасности страницы (О7 аудита 1.6.1).
//
// Ставится только в сборку выпуска, плагином в vite.config.ts. Стенд для
// Playwright это dev-сервер Vite, и плагин React вставляет туда встроенный
// скрипт, который такая политика запретила бы.
//
// Что нужно странице, проверено по @wailsio/runtime 3.0.0-beta.26: вызовы моста
// идут fetch на свой адрес (/wails/runtime), дополнительный скрипт тоже свой
// (/wails/custom.js). Картинки data: это QR-коды из моста и значки сервисов,
// шрифты лежат в сборке. Стили разрешены встроенными: React пишет их в атрибут
// style, а вреда от стиля без скрипта нет.
export const CSP = [
  "default-src 'none'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data:",
  "media-src 'self' blob:", // AURUM: свой фон-видео из файла (object URL)
  "font-src 'self' data:",
  "connect-src 'self'",
  "base-uri 'none'",
  "form-action 'none'",
  "object-src 'none'",
].join("; ");

export interface TegHtml {
  tag: string;
  attrs: Record<string, string>;
  injectTo: "head-prepend";
}

/** Тег для transformIndexHtml: первым в head, до любого скрипта. */
export function metaCsp(): TegHtml {
  return {
    tag: "meta",
    attrs: { "http-equiv": "Content-Security-Policy", content: CSP },
    injectTo: "head-prepend",
  };
}
