// Общее живое состояние оболочки: что нарисовать на планете и когда был
// последний замер. Не связано с настройками и ничего не пишет на диск.
import { useSyncExternalStore } from "react";
import type { Ton } from "./analiz";

export interface Metka {
  klyuch: string;
  kod: string;
  imya: string;
  lat: number;
  lon: number;
  ping: number | null;
  ton: Ton;
  aktivna: boolean;
  servery: number;
}

export interface Zhivoe {
  metki: Metka[];
  /** Растёт, когда планету надо повернуть к выбранной стране. */
  fokus: number;
  fokusKod: string | null;
  /** Когда замер закончился в последний раз, мс. 0 это ещё не было. */
  zameryAt: number;
  /** Сообщение от автовыбора. */
  izvestie: string;
}

let sost: Zhivoe = { metki: [], fokus: 0, fokusKod: null, zameryAt: 0, izvestie: "" };
const slushateli = new Set<() => void>();

export const zhivoe = () => sost;
export function opublikovat(chast: Partial<Zhivoe>): void {
  sost = { ...sost, ...chast };
  slushateli.forEach((f) => f());
}
export function povernutK(kod: string | null): void {
  opublikovat({ fokus: sost.fokus + 1, fokusKod: kod });
}
export function useZhivoe(): Zhivoe {
  return useSyncExternalStore((f) => { slushateli.add(f); return () => slushateli.delete(f); }, zhivoe);
}

/** Событие с планеты: нажали на метку страны. */
export const SOBYTIE_METKI = "aurum-metka";
