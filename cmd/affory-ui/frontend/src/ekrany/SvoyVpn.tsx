import { useEffect, useRef, useState, type ReactNode } from "react";
import { IkGalka, IkObnovit, IkPlyus, IkServer, IkVnimanie, IkZakryt } from "../ikonki";
import {
  Flazhok, Gruppa, Knopka, MenyuDeystviy, Neudacha, OknoPodtverzhdeniya, Panel, Pole, PoleTeksta, Ryad, Segment,
  Svorachivaemyy, Teg, Vertushka, type TonTega,
} from "./ui";
import { slovoPosleChisla } from "../chisla";
import { razobratSoobshchenie } from "../soobshchenieHostera";
import type { SvoyaTrevoga } from "../protokol";
import { tekstOshibki } from "../ponyatno";
import { podpisDeystviya, tekstOtkaza } from "./otkazy";
import { Vygruzka, type VygruzkaOtvet } from "./Vygruzka";
import { vozrast, vygruzkaIz } from "./Servery";
import {
  ocenitServer, portyNyneshnie, PUSTAYA_FORMA, svezhayaProverka, zapomnitUvidennuyu,
  type Forma, type HodVps, type KlyuchFayla, type KlyuchHoda, type Master, type OsmotrVps, type OtkazVps,
  type ProverkaVps, type SostoyanieServera, type SvoyServer, type SvoyVps,
} from "../svoyVps";

// Раздел «Свой VPN» (план 2026-11-01, §4 и Ж2): пустое состояние, мастер
// установки страницей вкладки и карточки поставленных серверов. Данные и
// команды приходят из useSvoyVps (App), экран мост не знает.

export interface SvoyVpnProps {
  svoy: SvoyVps;
  trevogi: SvoyaTrevoga[];
  /** Служба на связи: без неё дела не начинаются. */
  aktiven: boolean;
  naPodklyuchit: (id: string) => void;
  naServery: () => void;
  skopirovat: (tekst: string) => Promise<void>;
  kodyQr: (tekst: string) => Promise<string[]>;
  /** Выбор файла ключа SSH: null - человек закрыл диалог. */
  vybratKlyuch: () => Promise<KlyuchFayla | null>;
  /** Очищает буфер, если в нём ещё лежит этот текст: true - очищен. */
  ochistitBufer: (tekst: string) => Promise<boolean>;
  /** «Affory 1.11.0, Windows 11 (26200)» для отчёта. */
  versiya?: string;
}

// ---------------------------------------------------------------- словарь

/** Итог пункта осмотра или этапа установки. */
export type Itog = "da" | "vnimanie" | "net" | "idyot" | "zhdyot";

const SLOVO_ITOGA: Record<Itog, string> = {
  da: "готово",
  vnimanie: "есть замечание",
  net: "не прошло",
  idyot: "идёт",
  zhdyot: "впереди",
};

const SOSTOYANIE: Record<SostoyanieServera, { ton: TonTega; slovo: string; kromka?: TonPlashki }> = {
  horosho: { ton: "akcent", slovo: "работает" },
  vnimanie: { ton: "preduprezhdenie", slovo: "нужно внимание", kromka: "warn" },
  slomano: { ton: "opasnost", slovo: "не работает", kromka: "danger" },
};

/** Вид ключа по транспорту ссылки и по протоколу подбора. */
function vidKlyucha(transport: string): { imya: string; proto: "TCP" | "UDP" } {
  return transport === "hy2" || transport === "udp" ? { imya: "hysteria2", proto: "UDP" } : { imya: "reality", proto: "TCP" };
}

const ITOG_PROVERKI: Record<string, string> = {
  vezyot: "провёз 2 МБ",
  "net-otveta": "не ответил",
  zamerz: "ответ пошёл и встал",
  "chuzhoy-vyhod": "выходит в сеть не с адреса сервера",
  cel: "проверка не закончена",
};

// Начало отказа проверки, которое повторяет слово итога: «не ответил: нет
// ответа через ключ: …» читалось тремя слоями одного смысла.
const POVTOR_OTKAZA: Record<string, string> = {
  "net-otveta": "нет ответа через ключ: ",
  zamerz: "поток встал: ",
};

function itogKlyucha(k: KlyuchHoda): string {
  if (!k.vid) return "не проверялся";
  const slovo = ITOG_PROVERKI[k.vid] ?? k.vid;
  // Перехват порта на этом компьютере объясняет отказ, а слово ядра нет.
  if (k.pomeha && k.vid !== "vezyot") return `${slovo}: порт перехватывает ${k.pomeha} на этом компьютере`;
  // Адрес выхода нужен отчёту, а не подписи: слово итога и так говорит всё.
  if (!k.otkaz || k.vid === "vezyot" || k.vid === "chuzhoy-vyhod") return slovo;
  const nachalo = POVTOR_OTKAZA[k.vid];
  const otkaz = nachalo && k.otkaz.startsWith(nachalo) ? k.otkaz.slice(nachalo.length) : k.otkaz;
  return `${slovo}: ${otkaz}`;
}

/** Время по часам компьютера: «20:24». */
function chasy(iso: string): string {
  return new Date(iso).toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
}

/** Когда детектор проверит адрес ещё раз: время, а не отсчёт, который без
 *  событий службы стоял бы на месте. */
function povtoryTekstom(h: HodVps): string {
  if (h.povtory.length === 0) return "";
  return `Проверю ещё раз ${h.povtory.map((p) => `в ${chasy(p)}`).join(" и ")}`;
}

/** Что сказать о ключе, пока у него нет итога проверки. */
function bezProverki(k: KlyuchHoda, u: { ne_iz_seti: string[] }, idyot: boolean): string {
  if (u.ne_iz_seti.includes(k.transport)) return "из твоей сети не открылся";
  return idyot ? "проверяю" : "не проверялся";
}

function prichinaOtseva(prichina: string, process?: string): string {
  switch (prichina) {
    case "vne": return "вне 1-65535";
    case "ssh": return "на нём вход на сервер";
    case "sert": return "нужен сертификату";
    case "zanyat": return process && process !== "?" ? `занят программой ${process}` : "занят другой программой";
  }
  return prichina;
}

/** Путь до сервера словами окна. Служба пишет его для журнала: «напрямую
 *  через <адаптер>» или вход через свой туннель (ssh/put.go), второй окно
 *  называет VPN. */
function putSlovami(put: string): string {
  return put === "" || put.startsWith("напрямую") ? put : "через VPN";
}

function data(unix: number): string {
  return new Date(unix * 1000).toLocaleDateString("ru-RU", { day: "numeric", month: "long" });
}

/** Мегабайты сервера: от гигабайта гигабайтами с одним знаком. */
function mb(n: number): string {
  return n >= 1024
    ? `${(n / 1024).toLocaleString("ru-RU", { maximumFractionDigits: 1 })} ГБ`
    : `${n.toLocaleString("ru-RU")} МБ`;
}

/** Последний элемент: хук держит дела по возрастанию id. */
function posledniy<T>(a: T[]): T | undefined {
  return a[a.length - 1];
}

function sPropisnoy(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

// ---------------------------------------------------------------- проверки полей

/** Порт SSH из поля: пусто значит 22. */
export function portSshFormy(f: Forma): number {
  const t = f.portSshPolya.trim();
  return t === "" ? 22 : Number(t);
}

/** Порт, заданный руками. Пусто значит значение по умолчанию (22 для SSH,
 *  «подобрать» для ключа), иначе число 1-65535 без нулей впереди. `zanyaty`
 *  это порты, на которых ключ не встанет, с причиной (К1). Ответ это текст
 *  ошибки или null. */
export function oshibkaPorta(s: string, zanyaty: [number, string][] = []): string | null {
  const t = s.trim();
  if (t === "") return null;
  if (!/^[1-9]\d{0,4}$/.test(t) || Number(t) > 65535) return "число от 1 до 65535";
  const z = zanyaty.find(([port]) => port === Number(t));
  return z ? `${z[0]} ${z[1]}` : null;
}

/** На каких TCP-портах reality не встанет: вход SSH и выпуск сертификата.
 *  443 при продлении через него служба отсеет сама: до установки способ
 *  продления неизвестен. */
export function zanyatyeDlyaReality(portSsh: number): [number, string][] {
  const z: [number, string][] = [[22, "занят входом на сервер"], [80, "нужен выпуску сертификата"]];
  if (portSsh !== 22) z.push([portSsh, "занят входом на сервер"]);
  return z;
}

const IPV4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/;

/** Адрес из поля без пробелов и квадратных скобок IPv6. */
export function chistyyAdres(s: string): string {
  return s.trim().replace(/^\[(.*)\]$/, "$1");
}

/** Служба ставит только на IP: на него выпускается сертификат, а имя могло
 *  бы сменить адрес между проверкой и установкой. */
export function oshibkaAdresa(s: string): string | null {
  const a = chistyyAdres(s);
  if (a === "") return null;
  if (IPV4.test(a)) return null;
  if (a.includes(":") && /^[0-9a-f:.]+$/i.test(a)) return null;
  return "нужен IP-адрес из письма хостера, а не имя";
}

/** Тело входа для службы: только нужные поля, сообщение хостера и секрет
 *  другого способа сюда не едут. */
export function teloDostupa(f: Forma): Record<string, unknown> {
  const t: Record<string, unknown> = { adres: chistyyAdres(f.adres), port: portSshFormy(f), polzovatel: f.polzovatel.trim() };
  if (f.sposob === "parol") {
    t.parol = f.parol;
  } else if (f.klyuch) {
    t.klyuch = f.klyuch.base64;
    if (f.parolKlyucha !== "") t.parol_klyucha = f.parolKlyucha;
  }
  return t;
}

function portIzPolya(s: string): number | undefined {
  const t = s.trim();
  return t === "" ? undefined : Number(t);
}

// ---------------------------------------------------------------- детали

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

/** Колонка и шапка раздела. Ритм тот же, что у настроек: раздел в полосе, а
 *  не экран внутри другого раздела. */
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

function Podval({ sleva, sprava }: { sleva?: ReactNode; sprava: ReactNode }) {
  return (
    <footer className="flex flex-wrap items-center justify-between gap-4">
      <div className="min-w-0 flex-1">{sleva}</div>
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

// Ошибка стоит после подписи, а не внутри неё: иначе она входит в имя поля.
function PolePorta({ podpis, znachenie, naVvod, placeholder, oshibka }: {
  podpis: string;
  znachenie: string;
  naVvod: (v: string) => void;
  placeholder: string;
  oshibka: string | null;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <PolePodpisannoe podpis={podpis}>
        <Pole znachenie={znachenie} naVvod={naVvod} placeholder={placeholder} oshibka={oshibka !== null} className="tabular-nums" />
      </PolePodpisannoe>
      {oshibka && <span role="alert" className="text-danger text-xs">{oshibka}</span>}
    </div>
  );
}

type TonPlashki = "warn" | "danger";

const KROMKA: Record<TonPlashki, string> = { warn: "border-l-[3px] border-l-warn", danger: "border-l-[3px] border-l-danger" };

/** Строка-сообщение внутри карточки. Без тона кромки нет: цвет только там,
 *  где есть что сигналить. */
function Plashka({ ton, zagolovok, tekst, znachok, children, testId }: {
  ton?: TonPlashki;
  zagolovok: string;
  tekst?: ReactNode;
  znachok?: ReactNode;
  children?: ReactNode;
  testId?: string;
}) {
  return (
    <div data-testid={testId} className={`border-border flex flex-wrap items-center gap-4 border-t px-4 py-3.5 first:border-t-0 ${ton ? KROMKA[ton] : ""}`}>
      {znachok && <span className="flex h-4 w-4 shrink-0 items-center justify-center">{znachok}</span>}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-foreground break-words text-sm font-medium">{zagolovok}</span>
        {tekst && <span className="text-fg-muted break-words text-xs">{tekst}</span>}
      </div>
      {children}
    </div>
  );
}

/** Отказ дела: строка словаря по коду, текст службы и что с ним делать. */
function OtkazDela({ otkaz, deystviya }: {
  otkaz: OtkazVps;
  /** Что окно умеет сделать по действию этого кода. Нет обработчика - нет
   *  кнопки: строка словаря остаётся, кнопка в никуда нет. */
  deystviya?: Partial<Record<string, () => void>>;
}) {
  const z = tekstOtkaza[otkaz.kod];
  const deystvie = z ? deystviya?.[z.deystvie] : undefined;
  const podpis = z ? podpisDeystviya[z.deystvie] : null;
  const dobavki = [
    otkaz.tekst ? sPropisnoy(otkaz.tekst) : "",
    otkaz.otpechatok ? `Новый отпечаток ключа сервера: ${otkaz.otpechatok}` : "",
    otkaz.klyuch_ostalsya ? "Ключ этого компьютера остался на сервере, а войти им не вышло" : "",
    otkaz.podozhdat ? `Сервер просит подождать: следующий вход не раньше чем через ${otkaz.podozhdat} с` : "",
  ].filter((s) => s !== "");
  return (
    <Neudacha
      kod={otkaz.kod}
      tekst={dobavki.join(". ")}
      deystvie={deystvie}
      podpisDeystviya={podpis ?? undefined}
    />
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

function SvoyVpnPusto({ naPostavit, aktiven, delo }: { naPostavit: () => void; aktiven: boolean; delo?: ReactNode }) {
  return (
    <Kolonka svodka="Affory поставит VPN на купленный тобой сервер и сам добавит два ключа в список">
      {delo}
      <Gruppa nazvanie="Какой сервер купить" poyasnenie="Сверь условия до оплаты: неподходящий сервер видно только после покупки">
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
            <Knopka rang="glavnaya" bolshaya aktiven={aktiven} testId="svoy-postavit" onClick={naPostavit}>Поставить на свой сервер</Knopka>
          </div>
        </Panel>
      </Gruppa>
    </Kolonka>
  );
}

// ---------------------------------------------------------------- мастер

type ShagPolosy = "dostup" | "proverka" | "ustanovka" | "gotovo";

const SHAGI: { shag: ShagPolosy; imya: string }[] = [
  { shag: "dostup", imya: "Доступ" },
  { shag: "proverka", imya: "Проверка" },
  { shag: "ustanovka", imya: "Установка" },
  { shag: "gotovo", imya: "Готово" },
];

function PolosaShagov({ shag }: { shag: ShagPolosy }) {
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

/** Захват под управление идёт одним шагом доступа: полоса установки и слово
 *  «установка» ему не про то. */
function KolonkaMastera({ shag, adres, vzyat = false, children }: { shag: ShagPolosy; adres?: string; vzyat?: boolean; children: ReactNode }) {
  const svodka = vzyat
    ? `Управление сервером ${adres ? `${adres} ` : ""}с этого компьютера`
    : adres ? `Установка на ${adres}` : "Установка на свой сервер";
  return (
    <Kolonka svodka={svodka}>
      {!vzyat && <PolosaShagov shag={shag} />}
      {children}
    </Kolonka>
  );
}

/** Шаг «Доступ»: сообщение хостера разбирается по полям, поля правятся руками. */
function ShagDostup({ cel, forma, otkaz, idyot, aktiven, naSmenu, vybratKlyuch, ochistitBufer, naOtmenu, naDalshe }: {
  cel: "postavit" | "vzyat";
  forma: Forma;
  otkaz?: OtkazVps;
  idyot: boolean;
  aktiven: boolean;
  naSmenu: (f: Forma) => void;
  vybratKlyuch: () => Promise<KlyuchFayla | null>;
  ochistitBufer: (tekst: string) => Promise<boolean>;
  naOtmenu: () => void;
  naDalshe: () => void;
}) {
  // Пароль, вставленный сообщением, остаётся в буфере обмена, а Win+V держит
  // и журнал. Предложение очистить живёт, пока поле сообщения не уйдёт.
  const [vBufere, zadatVBufere] = useState<string | null>(null);
  const [itogBufera, zadatItogBufera] = useState<string | null>(null);
  const [otkazKlyucha, zadatOtkazKlyucha] = useState<string | null>(null);
  const pole = <K extends keyof Forma>(k: K) => (v: Forma[K]) => naSmenu({ ...forma, [k]: v });

  const naSoobshchenie = (tekst: string) => {
    const r = razobratSoobshchenie(tekst);
    const bylo = razobratSoobshchenie(forma.soobshchenie);
    // Вставка кладёт текст разом, набор по одному знаку: только вставка
    // оставляет пароль в буфере обмена.
    if (r.parol && tekst.length - forma.soobshchenie.length > 1) {
      zadatVBufere(r.parol);
      zadatItogBufera(null);
    }
    // Поле, поправленное руками, разбор письма не трогает: меняется только
    // пустое или то, что до сих пор равно прошлому разбору. Письмо, вставленное
    // в пустое поле, новое: оно берёт всё, что в нём нашлось.
    const novoePismo = forma.soobshchenie.trim() === "";
    const iz = (k: "adres" | "portSshPolya" | "polzovatel" | "parol", novoe: string | undefined, staroe: string | undefined) =>
      novoe !== undefined && (novoePismo || forma[k] === PUSTAYA_FORMA[k] || forma[k] === staroe) ? novoe : forma[k];
    const port = (x: number | undefined) => (x !== undefined ? String(x) : undefined);
    naSmenu({
      ...forma,
      soobshchenie: tekst,
      adres: iz("adres", r.adres, bylo.adres),
      portSshPolya: iz("portSshPolya", port(r.port), port(bylo.port)),
      polzovatel: iz("polzovatel", r.polzovatel, bylo.polzovatel),
      // Пароль из сообщения только для входа паролем: рядом с ключом он не едет.
      parol: forma.sposob === "parol" ? iz("parol", r.parol || undefined, bylo.parol) : forma.parol,
    });
  };

  const ochistit = async () => {
    if (vBufere === null) return;
    try {
      const ochishchen = await ochistitBufer(vBufere);
      zadatItogBufera(ochishchen
        ? "Буфер очищен. Если включён журнал буфера (Win+V), копия осталась в нём"
        : "В буфере уже другой текст, пароля там нет");
      zadatVBufere(null);
    } catch (e: unknown) {
      zadatItogBufera(`Буфер не очищен: ${tekstOshibki(e)}`);
    }
  };

  const naKlyuch = async () => {
    zadatOtkazKlyucha(null);
    try {
      const k = await vybratKlyuch();
      if (k) naSmenu({ ...forma, klyuch: k });
    } catch (e: unknown) {
      zadatOtkazKlyucha(tekstOshibki(e));
    }
  };

  const ssh = portSshFormy(forma);
  const oshibkaSsh = oshibkaPorta(forma.portSshPolya);
  const oshibkaReality = oshibkaPorta(forma.portReality, oshibkaSsh === null ? zanyatyeDlyaReality(ssh) : []);
  const oshibkaHy2 = oshibkaPorta(forma.portHy2);
  const oshibkaAdr = oshibkaAdresa(forma.adres);
  const vhod = forma.sposob === "parol" ? forma.parol !== "" : forma.klyuch !== null;
  const gotovo = chistyyAdres(forma.adres) !== "" && oshibkaAdr === null && forma.polzovatel.trim() !== "" && vhod
    && oshibkaSsh === null && (cel === "vzyat" || (oshibkaReality === null && oshibkaHy2 === null));

  return (
    <>
      {otkaz && <OtkazDela otkaz={otkaz} />}
      <Panel>
        <div className="grid gap-6 p-5 lg:grid-cols-2">
          <div className="flex min-w-0 flex-col gap-2">
            <PolePodpisannoe podpis="Сообщение от хостера">
              <PoleTeksta
                strok={8}
                testId="svoy-soobshchenie"
                znachenie={forma.soobshchenie}
                naVvod={naSoobshchenie}
                placeholder={"Вставь письмо или сообщение хостера с адресом, логином и паролем. Например:\n\nСервер готов\nIP-адрес: 203.0.113.10\nЛогин: root\nПароль: Kf8mQ2xLp"}
              />
            </PolePodpisannoe>
            <p className="text-fg-muted text-xs">Affory разбирает его здесь и никуда не отправляет</p>
            {vBufere !== null && (
              <p className="text-fg-secondary flex flex-wrap items-center gap-1 text-xs">
                Пароль остался в буфере обмена
                <Knopka rang="tekst" testId="svoy-ochistit-bufer" onClick={() => void ochistit()}>Очистить буфер</Knopka>
              </p>
            )}
            {itogBufera && <p role="status" className="text-fg-secondary text-xs">{itogBufera}</p>}
          </div>
          <div className="flex min-w-0 flex-col gap-4">
            <div className="grid grid-cols-[1fr_96px] items-start gap-3">
              <div className="flex min-w-0 flex-col gap-1.5">
                <PolePodpisannoe podpis="Адрес сервера">
                  <Pole testId="svoy-adres" znachenie={forma.adres} naVvod={pole("adres")} placeholder="203.0.113.10" oshibka={oshibkaAdr !== null} />
                </PolePodpisannoe>
                {oshibkaAdr && <span role="alert" className="text-danger text-xs">{oshibkaAdr}</span>}
              </div>
              <PolePorta podpis="Порт SSH" znachenie={forma.portSshPolya} naVvod={pole("portSshPolya")} placeholder="22" oshibka={oshibkaSsh} />
            </div>
            <PolePodpisannoe podpis="Пользователь">
              <Pole testId="svoy-polzovatel" znachenie={forma.polzovatel} naVvod={pole("polzovatel")} placeholder="root" />
            </PolePodpisannoe>
            <div className="flex flex-col gap-2">
              {/* Обёртка держит выбор по своей ширине: в колонке flex его
                  растянуло бы на всю. */}
              <div>
                <Segment
                  aria-label="Как войти"
                  ton="tihiy"
                  znacheniya={[{ z: "parol", podpis: "Пароль" }, { z: "klyuch", podpis: "Ключ" }]}
                  vybrano={forma.sposob}
                  // Секрет другого способа стирается сразу: пароль живёт только
                  // для входа по паролю (план, §3.6) и рядом с ключом не едет.
                  // Сообщение хостера уходит вместе с ним: пароль лежит и там.
                  naVybor={(z) => {
                    if (z === forma.sposob) return;
                    zadatVBufere(null);
                    naSmenu(z === "klyuch"
                      ? { ...forma, sposob: z, parol: "", soobshchenie: "" }
                      : { ...forma, sposob: z, klyuch: null, parolKlyucha: "" });
                  }}
                />
              </div>
              {forma.sposob === "parol" ? (
                <>
                  <PolePodpisannoe podpis="Пароль">
                    <Pole tip="password" testId="svoy-parol" znachenie={forma.parol} naVvod={pole("parol")} />
                  </PolePodpisannoe>
                  <p className="text-fg-muted text-xs">Нужен один раз: Affory войдёт, положит свой ключ и пароль забудет</p>
                </>
              ) : (
                <>
                  <div className="flex flex-wrap items-center gap-2">
                    <Knopka rang="vtoraya" testId="svoy-vybrat-klyuch" onClick={() => void naKlyuch()}>{forma.klyuch ? "Другой файл" : "Выбрать файл ключа"}</Knopka>
                    <span className={forma.klyuch ? "text-foreground break-all text-xs" : "text-fg-muted text-xs"}>
                      {forma.klyuch?.imya ?? "закрытый ключ, например id_ed25519"}
                    </span>
                  </div>
                  {otkazKlyucha && <span role="alert" className="text-danger text-xs">{sPropisnoy(otkazKlyucha)}</span>}
                  {forma.klyuch && (
                    <PolePodpisannoe podpis="Пароль ключа, если он есть">
                      <Pole tip="password" znachenie={forma.parolKlyucha} naVvod={pole("parolKlyucha")} />
                    </PolePodpisannoe>
                  )}
                </>
              )}
            </div>
          </div>
        </div>
        {cel === "postavit" && (
          <div className="px-5">
            <Svorachivaemyy
              zagolovok="Дополнительно"
              poyasnenie="порты ключей"
              otkrytPoUmolchaniyu={forma.portReality !== "" || forma.portHy2 !== ""}
              deti={(
                <div className="flex flex-col gap-3">
                  <div className="grid max-w-[420px] grid-cols-2 items-start gap-3">
                    {/* SSH живёт на TCP, поэтому его порт закрыт только для
                        reality: hysteria2 на UDP с ним не делит ничего. */}
                    <PolePorta podpis="reality, TCP" znachenie={forma.portReality} naVvod={pole("portReality")} placeholder="подобрать" oshibka={oshibkaReality} />
                    <PolePorta podpis="hysteria2, UDP" znachenie={forma.portHy2} naVvod={pole("portHy2")} placeholder="подобрать" oshibka={oshibkaHy2} />
                  </div>
                  <span className="text-fg-muted text-xs">Пустое поле: Affory сам найдёт порт, который проходит из твоей сети</span>
                </div>
              )}
            />
          </div>
        )}
      </Panel>
      <Podval
        sprava={(
          <>
            <Knopka rang="tekst" onClick={naOtmenu}>Отмена</Knopka>
            <Knopka rang="glavnaya" bolshaya testId="svoy-dalshe" aktiven={gotovo && aktiven} zhdyot={idyot} onClick={naDalshe}>
              {cel === "vzyat" ? "Взять под управление" : "Проверить сервер"}
            </Knopka>
          </>
        )}
      />
    </>
  );
}

// Название пункта осмотра по коду замечания. Тексты пишет служба, окно
// только раскладывает их по строкам.
const PUNKT_ZAMECHANIYA: Record<string, string> = {
  "ne-ubuntu": "Версия системы",
  "staraya-ubuntu": "Версия системы",
  "ubuntu-20.04": "Версия системы",
  "ubuntu-ne-proverena": "Версия системы",
  "ne-x86": "Процессор",
  "ne-systemd": "Запуск служб",
  konteyner: "Виртуализация",
  "malo-pamyati": "Память",
  "malo-diska": "Место",
  "net-utility": "Программы",
  "net-utility-zhelatelno": "Программы",
  "sboy-osmotra": "Осмотр",
  "net-ipv4": "Адрес",
  "chastnyy-ipv4": "Адрес",
  chasy: "Часы",
  nastraivaetsya: "Готовность",
  "ne-pustoy": "Чистота",
  "pod-centrom": "Чистота",
  "uzhe-stoit": "Чистота",
  "chuzhoy-brandmauer": "Брандмауэр",
  "net-bbr": "Скорость",
};

export interface Punkt { imya: string; itog: Itog; podrobno: string }

/** Пункты шага «Проверка» из снимка осмотра. */
export function punktyOsmotra(h: HodVps | undefined, sposob: Forma["sposob"] | null, port: number): Punkt[] {
  const kak = sposob === "parol" ? ", по паролю" : sposob === "klyuch" ? ", по ключу" : "";
  const o = h?.osmotr;
  if (!h || h.faza === "idyot" || !o) {
    const upal = h !== undefined && h.faza !== "idyot";
    return [
      { imya: "Вход", itog: upal ? "net" : "idyot", podrobno: upal ? "не прошёл" : `порт ${port}${kak}` },
      { imya: "Осмотр", itog: "zhdyot", podrobno: upal ? "не начинался" : "система, место, порты, чистота" },
    ];
  }
  const tunnel = putSlovami(o.put) === "через VPN";
  const punkty: Punkt[] = [
    { imya: "Вход", itog: "da", podrobno: `порт ${port}${kak}` },
    { imya: "Путь", itog: tunnel ? "vnimanie" : "da", podrobno: tunnel ? "через VPN: напрямую сервер не ответил" : putSlovami(o.put) || "напрямую" },
    { imya: "Права", itog: "da", podrobno: o.root ? "root" : "через sudo" },
    { imya: "Система", itog: "da", podrobno: o.sistema ?? "не определена" },
  ];
  for (const z of o.zamechaniya) {
    punkty.push({ imya: PUNKT_ZAMECHANIYA[z.kod] ?? "Замечание", itog: z.stop ? "net" : "vnimanie", podrobno: z.tekst });
  }
  if (o.zamechaniya.length === 0) punkty.push({ imya: "Замечания", itog: "da", podrobno: "нет" });
  return punkty;
}

function ShagProverka({ h, forma, pustoy, otkaz, idyot, aktiven, naPustoy, naNazad, naOtmenu, naPostavit, naVzyat, naPovtor, naServery, naPrinyatKlyuch }: {
  h: HodVps | undefined;
  forma: Forma;
  pustoy: boolean;
  otkaz?: OtkazVps;
  idyot: boolean;
  aktiven: boolean;
  naPustoy: (v: boolean) => void;
  naNazad: () => void;
  naOtmenu: () => void;
  naPostavit: (o: OsmotrVps) => void;
  naVzyat: (o: OsmotrVps) => void;
  naPovtor: () => void;
  naServery: () => void;
  naPrinyatKlyuch: (server: string, otpechatok: string) => void;
}) {
  const punkty = punktyOsmotra(h, forma.sposob, portSshFormy(forma));
  const o = h?.faza === "gotovo" ? h.osmotr : undefined;
  const otkazOsmotra = h?.faza === "oshibka" ? h.otkaz : undefined;
  const pokazOtkaza = otkaz ?? otkazOsmotra;
  // Свой Affory без ключа этого компьютера: ставить нельзя, брать можно.
  const vzyat = o?.zamechaniya.some((z) => z.kod === "uzhe-stoit" && z.stop) ?? false;
  const otpechatok = pokazOtkaza?.otpechatok;
  return (
    <>
      {pokazOtkaza && (
        <OtkazDela
          otkaz={pokazOtkaza}
          deystviya={{
            povtorit: naPovtor,
            "otkryt-servery": naServery,
            "prinyat-klyuch-hosta": h?.server && otpechatok ? () => naPrinyatKlyuch(h.server ?? "", otpechatok) : undefined,
          }}
        />
      )}
      <div className="flex flex-col gap-2">
        <Panel testId="svoy-punkty">
          {punkty.map((p, i) => (
            <Ryad key={`${i}-${p.imya}`} znachok={<ZnachokItoga itog={p.itog} />} nazvanie={p.imya} poyasnenie={p.podrobno} aktiven={p.itog !== "zhdyot"} />
          ))}
        </Panel>
        {o && (
          <p className="text-fg-faint break-all text-xs" title="Affory запомнил отпечаток. Если при следующем входе он будет другим, Affory остановится до твоего решения">
            Отпечаток сервера {o.otpechatok}
          </p>
        )}
      </div>
      {h?.faza === "idyot" && (
        <Podval sprava={<Knopka rang="vtoraya" onClick={naOtmenu}>Отменить проверку</Knopka>} />
      )}
      {h?.faza !== "idyot" && !o && (
        <Podval sleva={<Knopka rang="tekst" onClick={naNazad}>Ввести другой сервер</Knopka>} sprava={null} />
      )}
      {o && vzyat && (
        <Podval
          sleva={<Knopka rang="tekst" onClick={naNazad}>Назад</Knopka>}
          sprava={<Knopka rang="glavnaya" bolshaya aktiven={aktiven} zhdyot={idyot} onClick={() => naVzyat(o)}>Взять под управление</Knopka>}
        />
      )}
      {o && !vzyat && o.mozhno_stavit && (
        <>
          <Flazhok vkl={pustoy} naSmenu={naPustoy} testId="svoy-pustoy" podpis="На сервере нет ничего нужного, Affory может занять его целиком" />
          <Podval
            sleva={<Knopka rang="tekst" onClick={naNazad}>Назад</Knopka>}
            sprava={<Knopka rang="glavnaya" bolshaya testId="svoy-postavit-vpn" aktiven={pustoy && aktiven} zhdyot={idyot} onClick={() => naPostavit(o)}>Поставить VPN</Knopka>}
          />
        </>
      )}
      {o && !vzyat && !o.mozhno_stavit && (
        <Podval sleva={<span className="text-fg-muted text-xs">Сервер не подходит: причина в пунктах выше</span>} sprava={<Knopka rang="vtoraya" onClick={naNazad}>Ввести другой сервер</Knopka>} />
      )}
    </>
  );
}

// Этапы установки по шагам службы (upravlenie/ustanovka.go): программа,
// сертификат, подбор портов, проверка ключей.
type IdEtapa = "programma" | "sertifikat" | "porty" | "klyuchi";

const ETAP_SHAGA: Record<string, IdEtapa> = {
  zagruzka: "programma",
  vhod: "programma",
  nastroyka: "programma",
  osmotr: "programma",
  klyuch: "programma",
  programma: "programma",
  maska: "programma",
  sertifikat: "sertifikat",
  podbor: "porty",
  paket: "porty",
  porty: "porty",
  detektor: "porty",
  proverka: "klyuchi",
};

const CHTO_IDYOT: Record<string, string> = {
  zagruzka: "готовлю программу сервера",
  vhod: "вхожу на сервер",
  nastroyka: "жду, пока машина закончит настройку после создания",
  osmotr: "осматриваю сервер",
  klyuch: "кладу ключ управления этого компьютера",
  programma: "ставлю программу",
  maska: "выбираю сайт, под который маскируется reality",
  sertifikat: "выпускаю сертификат на адрес сервера",
  podbor: "пробую порты по одному из твоей сети",
  paket: "ставлю выбранные порты",
  porty: "открываю порты в брандмауэре сервера",
  detektor: "проверяю, почему порты не проходят из твоей сети",
  proverka: "качаю 2 МБ через каждый ключ",
};

const ETAPY: { id: IdEtapa; imya: string; zhdyot: string }[] = [
  { id: "programma", imya: "Программа сервера", zhdyot: "вход, осмотр и программа" },
  { id: "sertifikat", imya: "Сертификат", zhdyot: "на адрес сервера" },
  { id: "porty", imya: "Подбор портов", zhdyot: "по одному порту из твоей сети" },
  { id: "klyuchi", imya: "Проверка ключей", zhdyot: "загрузка 2 МБ через каждый ключ" },
];

// Куда отнести предупреждение дошедшей до ключей установки.
const ETAP_PREDUPREZHDENIYA: Record<string, IdEtapa> = {
  "vps-cert-blocked": "sertifikat",
  "vps-address-blocked": "porty",
  "vps-hoster-firewall": "porty",
  "vps-network-limited": "porty",
  "vps-ports-failed": "klyuchi",
};

export interface Etap { id: IdEtapa; imya: string; itog: Itog; podrobno: string }

export function etapyUstanovki(h: HodVps | undefined): Etap[] {
  const tekushchiy = ETAP_SHAGA[h?.shag ?? "zagruzka"] ?? "programma";
  const nomer = ETAPY.findIndex((e) => e.id === tekushchiy);
  return ETAPY.map((e, i): Etap => {
    if (!h) return { id: e.id, imya: e.imya, itog: "zhdyot", podrobno: e.zhdyot };
    if (h.faza === "gotovo") {
      const vnimanie = h.preduprezhdeniya.find((p) => (ETAP_PREDUPREZHDENIYA[p.kod] ?? "klyuchi") === e.id);
      if (vnimanie) return { id: e.id, imya: e.imya, itog: "vnimanie", podrobno: vnimanie.tekst };
      if (e.id === "porty" && h.ustanovlen) {
        return { id: e.id, imya: e.imya, itog: "da", podrobno: h.ustanovlen.klyuchi.map((k) => `${vidKlyucha(k.transport).imya} ${k.port}`).join(", ") };
      }
      return { id: e.id, imya: e.imya, itog: "da", podrobno: "готово" };
    }
    if (i < nomer) return { id: e.id, imya: e.imya, itog: "da", podrobno: "готово" };
    if (i > nomer) return { id: e.id, imya: e.imya, itog: "zhdyot", podrobno: h.faza === "idyot" ? e.zhdyot : "не начинался" };
    if (h.faza === "idyot") {
      // На проверке ключей служба называет ключ, который сейчас везёт 2 МБ.
      const podrobno = e.id === "klyuchi" && h.klyuch
        ? `${vidKlyucha(h.klyuch.transport).imya} ${h.klyuch.port}: качаю 2 МБ`
        : CHTO_IDYOT[h.shag ?? ""] ?? e.zhdyot;
      return { id: e.id, imya: e.imya, itog: "idyot", podrobno };
    }
    return { id: e.id, imya: e.imya, itog: "net", podrobno: h.faza === "otmeneno" ? "отменено" : "не прошёл" };
  });
}

type ItogPorta = "proshyol" | "ne-proshyol" | "probuyu";

interface ProbaPorta { port: number; itog: ItogPorta; prichina?: string }

const SLOVO_PORTA: Record<ItogPorta, string> = {
  proshyol: "прошёл",
  "ne-proshyol": "не прошёл",
  probuyu: "проверяется",
};

/** Фишки подбора по видам ключа. Порт пробуется не раз (сразу, через 2 и 10
 *  минут): у фишки последний итог, место по первой пробе. Текущий ключ
 *  фишкой «проверяется» только пока идёт сам подбор: на проверке готовых
 *  ключей служба шлёт его снова, а порт уже прошёл. */
export function probyPortov(h: HodVps, sTekushchim = true): { reality: ProbaPorta[]; hy2: ProbaPorta[] } {
  const vidy = { reality: new Map<number, ProbaPorta>(), hy2: new Map<number, ProbaPorta>() };
  const kuda = (transport: string) => (vidKlyucha(transport).proto === "UDP" ? vidy.hy2 : vidy.reality);
  for (const o of h.otseyany) kuda(o.proto).set(o.port, { port: o.port, itog: "ne-proshyol", prichina: prichinaOtseva(o.prichina, o.process) });
  for (const k of h.proverki) {
    kuda(k.transport).set(k.port, { port: k.port, itog: k.vid === "vezyot" ? "proshyol" : "ne-proshyol", prichina: k.vid === "vezyot" ? undefined : itogKlyucha(k) });
  }
  if (sTekushchim && h.klyuch) kuda(h.klyuch.transport).set(h.klyuch.port, { port: h.klyuch.port, itog: "probuyu" });
  return { reality: [...vidy.reality.values()], hy2: [...vidy.hy2.values()] };
}

const VID_PORTA: Record<ItogPorta, string> = {
  proshyol: "border-accent-ink/50 text-foreground",
  "ne-proshyol": "border-border text-fg-faint",
  probuyu: "border-border-active bg-fill-subtle text-foreground",
};

function FishkaPorta({ proba }: { proba: ProbaPorta }) {
  return (
    <li title={proba.prichina} className={`flex h-7 items-center gap-1.5 rounded-md border px-2 text-xs tabular-nums ${VID_PORTA[proba.itog]}`}>
      {proba.port}
      {proba.itog === "proshyol" && <IkGalka className="text-accent-ink h-3.5 w-3.5" />}
      {proba.itog === "ne-proshyol" && <IkZakryt className="h-3.5 w-3.5" strokeWidth={2} />}
      {proba.itog === "probuyu" && <Vertushka className="text-accent-ink h-3 w-3" />}
      {/* Причина и голосом: title читается только мышью. */}
      <span className="sr-only">{SLOVO_PORTA[proba.itog]}{proba.prichina ? `: ${proba.prichina}` : ""}</span>
    </li>
  );
}

function RyadPortov({ imya, proby }: { imya: string; proby: ProbaPorta[] }) {
  if (proby.length === 0) return null;
  return (
    <div className="flex items-start gap-3">
      <span className="text-fg-secondary w-28 shrink-0 pt-1.5 text-xs">{imya}</span>
      <ul aria-label={`Порты ${imya}`} className="flex flex-wrap gap-1.5">
        {proby.map((p) => <FishkaPorta key={p.port} proba={p} />)}
      </ul>
    </div>
  );
}

function chtoSeychas(h: HodVps): string {
  if (h.klyuch) {
    const v = vidKlyucha(h.klyuch.transport);
    return `Пробую ${h.klyuch.port} по ${v.proto} для ${v.imya}`;
  }
  const posledniy = h.proverki[h.proverki.length - 1];
  if (posledniy && posledniy.vid !== "vezyot") return `${posledniy.port} по ${vidKlyucha(posledniy.transport).proto}: ${itogKlyucha(posledniy)}`;
  return "";
}

/** Ход подбора: какой порт пробуется, какие уже не прошли (план, §4, шаг 3).
 *  Отступ слева равен значку и зазору строки Ryad: фишки стоят под названием
 *  этапа, а не под значком. */
function HodPodbora({ h }: { h: HodVps }) {
  const podborIdyot = h.faza === "idyot" && ETAP_SHAGA[h.shag ?? ""] === "porty";
  const p = probyPortov(h, podborIdyot);
  const seychas = podborIdyot ? chtoSeychas(h) : "";
  const perehvat = strokaPerehvata(h);
  if (p.reality.length === 0 && p.hy2.length === 0 && !seychas) return null;
  return (
    <div className="flex flex-col gap-2.5 pr-4 pb-4 pl-[62px]">
      <RyadPortov imya="reality, TCP" proby={p.reality} />
      <RyadPortov imya="hysteria2, UDP" proby={p.hy2} />
      {perehvat && <p className="text-fg-secondary text-xs">{perehvat}</p>}
      {seychas && <p aria-live="polite" className="text-fg-muted text-xs">{seychas}</p>}
    </div>
  );
}

/** Программа на этом компьютере, которая перехватывает порты подбора (П6):
 *  без этой строки «не ответил» на 993 читается как беда сервера. Порт
 *  судится по последней пробе, как его фишка. */
export function strokaPerehvata(h: HodVps): string {
  const posledniy = new Map<string, KlyuchHoda>();
  for (const k of h.proverki) posledniy.set(`${k.transport} ${k.port}`, k);
  const poProgrammam = new Map<string, number[]>();
  for (const k of posledniy.values()) {
    if (k.pomeha && k.vid !== "vezyot") poProgrammam.set(k.pomeha, [...(poProgrammam.get(k.pomeha) ?? []), k.port]);
  }
  return [...poProgrammam].map(([kto, porty]) => {
    const mnogo = porty.length > 1;
    const spisok = mnogo ? `${porty.slice(0, -1).join(", ")} и ${porty[porty.length - 1]}` : `${porty[0]}`;
    return `${kto} на этом компьютере перехватывает ${mnogo ? "порты" : "порт"} ${spisok}, VPN через ${mnogo ? "них" : "него"} не проходит`;
  }).join(". ");
}

/** Строка о перехвате для итога, где фишек подбора уже нет. Ключ, который
 *  сам упёрся в перехват, называет его своей строкой и предупреждением. */
function perehvatVItoge(h: HodVps): string {
  return h.ustanovlen?.klyuchi.some((k) => k.pomeha) ? "" : strokaPerehvata(h);
}

/** Отчёт без секретов: адрес и порт можно, логин, пароль и ключи нельзя
 *  (план, §3.6). */
export function otchyotDela(h: HodVps, adres: string | undefined, versiya: string | undefined): string {
  const stroki = [
    `Affory, свой VPN: ${h.vid}`,
    adres ? `сервер: ${adres}` : "",
    `фаза: ${h.faza}${h.shag ? `, шаг: ${h.shag}` : ""}`,
    h.put ? `путь: ${h.put}` : "",
    h.nachalo ? `начало: ${h.nachalo}` : "",
    h.konec ? `конец: ${h.konec}` : "",
    h.otkaz ? `отказ: ${h.otkaz.kod}: ${h.otkaz.tekst}` : "",
    ...h.preduprezhdeniya.map((p) => `предупреждение: ${p.kod}: ${p.tekst}`),
    ...h.otseyany.map((o) => `отсеян: ${o.proto} ${o.port}: ${o.prichina}${o.process ? ` (${o.process})` : ""}`),
    ...h.proverki.map((k) => `проверка: ${k.transport} ${k.port}: ${k.vid ?? "идёт"}${k.otkaz ? `: ${k.otkaz}` : ""}${k.pomeha ? ` (порт перехватывает ${k.pomeha})` : ""}`),
    h.krug ? `круг детектора: ${h.krug}` : "",
    versiya ?? "",
  ];
  return stroki.filter((s) => s !== "").join("\n");
}

function ShagUstanovka({ h, mozhnoPovtorit, aktiven, idyot, itogOtchyota, naOtmenu, naOtchyot, naPovtor, naZakryt, naServery, naNeZhdat }: {
  h: HodVps | undefined;
  mozhnoPovtorit: boolean;
  aktiven: boolean;
  idyot: boolean;
  itogOtchyota: string | null;
  naOtmenu: () => void;
  naOtchyot: () => void;
  naPovtor: () => void;
  naZakryt: () => void;
  naServery: () => void;
  /** Ключи записаны, перепроверка из сети идёт: перейти к ним, не дожидаясь. */
  naNeZhdat: () => void;
}) {
  const etapy = etapyUstanovki(h);
  const konchilas = h !== undefined && h.faza !== "idyot";
  // Первый круг детектора пуст, а повторы ещё через минуты (И7): человек
  // сразу узнаёт, в чём дело, и может не ждать.
  const zhdyomPovtora = h?.faza === "idyot" && h.ustanovlen && h.krug_tekst ? h : undefined;
  return (
    <>
      {zhdyomPovtora && (
        <Panel>
          <Plashka
            ton="warn"
            testId="svoy-povtor-iz-seti"
            znachok={<IkVnimanie className="text-warn h-4 w-4" />}
            zagolovok={sPropisnoy(zhdyomPovtora.krug_tekst ?? "")}
            tekst={`${povtoryTekstom(zhdyomPovtora)}. Ключи уже готовы: можно не ждать и проверить их с телефона по мобильному интернету`}
          >
            <Knopka rang="vtoraya" testId="svoy-ne-zhdat" onClick={naNeZhdat}>Не ждать</Knopka>
          </Plashka>
        </Panel>
      )}
      {h?.faza === "oshibka" && h.otkaz && (
        <OtkazDela otkaz={h.otkaz} deystviya={{ povtorit: mozhnoPovtorit ? naPovtor : undefined, "otkryt-servery": naServery }} />
      )}
      {h?.faza === "otmeneno" && (
        <Neudacha zagolovok="Установка отменена" tekst="Что успело встать, осталось на сервере" />
      )}
      <Panel testId="svoy-etapy">
        {etapy.map((e) => (
          <div key={e.id} className="border-border border-t first:border-t-0">
            <Ryad znachok={<ZnachokItoga itog={e.itog} />} nazvanie={e.imya} poyasnenie={e.podrobno} aktiven={e.itog !== "zhdyot"} />
            {e.id === "porty" && h && (e.itog !== "zhdyot") && <HodPodbora h={h} />}
          </div>
        ))}
      </Panel>
      <Podval
        sleva={(
          <span className="text-fg-muted text-xs">
            {konchilas
              ? mozhnoPovtorit ? "Повтор продолжит установку с того места, где она встала" : "Для повтора введи доступ к серверу заново: пароль окно не хранит"
              : "Окно можно закрыть: установка продолжится, мастер вернётся к ней при открытии"}
            {itogOtchyota && <span role="status" className="text-fg-secondary block">{itogOtchyota}</span>}
          </span>
        )}
        sprava={konchilas ? (
          <>
            <Knopka rang="tekst" onClick={naZakryt}>Закрыть</Knopka>
            <Knopka rang="vtoraya" onClick={naOtchyot}>Скопировать отчёт</Knopka>
            <Knopka rang="glavnaya" bolshaya aktiven={aktiven} zhdyot={idyot} onClick={naPovtor}>{mozhnoPovtorit ? "Попробовать снова" : "Ввести доступ"}</Knopka>
          </>
        ) : (
          <Knopka rang="vtoraya" testId="svoy-otmenit-ustanovku" onClick={naOtmenu}>Отменить установку</Knopka>
        )}
      />
    </>
  );
}

function RyadVhoda({ vkl, idyot, aktiven, naSmenu }: { vkl: boolean; idyot: boolean; aktiven: boolean; naSmenu: () => void }) {
  return (
    <Ryad
      nazvanie="Вход по паролю"
      poyasnenie={vkl
        ? "Включён: пароль от хостера открывает сервер. Если выключить, войдёт только этот компьютер, а с другого придётся через консоль хостера"
        : "Выключен: войти может только этот компьютер"}
    >
      <Knopka rang="vtoraya" aktiven={aktiven} zhdyot={idyot} testId="svoy-vhod-po-parolyu" onClick={naSmenu}>{vkl ? "Выключить" : "Включить"}</Knopka>
    </Ryad>
  );
}

function ShagGotovo({ h, server, idyotParol, itogParolya, aktiven, vygruzka, otkazVygruzki, zhdyotVygruzka, skopirovat, kodyQr, naPodklyuchit, naVygruzit, naZakrytVygruzku, naVhod, naZakryt }: {
  h: HodVps;
  server: SvoyServer | undefined;
  idyotParol: boolean;
  itogParolya: ReactNode;
  aktiven: boolean;
  vygruzka: VygruzkaOtvet | null;
  otkazVygruzki: string | null;
  zhdyotVygruzka: boolean;
  skopirovat: (tekst: string) => Promise<void>;
  kodyQr: (tekst: string) => Promise<string[]>;
  naPodklyuchit: () => void;
  naVygruzit: () => void;
  naZakrytVygruzku: () => void;
  naVhod: () => void;
  naZakryt: () => void;
}) {
  const klyuchi = h.ustanovlen?.klyuchi ?? [];
  const idyot = h.faza === "idyot";
  const estKlyuchi = (h.ustanovlen?.ids.length ?? 0) > 0;
  // Из этой сети не прошёл ни один ключ: главное теперь проверить их с
  // телефона, а не подключаться отсюда.
  const proshli = klyuchi.some((k) => k.vid === "vezyot");
  const vseProshli = klyuchi.length > 0 && klyuchi.every((k) => k.vid === "vezyot");
  const perehvat = perehvatVItoge(h);
  // Выгрузка открывается под ключами: окно само доводит её до глаз.
  const kVygruzke = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (vygruzka) kVygruzke.current?.scrollIntoView?.({ block: "nearest", behavior: "smooth" });
  }, [vygruzka]);
  return (
    <>
      {idyot && (
        <Panel>
          <Plashka
            testId="svoy-perepoverka"
            znachok={<Vertushka className="text-accent-ink h-4 w-4" />}
            zagolovok={zagolovokPerepoverki(h)}
            tekst={[povtoryTekstom(h), "Итог появится в карточке сервера"].filter(Boolean).join(". ")}
          />
        </Panel>
      )}
      {h.preduprezhdeniya.length > 0 && (
        <Panel>
          {h.preduprezhdeniya.map((p, i) => (
            <Plashka key={`${i}-${p.kod}`} ton="warn" znachok={<IkVnimanie className="text-warn h-4 w-4" />} zagolovok={sPropisnoy(tekstOtkaza[p.kod]?.tekst ?? "есть замечание")} tekst={sPropisnoy(p.tekst)} />
          ))}
        </Panel>
      )}
      <div className="flex flex-col gap-2">
        <Panel testId="svoy-gotovo">
          {klyuchi.map((k) => {
            const v = vidKlyucha(k.transport);
            return (
              <Ryad
                key={`${k.transport}-${k.port}`}
                znachok={<ZnachokItoga itog={k.vid === "vezyot" ? "da" : !k.vid && idyot && !h.ustanovlen?.ne_iz_seti.includes(k.transport) ? "idyot" : "vnimanie"} />}
                nazvanie={<span className="flex items-baseline gap-2">{v.imya}<span className="text-fg-muted text-[13px] font-normal tabular-nums">{v.proto} {k.port}</span></span>}
                poyasnenie={k.vid ? itogKlyucha(k) : bezProverki(k, h.ustanovlen ?? { ne_iz_seti: [] }, idyot)}
              >
                <Teg ton="akcent">в списке серверов</Teg>
              </Ryad>
            );
          })}
          <div className="border-border flex flex-wrap items-center gap-3 border-t px-4 py-4">
            <Knopka rang={proshli ? "glavnaya" : "vtoraya"} bolshaya aktiven={aktiven && estKlyuchi} testId="svoy-podklyuchit" onClick={naPodklyuchit}>Подключиться</Knopka>
            <Knopka rang={proshli ? "vtoraya" : "glavnaya"} bolshaya aktiven={aktiven && estKlyuchi} zhdyot={zhdyotVygruzka} testId="svoy-klyuchi-telefona" onClick={naVygruzit}>Ключи для телефона</Knopka>
            <Knopka rang="tekst" onClick={naZakryt}>Готово</Knopka>
          </div>
        </Panel>
        {perehvat && <p className="text-fg-muted text-xs">{perehvat}</p>}
        {vseProshli && (
          <p className="text-fg-muted text-xs">
            Affory подбирал порты из сети этого компьютера. С мобильного интернета на телефоне какой-то из ключей может не открыться
          </p>
        )}
      </div>
      {otkazVygruzki && <p role="alert" className="text-danger text-xs">{otkazVygruzki}</p>}
      {vygruzka && (
        <div ref={kVygruzke}>
          <Vygruzka vygruzka={vygruzka} zakryt={naZakrytVygruzku} skopirovat={skopirovat} kodyQr={kodyQr} zagolovok="Ключи для телефона" />
        </div>
      )}
      {server && (
        <Gruppa nazvanie="Доступ к серверу">
          <Panel>
            {/* Пока идёт перепроверка, служба занята ею и второе дело не начнёт. */}
            <RyadVhoda vkl={!server.parol_vyklyuchen} idyot={idyotParol} aktiven={aktiven && server.klyuch_upravleniya && !idyot} naSmenu={naVhod} />
            {itogParolya}
          </Panel>
        </Gruppa>
      )}
    </>
  );
}

// ---------------------------------------------------------------- дела над сервером

const DELO_IDYOT: Record<string, string> = {
  proverka: "Проверяю сервер",
  podbor: "Подбираю порты заново",
  snos: "Сношу VPN с сервера",
  vzyat: "Беру сервер под управление",
  otpusk: "Снимаю ключ этого компьютера с сервера",
  parol: "Меняю вход по паролю",
  osmotr: "Осматриваю сервер",
  ustanovka: "Ставлю VPN",
};

// Имя законченного дела в заголовке итога: «идёт» глаголом, итог существительным.
const DELO_ITOG: Record<string, string> = {
  proverka: "Проверка сервера",
  podbor: "Подбор портов",
  snos: "Снос VPN с сервера",
  vzyat: "Перенос управления сюда",
  otpusk: "Снятие ключа этого компьютера",
  parol: "Смена входа по паролю",
  osmotr: "Осмотр сервера",
  ustanovka: "Установка VPN",
};

/** Строка итога законченного дела. null - сказать нечего, строки не будет. */
export function itogDela(h: HodVps, adres: string | undefined): { ton?: TonPlashki; zagolovok: string; tekst?: string } | null {
  const gde = adres ? ` ${adres}` : "";
  if (h.faza === "otmeneno") return { zagolovok: `${DELO_ITOG[h.vid] ?? "Дело"}: отменено` };
  if (h.faza !== "gotovo") return null;
  switch (h.vid) {
    case "snos":
      return { zagolovok: `VPN снесён с сервера${gde}`, tekst: h.snyat?.parol_vernulsya ? "Вход по паролю снова такой, каким был до выключения" : undefined };
    case "otpusk":
      return {
        zagolovok: `Сервер${gde} забыт`,
        tekst: h.otpushchen?.klyucha_ne_bylo ? "Ключа этого компьютера на сервере уже не было. VPN на сервере работает дальше" : "Ключ этого компьютера снят с сервера. VPN на сервере работает дальше",
      };
    case "vzyat":
      return { zagolovok: `Сервер${gde} под управлением этого компьютера` };
    case "podbor":
      return {
        zagolovok: "Порты подобраны заново",
        tekst: [h.ustanovlen?.klyuchi.map((k) => `${vidKlyucha(k.transport).imya} ${k.port}`).join(", ") ?? "", perehvatVItoge(h)]
          .filter(Boolean).join(". ") || undefined,
      };
    case "parol": {
      const p = h.parol;
      if (!p) return null;
      if (p.vyklyuchen) return { zagolovok: "Вход по паролю выключен" };
      if (p.snaruzhi === "vyklyuchen_chuzhim") {
        return { ton: "warn", zagolovok: "Запрет Affory снят, но пароль держит выключенным другая настройка сервера", tekst: p.fayly?.length ? `Файлы: ${p.fayly.join(", ")}` : undefined };
      }
      if (p.snaruzhi === "neizvestno") return { ton: "warn", zagolovok: "Вход по паролю включён, а проверить настройку сервера не вышло" };
      return { zagolovok: "Вход по паролю включён" };
    }
  }
  return null;
}

/** Ключи уже записаны, и дело только перепроверяет их из сети (И7): повторы
 *  детектора смотрят адрес, проверка ключей сами ключи. */
function zagolovokPerepoverki(h: HodVps): string {
  return h.shag === "detektor" ? "Проверяю адрес из твоей сети ещё раз" : "Проверяю ключи из твоей сети";
}

/** Ход и итог дела одной строкой. Отказ ставит своё действие, если окно его
 *  умеет. */
function StrokaDela({ h, adres, aktiven, zanyato = false, bezSeti = false, naOtmenu, naSkryt, deystviya, dop }: {
  h: HodVps;
  adres: string | undefined;
  aktiven: boolean;
  /** Идёт другое дело: кнопка по отказу упрётся в занятую службу. «Отменить»
   *  от этого не гаснет. */
  zanyato?: boolean;
  /** Карточка уже показывает причину о сети человека из записи о сервере:
   *  в строке дела её предупреждения повторились бы. */
  bezSeti?: boolean;
  naOtmenu: () => void;
  naSkryt: () => void;
  deystviya: Partial<Record<string, () => void>>;
  /** Кнопка по коду, у которого в словаре действия нет. */
  dop?: ReactNode;
}) {
  if (h.faza === "idyot") {
    const imya = h.planovaya
      ? "Плановая проверка сервера"
      : h.ustanovlen ? zagolovokPerepoverki(h) : DELO_IDYOT[h.vid] ?? "Идёт дело со своим сервером";
    const tekst = h.shag === "detektor" && h.povtory.length > 0
      ? [h.krug_tekst ? sPropisnoy(h.krug_tekst) : "", povtoryTekstom(h)].filter(Boolean).join(". ")
      : CHTO_IDYOT[h.shag ?? ""];
    return (
      <Plashka testId="svoy-delo" znachok={<Vertushka className="text-accent-ink h-4 w-4" />} zagolovok={imya} tekst={tekst}>
        <Knopka rang="vtoraya" aktiven={aktiven} onClick={naOtmenu}>Отменить</Knopka>
      </Plashka>
    );
  }
  const preduprezhdeniya = bezSeti ? h.preduprezhdeniya.filter((p) => !p.iz_seti) : h.preduprezhdeniya;
  const zakryt = (
    <button type="button" aria-label="Скрыть итог" onClick={naSkryt} className="text-fg-muted hover:text-foreground inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
      <IkZakryt className="h-4 w-4" />
    </button>
  );
  if (h.faza === "oshibka" && h.otkaz) {
    const z = tekstOtkaza[h.otkaz.kod];
    const deystvie = z ? deystviya[z.deystvie] : undefined;
    const podpis = z ? podpisDeystviya[z.deystvie] : null;
    // Заголовок говорит, что не вышло, тело почему. Строка словаря тут
    // только запасная: причина службы её пересказывает, и вместе они
    // повторяли одно и то же дважды (И6).
    const tekst = [
      sPropisnoy(h.otkaz.tekst || z?.tekst || ""),
      h.otkaz.klyuch_ostalsya ? "Ключ этого компьютера остался на сервере, а войти им не вышло" : "",
      h.otkaz.podozhdat ? `Следующий вход не раньше чем через ${h.otkaz.podozhdat} с` : "",
    ].filter((s) => s !== "").join(". ");
    return (
      <Plashka testId="svoy-delo" ton="danger" zagolovok={`${DELO_ITOG[h.vid] ?? "Дело"}: не вышло`} tekst={tekst}>
        {deystvie && podpis && <Knopka rang="vtoraya" aktiven={aktiven && !zanyato} onClick={deystvie}>{podpis}</Knopka>}
        {dop}
        {zakryt}
      </Plashka>
    );
  }
  const itog = itogDela(h, adres);
  if (!itog && preduprezhdeniya.length === 0) return null;
  return (
    <>
      {itog && (
        <Plashka testId="svoy-delo" ton={itog.ton} znachok={itog.ton ? <IkVnimanie className="text-warn h-4 w-4" /> : <IkGalka className="text-accent-ink h-4 w-4" />} zagolovok={itog.zagolovok} tekst={itog.tekst}>
          {zakryt}
        </Plashka>
      )}
      {preduprezhdeniya.map((p, i) => (
        <Plashka key={`${i}-${p.kod}`} ton="warn" znachok={<IkVnimanie className="text-warn h-4 w-4" />} zagolovok={sPropisnoy(tekstOtkaza[p.kod]?.tekst ?? "есть замечание")} tekst={sPropisnoy(p.tekst)}>
          {!itog && zakryt}
        </Plashka>
      ))}
    </>
  );
}

// ---------------------------------------------------------------- карточка

/** Порты, которые человек задаёт руками поверх подбора (З9). */
function ZadatPorty({ server, aktiven, naPodobrat, naOtmenu }: {
  server: SvoyServer;
  aktiven: boolean;
  naPodobrat: (reality: number | undefined, hy2: number | undefined) => void;
  naOtmenu: () => void;
}) {
  const [reality, zadatReality] = useState(server.reality ? String(server.reality) : "");
  const [hy2, zadatHy2] = useState(server.hy2 ? String(server.hy2) : "");
  const oshR = oshibkaPorta(reality, zanyatyeDlyaReality(server.port_ssh));
  const oshH = oshibkaPorta(hy2);
  return (
    <div className="border-border flex flex-col gap-3 border-t px-4 py-4" data-testid="svoy-zadat-porty">
      <div className="grid max-w-[420px] grid-cols-2 items-start gap-3">
        <PolePorta podpis="reality, TCP" znachenie={reality} naVvod={zadatReality} placeholder="подобрать" oshibka={oshR} />
        <PolePorta podpis="hysteria2, UDP" znachenie={hy2} naVvod={zadatHy2} placeholder="подобрать" oshibka={oshH} />
      </div>
      <span className="text-fg-muted text-xs">Пустое поле: порт этого вида Affory подберёт сам. Ключи в списке сменятся, на телефоне их придётся добавить заново</span>
      <div className="flex gap-2">
        <Knopka rang="glavnaya" aktiven={aktiven && oshR === null && oshH === null} onClick={() => naPodobrat(portIzPolya(reality), portIzPolya(hy2))}>Поставить порты</Knopka>
        <Knopka rang="tekst" onClick={naOtmenu}>Отмена</Knopka>
      </div>
    </div>
  );
}

/** Строка ключа в карточке. Ключ, мёртвый на самом сервере, главнее всего:
 *  его чинит подбор портов, а не другой адрес. Дальше то, что он не прошёл
 *  из сети человека, хотя на сервере работает: так его и увидит человек. */
export function sostoyanieKlyucha(s: SvoyServer, p: ProverkaVps | undefined, transport: "reality-tcp" | "hy2"): string {
  const vid = transport === "hy2" ? "hy2" : "reality";
  const videla = portyNyneshnie(s, p).find((x) => x.vid === vid);
  if (videla && !videla.slushaet) return "на сервере не работает";
  if (s.ne_iz_seti.includes(transport)) return "из твоей сети не открывается";
  if (videla) return "работает на сервере";
  return s.porty_ruchnye ? "порт задан руками" : "порт подобран из твоей сети";
}

type Vopros = { vid: "snesti" | "zabyt"; server: SvoyServer };

/** Вход с адресом, портом и пользователем из записи о сервере. */
function formaIzZapisi(s: SvoyServer): Forma {
  return { ...PUSTAYA_FORMA, adres: s.adres, portSshPolya: s.port_ssh === 22 ? "" : String(s.port_ssh), polzovatel: s.polzovatel || "root" };
}

/** Мастер поверх записи о сервере. Пароль выключен этим или другим
 *  компьютером: войти можно только ключом. */
function formaPostavitZanovo(s: SvoyServer): Forma {
  return { ...formaIzZapisi(s), poverhZapisi: s.adres, sposob: s.parol_vyklyuchen ? "klyuch" : "parol" };
}

function KartochkaServera({ s, trevogi, hody, skryty, aktiven, zanyato, naDelo, naVzyat, naPostavitZanovo, naVopros, naSkryt, naOtchyot }: {
  s: SvoyServer;
  trevogi: SvoyaTrevoga[];
  hody: HodVps[];
  skryty: number[];
  aktiven: boolean;
  /** Над каким-то сервером уже идёт дело: служба второе не начнёт. */
  zanyato: boolean;
  naDelo: (imya: string, telo: Record<string, unknown>) => void;
  naVzyat: (s: SvoyServer) => void;
  naPostavitZanovo: (s: SvoyServer) => void;
  naVopros: (v: Vopros) => void;
  naSkryt: (id: number) => void;
  naOtchyot: (s: SvoyServer) => void;
}) {
  const [zadayu, zadatZadayu] = useState(false);
  const ocenka = ocenitServer(s, trevogi, hody);
  const vid = SOSTOYANIE[ocenka.sostoyanie];
  const upravlyaem = s.klyuch_upravleniya && !s.novyy_otpechatok && !s.klyuch_ne_chitaetsya;
  const mozhno = aktiven && upravlyaem && !zanyato;
  const delo = posledniy(hody.filter((h) => h.server === s.id && !skryty.includes(h.id)));
  const p = svezhayaProverka(s, hody);
  // «Повторить» повторяет упавшее дело. Снос и снятие ключа снова через
  // вопрос. Подбор повторяется сам по сети: порты, заданные руками, снимок
  // хода не хранит, их задают заново через «Задать порты».
  const povtor: Partial<Record<string, () => void>> = {
    proverka: () => naDelo("svoyVpsProverit", { id: s.id }),
    podbor: () => naDelo("svoyVpsPodobratPorty", { id: s.id }),
    parol: () => naDelo("svoyVpsParol", { id: s.id, vyklyuchit: !s.parol_vyklyuchen }),
    snos: () => naVopros({ vid: "snesti", server: s }),
    otpusk: () => naVopros({ vid: "zabyt", server: s }),
    vzyat: () => naVzyat(s),
  };
  const deystviya = {
    povtorit: delo ? povtor[delo.vid] : undefined,
    "vzyat-pod-upravlenie": () => naVzyat(s),
    "postavit-zanovo": () => naPostavitZanovo(s),
  };
  // Ключ этого компьютера оказался последним входом root: забыть можно и без
  // снятия (спека, таблица отказов, vps-last-login).
  const zabytBezSnyatiya = delo?.faza === "oshibka" && delo.otkaz?.kod === "vps-last-login"
    ? <Knopka rang="vtoraya" testId="svoy-zabyt-bez-snyatiya" aktiven={aktiven && !zanyato} onClick={() => naDelo("svoyVpsZabyt", { id: s.id })}>Забыть, не снимая ключа</Knopka>
    : undefined;
  const proveren = vozrast(s.proveren);
  return (
    <Panel testId={`svoy-server-${s.id}`}>
      <div className="flex flex-wrap items-center gap-3.5 px-4 py-4">
        <span className="text-fg-muted flex h-8 w-8 shrink-0 items-center justify-center"><IkServer className="h-[18px] w-[18px]" /></span>
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex flex-wrap items-center gap-2">
            <span className="text-foreground text-[15px] font-semibold tabular-nums">{s.adres}</span>
            <Teg ton={vid.ton} testId="svoy-sostoyanie">{vid.slovo}</Teg>
          </span>
          <span className="text-fg-muted text-xs">{proveren ? `Проверен ${proveren}` : "Ещё не проверялся с этого компьютера"}</span>
        </div>
        <Knopka rang="vtoraya" aktiven={mozhno} testId="svoy-proverit" onClick={() => naDelo("svoyVpsProverit", { id: s.id })}><IkObnovit className="h-3.5 w-3.5" />Проверить</Knopka>
        <MenyuDeystviy
          podpis="Действия с сервером"
          punkty={[
            { podpis: "Подобрать порты заново", aktiven: mozhno, naZhmyh: () => naDelo("svoyVpsPodobratPorty", { id: s.id }) },
            { podpis: "Задать порты", aktiven: mozhno, naZhmyh: () => zadatZadayu(true) },
            { podpis: "Скопировать отчёт", naZhmyh: () => naOtchyot(s) },
            { podpis: "Снести с сервера", opasnyy: true, aktiven: mozhno, naZhmyh: () => naVopros({ vid: "snesti", server: s }) },
            { podpis: "Забыть сервер", opasnyy: true, aktiven: aktiven && !zanyato, naZhmyh: () => naVopros({ vid: "zabyt", server: s }) },
          ]}
        />
      </div>
      {s.novyy_otpechatok && (
        <Plashka
          ton="danger"
          testId="svoy-klyuch-hosta"
          zagolovok="Ключ сервера сменился, Affory остановился и ничего не отправил"
          tekst={`Так бывает после переустановки системы в панели хостера. Если переустановки не было, на сервер не заходи и напиши хостеру. Новый отпечаток: ${s.novyy_otpechatok}`}
        >
          <Knopka rang="vtoraya" aktiven={aktiven && !zanyato} onClick={() => naDelo("svoyVpsPrinyatKlyuchHosta", { id: s.id, otpechatok: s.novyy_otpechatok ?? "" })}>Систему переустановили</Knopka>
        </Plashka>
      )}
      {!s.klyuch_upravleniya && !s.klyuch_ne_chitaetsya && (
        <Plashka
          ton="warn"
          testId="svoy-bez-klyucha"
          zagolovok="Ключ управления остался на другом компьютере"
          tekst="Ключи VPN работают. Чтобы проверять и менять сервер отсюда, войди паролем от хостера или своим ключом"
        >
          <Knopka rang="vtoraya" aktiven={aktiven && !zanyato} onClick={() => naVzyat(s)}>Взять под управление</Knopka>
        </Plashka>
      )}
      {s.klyuch_ne_chitaetsya && (
        <Plashka
          ton="warn"
          zagolovok="Ключ управления этого компьютера не читается"
          tekst={`${sPropisnoy(s.klyuch_ne_chitaetsya)}. Ключи VPN работают. Чтобы вернуть управление, забудь сервер и возьми его под управление заново`}
        />
      )}
      {ocenka.prichina && !(ocenka.izProverki && delo?.vid === "proverka" && delo.faza === "oshibka" && delo.otkaz?.tekst === s.proverka_otkaz) && (
        <Plashka ton={vid.kromka} zagolovok={ocenka.prichina} tekst={ocenka.sovet} />
      )}
      {delo && (
        <StrokaDela
          h={delo}
          adres={s.adres}
          aktiven={aktiven}
          zanyato={zanyato}
          bezSeti={ocenka.izSeti === true}
          naOtmenu={() => naDelo("svoyVpsOtmenit", {})}
          naSkryt={() => naSkryt(delo.id)}
          deystviya={deystviya}
          dop={zabytBezSnyatiya}
        />
      )}
      {zadayu && (
        <ZadatPorty
          server={s}
          aktiven={mozhno}
          naPodobrat={(reality, hy2) => {
            zadatZadayu(false);
            naDelo("svoyVpsPodobratPorty", { id: s.id, reality, hy2 });
          }}
          naOtmenu={() => zadatZadayu(false)}
        />
      )}
      <div className="border-border border-t">
        {s.reality !== undefined && (
          <Ryad nazvanie={<span className="flex items-baseline gap-2">reality<span className="text-fg-muted text-[13px] font-normal tabular-nums">TCP {s.reality}</span></span>}
            poyasnenie={sostoyanieKlyucha(s, p, "reality-tcp")} />
        )}
        {s.hy2 !== undefined && (
          <Ryad nazvanie={<span className="flex items-baseline gap-2">hysteria2<span className="text-fg-muted text-[13px] font-normal tabular-nums">UDP {s.hy2}</span></span>}
            poyasnenie={sostoyanieKlyucha(s, p, "hy2")} />
        )}
        <Ryad
          nazvanie="Сертификат"
          poyasnenie={s.prodlenie === "net"
            ? "Не выпущен: у хостера закрыты порты 80 и 443, поэтому hysteria2 не стоит"
            : `Let's Encrypt на адрес сервера, продлевается сам через порт ${s.prodlenie ?? "80"}`}
        >
          {s.sert_do !== undefined && <span className="text-fg-secondary text-[13px] tabular-nums">до {data(s.sert_do)}</span>}
        </Ryad>
        {upravlyaem && <RyadVhoda vkl={!s.parol_vyklyuchen} idyot={delo?.vid === "parol" && delo.faza === "idyot"} aktiven={mozhno} naSmenu={() => naDelo("svoyVpsParol", { id: s.id, vyklyuchit: !s.parol_vyklyuchen })} />}
        <Ryad nazvanie="Программа на сервере" poyasnenie={p?.programma_otkaz ? `не ответила: ${p.programma_otkaz}` : undefined}>
          <span className="text-fg-secondary text-[13px] tabular-nums">{p?.versiya ?? s.versiya ?? "версия неизвестна"}</span>
        </Ryad>
        {p && (p.pamyat_mb >= 0 || p.disk_mb >= 0) && (
          <Ryad nazvanie="Свободно на сервере">
            <span className="text-fg-secondary text-[13px] tabular-nums">
              {[p.pamyat_mb >= 0 ? `память ${mb(p.pamyat_mb)}` : "", p.disk_mb >= 0 ? `место ${mb(p.disk_mb)}` : ""].filter(Boolean).join(", ")}
            </span>
          </Ryad>
        )}
      </div>
    </Panel>
  );
}

/** Ход и итог дела, у которого нет карточки: сервер ещё не в списке (взять
 *  с проверки) или уже ушёл из него (снос, «забыть»). */
function DeloBezKartochki({ svoy, aktiven }: { svoy: SvoyVps; aktiven: boolean }) {
  const est = new Set((svoy.servery ?? []).map((s) => s.id));
  const delo = posledniy(svoy.hody
    .filter((h) => !svoy.skryty.includes(h.id) && h.vid !== "osmotr" && h.vid !== "ustanovka" && !(h.server && est.has(h.server))));
  if (!delo) return null;
  return (
    <Panel>
      <StrokaDela
        h={delo}
        adres={delo.server ? svoy.adresServera(delo.server) : undefined}
        aktiven={aktiven}
        naOtmenu={() => void svoy.komanda("svoyVpsOtmenit", {})}
        naSkryt={() => svoy.skryt(delo.id)}
        deystviya={{}}
      />
    </Panel>
  );
}

// ---------------------------------------------------------------- раздел

export function SvoyVpn(p: SvoyVpnProps) {
  const { svoy, aktiven } = p;
  const { master, zadatMaster, servery, hody, komanda, obnovit } = svoy;
  const [zhdyom, zadatZhdyom] = useState<string | null>(null);
  const [vopros, zadatVopros] = useState<Vopros | null>(null);
  const [otkazKomandy, zadatOtkazKomandy] = useState<OtkazVps | null>(null);
  const [vygruzka, zadatVygruzku] = useState<VygruzkaOtvet | null>(null);
  const [otkazVygruzki, zadatOtkazVygruzki] = useState<string | null>(null);
  const [itogOtchyota, zadatItogOtchyota] = useState<string | null>(null);

  // При открытии раздела список и ход заново: служба могла поменять запись,
  // пока окно смотрело на другую вкладку.
  useEffect(() => {
    obnovit();
  }, [obnovit]);

  // При открытии раздела проверить один давно не проверенный сервер
  // (З1: только если проверке больше 10 минут, каждая переписывает набор).
  // Список берётся тот, что пришёл уже после открытия: прежний мог устареть.
  // При открытом мастере проверки нет, иначе его шаги упрутся в занятую службу.
  const proverenoPriOtkrytii = useRef(false);
  const spisokDoOtkrytiya = useRef(servery);
  useEffect(() => {
    if (proverenoPriOtkrytii.current || servery === null || !aktiven) return;
    if (servery === spisokDoOtkrytiya.current) return;
    proverenoPriOtkrytii.current = true;
    if (master !== null || hody.some((h) => h.faza === "idyot")) return;
    const porog = Date.now() - 10 * 60 * 1000;
    const kogda = (s: SvoyServer) => (s.proveren ? Date.parse(s.proveren) : 0);
    const nado = servery
      .filter((s) => s.klyuch_upravleniya && !s.novyy_otpechatok && !s.klyuch_ne_chitaetsya && kogda(s) < porog)
      .sort((a, b) => kogda(a) - kogda(b));
    // Отказ (занято, нет связи) не повод для плашки: человек ничего не нажимал.
    if (nado[0]) void komanda("svoyVpsProverit", { id: nado[0].id });
  }, [servery, aktiven, hody, komanda, master]);

  const idyotDelo = hody.some((h) => h.faza === "idyot");

  /** Дело над сервером: отказ на старте встаёт строкой над списком. */
  const naDelo = async (imya: string, telo: Record<string, unknown>) => {
    zadatOtkazKomandy(null);
    zadatZhdyom(imya);
    const r = await komanda(imya, telo);
    zadatZhdyom(null);
    if ("otkaz" in r) {
      zadatOtkazKomandy(r.otkaz);
      return;
    }
    // Новый ключ хоста принят: проверка сразу, отказ её был про прежний.
    if (imya === "svoyVpsPrinyatKlyuchHosta") {
      const id = typeof telo.id === "string" ? telo.id : "";
      const pr = await komanda("svoyVpsProverit", { id });
      if ("otkaz" in pr) zadatOtkazKomandy(pr.otkaz);
    }
  };

  const naOtchyot = (s: SvoyServer) => {
    const h = posledniy(hody.filter((x) => x.server === s.id));
    const yadro = svezhayaProverka(s, hody)?.oshibka_yadra;
    const stroki = [
      `Affory, свой VPN: сервер ${s.adres}`,
      `программа: ${s.versiya ?? "неизвестна"}, порты: reality ${s.reality ?? "-"}, hysteria2 ${s.hy2 ?? "-"}${s.porty_ruchnye ? " (руками)" : ""}`,
      `сертификат: ${s.prodlenie ?? "-"}${s.sert_do ? `, до ${new Date(s.sert_do * 1000).toISOString()}` : ""}${s.sert_otkaz ? `, отказ продления: ${s.sert_otkaz}` : ""}`,
      `проверен: ${s.proveren ?? "никогда"}${s.proverka_otkaz ? `, отказ: ${s.proverka_otkaz}` : ""}`,
      `ключ управления: ${s.klyuch_upravleniya ? "есть" : "нет"}${s.novyy_otpechatok ? ", ключ хоста сменился" : ""}`,
      yadro ? `ошибка VPN на сервере: ${yadro}` : "",
      h ? otchyotDela(h, undefined, undefined) : "",
      p.versiya ?? "",
    ].filter((x) => x !== "");
    p.skopirovat(stroki.join("\n")).then(
      () => zadatItogOtchyota("Отчёт скопирован, в нём нет паролей и ключей"),
      (e: unknown) => zadatItogOtchyota(`Отчёт не скопирован: ${tekstOshibki(e)}`),
    );
  };

  const naVzyatIzKartochki = (s: SvoyServer) => {
    zadatMaster({ shag: "dostup", cel: "vzyat", forma: formaIzZapisi(s) });
  };

  // Установки на сервере нет или она не дошла: мастер ставит поверх записи.
  const naPostavitZanovo = (s: SvoyServer) => {
    zadatOtkazKomandy(null);
    zadatMaster({ shag: "dostup", cel: "postavit", forma: formaPostavitZanovo(s) });
  };

  const voprosOkno = vopros && (
    <OknoPodtverzhdeniya
      testId="svoy-vopros"
      zagolovok={vopros.vid === "snesti" ? `Снести VPN с сервера ${vopros.server.adres}?` : `Забыть сервер ${vopros.server.adres}?`}
      naOtmenu={() => zadatVopros(null)}
      otvety={vopros.vid === "snesti" ? (
        <Knopka rang="opasnaya" testId="svoy-snesti-da" onClick={() => {
          const s = vopros.server;
          zadatVopros(null);
          void naDelo("svoyVpsSnesti", { id: s.id });
        }}>Снести</Knopka>
      ) : (
        <>
          <Knopka rang="vtoraya" testId="svoy-zabyt-tolko" onClick={() => {
            const s = vopros.server;
            zadatVopros(null);
            void naDelo("svoyVpsZabyt", { id: s.id });
          }}>Только забыть</Knopka>
          {vopros.server.klyuch_upravleniya && !vopros.server.novyy_otpechatok && (
            <Knopka rang="opasnaya" testId="svoy-zabyt-snyat" onClick={() => {
              const s = vopros.server;
              zadatVopros(null);
              void naDelo("svoyVpsOtpustit", { id: s.id });
            }}>Снять ключ и забыть</Knopka>
          )}
        </>
      )}
    >
      {vopros.vid === "snesti" ? (
        <>
          <p>Affory уберёт с сервера свою программу, свои правила брандмауэра и настройки, а последним свой ключ входа. Система сервера останется</p>
          <p>Оба ключа уйдут из списка серверов только после удачного сноса. На телефонах они перестанут работать</p>
          {vopros.server.parol_vyklyuchen && <p>Вход по паролю вернётся таким, каким был до выключения</p>}
        </>
      ) : (
        <>
          <p>Ключи сервера уйдут из списка, запись и ключ управления сотрутся с этого компьютера. VPN на сервере работает дальше, ключи на телефонах тоже</p>
          {vopros.server.klyuch_upravleniya && !vopros.server.novyy_otpechatok && (
            <p>«Снять ключ и забыть» сначала уберёт с сервера ключ этого компьютера. Если это последний вход root, ключ останется и ничего не изменится</p>
          )}
        </>
      )}
    </OknoPodtverzhdeniya>
  );

  if (master) {
    return (
      <MasterSvoegoVpn
        {...p}
        m={master}
        zhdyom={zhdyom}
        zadatZhdyom={zadatZhdyom}
        vygruzka={vygruzka}
        zadatVygruzku={zadatVygruzku}
        otkazVygruzki={otkazVygruzki}
        zadatOtkazVygruzki={zadatOtkazVygruzki}
        naDelo={naDelo}
        otkazKomandy={otkazKomandy}
      />
    );
  }

  const naPostavit = () => {
    zadatOtkazKomandy(null);
    zadatMaster({ shag: "dostup", cel: "postavit", forma: PUSTAYA_FORMA });
  };

  const verh = (
    <>
      {otkazKomandy && <OtkazDela otkaz={otkazKomandy} deystviya={{ "otkryt-servery": p.naServery }} />}
      {itogOtchyota && <p role="status" className="text-fg-secondary text-xs">{itogOtchyota}</p>}
      <DeloBezKartochki svoy={svoy} aktiven={aktiven} />
    </>
  );

  if (servery === null) {
    return (
      <Kolonka svodka={svoy.spisokOtkaz ? "Список своих серверов не прочитан" : "Читаю список своих серверов"}>
        {svoy.spisokOtkaz && <Neudacha kod={svoy.spisokOtkaz.kod} tekst={svoy.spisokOtkaz.tekst} deystvie={obnovit} podpisDeystviya="Повторить" />}
      </Kolonka>
    );
  }

  if (servery.length === 0) {
    return <SvoyVpnPusto naPostavit={naPostavit} aktiven={aktiven} delo={verh} />;
  }

  const n = servery.length;
  return (
    <Kolonka
      svodka={n === 1
        ? "Ключи сервера лежат и в общем списке серверов"
        : `${n} ${slovoPosleChisla(n, "сервер", "сервера", "серверов")}, ключи каждого лежат и в общем списке`}
      deystviya={<Knopka rang="vtoraya" bolshaya aktiven={aktiven} testId="svoy-postavit-eshchyo" onClick={naPostavit}><IkPlyus className="h-4 w-4" />Поставить ещё</Knopka>}
    >
      {verh}
      {servery.map((s) => (
        <KartochkaServera
          key={s.id}
          s={s}
          trevogi={p.trevogi}
          hody={hody}
          skryty={svoy.skryty}
          aktiven={aktiven && zhdyom === null}
          zanyato={idyotDelo}
          naDelo={(imya, telo) => void naDelo(imya, telo)}
          naVzyat={naVzyatIzKartochki}
          naPostavitZanovo={naPostavitZanovo}
          naVopros={zadatVopros}
          naSkryt={svoy.skryt}
          naOtchyot={naOtchyot}
        />
      ))}
      {voprosOkno}
    </Kolonka>
  );
}

/** Мастер: доступ, проверка, установка и готово. Шаг держит App, ход дела
 *  приходит снимками службы. */
function MasterSvoegoVpn(p: SvoyVpnProps & {
  m: Master;
  zhdyom: string | null;
  zadatZhdyom: (s: string | null) => void;
  vygruzka: VygruzkaOtvet | null;
  zadatVygruzku: (v: VygruzkaOtvet | null) => void;
  otkazVygruzki: string | null;
  zadatOtkazVygruzki: (s: string | null) => void;
  naDelo: (imya: string, telo: Record<string, unknown>) => Promise<void>;
  otkazKomandy: OtkazVps | null;
}) {
  const { svoy, m, aktiven, zhdyom, zadatZhdyom } = p;
  const { zadatMaster, komanda, hody, servery } = svoy;
  const [itogOtchyota, zadatItogOtchyota] = useState<string | null>(null);
  const [otkazOtmeny, zadatOtkazOtmeny] = useState<{ id: number; otkaz: OtkazVps } | null>(null);
  const hod = "id" in m ? hody.find((h) => h.id === m.id) : undefined;

  // Служба отменяет без отказов, не дойти может сама команда. Отказ помнит
  // своё дело: на следующем шаге он не про него.
  const otmenit = async () => {
    zadatOtkazOtmeny(null);
    const id = hod?.id;
    const r = await komanda("svoyVpsOtmenit", {});
    if ("otkaz" in r && id !== undefined) zadatOtkazOtmeny({ id, otkaz: r.otkaz });
  };
  const plashkaOtmeny = otkazOtmeny && hod?.faza === "idyot" && hod.id === otkazOtmeny.id ? <OtkazDela otkaz={otkazOtmeny.otkaz} /> : null;

  const vyyti = () => {
    if (m.shag === "ustanovka") zapomnitUvidennuyu(m.id);
    p.zadatVygruzku(null);
    p.zadatOtkazVygruzki(null);
    zadatMaster(null);
  };

  const osmotret = async (forma: Forma) => {
    zadatZhdyom("svoyVpsOsmotret");
    const r = await komanda("svoyVpsOsmotret", teloDostupa(forma));
    zadatZhdyom(null);
    // Сообщение хостера уходит с шага: пароль из него уже в поле.
    const bezSoobshcheniya = { ...forma, soobshchenie: "" };
    if ("otkaz" in r) {
      zadatMaster({ shag: "dostup", cel: "postavit", forma, otkaz: r.otkaz });
      return;
    }
    if ("hod" in r) zadatMaster({ shag: "proverka", forma: bezSoobshcheniya, id: r.hod.id, pustoy: false });
  };

  const vzyat = async (forma: Forma, o?: OsmotrVps) => {
    zadatZhdyom("svoyVpsVzyat");
    const telo: Record<string, unknown> = { ...teloDostupa(forma), imya: "" };
    if (o) {
      telo.klyuch_hosta_tip = o.klyuch_hosta_tip;
      telo.klyuch_hosta = o.klyuch_hosta;
    }
    const r = await komanda("svoyVpsVzyat", telo);
    zadatZhdyom(null);
    if ("otkaz" in r) {
      zadatMaster({ shag: "dostup", cel: "vzyat", forma, otkaz: r.otkaz });
      return;
    }
    // Ход захвата виден в списке: строкой дела в карточке или над списком.
    zadatMaster(null);
  };

  const postavit = async (forma: Forma, o: OsmotrVps, idOsmotra: number) => {
    zadatZhdyom("svoyVpsUstanovit");
    const r = await komanda("svoyVpsUstanovit", {
      ...teloDostupa(forma),
      pustoy: true,
      klyuch_hosta_tip: o.klyuch_hosta_tip,
      klyuch_hosta: o.klyuch_hosta,
      imya: "",
      reality: portIzPolya(forma.portReality),
      hy2: portIzPolya(forma.portHy2),
      poverh: forma.poverhZapisi || undefined,
    });
    zadatZhdyom(null);
    if ("otkaz" in r) {
      zadatMaster({ shag: "proverka", forma, id: idOsmotra, pustoy: true, otkaz: r.otkaz });
      return;
    }
    if ("hod" in r) zadatMaster({ shag: "ustanovka", forma, id: r.hod.id });
  };

  switch (m.shag) {
    case "dostup":
      return (
        <KolonkaMastera shag="dostup" adres={chistyyAdres(m.forma.adres) || undefined} vzyat={m.cel === "vzyat"}>
          <ShagDostup
            cel={m.cel}
            forma={m.forma}
            otkaz={m.otkaz}
            idyot={zhdyom !== null}
            aktiven={aktiven}
            naSmenu={(forma) => zadatMaster({ ...m, forma })}
            vybratKlyuch={p.vybratKlyuch}
            ochistitBufer={p.ochistitBufer}
            naOtmenu={vyyti}
            naDalshe={() => void (m.cel === "vzyat" ? vzyat({ ...m.forma, soobshchenie: "" }) : osmotret(m.forma))}
          />
        </KolonkaMastera>
      );
    case "proverka":
      return (
        <KolonkaMastera shag="proverka" adres={chistyyAdres(m.forma.adres)}>
          {plashkaOtmeny}
          <ShagProverka
            h={hod}
            forma={m.forma}
            pustoy={m.pustoy}
            otkaz={m.otkaz}
            idyot={zhdyom !== null}
            aktiven={aktiven}
            naPustoy={(v) => zadatMaster({ ...m, pustoy: v })}
            naNazad={() => zadatMaster({ shag: "dostup", cel: "postavit", forma: m.forma })}
            naOtmenu={() => void otmenit()}
            naPostavit={(o) => void postavit(m.forma, o, m.id)}
            naVzyat={(o) => void vzyat(m.forma, o)}
            naPovtor={() => void osmotret(m.forma)}
            naServery={p.naServery}
            // Мимо naDelo: та после принятия сама начинает проверку сервера,
            // и осмотр ниже упёрся бы в занятую службу.
            naPrinyatKlyuch={(server, otpechatok) => void (async () => {
              const r = await komanda("svoyVpsPrinyatKlyuchHosta", { id: server, otpechatok });
              if ("otkaz" in r) {
                zadatMaster({ ...m, otkaz: r.otkaz });
                return;
              }
              await osmotret(m.forma);
            })()}
          />
        </KolonkaMastera>
      );
    case "ustanovka": {
      // Не стал ждать перепроверки: ключи записаны, она идёт в службе дальше.
      const gotovo = hod?.faza === "gotovo" || (m.neZhdat === true && hod?.faza === "idyot" && hod.ustanovlen !== undefined);
      // Сервер и адрес установки служба называет с первого события: без
      // формы (окно открыто посреди установки) адрес берётся из снимка.
      const id = hod?.ustanovlen?.server ?? hod?.server;
      const server = id ? servery?.find((s) => s.id === id) : undefined;
      const parol = id ? posledniy(hody.filter((h) => h.server === id && h.vid === "parol")) : undefined;
      const adres = m.forma ? chistyyAdres(m.forma.adres) : hod?.adres ?? (id ? svoy.adresServera(id) : undefined);
      if (gotovo && hod) {
        const ids = hod.ustanovlen?.ids ?? [];
        // Подключение к ключу, который провёз проверку: первому такому.
        const luchshiy = hod.ustanovlen?.klyuchi.findIndex((k) => k.vid === "vezyot") ?? -1;
        const itogParolya = parol && parol.faza !== "idyot"
          ? <StrokaDela h={parol} adres={adres} aktiven={aktiven} naOtmenu={() => undefined} naSkryt={() => svoy.skryt(parol.id)} deystviya={{}} />
          : null;
        return (
          <KolonkaMastera shag="gotovo" adres={adres}>
            {p.otkazKomandy && <OtkazDela otkaz={p.otkazKomandy} />}
            <ShagGotovo
              h={hod}
              server={server}
              idyotParol={parol?.faza === "idyot"}
              itogParolya={parol && !svoy.skryty.includes(parol.id) ? itogParolya : null}
              aktiven={aktiven}
              vygruzka={p.vygruzka}
              otkazVygruzki={p.otkazVygruzki}
              zhdyotVygruzka={zhdyom === "exportServers"}
              skopirovat={p.skopirovat}
              kodyQr={p.kodyQr}
              naPodklyuchit={() => {
                const cel = ids[luchshiy >= 0 ? luchshiy : 0];
                if (!cel) return;
                vyyti();
                p.naPodklyuchit(cel);
              }}
              naVygruzit={() => void (async () => {
                p.zadatOtkazVygruzki(null);
                zadatZhdyom("exportServers");
                const r = await komanda("exportServers", { ids });
                zadatZhdyom(null);
                if ("otkaz" in r) {
                  p.zadatOtkazVygruzki(`Ключи не выгружены: ${r.otkaz.tekst || tekstOtkaza[r.otkaz.kod]?.tekst || r.otkaz.kod}`);
                  return;
                }
                const v = vygruzkaIz("telo" in r ? r.telo : undefined);
                if (!v) {
                  p.zadatOtkazVygruzki("Служба ответила без ключей");
                  return;
                }
                p.zadatVygruzku(v);
              })()}
              naZakrytVygruzku={() => p.zadatVygruzku(null)}
              naVhod={() => {
                if (server) void p.naDelo("svoyVpsParol", { id: server.id, vyklyuchit: !server.parol_vyklyuchen });
              }}
              naZakryt={vyyti}
            />
          </KolonkaMastera>
        );
      }
      return (
        <KolonkaMastera shag="ustanovka" adres={adres}>
          {plashkaOtmeny}
          <ShagUstanovka
            h={hod}
            mozhnoPovtorit={m.forma !== null}
            aktiven={aktiven}
            idyot={zhdyom !== null}
            itogOtchyota={itogOtchyota}
            naOtmenu={() => void otmenit()}
            naOtchyot={() => {
              if (!hod) return;
              p.skopirovat(otchyotDela(hod, adres, p.versiya)).then(
                () => zadatItogOtchyota("Отчёт скопирован, в нём нет паролей и ключей"),
                (e: unknown) => zadatItogOtchyota(`Отчёт не скопирован: ${tekstOshibki(e)}`),
              );
            }}
            naPovtor={() => {
              zapomnitUvidennuyu(m.id);
              if (m.forma) void osmotret(m.forma);
              // Формы нет: окно открыто посреди установки. Сервер уже в разделе -
              // значит, это была установка поверх записи, и повтор тоже.
              else zadatMaster({
                shag: "dostup", cel: "postavit",
                forma: server ? formaPostavitZanovo(server) : { ...PUSTAYA_FORMA, adres: adres ?? "" },
              });
            }}
            naZakryt={vyyti}
            naServery={p.naServery}
            naNeZhdat={() => {
              // Окно, поднятое заново, не вернёт мастер к этой установке:
              // её ход виден в карточке.
              zapomnitUvidennuyu(m.id);
              zadatMaster({ ...m, neZhdat: true });
            }}
          />
        </KolonkaMastera>
      );
    }
  }
}
