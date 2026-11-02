import { useEffect, useId, useRef, useState } from "react";
import type { KeyboardEvent as SobytieKlavishi, ReactNode, Ref } from "react";
import { IkPoisk, IkStrelkaVniz, IkTochki, IkTreugolnik } from "../ikonki";

// Примитивы редизайна. Отдельным файлом, а не внутри ui.tsx, потому что ui.tsx
// продолжает обслуживать ещё не переписанные экраны: одна правка на файл
// вместо одного файла на два поколения разметки. Реэкспорт стоит в ui.tsx,
// поэтому для экранов это по-прежнему один адрес.

/** Вертушка ожидания. Одна на всю программу: три разные крутилки на трёх
 *  экранах это три разных ответа на вопрос «программа жива?». */
export function Vertushka({ className = "h-4 w-4" }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={`shrink-0 animate-spin ${className}`} fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="2.4" opacity="0.25" />
      <path d="M21 12a9 9 0 00-9-9" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" />
    </svg>
  );
}

/** Флажок: тот же родной checkbox, только квадратный. Для строки таблицы,
 *  где переключатель-пилюля читался бы как настройка, а не как отметка. */
export function Flazhok({ vkl, naSmenu, podpis, golos, skrytPodpis = false, aktiven = true, testId }: {
  vkl: boolean;
  naSmenu: (v: boolean) => void;
  podpis: string;
  /** Имя для чтения с экрана, когда видимая подпись одинакова во всех строках
   *  списка. Глазами строка читается вместе со своим приложением, а голосом
   *  десять одинаковых «И запущенные им программы» подряд не различить. */
  golos?: string;
  skrytPodpis?: boolean;
  aktiven?: boolean;
  testId?: string;
}) {
  const id = useId();
  return (
    <span className="inline-flex items-center gap-2">
      <input
        id={id}
        type="checkbox"
        data-testid={testId}
        className="flazhok"
        checked={vkl}
        disabled={!aktiven}
        aria-label={golos ?? (skrytPodpis ? podpis : undefined)}
        onChange={(e) => naSmenu(e.target.checked)}
      />
      {!skrytPodpis && (
        <label htmlFor={id} className="text-fg-secondary cursor-pointer select-none text-[13px]">{podpis}</label>
      )}
    </span>
  );
}

/** Панель: поверхность с тонкой кромкой, внутри строки. То же, что Karta, но
 *  с радиусом редизайна; Karta живёт, пока жив хоть один непереписанный
 *  экран, и уходит вместе с последним. */
export function Panel({ children, className = "", testId }: { children: ReactNode; className?: string; testId?: string }) {
  return (
    <div data-testid={testId} className={`bg-surface border-border overflow-hidden rounded-xl border ${className}`}>
      {children}
    </div>
  );
}

/** Выбор маршрута в строке таблицы. Родной select: клавиатура и список
 *  приезжают от системы, нам остаётся только покрасить. */
export function Vybor({ znachenie, naVybor, znacheniya, aktiven = true, testId, "aria-label": podpis }: {
  znachenie: string;
  naVybor: (v: string) => void;
  znacheniya: string[];
  aktiven?: boolean;
  testId?: string;
  "aria-label": string;
}) {
  return (
    <div className="relative">
      <select
        data-testid={testId}
        aria-label={podpis}
        value={znachenie}
        disabled={!aktiven}
        onChange={(e) => naVybor(e.target.value)}
        className="bg-elevated border-border text-foreground hover:border-border-hover h-9 w-full appearance-none rounded-md border pl-3 pr-8 text-[13px] disabled:opacity-40"
      >
        {znacheniya.map((z) => <option key={z} value={z}>{z}</option>)}
      </select>
      <IkStrelkaVniz className="text-fg-muted pointer-events-none absolute right-2 top-1/2 h-4 w-4 -translate-y-1/2" />
    </div>
  );
}

/** Поиск: поле со значком, на всю ширину строки инструментов. */
export function Poisk({ znachenie, naVvod, placeholder, id, priv, aktiven = true, testId, "aria-label": podpis }: {
  znachenie: string;
  naVvod: (v: string) => void;
  placeholder: string;
  id?: string;
  /** Ссылка на само поле: форма ставит в него курсор при открытии. */
  priv?: Ref<HTMLInputElement>;
  aktiven?: boolean;
  testId?: string;
  "aria-label": string;
}) {
  return (
    <div className="relative min-w-0 flex-1">
      <IkPoisk className="text-fg-muted pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" />
      <input
        id={id}
        ref={priv}
        type="search"
        data-testid={testId}
        aria-label={podpis}
        value={znachenie}
        disabled={!aktiven}
        placeholder={placeholder}
        spellCheck={false}
        onChange={(e) => naVvod(e.target.value)}
        className="bg-elevated border-border text-foreground placeholder:text-fg-muted hover:border-border-hover focus:border-border-active h-10 w-full rounded-lg border pl-9 pr-3 text-sm disabled:opacity-40"
      />
    </div>
  );
}

/** Сворачиваемая строка: треугольник, заголовок, содержимое. Одна и та же на
 *  всех экранах, поэтому «Дополнительно» везде выглядит одинаково. */
export function Svorachivaemyy({ zagolovok, poyasnenie, deti, otkrytPoUmolchaniyu = false, testId }: {
  zagolovok: string;
  poyasnenie?: string;
  deti: ReactNode;
  otkrytPoUmolchaniyu?: boolean;
  testId?: string;
}) {
  const [otkryt, zadat] = useState(otkrytPoUmolchaniyu);
  return (
    <div className="border-border border-t">
      <button
        type="button"
        data-testid={testId}
        aria-expanded={otkryt}
        onClick={() => zadat(!otkryt)}
        className="group flex w-full items-center gap-2.5 py-3 text-left"
      >
        <IkTreugolnik className={`text-fg-muted group-hover:text-fg-secondary h-3 w-3 shrink-0 transition-transform ${otkryt ? "rotate-90" : ""}`} />
        <span className="text-fg-secondary group-hover:text-foreground text-sm font-medium">{zagolovok}</span>
        {poyasnenie && <span className="text-fg-muted truncate text-[13px]">{poyasnenie}</span>}
      </button>
      {otkryt && <div className="text-fg-secondary pb-4 pl-[22px] pr-1 text-[13px] leading-relaxed">{deti}</div>}
    </div>
  );
}

/** Группа раздела: заголовок, строка о том, что внутри, и панель. Одна на
 *  настройки и «Свой VPN»: раздел в полосе читается одной грамматикой. */
export function Gruppa({ nazvanie, poyasnenie, children }: { nazvanie: string; poyasnenie?: string; children: ReactNode }) {
  return (
    <section aria-label={nazvanie} className="flex flex-col gap-3">
      <div>
        <h3 className="text-foreground text-[17px] font-semibold leading-tight">{nazvanie}</h3>
        {poyasnenie && <p className="text-fg-muted mt-1 text-[13px]">{poyasnenie}</p>}
      </div>
      {children}
    </section>
  );
}

/** Окно подтверждения поверх экрана: вопрос, что случится, и ответы. Та же
 *  подложка и тот же порядок, что у окна ключа (Vygruzka.tsx): Esc и щелчок
 *  мимо закрывают, фокус встаёт в окно. */
export function OknoPodtverzhdeniya({ zagolovok, children, otvety, naOtmenu, testId }: {
  zagolovok: string;
  children: ReactNode;
  /** Кнопки ответов, «Отмена» окно ставит само последней. */
  otvety: ReactNode;
  naOtmenu: () => void;
  testId?: string;
}) {
  const okno = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const naKlavishu = (e: globalThis.KeyboardEvent) => {
      if (e.key === "Escape") naOtmenu();
    };
    document.addEventListener("keydown", naKlavishu);
    okno.current?.focus();
    return () => document.removeEventListener("keydown", naKlavishu);
  }, [naOtmenu]);
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) naOtmenu();
      }}
    >
      <div
        ref={okno}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-label={zagolovok}
        data-testid={testId}
        className="border-border bg-surface flex max-h-[85vh] w-full max-w-md flex-col gap-4 overflow-y-auto rounded-xl border p-5 focus-visible:outline-none"
      >
        <h3 className="text-foreground text-base font-semibold">{zagolovok}</h3>
        <div className="text-fg-secondary flex flex-col gap-2 text-[13px] leading-relaxed">{children}</div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          {otvety}
          <button
            type="button"
            onClick={naOtmenu}
            className="text-fg-muted hover:text-foreground inline-flex h-8 items-center px-1.5 text-[13px] font-medium"
          >
            Отмена
          </button>
        </div>
      </div>
    </div>
  );
}

/** Строка-раздел: значок, название, пояснение, содержимое под ней. */
export function RyadRazdela({ znachok, nazvanie, poyasnenie, otkryt, naZhmyh, deti, testId }: {
  znachok: ReactNode;
  nazvanie: string;
  poyasnenie: string;
  otkryt: boolean;
  naZhmyh: () => void;
  deti: ReactNode;
  testId?: string;
}) {
  return (
    <div className="border-border border-t first:border-t-0">
      <button type="button" data-testid={testId} aria-expanded={otkryt} onClick={naZhmyh} className="group flex w-full items-center gap-3.5 py-3.5 text-left">
        <span className={`flex h-8 w-8 shrink-0 items-center justify-center transition-transform ${otkryt ? "text-accent-ink rotate-90" : "text-fg-muted"}`}>{znachok}</span>
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-foreground text-sm font-medium">{nazvanie}</span>
          <span className="text-fg-muted truncate text-[13px]">{poyasnenie}</span>
        </span>
      </button>
      {otkryt && <div className="pb-4 pl-[46px] pr-1">{deti}</div>}
    </div>
  );
}

/** Меню действий строки. Своё, а не родное контекстное: пункты подписаны
 *  словами человека, а удаление живёт внутри, а не ярким словом в каждой
 *  строке таблицы. */
export function MenyuDeystviy({ punkty, podpis, testId }: {
  punkty: { podpis: string; znachok?: ReactNode; opasnyy?: boolean; aktiven?: boolean; naZhmyh: () => void }[];
  podpis: string;
  testId?: string;
}) {
  const [otkryto, zadat] = useState(false);
  const koren = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!otkryto) return;
    const mimo = (e: MouseEvent) => { if (koren.current && !koren.current.contains(e.target as Node)) zadat(false); };
    const pobeg = (e: KeyboardEvent) => { if (e.key === "Escape") zadat(false); };
    document.addEventListener("mousedown", mimo);
    document.addEventListener("keydown", pobeg);
    return () => { document.removeEventListener("mousedown", mimo); document.removeEventListener("keydown", pobeg); };
  }, [otkryto]);
  return (
    <div ref={koren} className="relative">
      <button
        type="button"
        data-testid={testId}
        aria-label={podpis}
        aria-haspopup="menu"
        aria-expanded={otkryto}
        onClick={() => zadat(!otkryto)}
        className={`flex h-9 w-9 items-center justify-center rounded-md border transition-colors ${
          otkryto ? "border-border-active bg-surface-hover text-foreground" : "border-border text-fg-muted hover:border-border-hover hover:bg-surface-hover hover:text-foreground"
        }`}
      >
        <IkTochki className="h-4 w-4" />
      </button>
      {otkryto && (
        <div role="menu" className="border-border bg-elevated absolute right-0 top-10 z-20 w-56 overflow-hidden rounded-lg border py-1 shadow-[0_12px_32px_rgba(0,0,0,0.6)]">
          {punkty.map((p) => (
            <button
              key={p.podpis}
              type="button"
              role="menuitem"
              disabled={p.aktiven === false}
              onClick={() => { p.naZhmyh(); zadat(false); }}
              className={`hover:bg-surface-hover flex w-full items-center gap-2.5 px-3 py-2 text-left text-[13px] transition-colors disabled:pointer-events-none disabled:opacity-40 ${p.opasnyy ? "text-danger" : "text-fg-secondary hover:text-foreground"}`}
            >
              {p.znachok && <span className="flex h-4 w-4 shrink-0 items-center justify-center">{p.znachok}</span>}
              {p.podpis}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/** Значок сервиса одним цветом: маска по файлу, цвет от текста. Так восемь
 *  чужих логотипов перестают быть восемью чужими палитрами.
 *
 *  Кавычки вокруг адреса обязательны: сборщик подставляет сюда data-URI с
 *  одинарными кавычками внутри, и без внешних двойных браузер молча
 *  выбрасывает всё свойство, а значок превращается в закрашенный квадрат. */
export function ZnachokServisa({ src, className = "" }: { src: string; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`inline-block bg-current ${className}`}
      style={{
        WebkitMaskImage: `url("${src}")`,
        maskImage: `url("${src}")`,
        WebkitMaskRepeat: "no-repeat",
        maskRepeat: "no-repeat",
        WebkitMaskPosition: "center",
        maskPosition: "center",
        WebkitMaskSize: "contain",
        maskSize: "contain",
      }}
    />
  );
}

/** Стрелки внутри группы переключателей.
 *
 *  По практике ARIA стрелка в radiogroup и двигает фокус, и меняет выбор -
 *  это первое, что пробует человек с клавиатуры. До 23.09.2026 не
 *  происходило ничего: обе группы окна слушали только Tab и пробел. */
export function strelkiRadio<T extends string>(
  znacheniya: { z: T; disabled?: boolean }[],
  tekushchiy: T,
  naVybor: (z: T) => void,
): (e: SobytieKlavishi<HTMLDivElement>) => void {
  return e => {
    const vpered = e.key === "ArrowRight" || e.key === "ArrowDown";
    const nazad = e.key === "ArrowLeft" || e.key === "ArrowUp";
    if (!vpered && !nazad) return;
    const dostupnye = znacheniya.filter(v => !v.disabled);
    if (dostupnye.length < 2) return;
    const seychas = dostupnye.findIndex(v => v.z === tekushchiy);
    const sled = dostupnye[((seychas < 0 ? 0 : seychas) + (vpered ? 1 : -1) + dostupnye.length) % dostupnye.length];
    e.preventDefault();
    naVybor(sled.z);
    // Фокус идёт за выбором. Иначе стрелка меняет режим, а следующий пробел
    // на неподвинувшемся фокусе возвращает прежний.
    const knopki = [...e.currentTarget.querySelectorAll<HTMLButtonElement>("[role=radio]:not(:disabled)")];
    knopki[dostupnye.indexOf(sled)]?.focus();
  };
}

/** Сегмент в столбик. Тот же выбор, что и Segment, но в узкой боковой
 *  колонке: две подписи в строку там не помещаются, а обрезать их значит
 *  спрятать разницу между режимами ровно в том месте, где её и выбирают. */
export function SegmentStolbik<T extends string>({ znacheniya, vybrano, naVybor, aktiven = true, "aria-label": podpis }: {
  znacheniya: { z: T; podpis: string; disabled?: boolean }[];
  vybrano: T;
  naVybor: (z: T) => void;
  aktiven?: boolean;
  "aria-label": string;
}) {
  return (
    <div role="radiogroup" aria-label={podpis} onKeyDown={aktiven ? strelkiRadio(znacheniya, vybrano, naVybor) : undefined}
      className="bg-elevated border-border flex flex-col rounded-lg border p-1">
      {znacheniya.map(({ z, podpis: p, disabled }) => {
        const on = z === vybrano;
        return (
          <button
            key={z}
            type="button"
            role="radio"
            aria-checked={on}
            disabled={!aktiven || disabled}
            onClick={() => naVybor(z)}
            className={`h-10 rounded-md px-3 text-left text-sm font-medium transition-colors disabled:opacity-40 ${
              on ? "bg-accent/45 text-foreground" : "text-fg-secondary hover:bg-fill-subtle hover:text-foreground"
            }`}
          >
            {p}
          </button>
        );
      })}
    </div>
  );
}

/** Меню у курсора: открывается правой кнопкой по строке списка.
 *
 *  Своё, а не родное браузерное: пункты подписаны словами человека. Отдельно от
 *  MenyuDeystviy выше, потому что вопрос другой. То меню это кнопка, видимая
 *  всегда; это открывается только когда человек его просит, и за место на
 *  экране не платит ничего - ради этого оно и заведено (просьба владельца
 *  22.09.2026: «не мусорить на главном экране»).
 *
 *  Клавиатура закрыта тем же путём: `Shift+F10` и клавиша контекстного меню
 *  открывают его у самой строки. Меню, доступное только мышью, это функция,
 *  которой для половины людей нет вовсе.
 */
export function MenyuUKursora({ x, y, punkty, podpis, naZakrytie, testId }: {
  x: number;
  y: number;
  punkty: { podpis: string; opasnyy?: boolean; aktiven?: boolean; naZhmyh: () => void }[];
  podpis: string;
  naZakrytie: () => void;
  testId?: string;
}) {
  const koren = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const mimo = (e: MouseEvent) => { if (koren.current && !koren.current.contains(e.target as Node)) naZakrytie(); };
    const pobeg = (e: KeyboardEvent) => { if (e.key === "Escape") naZakrytie(); };
    document.addEventListener("mousedown", mimo);
    document.addEventListener("keydown", pobeg);
    // Прокрутка уводит строку из-под меню, и оно остаётся висеть над чужой.
    window.addEventListener("scroll", naZakrytie, true);
    return () => {
      document.removeEventListener("mousedown", mimo);
      document.removeEventListener("keydown", pobeg);
      window.removeEventListener("scroll", naZakrytie, true);
    };
  }, [naZakrytie]);
  // Фокус уезжает в меню сразу: иначе открытое с клавиатуры меню невозможно
  // ни пройти стрелками, ни закрыть иначе как Escape вслепую.
  useEffect(() => {
    koren.current?.querySelector<HTMLButtonElement>("button:not([disabled])")?.focus();
  }, []);
  return (
    <div
      ref={koren}
      role="menu"
      aria-label={podpis}
      data-testid={testId}
      style={{ left: x, top: y }}
      className="border-border bg-elevated fixed z-30 w-56 overflow-hidden rounded-lg border py-1 shadow-[0_12px_32px_rgba(0,0,0,0.6)]"
    >
      {punkty.map((p) => (
        <button
          key={p.podpis}
          type="button"
          role="menuitem"
          disabled={p.aktiven === false}
          onClick={(e) => { e.stopPropagation(); p.naZhmyh(); naZakrytie(); }}
          className={`hover:bg-surface-hover flex w-full items-center gap-2.5 px-3 py-2 text-left text-[13px] transition-colors disabled:pointer-events-none disabled:opacity-40 ${p.opasnyy ? "text-danger" : "text-fg-secondary hover:text-foreground"}`}
        >
          {p.podpis}
        </button>
      ))}
    </div>
  );
}
