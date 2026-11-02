import { useRef } from "react";
import type { CSSProperties, ReactNode } from "react";
import { VKLADKI, nazvanieVkladki, type Vkladka } from "./vkladki";
import { sleduyushchayaVkladka } from "./klavishi-vkladok";
import { IkNastroyki, IkPodklyuchenie, IkPravila, IkRazvernut, IkServer, IkSvernut, IkZakryt } from "../ikonki";
import { ZnachokServisa } from "./ui";
import sphere from "../assets/affory-sphere.png";
import github from "../assets/github-white.svg";

// Window frame: our own title bar (the window is frameless, §8.3) and the
// tabs. Pure over props like every screen; minimise, maximise and close are
// calls UP to App, which owns the bridge. This file never imports the runtime.
//
// Разделов в полосе четыре: «Серверы» остались вкладкой в коде, но в полосу не
// выходят и открываются кнопкой «Управлять» на экране подключения.

export interface KarkasProps {
  vkladka: Vkladka;
  naVkladku: (v: Vkladka) => void;
  naSvernut: () => void;
  naRazvernut?: () => void;
  naZakryt: () => void;
  naGitHub?: () => void;
  /** First run (§9.2): tabs are disabled until the service exists. The
   *  window controls stay live so the window can still be closed. */
  zablokirovany?: boolean;
  /** Точка у «Свой VPN», пока свой сервер «нужно внимание» (warn) или «не
   *  работает» (danger): общий статус виден с любой вкладки (план, §4). */
  metkaSvoegoVpn?: "warn" | "danger";
  children: ReactNode;
}

// Wails reads these custom properties to decide what drags the window.
// Inline, not in a class: the test asserts on the computed inline value, and
// a class would silently stop working the day someone renames it.
const TASHCHIT: CSSProperties = { ["--wails-draggable" as string]: "drag" } as CSSProperties;
const NE_TASHCHIT: CSSProperties = { ["--wails-draggable" as string]: "no-drag" } as CSSProperties;

const ZNACHKI: Partial<Record<Vkladka, typeof IkPodklyuchenie>> = {
  podklyuchenie: IkPodklyuchenie,
  pravila: IkPravila,
  "svoy-vpn": IkServer,
  nastroyki: IkNastroyki,
};

// «Серверы» живут вкладкой в коде, но в полосу не выходят: список неизменный,
// поэтому и порядок для стрелок берётся отсюда, а не из общего VKLADKI.
const VIDIMYE = VKLADKI.filter((v) => v !== "servery");

const SLOVO_METKI = { warn: "нужно внимание", danger: "не работает" } as const;

export function Karkas({ vkladka, naVkladku, naSvernut, naRazvernut, naZakryt, naGitHub, zablokirovany = false, metkaSvoegoVpn, children }: KarkasProps) {
  const knopki = useRef<(HTMLButtonElement | null)[]>([]);
  return (
    <div className="flex h-screen min-w-0 flex-col">
      <header
        data-testid="polosa"
        style={TASHCHIT}
        className="af-titlebar border-border bg-rail flex h-polosa shrink-0 items-stretch justify-between border-b pl-4 select-none"
      >
        <div className="af-title-left flex min-w-0 items-center gap-7">
          <span className="af-brand flex shrink-0 items-center gap-2.5 text-[15px] font-semibold tracking-tight">
            <img src={sphere} alt="" className="h-[22px] w-[22px]" />
            Affory
          </span>
          <nav role="tablist" aria-label="Разделы" className="flex h-polosa items-stretch">
            {VIDIMYE.map((v, nomer) => {
              const aktivna = v === vkladka || (v === "podklyuchenie" && vkladka === "servery");
              const Znachok = ZNACHKI[v];
              return (
                <button
                  key={v}
                  ref={(el) => { knopki.current[nomer] = el; }}
                  type="button"
                  role="tab"
                  aria-selected={aktivna}
                  // Roving tabindex: Tab доносит до полосы разделов один раз, а
                  // дальше по ней ходят стрелки. Прежде каждый раздел стоял в
                  // общем порядке, и до содержимого окна надо было пройти все.
                  tabIndex={aktivna ? 0 : -1}
                  style={NE_TASHCHIT}
                  disabled={zablokirovany}
                  onClick={() => naVkladku(v)}
                  onKeyDown={(e) => {
                    const kuda = sleduyushchayaVkladka(e.key, nomer, VIDIMYE.length);
                    if (kuda === null) return;
                    e.preventDefault();
                    naVkladku(VIDIMYE[kuda]);
                    knopki.current[kuda]?.focus();
                  }}
                  className={
                    // Ink, never fill: a fill-only tab has no edge on black.
                    aktivna
                      ? "text-accent-ink relative flex items-center gap-2 px-3.5 text-sm font-medium disabled:opacity-40"
                      : "text-fg-muted hover:text-fg-secondary relative flex items-center gap-2 px-3.5 text-sm font-medium disabled:opacity-40 disabled:hover:text-fg-muted"
                  }
                >
                  {/* Мягкая поверхность и тонкая линия: заливка отвечает за
                      «где я», линия за то, чтобы это было видно на чёрном. */}
                  {aktivna && <span aria-hidden className="bg-fill-subtle absolute inset-x-1 inset-y-[7px] -z-10 rounded-md" />}
                  {Znachok && <Znachok className="h-[18px] w-[18px]" />}
                  <span className={aktivna ? "text-foreground" : undefined}>{nazvanieVkladki(v)}</span>
                  {/* Точка, а не заливка: цвет здесь только меткой. Голосом
                      состояние читается вместе с именем вкладки. */}
                  {v === "svoy-vpn" && metkaSvoegoVpn && (
                    <>
                      <span aria-hidden data-testid="metka-svoego-vpn" className={`absolute top-[15px] right-2 h-1.5 w-1.5 rounded-full ${metkaSvoegoVpn === "warn" ? "bg-warn" : "bg-danger"}`} />
                      <span className="sr-only">{`, ${SLOVO_METKI[metkaSvoegoVpn]}`}</span>
                    </>
                  )}
                  {aktivna && <span aria-hidden className="bg-accent-ink absolute inset-x-1 bottom-0 h-[2px] rounded-t" />}
                </button>
              );
            })}
          </nav>
        </div>
        <div className="af-window-controls flex shrink-0 items-stretch">
          <button
            type="button"
            className="af-github text-fg-secondary hover:text-foreground mr-1 flex items-center px-3 transition-colors"
            style={NE_TASHCHIT}
            onClick={naGitHub}
            aria-label="Открыть GitHub Affory"
            title="GitHub Affory"
          >
            {/* Готовый логотип со словом внутри: своё слово рядом дало бы
                «GitHub GitHub», а резать логотип ради кота значит держать два
                файла вместо одного. */}
            <ZnachokServisa src={github} className="h-4 w-[70px]" />
          </button>
          <button
            type="button"
            data-testid="svernut"
            aria-label="Свернуть"
            style={NE_TASHCHIT}
            onClick={naSvernut}
            className="text-fg-muted hover:bg-fill-subtle hover:text-foreground flex w-11 items-center justify-center transition-colors"
          >
            <IkSvernut className="h-4 w-4" />
          </button>
          <button
            type="button"
            data-testid="razvernut"
            aria-label="Развернуть"
            style={NE_TASHCHIT}
            onClick={naRazvernut}
            disabled={!naRazvernut}
            className="text-fg-muted hover:bg-fill-subtle hover:text-foreground flex w-11 items-center justify-center transition-colors disabled:opacity-40"
          >
            <IkRazvernut className="h-4 w-4" />
          </button>
          <button
            type="button"
            data-testid="zakryt"
            aria-label="Закрыть"
            style={NE_TASHCHIT}
            onClick={naZakryt}
            className="text-fg-muted hover:bg-danger hover:text-background flex w-11 items-center justify-center transition-colors"
          >
            <IkZakryt className="h-4 w-4" />
          </button>
        </div>
      </header>
      <div className="af-viewport flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
    </div>
  );
}
