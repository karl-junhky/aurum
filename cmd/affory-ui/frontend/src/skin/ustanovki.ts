// AURUM: настройки оформления и хранилище своего фона. Живут только в
// localStorage и IndexedDB окна и не пересекаются с настройками Affory ни
// ключами, ни файлами, поэтому слияние с апстримом их не задевает.
import { useSyncExternalStore } from "react";

export type Fon = "net" | "planeta" | "galaktika" | "gradient" | "izgib" | "perelivy" | "svoy";
export type Palitra = "okean" | "zakat" | "avrora" | "zoloto";
export type Akcent = "zoloto" | "led" | "ametist" | "izumrud" | "roza";
export type Plotnost = "steklo" | "tuman" | "mat";

export interface Ustanovki {
  fon: Fon;
  palitra: Palitra;
  akcent: Akcent;
  plotnost: Plotnost;
  /** Плавающая «пилюля» вместо полного окна. */
  gud: boolean;
  /** Яркость фона, 20..100. */
  yarkost: number;
  zerno: boolean;
  /** Показывать скорость и задержку под пилюлей. */
  pokazatSkorost: boolean;
  /** Как часто сама проверять пинг серверов, минут. 0 это выключено. */
  avtoMin: number;
  /** После проверки переключаться на лучшую страну, если она заметно быстрее. */
  avtoVybor: boolean;
  /** Что лежит в «Своём фоне»: видео или картинка, и как файл называется. */
  tipMedia: "video" | "image";
  imyaFayla: string;
  /** Адрес для <video>/<img>. Не сохраняется: собирается из IndexedDB при запуске. */
  mediaUrl: string;
}

export const UMOLCHANIE: Ustanovki = {
  fon: "perelivy",
  palitra: "okean",
  akcent: "zoloto",
  plotnost: "steklo",
  gud: true,
  yarkost: 90,
  zerno: true,
  pokazatSkorost: true,
  avtoMin: 5,
  avtoVybor: false,
  tipMedia: "image",
  imyaFayla: "",
  mediaUrl: "",
};

const KLYUCH = "aurum.skin.v2";

function chitat(): Ustanovki {
  try {
    const t = localStorage.getItem(KLYUCH);
    return t ? { ...UMOLCHANIE, ...(JSON.parse(t) as Partial<Ustanovki>), mediaUrl: "" } : { ...UMOLCHANIE };
  } catch {
    return { ...UMOLCHANIE };
  }
}

let tekushchie: Ustanovki = chitat();
const slushateli = new Set<() => void>();
const uvedomit = () => slushateli.forEach((f) => f());

export const ustanovki = (): Ustanovki => tekushchie;

export function zadat(chast: Partial<Ustanovki>): void {
  tekushchie = { ...tekushchie, ...chast };
  try {
    const { mediaUrl: _ne_sohranyaem, ...ostalnoe } = tekushchie;
    localStorage.setItem(KLYUCH, JSON.stringify(ostalnoe));
  } catch {
    /* память окна недоступна: оформление действует до закрытия */
  }
  primenit(tekushchie);
  uvedomit();
}

export function podpisatsya(f: () => void): () => void {
  slushateli.add(f);
  return () => slushateli.delete(f);
}

export const useUstanovki = (): Ustanovki => useSyncExternalStore(podpisatsya, ustanovki);

/** Применяет настройки к <html>: CSS читает их атрибутами. */
export function primenit(u: Ustanovki = tekushchie): void {
  if (typeof document === "undefined") return;
  const h = document.documentElement;
  h.dataset.skin = "aurum";
  h.dataset.akcent = u.akcent;
  h.dataset.plotnost = u.plotnost;
  h.dataset.fon = u.fon;
  h.dataset.gud = u.gud ? "1" : "0";
  h.dataset.zerno = u.zerno ? "1" : "0";
  h.style.setProperty("--skin-yarkost", String(u.yarkost / 100));
}

// ---------- свой фон: IndexedDB, без лимита localStorage в 5 МБ ----------

const BAZA = "aurum_media_db";
const HRANILISHE = "wallpapers";
const KLYUCH_MEDIA = "custom_bg";
export const MAX_VIDEO_MB = 200;

interface ZapisMedia { imya: string; tip: "video" | "image"; blob?: Blob; dataUrl?: string }

function otkrytBazu(): Promise<IDBDatabase> {
  return new Promise((ok, err) => {
    if (typeof indexedDB === "undefined") return err(new Error("в этом окне нет хранилища для фона"));
    const r = indexedDB.open(BAZA, 1);
    r.onupgradeneeded = () => { if (!r.result.objectStoreNames.contains(HRANILISHE)) r.result.createObjectStore(HRANILISHE); };
    r.onsuccess = () => ok(r.result);
    r.onerror = () => err(r.error ?? new Error("ошибка хранилища фона"));
  });
}

/** Уменьшает картинку до 1920 px: фон на весь экран не требует оригинала с камеры. */
function szhatKartinku(file: File, maxStorona = 1920): Promise<string> {
  return new Promise((ok, err) => {
    const r = new FileReader();
    r.onerror = () => err(new Error("не удалось прочитать файл"));
    r.onload = () => {
      const im = new Image();
      im.onerror = () => err(new Error("это не картинка"));
      im.onload = () => {
        const k = Math.min(1, maxStorona / Math.max(im.width, im.height));
        const c = document.createElement("canvas");
        c.width = Math.round(im.width * k);
        c.height = Math.round(im.height * k);
        c.getContext("2d")!.drawImage(im, 0, 0, c.width, c.height);
        ok(c.toDataURL("image/jpeg", 0.86));
      };
      im.src = String(r.result);
    };
    r.readAsDataURL(file);
  });
}

let aktivnyyBlob = "";
function ssylkaIz(z: ZapisMedia): string {
  if (aktivnyyBlob) { URL.revokeObjectURL(aktivnyyBlob); aktivnyyBlob = ""; }
  if (z.tip === "video" && z.blob) return (aktivnyyBlob = URL.createObjectURL(z.blob));
  return z.dataUrl ?? "";
}

export async function sohranitMediaFayl(file: File): Promise<void> {
  const video = file.type.startsWith("video/") || /\.(mp4|webm|mov|m4v)$/i.test(file.name);
  if (video && file.size > MAX_VIDEO_MB * 1024 * 1024) throw new Error(`видео больше ${MAX_VIDEO_MB} МБ, возьми покороче`);
  const z: ZapisMedia = video ? { imya: file.name, tip: "video", blob: file } : { imya: file.name, tip: "image", dataUrl: await szhatKartinku(file) };
  try {
    const db = await otkrytBazu();
    await new Promise<void>((ok, err) => {
      const tx = db.transaction(HRANILISHE, "readwrite");
      tx.objectStore(HRANILISHE).put(z, KLYUCH_MEDIA);
      tx.oncomplete = () => ok();
      tx.onerror = () => err(tx.error ?? new Error("не удалось записать фон"));
    });
  } catch {
    /* без хранилища фон живёт до закрытия окна */
  }
  zadat({ fon: "svoy", imyaFayla: z.imya, tipMedia: z.tip, mediaUrl: ssylkaIz(z) });
}

/** При запуске достаёт сохранённый фон и собирает адрес для <video>/<img>. */
export async function vosstanovitMediaFayl(): Promise<void> {
  try {
    const db = await otkrytBazu();
    const z = await new Promise<ZapisMedia | undefined>((ok, err) => {
      const r = db.transaction(HRANILISHE, "readonly").objectStore(HRANILISHE).get(KLYUCH_MEDIA);
      r.onsuccess = () => ok(r.result as ZapisMedia | undefined);
      r.onerror = () => err(r.error);
    });
    if (z) { tekushchie = { ...tekushchie, imyaFayla: z.imya, tipMedia: z.tip, mediaUrl: ssylkaIz(z) }; uvedomit(); }
  } catch {
    /* нет хранилища: остаётся то, что в памяти */
  }
}
