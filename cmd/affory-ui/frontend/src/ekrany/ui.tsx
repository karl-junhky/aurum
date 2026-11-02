import type { ButtonHTMLAttributes, ReactNode, Ref } from "react";
import { TEKST_BEZ_KODA, tekstOtkaza } from "./otkazy";
import { strelkiRadio, Vertushka } from "./ui-novye";

// Примитивы редизайна лежат рядом и выходят наружу отсюда: для экранов
// адрес один, а править их можно, не трогая старую грамматику.
export * from "./ui-novye";

// The layout grammar of the whole window, accepted on the mockup 02.09.2026.
// Screens compose these pieces and never restyle them: one column, cards of
// rows, three ranks of buttons, a native checkbox dressed as a switch. Every
// class string lives here once, so "make the buttons compact" is one edit.

/** The shared column every tab renders into: 32px gutter, 800px cap. */
export function Kolonka({ children, "aria-label": podpis }: { children: ReactNode; "aria-label"?: string }) {
  return (
    <section aria-label={podpis} className="mx-auto flex w-full max-w-[980px] flex-col gap-6 px-8 pt-6 pb-8">
      {children}
    </section>
  );
}

/** Tab header: title and one-line summary on the left, the primary action
 *  on the right, in the same place on every tab. */
export function Shapka({ zagolovok, svodka, uZagolovka, children }: {
  zagolovok: string;
  svodka?: ReactNode;
  /** Мелочь ВПЛОТНУЮ к заголовку: значок справки и подобное. Отдельно от
   *  children, которые уходят к правому краю строки и читаются как действия
   *  экрана. Значок, уехавший туда, потерял бы связь со словом, к которому
   *  относится. */
  uZagolovka?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <header className="flex min-h-9 items-center justify-between gap-4">
      <div>
        <div className="flex items-center gap-1.5">
          <h2 className="text-foreground text-xl font-semibold leading-tight">{zagolovok}</h2>
          {uZagolovka}
        </div>
        {svodka && <div className="text-fg-muted mt-0.5 text-[13px]">{svodka}</div>}
      </div>
      {children && <div className="flex shrink-0 gap-2">{children}</div>}
    </header>
  );
}

/** Section: small muted label over a card. */
export function Razdel({ nazvanie, children, "aria-label": podpis }: { nazvanie?: string; children: ReactNode; "aria-label"?: string }) {
  return (
    <section aria-label={podpis ?? nazvanie} className="flex flex-col gap-2">
      {nazvanie && <h3 className="text-fg-muted text-xs font-medium tracking-wide">{nazvanie}</h3>}
      {children}
    </section>
  );
}

/** The card: a surface with a hairline border, rows inside. */
export function Karta({ children, testId, bezObrezki = false }: {
  children: ReactNode;
  testId?: string;
  /** Let a popup inside the card hang past its edge. The clipping is there
   *  for the rounded corners of the rows, and a card WITHOUT rows pays for
   *  it with a cut popup: the process picker on the rules tab showed one row
   *  and the rest died at the card's border (owner, 04.09.2026). */
  bezObrezki?: boolean;
}) {
  return (
    <div data-testid={testId} className={"bg-surface border-border rounded-lg border" + (bezObrezki ? "" : " overflow-hidden")}>
      {children}
    </div>
  );
}

/** A settings row: name and explanation on the left, the control on the
 *  right. `aktiven=false` greys the name; the control disables itself. */
export function Ryad({ znachok, nazvanie, poyasnenie, podskazka, aktiven = true, lomat = false, children, testId }: {
  /** Значок слева от названия. Для строк настроек, где он помогает найти
   *  нужную глазами; в таблицах правил значков нет по решению владельца. */
  znachok?: ReactNode;
  nazvanie: ReactNode;
  poyasnenie?: ReactNode;
  /** Подсказка при наведении. Для того, что нужно знать ПОТОМ, а не при
   *  первом чтении экрана: постоянная строка тут только мешала бы. Родной
   *  title, а не свой всплывающий слой: собственного паттерна подсказок в
   *  программе нет, и заводить его ради одной строки не за что. */
  podskazka?: string;
  aktiven?: boolean;
  /** The name is a path or a domain, i.e. one word 200 characters long.
   *  Without break-all such a row simply widens the column and the window
   *  scrolls sideways (Karkas has overflow-auto). */
  lomat?: boolean;
  children?: ReactNode;
  testId?: string;
}) {
  const ton = aktiven ? "text-foreground text-sm font-medium" : "text-fg-muted text-sm font-medium";
  return (
    <div data-testid={testId} title={podskazka} className="border-border flex min-h-[56px] items-center gap-3.5 border-t px-4 py-3 first:border-t-0">
      {znachok && <span className="text-fg-muted flex h-8 w-8 shrink-0 items-center justify-center">{znachok}</span>}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className={lomat ? `${ton} break-all` : ton}>{nazvanie}</span>
        {poyasnenie && <span className="text-fg-muted break-words text-xs">{poyasnenie}</span>}
      </div>
      {children && <div className="flex shrink-0 items-center gap-2">{children}</div>}
    </div>
  );
}

/** Либо код словаря, либо своя фраза экрана, и одно из двух ОБЯЗАТЕЛЬНО.
 *
 *  Без второй половины экран, у которого отказ не приезжает от службы, вынужден
 *  выдумывать код: ровно так `qr-s-ekrana` и появился в Servery.tsx, а
 *  компонент пережил его молча. Чтение QR с экрана делает окно, кода на проводе
 *  у такого отказа нет и быть не должно, а сказать человеку есть что. */
type NeudachaProps = {
  tekst?: string;
  deystvie?: () => void;
  podpisDeystviya?: string;
  testId?: string;
} & (
  | {
    kod: string;
    /** The tab's own sentence, when the code's wording is about something
     *  else: `secrets-unreadable` says "серверы", and on the rules tab that
     *  would be a lie about which list failed. The code's own text then
     *  moves to the second line, so nothing is lost. */
    zagolovok?: string;
  }
  | { kod?: undefined; zagolovok: string }
);

/** §9.1 refusal drawn INSIDE a tab: what failed, and the one way out of the
 *  tab it broke. Otkaz.tsx draws the window-wide banner for the same code;
 *  this is the local twin for a refusal that belongs to one list, and it
 *  names its own way out, because "Открыть серверы" on the servers tab is
 *  not a way out of anything. */
export function Neudacha({ kod, zagolovok, tekst, deystvie, podpisDeystviya, testId }: NeudachaProps) {
  const poKodu = kod ? tekstOtkaza[kod]?.tekst : undefined;
  // An unknown code still renders (Otkaz.tsx does the same): raw as the title
  // when there is nothing else, and silently when the screen brought its own
  // sentence. A code printed as a second line would be noise, not a reason.
  const prichina = [zagolovok ? poKodu : undefined, tekst].filter(Boolean).join("; ");
  return (
    <section
      role="alert"
      data-testid={testId}
      data-kod={kod}
      className="border-danger/40 bg-surface flex flex-col gap-3 rounded-lg border p-5"
    >
      {/* Код на месте заголовка не рисуется: он имя для нас, а не ответ
          человеку. Остаётся в data-kod выше. Написание с прописной и
          полужирный те же, что у баннера Otkaz.tsx: один и тот же отказ не
          должен выглядеть на двух экранах по-разному. */}
      <p className="text-foreground break-words text-base font-medium first-letter:uppercase">{zagolovok ?? poKodu ?? TEKST_BEZ_KODA}</p>
      {prichina && <p className="text-fg-secondary break-words text-sm">{prichina}</p>}
      {deystvie && podpisDeystviya && (
        <Knopka rang="glavnaya" className="self-start" testId={testId ? `${testId}-deystvie` : undefined} onClick={deystvie}>
          {podpisDeystviya}
        </Knopka>
      )}
    </section>
  );
}

export type RangKnopki = "glavnaya" | "vtoraya" | "opasnaya" | "tekst" | "znachok";

const RANG: Record<RangKnopki, string> = {
  glavnaya: "bg-accent text-foreground hover:brightness-110 px-3",
  vtoraya: "border-border-hover text-foreground hover:bg-fill-subtle border px-3",
  opasnaya: "border-border-hover text-danger hover:bg-fill-subtle border px-3",
  tekst: "text-fg-muted hover:text-foreground px-1.5",
  znachok: "border-border-hover text-fg-muted hover:text-foreground w-8 border px-0",
};

/** Button. 32px tall, 13px text, flex-centred label: the mockup's crooked
 *  captions were inline-block plus padding, and this is the fix. `bolshaya`
 *  is the 40px variant for the one main action of the connection card. */
export function Knopka({ rang, aktiven = true, bolshaya = false, zhdyot = false, testId, priv, tip = "button", className = "", children, ...rest }: {
  rang: RangKnopki;
  aktiven?: boolean;
  bolshaya?: boolean;
  /** Кнопка занята и ждёт ответа: сама рисует вертушку и запрещает второе
   *  нажатие. Без этого «нажал и ничего» читается как зависшая программа. */
  zhdyot?: boolean;
  testId?: string;
  /** Ссылка на саму кнопку: сюда возвращают фокус, когда исчезает элемент, на
   *  котором он стоял. Проп, а не forwardRef, как у `Pole` выше. */
  priv?: Ref<HTMLButtonElement>;
  /** `submit` для единственной кнопки формы, чтобы Enter в поле работал. */
  tip?: "button" | "submit";
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  const razmer = bolshaya ? "h-10 px-5 text-sm" : "h-8 text-[13px]";
  return (
    <button
      ref={priv}
      type={tip}
      data-testid={testId}
      aria-busy={zhdyot || undefined}
      disabled={!aktiven || zhdyot}
      className={`inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-md font-medium leading-none disabled:opacity-40 ${razmer} ${RANG[rang]} ${className}`}
      {...rest}
    >
      {zhdyot && <Vertushka className={bolshaya ? "h-4 w-4" : "h-3.5 w-3.5"} />}
      {children}
    </button>
  );
}

/** Switch. A native checkbox with role=switch: keyboard, focus and disabled
 *  come from the browser, only the paint is ours. */
export function Tumbler({ testId, vkl, aktiven, naSmenu, podpis }: {
  testId: string;
  vkl: boolean;
  aktiven: boolean;
  naSmenu: (vkl: boolean) => void;
  podpis: string;
}) {
  return (
    <input
      type="checkbox"
      role="switch"
      aria-checked={vkl}
      aria-label={podpis}
      data-testid={testId}
      checked={vkl}
      disabled={!aktiven}
      onChange={(e) => naSmenu(e.target.checked)}
      className="tumbler"
    />
  );
}

export type TonTega = "obychnyy" | "akcent" | "preduprezhdenie" | "opasnost";

const TON: Record<TonTega, string> = {
  obychnyy: "bg-fill text-fg-secondary",
  akcent: "bg-accent-ink/15 text-accent-ink",
  preduprezhdenie: "bg-warn/12 text-warn",
  opasnost: "bg-danger/12 text-danger",
};

/** Small label chip next to a list row: "нет в подписке" and friends. */
export function Teg({ ton = "obychnyy", testId, children }: { ton?: TonTega; testId?: string; children: ReactNode }) {
  return (
    <span data-testid={testId} className={`shrink-0 whitespace-nowrap rounded-md px-1.5 py-0.5 text-[11px] ${TON[ton]}`}>
      {children}
    </span>
  );
}

/** Segmented control: a radio group drawn as one pill. */
export function Segment<T extends string>({ znacheniya, vybrano, naVybor, aktiven = true, rastyanut = false, ton = "yarkiy", "aria-label": podpis }: {
  znacheniya: { z: T; podpis: string; disabled?: boolean }[];
  vybrano: T;
  naVybor: (z: T) => void;
  aktiven?: boolean;
  rastyanut?: boolean;
  /** Два веса выбранного сегмента. Яркий там, где выбор это действие; тихий
   *  там, где это режим, живущий постоянно: постоянная заливка в полную силу
   *  сама становится шумом. */
  ton?: "yarkiy" | "tihiy";
  "aria-label"?: string;
}) {
  return (
    <div role="radiogroup" aria-label={podpis} onKeyDown={aktiven ? strelkiRadio(znacheniya, vybrano, naVybor) : undefined}
      className={`bg-elevated border-border inline-flex rounded-lg border p-1 text-[13px] ${rastyanut ? "w-full" : ""}`}>
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
            className={`h-8 rounded-md px-4 font-medium transition-colors ${rastyanut ? "flex-1" : ""} ${
              on
                ? (ton === "yarkiy" ? "bg-accent/70 text-foreground" : "bg-accent/45 text-foreground")
                : "text-fg-secondary hover:bg-fill-subtle hover:text-foreground disabled:opacity-40"
            }`}
          >
            {p}
          </button>
        );
      })}
    </div>
  );
}

/** Text field, same height as a button so the two sit in one line.
 *
 *  `priv` exists so a screen can put the caret where the human is about to
 *  type. Nothing else reaches into the input from outside. */
export function Pole({ testId, znachenie, naVvod, placeholder, aktiven = true, tip = "text", oshibka = false, priv, "aria-label": podpis, className = "" }: {
  testId?: string;
  znachenie: string;
  naVvod: (v: string) => void;
  placeholder?: string;
  aktiven?: boolean;
  /** password прячет знаки и не зовёт браузерную память паролей. */
  tip?: "text" | "search" | "password";
  /** Значение не годится: поле так и объявляется голосом. Текст ошибки
   *  экран ставит рядом сам. */
  oshibka?: boolean;
  priv?: Ref<HTMLInputElement>;
  "aria-label"?: string;
  className?: string;
}) {
  return (
    <input
      type={tip}
      ref={priv}
      data-testid={testId}
      aria-label={podpis}
      aria-invalid={oshibka || undefined}
      autoComplete={tip === "password" ? "off" : undefined}
      value={znachenie}
      disabled={!aktiven}
      placeholder={placeholder}
      spellCheck={false}
      onChange={(e) => naVvod(e.target.value)}
      className={`bg-elevated border-border text-foreground placeholder:text-fg-faint hover:border-border-hover focus:border-border-active h-9 min-w-0 rounded-md border px-3 text-[13px] disabled:opacity-40 ${className}`}
    />
  );
}

/** Многострочное поле (26.09.2026): ключи вставляют пачкой, по строке на
 *  сервер, и однострочное поле склеивало их в одну строку. Облик тот же, что
 *  у Pole, чтобы два поля рядом не выглядели разными наборами. */
export function PoleTeksta({ testId, znachenie, naVvod, placeholder, aktiven = true, tolkoChtenie = false, strok = 4, priv, "aria-label": podpis, className = "" }: {
  testId?: string;
  znachenie: string;
  naVvod?: (v: string) => void;
  placeholder?: string;
  aktiven?: boolean;
  tolkoChtenie?: boolean;
  strok?: number;
  priv?: Ref<HTMLTextAreaElement>;
  "aria-label"?: string;
  className?: string;
}) {
  return (
    <textarea
      ref={priv}
      data-testid={testId}
      aria-label={podpis}
      value={znachenie}
      disabled={!aktiven}
      readOnly={tolkoChtenie}
      placeholder={placeholder}
      rows={strok}
      spellCheck={false}
      onChange={(e) => naVvod?.(e.target.value)}
      className={`bg-elevated border-border text-foreground placeholder:text-fg-faint hover:border-border-hover focus:border-border-active min-w-0 resize-y rounded-md border px-3 py-2 text-[13px] leading-5 break-all disabled:opacity-40 ${className}`}
    />
  );
}

/** Полоса ожидания.
 *
 *  `dolya` в процентах, когда считать есть из чего; `null` оставляет полосу
 *  бегущей. Второе не украшение: на сверке хеша, распаковке и установке службы
 *  считать нечего, а строка без единого движущегося пикселя неотличима от
 *  зависшей программы. Живой отзыв 13.09.2026 про обновление был ровно об этом.
 *
 *  Разметка одна на все места: у обновления в настройках и у установки службы
 *  на первом запуске полоса обязана выглядеть одинаково. */
export function Polosa({ dolya, podpis, testId }: {
  dolya?: number | null;
  /** Идёт в aria-label: у полосы обязано быть имя, иначе она немая. */
  podpis: string;
  testId?: string;
}) {
  const znaem = typeof dolya === "number";
  return (
    <span
      role="progressbar"
      data-testid={testId}
      aria-label={podpis}
      aria-valuemin={0}
      aria-valuemax={100}
      {...(znaem ? { "aria-valuenow": dolya } : {})}
      className="bg-fill-subtle relative block h-1 w-full max-w-[220px] overflow-hidden rounded-full"
    >
      <span
        className={znaem ? "bg-accent absolute inset-y-0 left-0 rounded-full transition-[width] duration-300" : "bg-accent af-hod-begushchaya absolute inset-y-0 rounded-full"}
        style={znaem ? { width: `${dolya}%` } : undefined}
      />
    </span>
  );
}

// A hyphen, not an em dash: banned everywhere, screens included.
export const PROCHERK = "-";

/** The bin: delete affordance on servers and rules, one drawing for both. */
export function IkonkaKorzina() {
  return <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" /></svg>;
}
