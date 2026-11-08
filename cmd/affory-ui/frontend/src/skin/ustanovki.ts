// AURUM: настройки оформления. Живут только в localStorage окна и никак не
// пересекаются с настройками самого Affory (ни ключей, ни файлов в общем
// профиле), поэтому слияние с апстримом их не задевает.
import { useSyncExternalStore } from "react";

export type Fon = "net" | "planeta" | "galaktika" | "gradient" | "izgib" | "svoy";
export type Akcent = "zoloto" | "led" | "ametist" | "izumrud" | "roza";
export type Plotnost = "steklo" | "tuman" | "mat";

export interface Ustanovki {
  fon: Fon;
  akcent: Akcent;
  plotnost: Plotnost;
  /** Плавающая «пилюля» вместо полного окна. */
  gud: boolean;
  /** Яркость фона, 20..100. */
  yarkost: number;
  /** Картинка для фона «Свой» (data: URL, уменьшенная). */
  kartinka: string;
  /** Зерно плёнки поверх фона. */
  zerno: boolean;
}

export const UMOLCHANIE: Ustanovki = {
  fon: "galaktika",
  akcent: "zoloto",
  plotnost: "steklo",
  gud: true,
  yarkost: 85,
  kartinka: "",
  zerno: true,
};

const KLYUCH = "aurum.skin.v1";
const KLYUCH_KARTINKI = "aurum.skin.img.v1";

function chitat(): Ustanovki {
  try {
    const t = localStorage.getItem(KLYUCH);
    const k = localStorage.getItem(KLYUCH_KARTINKI) ?? "";
    if (!t) return { ...UMOLCHANIE, kartinka: k };
    return { ...UMOLCHANIE, ...(JSON.parse(t) as Partial<Ustanovki>), kartinka: k };
  } catch {
    return { ...UMOLCHANIE };
  }
}

let tekushchie: Ustanovki = chitat();
const slushateli = new Set<() => void>();

export function ustanovki(): Ustanovki {
  return tekushchie;
}

export function zadat(chast: Partial<Ustanovki>): void {
  tekushchie = { ...tekushchie, ...chast };
  try {
    const { kartinka, ...ostalnoe } = tekushchie;
    localStorage.setItem(KLYUCH, JSON.stringify(ostalnoe));
    if ("kartinka" in chast) localStorage.setItem(KLYUCH_KARTINKI, kartinka);
  } catch {
    /* память окна недоступна: оформление действует до закрытия */
  }
  slushateli.forEach((f) => f());
}

export function podpisatsya(f: () => void): () => void {
  slushateli.add(f);
  return () => slushateli.delete(f);
}

export function useUstanovki(): Ustanovki {
  return useSyncExternalStore(podpisatsya, ustanovki);
}

/** Применяет настройки к <html>: CSS читает их атрибутами. */
export function primenit(u: Ustanovki = tekushchie): void {
  const h = document.documentElement;
  h.dataset.skin = "aurum";
  h.dataset.akcent = u.akcent;
  h.dataset.plotnost = u.plotnost;
  h.dataset.fon = u.fon;
  h.dataset.gud = u.gud ? "1" : "0";
  h.dataset.zerno = u.zerno ? "1" : "0";
  h.style.setProperty("--skin-yarkost", String(u.yarkost / 100));
}

/** Уменьшает выбранную картинку до разумного размера: localStorage окна
 *  невелик, а фон на весь экран не требует оригинала с камеры. */
export function szhatKartinku(file: File, maxStorona = 1920): Promise<string> {
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
        let q = 0.82;
        let out = c.toDataURL("image/jpeg", q);
        while (out.length > 2_200_000 && q > 0.4) {
          q -= 0.12;
          out = c.toDataURL("image/jpeg", q);
        }
        ok(out);
      };
      im.src = String(r.result);
    };
    r.readAsDataURL(file);
  });
}
