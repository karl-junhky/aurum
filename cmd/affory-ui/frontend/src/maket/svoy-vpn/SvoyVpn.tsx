import type { ReactNode } from "react";
import { IkGalka, IkObnovit, IkPlyus, IkServer, IkZakryt } from "../../ikonki";
import {
  Flazhok, Knopka, MenyuDeystviy, Neudacha, Panel, Pole, PoleTeksta, Ryad, Segment, Svorachivaemyy, Teg, Vertushka,
  type TonTega,
} from "../../ekrany/ui";
import { slovoPosleChisla } from "../../chisla";

// Шаблон раздела «Свой VPN» (план 2026-11-01, §4 и блок Ж). Только разметка:
// данные и действия приходят пропсами, к службе ничего не подключено. Карта
// встраивания в окно лежит в плане, §4.1.

export type ShagMastera = "dostup" | "proverka" | "ustanovka" | "gotovo";

export const SHAGI: { shag: ShagMastera; imya: string }[] = [
  { shag: "dostup", imya: "Доступ" },
  { shag: "proverka", imya: "Проверка" },
  { shag: "ustanovka", imya: "Установка" },
  { shag: "gotovo", imya: "Готово" },
];

/** Итог пункта осмотра или этапа установки. */
export type Itog = "da" | "vnimanie" | "net" | "idyot" | "zhdyot";

export interface Punkt { imya: string; itog: Itog; podrobno: string }

export interface Etap { id: "programma" | "sertifikat" | "porty" | "klyuchi"; imya: string; itog: Itog; podrobno: string }

export type ItogPorta = "proshyol" | "ne-proshyol" | "probuyu" | "v-ocheredi";

export interface ProbaPorta { port: number; itog: ItogPorta; prichina?: string }

export interface PodborPortov { reality: ProbaPorta[]; hy2: ProbaPorta[]; seychas: string }

export interface KlyuchSvoego { imya: string; proto: "TCP" | "UDP"; port: number; proverka: string }

export type SostoyanieServera = "horosho" | "vnimanie" | "slomano";

export interface SvoyServer {
  adres: string;
  sostoyanie: SostoyanieServera;
  prichina?: string;
  sovet?: string;
  klyuchi: KlyuchSvoego[];
  sertifikatDo: string;
  trafik: string;
  vhodPoParolyu: boolean;
  programma: string;
  provereno: string;
  /** Запись пришла с профилем другого компьютера, ключа управления здесь нет. */
  bezKlyuchaUpravleniya?: boolean;
  /** Ключ хоста не тот, что при установке: всё стоит до решения человека. */
  klyuchHostaSmenilsya?: boolean;
}

export interface Dostup {
  soobshchenie: string;
  adres: string;
  portSsh: string;
  polzovatel: string;
  sposob: "parol" | "klyuch";
  parol: string;
  /** Имя выбранного файла ключа: само содержимое окно читает и передаёт
   *  службе отдельно (план, §3.6). Пусто, пока файл не выбран. */
  klyuchFayl: string;
  /** Пароль самого ключа, если ключ зашифрован. */
  parolKlyucha: string;
  portReality: string;
  portHy2: string;
}

export interface Otkaz { zagolovok: string; tekst: string }

/** Действия карточки своего сервера (план, блок З). */
export type DeystvieServera =
  | "proverit" | "vzyat" | "pereustanovil" | "podobrat" | "zadat" | "otchyot" | "snesti" | "zabyt" | "vhod";

/** Порт SSH из поля: пусто значит 22. */
export function portSsh(d: Dostup): number {
  const t = d.portSsh.trim();
  return t === "" ? 22 : Number(t);
}

/** Порт, заданный руками. Пусто значит значение по умолчанию (22 для SSH,
 *  «подобрать» для ключа), иначе число 1-65535 без нулей впереди. `zanyat`
 *  это порт SSH сервера: ключ на нём не встанет (план, К1). Ответ это текст
 *  ошибки или null. */
export function oshibkaPorta(s: string, zanyat?: number): string | null {
  const t = s.trim();
  if (t === "") return null;
  if (!/^[1-9]\d{0,4}$/.test(t) || Number(t) > 65535) return "число от 1 до 65535";
  if (zanyat !== undefined && Number(t) === zanyat) return `${zanyat} занят входом по SSH`;
  return null;
}

const SLOVO_ITOGA: Record<Itog, string> = {
  da: "готово",
  vnimanie: "есть замечание",
  net: "не прошло",
  idyot: "идёт",
  zhdyot: "впереди",
};

const SLOVO_PORTA: Record<ItogPorta, string> = {
  proshyol: "прошёл",
  "ne-proshyol": "не прошёл",
  probuyu: "проверяется",
  "v-ocheredi": "в очереди",
};

const SOSTOYANIE: Record<SostoyanieServera, { ton: TonTega; slovo: string; kromka?: TonPlashki }> = {
  horosho: { ton: "akcent", slovo: "работает" },
  vnimanie: { ton: "preduprezhdenie", slovo: "нужно внимание", kromka: "warn" },
  slomano: { ton: "opasnost", slovo: "не работает", kromka: "danger" },
};

// В наборе значков нет восклицания, а рисовать его одним штрихом с ними
// дешевле, чем заводить ради шаблона новый значок в ikonki.tsx.
function IkVnimanie({ className }: { className: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" aria-hidden="true">
      <circle cx="12" cy="12" r="8.4" />
      <path d="M12 7.8v5M12 16.2h.01" />
    </svg>
  );
}

function ZnachokItoga({ itog }: { itog: Itog }) {
  const znachok = {
    da: <IkGalka className="text-accent-ink h-4 w-4" />,
    vnimanie: <IkVnimanie className="text-warn h-4 w-4" />,
    net: <IkZakryt className="text-danger h-4 w-4" strokeWidth={2} />,
    idyot: <Vertushka className="text-accent-ink h-4 w-4" />,
    zhdyot: <span className="border-border-active h-2 w-2 rounded-full border" />,
  }[itog];
  return (
    <>
      {znachok}
      <span className="sr-only">{SLOVO_ITOGA[itog]}</span>
    </>
  );
}

/** Колонка и шапка раздела. Ритм тот же, что у настроек (Nastroyki.tsx:215):
 *  раздел в полосе, а не экран внутри другого раздела. */
function Kolonka({ svodka, deystviya, children }: { svodka: string; deystviya?: ReactNode; children: ReactNode }) {
  return (
    <section aria-label="Свой VPN" className="mx-auto flex w-full max-w-[940px] flex-col gap-7 px-8 py-7">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-foreground text-[26px] font-semibold leading-tight">Свой VPN</h2>
          <p className="text-fg-muted mt-1.5 text-sm">{svodka}</p>
        </div>
        {deystviya && <div className="flex shrink-0 gap-2">{deystviya}</div>}
      </header>
      {children}
    </section>
  );
}

// Копия Gruppa из Nastroyki.tsx:824. При переносе обе уходят в ui-novye.tsx
// одной общей, вторую копию в коде не оставлять.
function Gruppa({ nazvanie, poyasnenie, children }: { nazvanie: string; poyasnenie?: string; children: ReactNode }) {
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

function Podval({ sleva, sprava }: { sleva?: ReactNode; sprava: ReactNode }) {
  return (
    <footer className="flex items-center justify-between gap-4">
      <div className="min-w-0">{sleva}</div>
      <div className="flex shrink-0 items-center gap-2">{sprava}</div>
    </footer>
  );
}

function PolePodpisannoe({ podpis, children }: { podpis: string; children: ReactNode }) {
  return (
    <label className="flex min-w-0 flex-col gap-1.5">
      <span className="text-fg-secondary text-xs font-medium">{podpis}</span>
      {children}
    </label>
  );
}

// ---------------------------------------------------------------- пусто

const PODOYDYOT = ["Виртуализация KVM", "Свой адрес IPv4", "Ubuntu 20.04 или новее", "От 1 ГБ памяти"];
const NE_PODOYDYOT = ["NAT: один адрес на много серверов", "Только IPv6, без IPv4", "OpenVZ и LXC"];

function SpisokUsloviy({ zagolovok, punkty, goditsya }: { zagolovok: string; punkty: string[]; goditsya: boolean }) {
  return (
    <div className="flex flex-col gap-3 p-5">
      <h4 className="text-fg-secondary text-xs font-medium">{zagolovok}</h4>
      <ul className="flex flex-col gap-2.5">
        {punkty.map((p) => (
          <li key={p} className="text-foreground flex items-center gap-2.5 text-sm">
            {goditsya
              ? <IkGalka className="text-accent-ink h-4 w-4 shrink-0" />
              : <IkZakryt className="text-fg-faint h-4 w-4 shrink-0" strokeWidth={1.8} />}
            <span className={goditsya ? undefined : "text-fg-secondary"}>{p}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function SvoyVpnPusto({ naPostavit }: { naPostavit: () => void }) {
  return (
    <Kolonka svodka="Affory поставит VPN на купленный тобой сервер и сам добавит два ключа в список">
      <Gruppa nazvanie="Какой сервер купить" poyasnenie="Проверь до оплаты: что сервер не подходит, станет видно только после неё">
        <Panel>
          <div className="grid sm:grid-cols-2">
            <SpisokUsloviy zagolovok="Подойдёт" punkty={PODOYDYOT} goditsya />
            <div className="border-border border-t sm:border-t-0 sm:border-l">
              <SpisokUsloviy zagolovok="Не подойдёт" punkty={NE_PODOYDYOT} goditsya={false} />
            </div>
          </div>
          <div className="border-border flex flex-wrap items-center gap-4 border-t px-5 py-4">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <span className="text-foreground text-sm font-medium">Понадобится письмо хостера с адресом и паролем</span>
              <span className="text-fg-muted flex items-center gap-1.5 text-xs">
                <IkVnimanie className="text-warn h-3.5 w-3.5 shrink-0" />
                Сервер нужен пустой: Affory займёт его целиком, сайт или другой VPN на нём помешают
              </span>
            </div>
            <Knopka rang="glavnaya" bolshaya onClick={naPostavit}>Поставить на свой сервер</Knopka>
          </div>
        </Panel>
      </Gruppa>
    </Kolonka>
  );
}

// ---------------------------------------------------------------- мастер

export function PolosaShagov({ shag }: { shag: ShagMastera }) {
  const nomer = SHAGI.findIndex((s) => s.shag === shag);
  return (
    <ol aria-label="Шаги установки" className="flex items-center gap-3">
      {SHAGI.map((s, i) => {
        const proyden = i < nomer;
        const tekushchiy = i === nomer;
        const krug = proyden
          ? "border-accent-ink/50 text-accent-ink"
          : tekushchiy ? "border-accent-ink bg-accent-soft text-accent-ink" : "border-border-hover text-fg-faint";
        const tekst = tekushchiy ? "text-foreground" : proyden ? "text-fg-secondary" : "text-fg-faint";
        return (
          <li key={s.shag} aria-current={tekushchiy ? "step" : undefined} className="flex flex-1 items-center gap-3 last:flex-none">
            <span className="flex shrink-0 items-center gap-2">
              <span className={`flex h-6 w-6 items-center justify-center rounded-full border text-xs font-semibold tabular-nums ${krug}`}>
                {proyden ? <IkGalka className="h-3.5 w-3.5" /> : i + 1}
              </span>
              <span className={`text-sm font-medium ${tekst}`}>{s.imya}</span>
              {proyden && <span className="sr-only">пройден</span>}
            </span>
            {i < SHAGI.length - 1 && <span aria-hidden className={`h-px min-w-4 flex-1 ${proyden ? "bg-accent-ink/40" : "bg-border"}`} />}
          </li>
        );
      })}
    </ol>
  );
}

export function SvoyVpnMaster({ shag, adres, children }: { shag: ShagMastera; adres?: string; children: ReactNode }) {
  return (
    <Kolonka svodka={adres ? `Установка на ${adres}` : "Установка на свой сервер"}>
      <PolosaShagov shag={shag} />
      {children}
    </Kolonka>
  );
}

type StrokovoePole = Exclude<keyof Dostup, "sposob">;

// Поле пароля. У Pole нет типа password, при переносе он добавляется туда, а
// эта копия классов уходит.
function PoleParolya({ znachenie, naVvod }: { znachenie: string; naVvod: (v: string) => void }) {
  return (
    <input
      type="password"
      value={znachenie}
      autoComplete="off"
      spellCheck={false}
      onChange={(e) => naVvod(e.target.value)}
      className="bg-elevated border-border text-foreground placeholder:text-fg-faint hover:border-border-hover focus:border-border-active h-9 min-w-0 rounded-md border px-3 text-[13px]"
    />
  );
}

// Ошибка стоит после подписи, а не внутри неё: иначе она входит в имя поля.
// У Pole нет aria-invalid, при переносе он добавляется туда.
function PolePorta({ podpis, znachenie, naVvod, placeholder, zanyat }: {
  podpis: string;
  znachenie: string;
  naVvod: (v: string) => void;
  placeholder: string;
  zanyat?: number;
}) {
  const oshibka = oshibkaPorta(znachenie, zanyat);
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <PolePodpisannoe podpis={podpis}>
        <Pole znachenie={znachenie} naVvod={naVvod} placeholder={placeholder} className="tabular-nums" />
      </PolePodpisannoe>
      {oshibka && <span role="alert" className="text-danger text-xs">{oshibka}</span>}
    </div>
  );
}

export function ShagDostup({ dostup, naSmenu, buferSParolem, naOchistitBufer, naVybratKlyuch, naOtmenu, naDalshe }: {
  dostup: Dostup;
  naSmenu: (d: Dostup) => void;
  /** В буфере обмена лежит вставленное сообщение: Win+V хранит его журналом. */
  buferSParolem: boolean;
  naOchistitBufer: () => void;
  /** Окно открывает выбор файла, читает ключ само и кладёт имя в klyuchFayl. */
  naVybratKlyuch: () => void;
  naOtmenu: () => void;
  naDalshe: () => void;
}) {
  const pole = (k: StrokovoePole) => (v: string) => naSmenu({ ...dostup, [k]: v });
  const vhod = dostup.sposob === "parol" ? dostup.parol !== "" : dostup.klyuchFayl !== "";
  const ssh = portSsh(dostup);
  const portyVerny = oshibkaPorta(dostup.portSsh) === null
    && oshibkaPorta(dostup.portReality, ssh) === null
    && oshibkaPorta(dostup.portHy2) === null;
  const gotovo = dostup.adres.trim() !== "" && dostup.polzovatel.trim() !== "" && vhod && portyVerny;
  return (
    <>
      <Panel>
        <div className="grid gap-6 p-5 lg:grid-cols-2">
          <div className="flex min-w-0 flex-col gap-2">
            <PolePodpisannoe podpis="Сообщение от хостера">
              <PoleTeksta
                strok={8}
                znachenie={dostup.soobshchenie}
                naVvod={pole("soobshchenie")}
                placeholder="Вставь письмо или сообщение хостера с адресом, логином и паролем"
              />
            </PolePodpisannoe>
            <p className="text-fg-muted text-xs">Разбирается на этом компьютере и никуда не уходит</p>
            {buferSParolem && (
              <p className="text-fg-secondary flex items-center gap-1 text-xs">
                Пароль остался в буфере обмена
                <Knopka rang="tekst" onClick={naOchistitBufer}>Очистить буфер</Knopka>
              </p>
            )}
          </div>
          <div className="flex min-w-0 flex-col gap-4">
            <div className="grid grid-cols-[1fr_96px] items-start gap-3">
              <PolePodpisannoe podpis="Адрес сервера">
                <Pole znachenie={dostup.adres} naVvod={pole("adres")} placeholder="203.0.113.10" />
              </PolePodpisannoe>
              <PolePorta podpis="Порт SSH" znachenie={dostup.portSsh} naVvod={pole("portSsh")} placeholder="22" />
            </div>
            <PolePodpisannoe podpis="Пользователь">
              <Pole znachenie={dostup.polzovatel} naVvod={pole("polzovatel")} placeholder="root" />
            </PolePodpisannoe>
            <div className="flex flex-col gap-2">
              {/* Обёртка держит выбор по своей ширине: в колонке flex его
                  растянуло бы на всю. */}
              <div>
                <Segment
                  aria-label="Как войти"
                  ton="tihiy"
                  znacheniya={[{ z: "parol", podpis: "Пароль" }, { z: "klyuch", podpis: "Ключ" }]}
                  vybrano={dostup.sposob}
                  // Секрет другого способа стирается сразу: пароль живёт только
                  // для входа по паролю (план, §3.6) и не едет рядом с ключом.
                  naVybor={(z) => naSmenu(z === "klyuch"
                    ? { ...dostup, sposob: z, parol: "" }
                    : { ...dostup, sposob: z, klyuchFayl: "", parolKlyucha: "" })}
                />
              </div>
              {dostup.sposob === "parol" ? (
                <>
                  <PolePodpisannoe podpis="Пароль">
                    <PoleParolya znachenie={dostup.parol} naVvod={pole("parol")} />
                  </PolePodpisannoe>
                  <p className="text-fg-muted text-xs">Нужен один раз: Affory войдёт, положит свой ключ и пароль забудет</p>
                </>
              ) : (
                <>
                  <div className="flex flex-wrap items-center gap-2">
                    <Knopka rang="vtoraya" onClick={naVybratKlyuch}>{dostup.klyuchFayl ? "Другой файл" : "Выбрать файл ключа"}</Knopka>
                    <span className={dostup.klyuchFayl ? "text-foreground break-all text-xs" : "text-fg-muted text-xs"}>
                      {dostup.klyuchFayl || "закрытый ключ, например id_ed25519"}
                    </span>
                  </div>
                  {dostup.klyuchFayl && (
                    <PolePodpisannoe podpis="Пароль ключа, если он есть">
                      <PoleParolya znachenie={dostup.parolKlyucha} naVvod={pole("parolKlyucha")} />
                    </PolePodpisannoe>
                  )}
                </>
              )}
            </div>
          </div>
        </div>
        <div className="px-5">
          <Svorachivaemyy
            zagolovok="Дополнительно"
            poyasnenie="порты ключей"
            deti={(
              <div className="flex flex-col gap-3">
                <div className="grid max-w-[420px] grid-cols-2 items-start gap-3">
                  {/* SSH живёт на TCP, поэтому его порт закрыт только для
                      reality: hysteria2 на UDP с ним не делит ничего. */}
                  <PolePorta podpis="reality, TCP" znachenie={dostup.portReality} naVvod={pole("portReality")} placeholder="подобрать" zanyat={ssh} />
                  <PolePorta podpis="hysteria2, UDP" znachenie={dostup.portHy2} naVvod={pole("portHy2")} placeholder="подобрать" />
                </div>
                <span className="text-fg-muted text-xs">Пустое поле: Affory сам найдёт порт, который проходит из твоей сети</span>
              </div>
            )}
          />
        </div>
      </Panel>
      <Podval
        sprava={(
          <>
            <Knopka rang="tekst" onClick={naOtmenu}>Отмена</Knopka>
            <Knopka rang="glavnaya" bolshaya aktiven={gotovo} onClick={naDalshe}>Проверить сервер</Knopka>
          </>
        )}
      />
    </>
  );
}

export function ShagProverka({ punkty, otpechatok, otkaz, pustoy, naPustoy, naNazad, naDalshe }: {
  punkty: Punkt[];
  otpechatok: string;
  otkaz?: Otkaz;
  pustoy: boolean;
  naPustoy: (v: boolean) => void;
  naNazad: () => void;
  naDalshe: () => void;
}) {
  return (
    <>
      {otkaz && <Neudacha zagolovok={otkaz.zagolovok} tekst={otkaz.tekst} deystvie={naNazad} podpisDeystviya="Ввести другой сервер" />}
      <div className="flex flex-col gap-2">
        <Panel>
          {punkty.map((p) => (
            <Ryad key={p.imya} znachok={<ZnachokItoga itog={p.itog} />} nazvanie={p.imya} poyasnenie={p.podrobno} aktiven={p.itog !== "zhdyot"} />
          ))}
        </Panel>
        <p className="text-fg-faint break-all text-xs" title="Отпечаток запомнен. Если при следующем входе он будет другим, Affory остановится до твоего решения">
          Ключ сервера {otpechatok}
        </p>
      </div>
      {/* При отказе выход один, кнопка в Neudacha выше: второй «Назад» с тем
          же действием заставлял бы гадать, чем они отличаются. */}
      {!otkaz && (
        <>
          <Flazhok vkl={pustoy} naSmenu={naPustoy} podpis="На сервере нет ничего нужного, Affory может занять его целиком" />
          <Podval
            sleva={<Knopka rang="tekst" onClick={naNazad}>Назад</Knopka>}
            sprava={<Knopka rang="glavnaya" bolshaya aktiven={pustoy} onClick={naDalshe}>Поставить VPN</Knopka>}
          />
        </>
      )}
    </>
  );
}

const VID_PORTA: Record<ItogPorta, string> = {
  proshyol: "border-accent-ink/50 text-foreground",
  "ne-proshyol": "border-border text-fg-faint",
  probuyu: "border-border-active bg-fill-subtle text-foreground",
  "v-ocheredi": "border-border border-dashed text-fg-muted",
};

function FishkaPorta({ proba }: { proba: ProbaPorta }) {
  return (
    <li title={proba.prichina} className={`flex h-7 items-center gap-1.5 rounded-md border px-2 text-xs tabular-nums ${VID_PORTA[proba.itog]}`}>
      {proba.port}
      {proba.itog === "proshyol" && <IkGalka className="text-accent-ink h-3.5 w-3.5" />}
      {proba.itog === "ne-proshyol" && <IkZakryt className="h-3.5 w-3.5" strokeWidth={2} />}
      {proba.itog === "probuyu" && <Vertushka className="text-accent-ink h-3 w-3" />}
      {/* Причина и голосом: title читается только мышью. Глазами последнюю
          причину называет строка «что сейчас» под фишками. */}
      <span className="sr-only">{SLOVO_PORTA[proba.itog]}{proba.prichina ? `: ${proba.prichina}` : ""}</span>
    </li>
  );
}

function RyadPortov({ imya, proby }: { imya: string; proby: ProbaPorta[] }) {
  return (
    <div className="flex items-start gap-3">
      <span className="text-fg-secondary w-28 shrink-0 pt-1.5 text-xs">{imya}</span>
      <ul aria-label={`Порты ${imya}`} className="flex flex-wrap gap-1.5">
        {/* Порт может пробоваться повторно (через 2 и 10 минут, §3.5), поэтому
            ключ с номером попытки, а не один номер порта. */}
        {proby.map((p, i) => <FishkaPorta key={`${i}-${p.port}`} proba={p} />)}
      </ul>
    </div>
  );
}

/** Ход подбора: какой порт пробуется, какие уже не прошли (план, §4, шаг 3).
 *  Отступ слева равен значку и зазору строки Ryad: фишки стоят под названием
 *  этапа, а не под значком. */
function HodPodbora({ podbor }: { podbor: PodborPortov }) {
  return (
    <div className="flex flex-col gap-2.5 pr-4 pb-4 pl-[62px]">
      <RyadPortov imya="reality, TCP" proby={podbor.reality} />
      <RyadPortov imya="hysteria2, UDP" proby={podbor.hy2} />
      <p aria-live="polite" className="text-fg-muted text-xs">{podbor.seychas}</p>
    </div>
  );
}

export function ShagUstanovka({ etapy, podbor, otkaz, naOtmenu, naOtchyot, naPovtor }: {
  etapy: Etap[];
  podbor?: PodborPortov;
  otkaz?: Otkaz;
  naOtmenu: () => void;
  naOtchyot: () => void;
  naPovtor: () => void;
}) {
  return (
    <>
      {otkaz && <Neudacha zagolovok={otkaz.zagolovok} tekst={otkaz.tekst} />}
      <Panel>
        {etapy.map((e) => (
          <div key={e.id} className="border-border border-t first:border-t-0">
            <Ryad znachok={<ZnachokItoga itog={e.itog} />} nazvanie={e.imya} poyasnenie={e.podrobno} aktiven={e.itog !== "zhdyot"} />
            {e.id === "porty" && podbor && <HodPodbora podbor={podbor} />}
          </div>
        ))}
      </Panel>
      {/* «Отменить установку» в обоих состояниях на одном месте и одного
          ранга: при отказе к ней справа добавляются отчёт и повтор. */}
      <Podval
        sleva={(
          <span className="text-fg-muted text-xs">
            {otkaz
              ? "Отмена уберёт с сервера всё, что успело встать"
              : "Окно можно закрыть: установка продолжится, мастер вернётся к ней при открытии"}
          </span>
        )}
        sprava={(
          <>
            <Knopka rang="vtoraya" onClick={naOtmenu}>Отменить установку</Knopka>
            {otkaz && (
              <>
                <Knopka rang="vtoraya" onClick={naOtchyot}>Скопировать отчёт</Knopka>
                <Knopka rang="glavnaya" bolshaya onClick={naPovtor}>Попробовать снова</Knopka>
              </>
            )}
          </>
        )}
      />
    </>
  );
}

function RyadKlyucha({ klyuch, znachok, children }: { klyuch: KlyuchSvoego; znachok?: ReactNode; children?: ReactNode }) {
  return (
    <Ryad
      znachok={znachok}
      nazvanie={<span className="flex items-baseline gap-2">{klyuch.imya}<span className="text-fg-muted text-[13px] font-normal tabular-nums">{klyuch.proto} {klyuch.port}</span></span>}
      poyasnenie={klyuch.proverka}
    >
      {children}
    </Ryad>
  );
}

function RyadVhoda({ vkl, naSmenu }: { vkl: boolean; naSmenu: () => void }) {
  return (
    <Ryad
      nazvanie="Вход по паролю"
      poyasnenie={vkl
        ? "Включён: пароль от хостера открывает сервер. Если выключить, войдёт только этот компьютер, а с другого придётся через консоль хостера"
        : "Выключен: войти может только этот компьютер"}
    >
      <Knopka rang="vtoraya" onClick={naSmenu}>{vkl ? "Выключить" : "Включить"}</Knopka>
    </Ryad>
  );
}

export function ShagGotovo({ klyuchi, vhodPoParolyu, naPodklyuchit, naVygruzit, naVhod }: {
  klyuchi: KlyuchSvoego[];
  vhodPoParolyu: boolean;
  naPodklyuchit: () => void;
  /** Существующая выгрузка: exportServers по ids и модалка Vygruzka.tsx. */
  naVygruzit: () => void;
  naVhod: () => void;
}) {
  return (
    <>
      <div className="flex flex-col gap-2">
        <Panel>
          {klyuchi.map((k) => (
            <RyadKlyucha key={`${k.imya}-${k.port}`} klyuch={k} znachok={<ZnachokItoga itog="da" />}>
              <Teg ton="akcent">в списке серверов</Teg>
            </RyadKlyucha>
          ))}
          <div className="border-border flex flex-wrap items-center gap-3 border-t px-4 py-4">
            <Knopka rang="glavnaya" bolshaya onClick={naPodklyuchit}>Подключиться</Knopka>
            <Knopka rang="vtoraya" bolshaya onClick={naVygruzit}>Ключи для телефона</Knopka>
          </div>
        </Panel>
        <p className="text-fg-muted text-xs">
          Порты подобраны из сети этого компьютера. На телефоне с мобильным интернетом какой-то из ключей может не открыться
        </p>
      </div>
      <Gruppa nazvanie="Доступ к серверу">
        <Panel>
          <RyadVhoda vkl={vhodPoParolyu} naSmenu={naVhod} />
        </Panel>
      </Gruppa>
    </>
  );
}

// ---------------------------------------------------------------- после установки

type TonPlashki = "warn" | "danger";

const KROMKA: Record<TonPlashki, string> = { warn: "border-l-[3px] border-l-warn", danger: "border-l-[3px] border-l-danger" };

/** Строка-сообщение внутри карточки. Без тона кромки нет: цвет только там,
 *  где есть что сигналить. */
function Plashka({ ton, zagolovok, tekst, children }: { ton?: TonPlashki; zagolovok: string; tekst?: string; children?: ReactNode }) {
  return (
    <div className={`border-border flex flex-wrap items-center gap-4 border-t px-4 py-3.5 ${ton ? KROMKA[ton] : ""}`}>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-foreground text-sm font-medium">{zagolovok}</span>
        {tekst && <span className="text-fg-muted text-xs">{tekst}</span>}
      </div>
      {children}
    </div>
  );
}

export function KartochkaServera({ server, naDeystvie }: { server: SvoyServer; naDeystvie: (d: DeystvieServera) => void }) {
  const s = SOSTOYANIE[server.sostoyanie];
  const upravlyaem = !server.bezKlyuchaUpravleniya && !server.klyuchHostaSmenilsya;
  return (
    <Panel>
      <div className="flex items-center gap-3.5 px-4 py-4">
        <span className="text-fg-muted flex h-8 w-8 shrink-0 items-center justify-center"><IkServer className="h-[18px] w-[18px]" /></span>
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex items-center gap-2">
            <span className="text-foreground text-[15px] font-semibold tabular-nums">{server.adres}</span>
            <Teg ton={s.ton}>{s.slovo}</Teg>
          </span>
          <span className="text-fg-muted text-xs">Проверен {server.provereno}</span>
        </div>
        <Knopka rang="vtoraya" aktiven={upravlyaem} onClick={() => naDeystvie("proverit")}><IkObnovit className="h-3.5 w-3.5" />Проверить</Knopka>
        <MenyuDeystviy
          podpis="Действия с сервером"
          punkty={[
            { podpis: "Подобрать порты заново", aktiven: upravlyaem, naZhmyh: () => naDeystvie("podobrat") },
            { podpis: "Задать порты", aktiven: upravlyaem, naZhmyh: () => naDeystvie("zadat") },
            { podpis: "Скопировать отчёт", naZhmyh: () => naDeystvie("otchyot") },
            { podpis: "Снести с сервера", opasnyy: true, aktiven: upravlyaem, naZhmyh: () => naDeystvie("snesti") },
            { podpis: "Забыть сервер", opasnyy: true, naZhmyh: () => naDeystvie("zabyt") },
          ]}
        />
      </div>
      {server.klyuchHostaSmenilsya && (
        <Plashka
          ton="danger"
          zagolovok="Ключ сервера сменился, Affory остановился и ничего не отправил"
          tekst="Так бывает после переустановки системы в панели хостера. Если ты её не переустанавливал, на сервер не заходи и напиши хостеру"
        >
          <Knopka rang="vtoraya" onClick={() => naDeystvie("pereustanovil")}>Я переустановил систему</Knopka>
        </Plashka>
      )}
      {server.bezKlyuchaUpravleniya && (
        <Plashka
          ton="warn"
          zagolovok="Ключ управления остался на другом компьютере"
          tekst="Ключи VPN работают. Чтобы проверять и менять сервер отсюда, войди паролем от хостера или своим ключом"
        >
          <Knopka rang="vtoraya" onClick={() => naDeystvie("vzyat")}>Взять под управление</Knopka>
        </Plashka>
      )}
      {server.prichina && <Plashka ton={s.kromka} zagolovok={server.prichina} tekst={server.sovet} />}
      <div className="border-border border-t">
        {server.klyuchi.map((k) => <RyadKlyucha key={`${k.imya}-${k.port}`} klyuch={k} />)}
        <Ryad nazvanie="Сертификат" poyasnenie="Let's Encrypt на адрес сервера, продлевается сам">
          <span className="text-fg-secondary text-[13px] tabular-nums">до {server.sertifikatDo}</span>
        </Ryad>
        <Ryad nazvanie="Трафик за 30 дней" poyasnenie="по журналу программы на сервере">
          <span className="text-fg-secondary text-[13px] tabular-nums">{server.trafik}</span>
        </Ryad>
        {upravlyaem && <RyadVhoda vkl={server.vhodPoParolyu} naSmenu={() => naDeystvie("vhod")} />}
        <Ryad nazvanie="Программа на сервере">
          <span className="text-fg-secondary text-[13px] tabular-nums">{server.programma}</span>
        </Ryad>
      </div>
    </Panel>
  );
}

export function SvoyVpnServery({ servery, naPostavit, kartochka }: {
  servery: SvoyServer[];
  naPostavit: () => void;
  kartochka: (s: SvoyServer) => ReactNode;
}) {
  const n = servery.length;
  return (
    <Kolonka
      svodka={n === 1
        ? "Ключи сервера лежат и в общем списке серверов"
        : `${n} ${slovoPosleChisla(n, "сервер", "сервера", "серверов")}, ключи каждого лежат и в общем списке`}
      deystviya={<Knopka rang="vtoraya" bolshaya onClick={naPostavit}><IkPlyus className="h-4 w-4" />Поставить ещё</Knopka>}
    >
      {servery.map((s) => <div key={s.adres}>{kartochka(s)}</div>)}
    </Kolonka>
  );
}
