import { useEffect, useMemo, useRef, useState } from "react";
import type { OtkazNaEkrane, OtkazStroki, Server, StatusOtvet } from "../protokol";
import { slovoPosleChisla } from "../chisla";
import { IkonkaKorzina, Karta, Knopka, Kolonka, MenyuUKursora, Neudacha, Pole, PoleTeksta, Razdel, Ryad, Shapka, Teg } from "./ui";
import { KnopkaSpravki, SpravkaProtokolov } from "./SpravkaProtokolov";
import { OknoKlyucha, Vygruzka, type KlyuchServera, type VygruzkaOtvet } from "./Vygruzka";
import { tekstOshibki } from "../ponyatno";

// Servers tab (task 4.9). Pure over props like every screen: App fetches
// listServers and hands the answer down whole; every button sends one named
// command up and waits for the list to come back changed.
//
// Two ids, two questions. `vybran` (and status.vybran_id) is the human's
// choice; status.nesushchiy_id is what the core actually carries. In auto
// mode the choice is empty by construction, and that is not "nothing chosen".

/** A subscription address is an http(s) URL and nothing else. */
export function pohozheNaAdres(t: string): boolean {
  const a = t.trim().toLowerCase();
  return a.startsWith("http://") || a.startsWith("https://");
}

/** Что лежит во вставке (26.09.2026). Одно поле вместо двух вкладок: адреса
 *  подписок уходят addSubscription, всё остальное одним текстом в addServers.
 *
 *  Схемы здесь больше не перечисляются. Список жил отдельно от разбора в
 *  службе и отстал от него: anytls:// и tuic:// кнопка буфера отвергала
 *  словами «не ссылка на сервер», хотя служба их принимала. Судит служба, а
 *  здесь отсекается только явный мусор: текст без единого «://», который не
 *  похож и на base64, каким подписки и чужие клиенты отдают список. */
export function razlozhitVstavku(tekst: string): { adresa: string[]; estKlyuchi: boolean } {
  const stroki = tekst.split(/\r\n|\r|\n/).map((s) => s.trim()).filter(Boolean);
  const adresa = stroki.filter(pohozheNaAdres);
  const ostalnye = stroki.filter((s) => !pohozheNaAdres(s));
  const base64 = /^[A-Za-z0-9+/=_-]{16,}$/.test(ostalnye.join(""));
  return { adresa, estKlyuchi: ostalnye.some((s) => s.includes("://")) || (ostalnye.length > 0 && base64) };
}

/** Ответ addServers, см. cmd/affory-svc/komandy_pachki.go. */
export interface ItogVstavki {
  dobavleno: number;
  obnovleno: number;
  uzhe_bylo: number;
  otkazy: OtkazStroki[];
}

function chislo(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

export function itogVstavkiIz(telo: unknown): ItogVstavki {
  const t = (telo && typeof telo === "object" ? telo : {}) as Record<string, unknown>;
  const otkazy = Array.isArray(t.otkazy)
    ? t.otkazy.filter((o): o is OtkazStroki => !!o && typeof o === "object" && typeof (o as OtkazStroki).stroka === "number" && typeof (o as OtkazStroki).prichina === "string")
    : [];
  return { dobavleno: chislo(t.dobavleno), obnovleno: chislo(t.obnovleno), uzhe_bylo: chislo(t.uzhe_bylo), otkazy };
}

/** Строка итога. Слова те же, что у QR с экрана (itogPachki в qr.go). */
export function strokaItoga(i: ItogVstavki): string {
  const chasti = [`добавлено ${i.dobavleno}`];
  if (i.obnovleno > 0) chasti.push(`переименовано ${i.obnovleno}`);
  if (i.uzhe_bylo > 0) chasti.push(`уже были ${i.uzhe_bylo}`);
  if (i.otkazy.length > 0) chasti.push(`пропущено ${i.otkazy.length}`);
  return chasti.join(", ");
}

export function vygruzkaIz(telo: unknown): VygruzkaOtvet | null {
  const t = (telo && typeof telo === "object" ? telo : {}) as Record<string, unknown>;
  if (typeof t.tekst !== "string" || typeof t.base64 !== "string") return null;
  const propushcheny = Array.isArray(t.propushcheny)
    ? t.propushcheny.filter((p): p is { imya: string; prichina: string } =>
        !!p && typeof p === "object" && typeof (p as { imya?: unknown }).imya === "string" && typeof (p as { prichina?: unknown }).prichina === "string")
    : [];
  const servery = Array.isArray(t.servery)
    ? t.servery.flatMap((x: unknown): KlyuchServera[] => {
        if (!x || typeof x !== "object") return [];
        const { id, imya, transport, adres, ssylka } = x as Record<string, unknown>;
        return typeof id === "string" && typeof imya === "string" && typeof transport === "string" && typeof adres === "string" && typeof ssylka === "string"
          ? [{ id, imya, transport, adres, ssylka }]
          : [];
      })
    : klyuchiIzTeksta(t.tekst);
  return { tekst: t.tekst, base64: t.base64, vsego: chislo(t.vsego), servery, propushcheny };
}

/** Строки выгрузки от службы старее окна, где поля servery ещё нет: имя из
 *  хвоста ссылки, протокол по схеме, адреса нет. Хуже, чем от службы, но
 *  лучше сплошного текста, от которого выгрузку и уводили (02.10.2026). */
function klyuchiIzTeksta(tekst: string): KlyuchServera[] {
  return tekst.split("\n").map((s) => s.trim()).filter(Boolean).map((ssylka, i) => {
    const reshyotka = ssylka.lastIndexOf("#");
    const imya = reshyotka >= 0 ? raskodirovat(ssylka.slice(reshyotka + 1)) : "";
    return { id: `stroka-${i}`, imya: imya || `сервер ${i + 1}`, transport: ssylka.split("://")[0] ?? "", adres: "", ssylka };
  });
}

/** Кривое процентное кодирование оставляет имя как пришло: по нему строку
 *  всё равно можно узнать. */
function raskodirovat(s: string): string {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

export interface SpisokServerov {
  versii?: Record<string, string>;
  servery: Server[];
  vybran: string;
  podpiska_zadana: boolean;
  /** Host of the subscription URL. The URL itself is a secret of the same
   *  class as a key and never leaves the service. */
  podpiska_uzel: string;
  podpiska_obnovlena?: string;
}

/** Одна подписка в списке экрана.
 *
 *  Адреса здесь нет: он секрет класса ключа и из службы не выезжает. Опознают
 *  подписку по узлу и по имени, которое человек ей дал. */
export interface PodpiskaNaEkrane {
  servery?: Server[];
  id: string;
  uzel: string;
  imya?: string;
  obnovlena?: string;
  aktivnaya: boolean;
  /** Сколько ключей у этой подписки готово. У запасной они приезжают обходом
   *  раз в 12 часов, поэтому переключение мгновенно. */
  serverov?: number;
  /** Причина последней неудачи обхода. Без неё живая запасная и просроченная
   *  выглядят одинаково, и узнать разницу можно только переключившись. */
  otkaz?: string;
}

/** Пинг одного сервера (01.10.2026): одно число, то же, что строка «Задержка»
 *  на главном экране и пинг в Discord, то есть один круг через сервер по
 *  прогретому соединению. Прежние два числа (узел мимо туннеля и полный
 *  запрос с рукопожатием) владелец снял: второе выходило втрое больше
 *  привычного. Отсутствие числа это отказ с текстом, а НЕ ноль: ноль
 *  читается как «мгновенно» и ставит мёртвый сервер первым. */
export interface ZamerZaderzhki {
  versiya?: string;
  id: string;
  ping_ms?: number | null;
  ping_otkaz?: string;
  /** Короткая причина отказа, которую назвало ядро (02.10.2026): встаёт в
   *  строку вместо «недоступен». Полная фраза остаётся в `ping_otkaz`. */
  ping_prichina?: string;
}

export interface ServeryProps {
  status: StatusOtvet;
  /** `null` until the first listServers answer arrives. */
  spisok: SpisokServerov | null;
  /** Why the list is missing, when it is missing because of a refusal and
   *  not because it is still on its way. `null` means "still on its way". */
  spisokOtkaz?: OtkazNaEkrane | null;
  /** Repeats listServers. A refusal without a way back is a dead end, and
   *  this tab had one: no list, no button, no form (03.09.2026). */
  obnovitSpisok?: () => void;
  /** naOtvet получает тело удачного ответа. naOtkaz забирает отказ себе
   *  вместо общего баннера: отказ вставки пачкой показывается в форме, у
   *  баннера для его кода слова про подписку, а не про вставленный текст. */
  naKomandu: (komanda: string, telo: unknown, naOtvet?: (telo: unknown) => void, naOtkaz?: (o: { kod: string; tekst: string }) => void) => void;
  /** Выгрузка: запись в буфер и QR рисует оболочка окна. */
  skopirovat?: (tekst: string) => Promise<void>;
  kodyQr?: (tekst: string) => Promise<string[]>;
  /** Команды, которые сейчас в полёте. Кнопка над долгой командой рисует
   *  вертушку сама: замер задержек и поход за подпиской занимают секунды, и
   *  неподвижный экран всё это время читается как зависшая программа. */
  zanyatyeKomandy?: Record<string, boolean>;
  /** Строки последней подписки, которые не разобрались. Пустой список и
   *  отсутствие это одно и то же: разговора нет. */
  otkazyPodpiski?: OtkazStroki[];
  /** Reads the clipboard through the shell; absent in tests that do not care. */
  chitatBufer?: () => Promise<string>;
  /** QR с экрана, делает оболочка. Нет пропа - нет кнопки. */
  qr?: QrSEkrana;
  /** Подписки списком: активная одна, остальные про запас. Пустой список это
   *  «подписок нет», и тогда раздела нет вовсе. */
  podpiski?: PodpiskaNaEkrane[];
  /** Последний замер задержек, по одному на сервер. Пусто значит «не мерили»,
   *  и тогда строки задержки нет вовсе: пустая строка врала бы про замер,
   *  которого не было. */
  zaderzhki?: ZamerZaderzhki[];
}

const TRANSPORT: Record<string, string> = {
  // xhttp left here ON PURPOSE after the core dropped it on 06.09.2026. The Go
  // registry no longer lists it, so nothing new can carry it, but records saved
  // by older versions still sit in people's lists. Delete this key and such a
  // record shows an empty cell, which reads as a broken entry rather than an
  // unsupported one. It names itself, and connecting to it fails by name.
  xhttp: "xhttp", "reality-tcp": "reality", ws: "websocket", grpc: "grpc", hy2: "hysteria2", ss: "shadowsocks",
  // Added 03.09.2026. A transport missing from this map shows as an empty
  // cell in the list, which reads as a broken record rather than a new one.
  httpupgrade: "httpupgrade", trojan: "trojan", "trojan-ws": "trojan + websocket",
  vmess: "vmess", "vmess-ws": "vmess + websocket",
  // Added 05.09.2026. The Go-side test TestEkranNazyvaetVseTransportyYadra now
  // compares this map against the core registry, so a transport added without
  // a name here fails the build instead of showing an empty cell.
  anytls: "anytls", tuic: "tuic",
  // Added 26.09.2026: reality over grpc became its own transport once the link
  // parser stopped folding it into plain grpc with ordinary TLS.
  "reality-grpc": "reality + grpc",
  // Added 28.09.2026: vless over bare TCP with ordinary TLS (audit 1.6.1, П1).
  "tls-tcp": "vless + tls",
};

/** Что нашлось в QR, без секретов: ключ именем, протоколом и адресом узла,
 *  подписка только узлом. См. svodkaQr в cmd/affory-ui/qr.go. */
export interface SvodkaQr {
  klyuchi: { imya: string; transport: string; adres: string }[];
  podpiski: string[];
  negodnyh: number;
}

/** QR с экрана в два шага (О8 аудита 1.6.1). Раньше найденное сразу уходило
 *  в службу, и любая страница с кодом на экране подсовывала свой сервер.
 *  nayti снимает экран и отдаёт сводку, dobavit отправляет ровно её и
 *  возвращает готовую строку итога, zabyt отменяет находку. */
export interface QrSEkrana {
  nayti: () => Promise<SvodkaQr>;
  dobavit: () => Promise<string>;
  zabyt: () => void;
}

export function svodkaQrIz(x: unknown): SvodkaQr {
  const o = (typeof x === "object" && x !== null ? x : {}) as Record<string, unknown>;
  const stroka = (v: unknown): v is string => typeof v === "string";
  const klyuchi = Array.isArray(o.klyuchi)
    ? o.klyuchi.flatMap((k: unknown) => {
      if (typeof k !== "object" || k === null) return [];
      const { imya, transport, adres } = k as Record<string, unknown>;
      return stroka(imya) && stroka(transport) && stroka(adres) ? [{ imya, transport, adres }] : [];
    })
    : [];
  return {
    klyuchi,
    podpiski: Array.isArray(o.podpiski) ? o.podpiski.filter(stroka) : [],
    negodnyh: typeof o.negodnyh === "number" ? o.negodnyh : 0,
  };
}

/** "3 ч назад" for the subscription row; `undefined` when unknown. */
export function vozrast(iso: string | undefined, seychas: number = Date.now()): string | undefined {
  if (!iso) return undefined;
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return undefined;
  const min = Math.max(0, Math.round((seychas - t) / 60000));
  if (min < 1) return "только что";
  if (min < 60) return `${min} мин назад`;
  const ch = Math.round(min / 60);
  if (ch < 48) return `${ch} ч назад`;
  return `${Math.round(ch / 24)} дн назад`;
}

function sovpadaet(s: Server, zapros: string): boolean {
  const z = zapros.trim().toLowerCase();
  if (!z) return true;
  return s.imya.toLowerCase().includes(z) || s.host.toLowerCase().includes(z);
}

export function Servery({ status, spisok, spisokOtkaz = null, obnovitSpisok, naKomandu, skopirovat, kodyQr, zanyatyeKomandy = {}, otkazyPodpiski = [], chitatBufer, qr, zaderzhki = [], podpiski = [] }: ServeryProps) {
  const aktiven = status.sostoyanie !== "sluzhba-molchit";
  const zhdyot = (k: string) => zanyatyeKomandy[k] === true;
  const [poisk, zadatPoisk] = useState("");
  // One "Добавить", one question: what is being added. A server link and a
  // subscription address used to live behind two buttons with two verbs
  // (owner's remark 02.09.2026), and that was two meanings for one act.
  const [dobavlyayu, zadatDobavlyayu] = useState(false);
  // Справка о протоколах: список даёт имена, но не даёт выбора между ними.
  const [spravka, zadatSpravku] = useState(false);
  const [ssylka, zadatSsylku] = useState("");
  // Итог последней вставки пачкой. Живёт до закрытия формы: пропущенные
  // строки человек ищет глазами в своём тексте, и итог не должен исчезать
  // вместе с обновлением списка.
  const [itog, zadatItog] = useState<ItogVstavki | null>(null);
  const [vygruzka, zadatVygruzku] = useState<VygruzkaOtvet | null>(null);
  // Outcome line under the link field: the clipboard held junk, the screen
  // held no QR, or a server was added from it. Never the link text itself.
  const [ishodVvoda, zadatIshodVvoda] = useState<string | null>(null);
  // The shell's refusal is a refusal, not a footnote: it used to be small grey
  // text under the field, past §9.1 and with no way to try again.
  // Своя фраза, а не OtkazNaEkrane: чтение QR делает окно, и кода на проводе у
  // этого отказа нет. Прежде здесь стоял выдуманный `qr-s-ekrana`, которого нет
  // ни в словаре протокола, ни в §9.1, ни в otkazy.ts.
  const [otkazQr, zadatOtkazQr] = useState<{ zagolovok: string; tekst: string } | null>(null);
  // Найденное в QR ждёт ответа человека: добавить или забыть (О8).
  const [naydenoQr, zadatNaydenoQr] = useState<SvodkaQr | null>(null);
  const [dobavlyayuQr, zadatDobavlyayuQr] = useState(false);
  // Caret goes where the human is about to type. Without it the focus stayed
  // on the segment button, and its :focus-visible ring hung around the form
  // like a selection nobody made (живой отзыв 13.09.2026).
  const poleSsylki = useRef<HTMLTextAreaElement>(null);
  // Two-click delete: the first click turns the icon into a question, the
  // second answers it. One click on a trash icon next to the row you are
  // hovering is how a server disappears by accident.
  const [udalyayu, zadatUdalyayu] = useState<string | null>(null);
  const [udalyayuPodpisku, zadatUdalyayuPodpisku] = useState<string | null>(null);
  // Меню правой кнопки: id строки и место, где его открыли. Место именно
  // курсора, а не строки: меню, появляющееся в стороне от нажатия, читается как
  // чужое.
  const [menyu, zadatMenyu] = useState<{ id: string; x: number; y: number; yVverh?: number } | null>(null);
  // Ключ одного сервера из меню (02.10.2026). Окно открывается сразу, а
  // ключ доезжает ответом службы.
  const [oknoKlyucha, zadatOknoKlyucha] = useState<{ id: string; imya: string; klyuch: KlyuchServera | null; otkaz: string | null } | null>(null);
  // Строка, чей ключ только что ушёл в буфер из меню. Меню к этому времени
  // закрыто, и без отметки в самой строке нажатие выглядело бы пустым.
  const [skopirovanKlyuch, zadatSkopirovanKlyuch] = useState<string | null>(null);
  useEffect(() => {
    if (!skopirovanKlyuch) return;
    const t = setTimeout(() => zadatSkopirovanKlyuch(null), 3000);
    return () => clearTimeout(t);
  }, [skopirovanKlyuch]);

  const servery = spisok?.servery ?? [];
  const vidimye = useMemo(() => servery.filter((s) => sovpadaet(s, poisk)), [servery, poisk]);
  // По id, а не поиском в списке на каждую строку: серверов бывает под шесть
  // десятков, и вложенный обход превратил бы отрисовку в квадрат.
  const poZaderzhkam = useMemo(() => new Map(zaderzhki.map((z) => [z.id, z])), [zaderzhki]);
  const vybran = spisok?.vybran || status.vybran_id || "";
  const vybranPropal = vybran !== "" && !servery.some((s) => s.id === vybran);
  const avto = status.rezhim_marshruta === "avto";
  const izPodpiski = servery.filter((s) => s.iz_podpiski).length;
  // Область автовыбора (A5). Считается по ВСЕМУ списку, а не по видимому:
  // поиск и свёрнутая полоса прячут строки на экране и не трогают то, из чего
  // выбирает автомат. Прежде область нигде не называлась вовсе.
  const vAvto = servery.filter((s) => !s.vne_avto).length;
  const posledniyVAvto = vAvto <= 1;

  // Служба старее окна не знает команды listSubscriptions, и список приходит
  // пустым. Одна строка из полей listServers это не украшение, а единственное,
  // что тогда вообще можно показать про подписку.
  const stroki: PodpiskaNaEkrane[] = podpiski.length > 0
    ? podpiski
    : spisok?.podpiska_zadana
      ? [{ id: "odna", uzel: spisok.podpiska_uzel, obnovlena: spisok.podpiska_obnovlena, aktivnaya: true }]
      : [];

  // Список серверов полосами, а не сплошным столбцом.
  //
  // Две подписки подряд дают полтора десятка одинаковых строк, и понять, какая
  // из них откуда, нельзя ничем: ключи активной лежат в общем списке, а остатки
  // прежней остаются в нём же до переподключения. Заголовок полосы отвечает на
  // это одним словом, а хвост строки («пропал из подписки», «добавлен вручную»)
  // после него не нужен: он говорил то же самое на каждой строке.
  //
  // Порядок полос задан смыслом, а не алфавитом: подписка, её остатки, свои
  // серверы по адресу, ручные.
  // Порядок ВНУТРИ полосы оставлен тот, что прислала служба: его задаёт сама
  // подписка, и своя сортировка меняла бы список под руками на каждом обходе.
  const polosy = useMemo(() => {
    const aktivnaya = stroki.find((p) => p.aktivnaya);
    const imyaPodpiski = aktivnaya?.imya || aktivnaya?.uzel || "";
    return [
      {
        klyuch: "podpiska",
        podpis: imyaPodpiski ? `Из подписки ${imyaPodpiski}` : "Из подписки",
        servery: vidimye.filter((s) => s.iz_podpiski && !s.uderzhan),
      },
      {
        klyuch: "uderzhannye",
        podpis: "Пропали из подписки, работают до переподключения",
        servery: vidimye.filter((s) => s.uderzhan),
      },
      ...[...new Set(vidimye.filter((s) => s.svoy && !s.iz_podpiski && !s.uderzhan).map((s) => s.host))].map((host) => ({
        klyuch: `svoy:${host}`,
        podpis: `Свой сервер ${host}`,
        servery: vidimye.filter((s) => s.svoy && !s.iz_podpiski && !s.uderzhan && s.host === host),
      })),
      {
        klyuch: "ruchnye",
        podpis: "Добавлены вручную",
        servery: vidimye.filter((s) => !s.iz_podpiski && !s.uderzhan && !s.svoy),
      },
    ].filter((p) => p.servery.length > 0);
  }, [vidimye, stroki]);

  // The form opens by itself only on the honest empty list (§9.2 first run).
  // A refused list is NOT an empty one, so it gets the header button instead.
  const pervyyZapusk = spisok !== null && servery.length === 0;

  // Открытая форма ставит курсор в поле.
  useEffect(() => {
    if (!dobavlyayu && !pervyyZapusk) return;
    poleSsylki.current?.focus();
  }, [dobavlyayu, pervyyZapusk]);

  const svodka = spisok === null
    ? spisokOtkaz
      ? "список серверов не прочитался"
      : undefined
    : servery.length === 0
      ? "серверов пока нет"
      : `${servery.length} ${sklon(servery.length)}${izPodpiski ? `, ${izPodpiski} из подписки` : ""} · ${
        avto ? "выбор автоматический" : vybranPropal ? "выбранного нет в списке" : `выбран ${imyaPo(servery, vybran)}`
      }`;

  // Одна дорога для поля и для буфера. Текст ключей в окно не возвращается:
  // из буфера он уходит в службу, минуя поле, и на экране остаётся только итог.
  const otpravit = (tekst: string, otkuda: "pole" | "bufer"): boolean => {
    const { adresa, estKlyuchi } = razlozhitVstavku(tekst);
    zadatOtkazQr(null);
    if (!estKlyuchi && adresa.length === 0) {
      zadatIshodVvoda(
        tekst.trim() === ""
          ? otkuda === "bufer" ? "буфер обмена пуст" : null
          : `${otkuda === "bufer" ? "в буфере" : "в поле"} не ключ и не адрес подписки`,
      );
      return false;
    }
    zadatIshodVvoda(null);
    zadatItog(null);
    // Форма остаётся открытой ради итога. На первом запуске она живёт внутри
    // пустого списка и пропала бы вместе с ним, как только список наполнится.
    zadatDobavlyayu(true);
    if (estKlyuchi) {
      // Весь текст, вместе со строками подписок: служба пропускает их сама, и
      // номера пропущенных строк совпадают с тем, что человек вставил.
      naKomandu("addServers", { tekst }, (telo) => zadatItog(itogVstavkiIz(telo)), (o) => zadatIshodVvoda(o.tekst));
    }
    // addSubscription, а не setSubscription: вторая подписка ложится ПРО ЗАПАС.
    // Адрес это секрет того же разряда, что и ключ: на экран он не попадает.
    for (const adres of adresa) naKomandu("addSubscription", { adres });
    return true;
  };
  const izBufera = async () => {
    if (!chitatBufer) return;
    otpravit(await chitatBufer(), "bufer");
  };
  const sEkrana = async () => {
    if (!qr) return;
    zadatOtkazQr(null);
    zadatIshodVvoda(null);
    zadatNaydenoQr(null);
    try {
      zadatNaydenoQr(await qr.nayti());
    } catch (e: unknown) {
      zadatOtkazQr({ zagolovok: "QR с экрана не прочитался", tekst: tekstOshibki(e) });
    }
  };
  const dobavitQr = async () => {
    if (!qr) return;
    zadatDobavlyayuQr(true);
    try {
      // Строка исхода приходит готовой: в коде может лежать и ключ, и адрес
      // подписки, и собрать фразу здесь значило бы гадать, что из двух
      // добавилось.
      const itog = await qr.dobavit();
      zadatIshodVvoda(itog || null);
    } catch (e: unknown) {
      zadatOtkazQr({ zagolovok: "QR не добавился", tekst: tekstOshibki(e) });
    } finally {
      zadatNaydenoQr(null);
      zadatDobavlyayuQr(false);
    }
  };
  const zabytQr = () => {
    if (naydenoQr) qr?.zabyt();
    zadatNaydenoQr(null);
  };
  const otpravitPole = () => {
    if (otpravit(ssylka, "pole")) zadatSsylku("");
  };
  const zakrytFormu = () => { zabytQr(); zadatDobavlyayu(false); zadatSsylku(""); zadatItog(null); zadatIshodVvoda(null); };

  // Ключ одного сервера: та же exportServers, но с одним id. Сервер, который
  // в ссылку не переводится, приходит в propushcheny со своей причиной.
  const vzyatKlyuch = (s: Server, naKlyuch: (k: KlyuchServera) => void, naOtkaz: (tekst: string) => void) => {
    naKomandu("exportServers", { ids: [s.id] }, (telo) => {
      const v = vygruzkaIz(telo);
      const k = v?.servery.find((x) => x.id === s.id) ?? v?.servery[0];
      if (k) naKlyuch(k);
      else naOtkaz(v?.propushcheny?.[0]?.prichina ?? "служба не отдала ключ");
    }, (o) => naOtkaz(o.tekst));
  };
  const pokazatKlyuch = (s: Server) => {
    zadatOknoKlyucha({ id: s.id, imya: s.imya, klyuch: null, otkaz: null });
    vzyatKlyuch(
      s,
      (klyuch) => zadatOknoKlyucha((o) => (o?.id === s.id ? { ...o, klyuch } : o)),
      (otkaz) => zadatOknoKlyucha((o) => (o?.id === s.id ? { ...o, otkaz } : o)),
    );
  };
  // Отказ копирования открывает окно ключа: в нём видно, почему не вышло, а
  // сам ключ можно выделить и скопировать руками.
  const skopirovatKlyuch = (s: Server) => {
    if (!skopirovat) return;
    vzyatKlyuch(
      s,
      (klyuch) => {
        skopirovat(klyuch.ssylka).then(
          () => zadatSkopirovanKlyuch(s.id),
          (e: unknown) => zadatOknoKlyucha({ id: s.id, imya: s.imya, klyuch, otkaz: `не скопировалось: ${tekstOshibki(e)}` }),
        );
      },
      (otkaz) => zadatOknoKlyucha({ id: s.id, imya: s.imya, klyuch: null, otkaz }),
    );
  };

  const otmena = !pervyyZapusk && <Knopka rang="tekst" onClick={zakrytFormu}>{itog ? "Готово" : "Отмена"}</Knopka>;
  // Служба ходит за подпиской по сети прямо в команде, и ответа ждать секунды.
  const zhdyomDobavleniya = zhdyot("addServers") || zhdyot("addSubscription");
  const knopkaQr = qr && (
    <Knopka
      rang="vtoraya"
      testId="qr-s-ekrana"
      aktiven={aktiven && !dobavlyayuQr}
      onClick={() => void sEkrana()}
      title="Окно спрячется, снимет экраны и найдёт на них QR. Годится ключ, пачка ключей и адрес подписки. Перед добавлением окно покажет найденное."
    >
      QR с экрана
    </Knopka>
  );
  const blokNaydenogo = naydenoQr && (
    <div className="flex flex-col gap-2 text-xs" data-testid="naydeno-qr">
      <span className="text-fg-secondary">Найдено в QR</span>
      <ul className="flex flex-col gap-0.5">
        {naydenoQr.klyuchi.map((k, i) => (
          <li key={`k${i}`} className="break-all">
            {`${k.imya} · ${TRANSPORT[k.transport] ?? k.transport} · ${k.adres}`}
          </li>
        ))}
        {naydenoQr.podpiski.map((uzel, i) => (
          <li key={`p${i}`} className="break-all">{`подписка с ${uzel}`}</li>
        ))}
      </ul>
      {naydenoQr.negodnyh > 0 && (
        <span className="text-fg-muted">
          {`${slovoPosleChisla(naydenoQr.negodnyh, "не разобралась", "не разобрались", "не разобрались")} ${naydenoQr.negodnyh} ${slovoPosleChisla(naydenoQr.negodnyh, "строка", "строки", "строк")}, добавится остальное`}
        </span>
      )}
      <div className="flex items-center gap-2">
        <Knopka rang="glavnaya" testId="qr-dobavit" zhdyot={dobavlyayuQr} aktiven={aktiven && !dobavlyayuQr} onClick={() => void dobavitQr()}>
          {dobavlyayuQr ? "Добавляю" : "Добавить"}
        </Knopka>
        {/* Не «Отмена»: та уже стоит у формы и закрывает её целиком. */}
        <Knopka rang="tekst" testId="qr-otmena" aktiven={!dobavlyayuQr} onClick={zabytQr}>
          Не добавлять
        </Knopka>
      </div>
    </div>
  );
  const forma = (
    <Karta testId="forma">
      <div className="flex flex-col gap-3 px-4 py-3">
        <PoleTeksta
          testId="ssylka"
          priv={poleSsylki}
          aria-label="ключи или адрес подписки"
          znachenie={ssylka}
          naVvod={zadatSsylku}
          placeholder={"vless://, hy2://, anytls://, tuic://, trojan://, ss://\nпо строке на ключ, одной закодированной строкой или адрес подписки https://"}
          aktiven={aktiven}
          strok={4}
        />
        <div className="flex flex-wrap items-center gap-2">
          <Knopka rang="glavnaya" testId="dobavit-ssylku" zhdyot={zhdyomDobavleniya} aktiven={aktiven && ssylka.trim() !== ""} onClick={otpravitPole}>
            {zhdyomDobavleniya ? "Добавляю" : "Добавить"}
          </Knopka>
          {chitatBufer && (
            <Knopka rang="vtoraya" testId="iz-bufera" aktiven={aktiven} onClick={() => void izBufera()}>
              из буфера
            </Knopka>
          )}
          {knopkaQr}
          <span className="flex-1" />
          {otmena}
        </div>
        {itog && (
          <div className="flex flex-col gap-0.5 text-xs" data-testid="itog-vstavki">
            <span className="text-fg-secondary">{strokaItoga(itog)}</span>
            {itog.otkazy.length > 0 && (
              <ul className="text-fg-muted">
                {itog.otkazy.map((o) => (
                  <li key={o.stroka}>{`строка ${o.stroka}: ${o.prichina}`}</li>
                ))}
              </ul>
            )}
          </div>
        )}
        {ishodVvoda && (
          <p className="text-fg-secondary text-xs" data-testid="ishod-vvoda">{ishodVvoda}</p>
        )}
        {blokNaydenogo}
        {otkazQr && (
          <Neudacha
            testId="otkaz-qr"
            zagolovok={otkazQr.zagolovok}
            tekst={otkazQr.tekst}
            deystvie={() => void sEkrana()}
            podpisDeystviya="Повторить"
          />
        )}
      </div>
    </Karta>
  );

  return (
    <Kolonka aria-label="Серверы">
      {spravka && (
        <SpravkaProtokolov
          zakryt={() => zadatSpravku(false)}
          svoi={servery.map((s) => s.transport)}
        />
      )}
      <Shapka
        zagolovok="Серверы"
        svodka={<span data-testid="svodka">{svodka}</span>}
        uZagolovka={<KnopkaSpravki onClick={() => zadatSpravku(true)} />}
      >
        {!pervyyZapusk && servery.length > 0 && (
          // С 01.10.2026 выгрузка прав администратора не просит (решение
          // владельца). Отказ уходит общим баннером.
          <Knopka
            rang="vtoraya"
            testId="vygruzit"
            zhdyot={zhdyot("exportServers")}
            aktiven={aktiven}
            onClick={() => naKomandu("exportServers", {}, (telo) => zadatVygruzku(vygruzkaIz(telo)))}
          >
            Выгрузить
          </Knopka>
        )}
        {!pervyyZapusk && (
          <Knopka rang="glavnaya" testId="dobavit" aktiven={aktiven && !dobavlyayu} onClick={() => zadatDobavlyayu(true)}>
            <Plyus />Добавить
          </Knopka>
        )}
      </Shapka>

      {spisok === null && spisokOtkaz && (
        // The dead end this screen used to be: no list, no button, no form.
        // The reason is named, the way back is one click, and adding a server
        // by link still works without any list at all.
        <Neudacha
          testId="otkaz-spiska"
          kod={spisokOtkaz.kod}
          tekst={spisokOtkaz.tekst}
          deystvie={obnovitSpisok}
          podpisDeystviya="Повторить"
        />
      )}

      {spisok === null && !spisokOtkaz && (
        <p className="text-fg-muted text-sm" data-testid="zagruzka">список серверов загружается</p>
      )}

      {spisok !== null && vybranPropal && (
        <div
          data-testid="vybran-propal"
          className="border-warn/35 bg-surface flex items-center gap-3 rounded-lg border px-4 py-3 text-sm"
        >
          <div className="flex flex-1 flex-col gap-0.5">
            <span className="text-foreground">выбранного сервера больше нет в подписке</span>
            <span className="text-fg-muted text-xs">работает первый по списку; выбери другой или обнови подписку</span>
          </div>
          <Teg ton="preduprezhdenie">нет в подписке</Teg>
        </div>
      )}

      {!pervyyZapusk && dobavlyayu && forma}

      {vygruzka && (
        <Vygruzka
          vygruzka={vygruzka}
          zakryt={() => zadatVygruzku(null)}
          skopirovat={skopirovat}
          kodyQr={kodyQr}
          nazvatTransport={nazvatTransport}
        />
      )}

      {oknoKlyucha && (
        <OknoKlyucha
          imya={oknoKlyucha.imya}
          klyuch={oknoKlyucha.klyuch}
          // Служба замолчала, пока ключ был в пути: ответа не будет, и окно
          // не должно обещать его вечно. Причину называет общий баннер.
          otkaz={oknoKlyucha.otkaz ?? (!aktiven && !oknoKlyucha.klyuch ? "служба не отвечает, ключ не получен" : null)}
          zakryt={() => zadatOknoKlyucha(null)}
          skopirovat={skopirovat}
          kodyQr={kodyQr}
          nazvatTransport={nazvatTransport}
        />
      )}

      {spisok !== null && (spisok.podpiska_zadana || stroki.length > 0) && (
        <Razdel nazvanie={stroki.length > 1 ? "Подписки" : "Подписка"}>
          <Karta testId="podpiska">
            {stroki.map((p) => (
              <Ryad
                key={p.id}
                testId={`podpiska-${p.id}`}
                nazvanie={
                  <span className="flex items-center gap-2">
                    {p.imya || p.uzel || "подписка задана"}
                  </span>
                }
                poyasnenie={[
                  vozrast(p.obnovlena) ? `обновлена ${vozrast(p.obnovlena)}` : "ещё не обновлялась",
                  `${p.serverov ?? (p.aktivnaya ? izPodpiski : 0)} ${sklon(p.serverov ?? (p.aktivnaya ? izPodpiski : 0))}`,
                  "проверка раз в 12 часов",
                  // Причина отказа последней в строке: она важнее остального,
                  // но и длиннее всего, а перенос строки тут один.
                  p.otkaz || undefined,
                ].filter(Boolean).join(" · ")}
                aktiven={aktiven}
              >
                <Knopka rang="vtoraya" testId={p.aktivnaya ? "obnovit-podpisku" : `obnovit-podpisku-${p.id}`} zhdyot={(zhdyot("refreshSubscription") || zhdyot(`refreshSubscription:${p.id}`))}
                        aktiven={aktiven} onClick={() => naKomandu("refreshSubscription", podpiski.length ? { id: p.id } : {})}>
                  {(zhdyot("refreshSubscription") || zhdyot(`refreshSubscription:${p.id}`)) ? "Спрашиваю" : <><Obnovit />Обновить</>}
                </Knopka>
                {/* Удаление в два нажатия, как у сервера: подписка уносит с
                    собой весь список ключей, а отмены у этого действия нет. */}
                {(podpiski.length > 0) && (
                  udalyayuPodpisku === p.id ? (
                    <Knopka
                      rang="opasnaya"
                      testId={`udalit-podpisku-${p.id}`}
                      aktiven={aktiven}
                      onClick={() => { naKomandu("removeSubscription", { id: p.id }); zadatUdalyayuPodpisku(null); }}
                    >
                      удалить?
                    </Knopka>
                  ) : (
                    <Knopka
                      rang="tekst"
                      testId={`udalit-podpisku-${p.id}`}
                      // Кнопка рисуется одной иконкой, и без подписи у неё нет
                      // имени вовсе: в дереве доступности 12.09.2026 её было не
                      // отличить от соседних корзин, и с клавиатуры тоже.
                      aria-label={`удалить подписку ${p.imya || p.uzel}`}
                      aktiven={aktiven}
                      onClick={() => zadatUdalyayuPodpisku(p.id)}
                    >
                      <IkonkaKorzina />
                    </Knopka>
                  )
                )}
              </Ryad>
            ))}
          </Karta>

          {/* Строки, которые панель прислала, а клиент не понял. Без этой
              карточки они исчезают молча, и человек видит просто меньше
              серверов, чем ожидал. Причина приходит от службы уже без самой
              ссылки: в ней uuid и ключи. */}
          {otkazyPodpiski.length > 0 && (
            <Karta testId="otkazy-podpiski">
              <Ryad
                nazvanie={`${otkazyPodpiski.length} ${slovoPosleChisla(otkazyPodpiski.length, "строка не разобрана", "строки не разобраны", "строк не разобрано")}`}
                poyasnenie="остальные серверы подписки работают"
                aktiven={aktiven}
              />
              <ul className="px-4 pb-3 text-xs text-neutral-400">
                {otkazyPodpiski.map((o) => (
                  <li key={o.stroka} className="py-0.5">
                    строка {o.stroka}: {o.prichina}
                  </li>
                ))}
              </ul>
            </Karta>
          )}
        </Razdel>
      )}

      {pervyyZapusk && (
        <Razdel nazvanie="Первый сервер" aria-label="Первый сервер">
          <p className="text-fg-secondary text-sm" data-testid="pusto">
            серверов пока нет: вставь ссылку на сервер или адрес подписки
          </p>
          {forma}
        </Razdel>
      )}

      {spisok !== null && servery.length > 0 && (
        <Razdel nazvanie="Серверы" aria-label="Список серверов">
          <div className="flex items-center gap-2">
            <Pole
              tip="search"
              testId="poisk"
              aria-label="поиск"
              znachenie={poisk}
              naVvod={zadatPoisk}
              placeholder="Поиск по имени или адресу"
              className="w-full"
            />
            {/* Работает и при выключенном VPN: служба поднимает для замера
                своё ядро без туннеля, а выбирать сервер человеку надо как раз
                до подключения. */}
            <Knopka
              rang="vtoraya"
              testId="zamerit-zaderzhki"
              zhdyot={zhdyot("measureDelays")}
              aktiven={aktiven}
              onClick={() => naKomandu("measureDelays", {})}
            >
              {zhdyot("measureDelays") ? "Меряю" : "Проверить"}
            </Knopka>
          </div>
          {/* Из чего выбирает автомат, прежде не говорил ни один экран: политика
              была, объяснения не было. Строка стоит над списком, а не внутри
              карточки, потому что отвечает на вопрос про весь список сразу. */}
          <p className="text-fg-muted text-xs" data-testid="oblast-avto">
            {vAvto === servery.length
              ? `Автовыбор перебирает все ${servery.length}: серверы активной подписки и добавленные вручную.`
              : `Автовыбор перебирает ${vAvto} из ${servery.length}: остальные ты убрал правой кнопкой.`}
            {" Серверы запасных подписок в него не входят."}
          </p>
          <div
            role="listbox"
            aria-label="серверы"
            className="bg-surface border-border max-h-[60vh] overflow-y-auto rounded-lg border"
          >
            {vidimye.length === 0 && (
              <p className="text-fg-muted px-4 py-3 text-sm">ничего не найдено</p>
            )}
            {polosy.map((polosa) => (
            <div key={polosa.klyuch} role="group" aria-label={polosa.podpis} data-testid={`polosa-${polosa.klyuch}`}>
              <div className="bg-elevated text-fg-muted border-border border-t px-3.5 py-1.5 text-[11px] font-medium tracking-wide">
                {polosa.podpis}
              </div>
              {polosa.servery.map((s) => {
              const on = s.id === vybran;
              const neset = s.id === status.nesushchiy_id;
              return (
                <div
                  key={s.id}
                  role="option"
                  aria-selected={on}
                  tabIndex={0}
                  data-testid={`server-${s.id}`}
                  onClick={() => aktiven && naKomandu("setServer", { id: s.id })}
                  onContextMenu={(e) => {
                    e.preventDefault();
                    if (aktiven) zadatMenyu({ id: s.id, x: e.clientX, y: e.clientY });
                  }}
                  onKeyDown={(e) => {
                    // Enter с корзины внутри строки всплывает сюда же: он жмёт
                    // корзину, а сервер не выбирает.
                    if (e.key === "Enter" && e.target === e.currentTarget && aktiven) naKomandu("setServer", { id: s.id });
                    // Тот же путь без мыши. Меню, доступное только правой
                    // кнопкой, это функция, которой для половины людей нет.
                    if ((e.key === "ContextMenu" || (e.key === "F10" && e.shiftKey)) && aktiven) {
                      e.preventDefault();
                      const r = e.currentTarget.getBoundingClientRect();
                      zadatMenyu({ id: s.id, x: r.left + 24, y: r.bottom, yVverh: r.top });
                    }
                  }}
                  className={
                    "border-border hover:bg-fill-subtle group flex h-11 cursor-pointer items-center gap-3 border-t px-3.5 text-[13px] first:border-t-0 " +
                    (on ? "bg-fill-subtle shadow-[inset_2px_0_0_var(--color-accent-ink)]" : "")
                  }
                >
                  <div className="flex min-w-0 flex-1 items-baseline gap-2.5">
                    <span
                      data-testid="imya-servera"
                      className={on ? "text-accent-ink min-w-0 truncate text-sm font-medium" : "text-foreground min-w-0 truncate text-sm font-medium"}
                    >
                      {s.imya}
                    </span>
                    <span className="text-fg-muted truncate text-xs">
                      {s.host}:{s.port} · {TRANSPORT[s.transport] ?? s.transport}
                    </span>
                  </div>
                  {skopirovanKlyuch === s.id && <Teg ton="akcent" testId={`klyuch-skopirovan-${s.id}`}>ключ скопирован</Teg>}
                  <Zaderzhka zamer={poZaderzhkam.get(s.id)} />
                  {s.s_pinom && <Teg ton="akcent">сертификат закреплён</Teg>}
                  {s.nebezopasnyy_ignorirovan && <Teg ton="preduprezhdenie">проверка сертификата включена принудительно</Teg>}
                  {s.otpechatok_ne_proveryaetsya && <Teg ton="preduprezhdenie">отпечаток сертификата не проверяется</Teg>}
                  {neset && <Teg ton="akcent">активен</Teg>}
                  {/* Состояние, которое человек задал сам, обязано быть видно
                      без открытия меню: иначе «почему автомат его не берёт»
                      остаётся вопросом без ответа на экране. */}
                  {s.vne_avto && <Teg testId={`vne-avto-${s.id}`}>не в автовыборе</Teg>}
                  {udalyayu === s.id ? (
                    <Knopka
                      rang="opasnaya"
                      testId={`udalit-${s.id}`}
                      aktiven={aktiven}
                      onClick={(e) => { e.stopPropagation(); naKomandu("removeServer", { id: s.id }); zadatUdalyayu(null); }}
                    >
                      удалить?
                    </Knopka>
                  ) : (
                    <Knopka
                      rang="tekst"
                      testId={`udalit-${s.id}`}
                      aria-label={`удалить ${s.imya}`}
                      aktiven={aktiven}
                      className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                      onClick={(e) => { e.stopPropagation(); zadatUdalyayu(s.id); }}
                    >
                      <IkonkaKorzina />
                    </Knopka>
                  )}
                </div>
              );
              })}
            </div>
            ))}
          </div>
        </Razdel>
      )}

      {menyu && (() => {
        const s = servery.find((x) => x.id === menyu.id);
        if (!s) return null;
        return (
          <MenyuUKursora
            x={menyu.x}
            y={menyu.y}
            yVverh={menyu.yVverh}
            testId="menyu-servera"
            podpis={`Что сделать с сервером ${s.imya}`}
            naZakrytie={() => zadatMenyu(null)}
            punkty={[
              ...(skopirovat ? [{ podpis: "Скопировать ключ", naZhmyh: () => skopirovatKlyuch(s) }] : []),
              { podpis: "Показать ключ и QR", naZhmyh: () => pokazatKlyuch(s) },
              s.vne_avto
                ? {
                    podpis: "Вернуть в автовыбор",
                    naZhmyh: () => naKomandu("setAutoMember", { id: s.id, uchastvuet: true }),
                  }
                : {
                    podpis: "Убрать из автовыбора",
                    // Последнего убрать нельзя: пустой автомат это конфиг,
                    // который ядро отвергнет целиком. Гасим пункт здесь, чтобы
                    // человек не жал на кнопку ради отказа.
                    aktiven: !posledniyVAvto,
                    naZhmyh: () => naKomandu("setAutoMember", { id: s.id, uchastvuet: false }),
                  },
            ]}
          />
        );
      })()}
    </Kolonka>
  );
}

/** Пинг сервера одним числом. Отсутствие числа это отказ, не ноль.
 *
 *  Ноль на экране читается как «мгновенно» и ставит сервер первым по пингу,
 *  то есть ровно наверх списка, что противоположно правде о мёртвом. Отказ
 *  в строке короткий, причина целиком в подсказке: длинный текст в строке
 *  сервера не помещается и наезжает на соседние (16.09.2026). Если ядро
 *  назвало причину, в строке она, а не «недоступен» (02.10.2026). */
export function Zaderzhka({ zamer }: { zamer?: ZamerZaderzhki }) {
  if (!zamer) return null;
  const tekst = typeof zamer.ping_ms === "number" ? `${zamer.ping_ms} мс`
    : zamer.ping_prichina || (zamer.ping_otkaz ? "недоступен" : "не измерен");
  return (
    <span
      data-testid={`zaderzhka-${zamer.id}`}
      className="text-fg-muted shrink-0 text-xs tabular-nums"
      title={zamer.ping_otkaz || undefined}
    >
      {tekst}
    </span>
  );
}

function nazvatTransport(t: string): string {
  return TRANSPORT[t] ?? t;
}

function imyaPo(servery: Server[], id: string): string {
  return servery.find((s) => s.id === id)?.imya ?? id;
}

function sklon(n: number): string {
  const d = n % 10;
  const s = n % 100;
  if (s >= 11 && s <= 14) return "серверов";
  if (d === 1) return "сервер";
  if (d >= 2 && d <= 4) return "сервера";
  return "серверов";
}

// Three 16px line icons, inline: no icon library for three glyphs.
function Plyus() {
  return <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round"><path d="M12 5v14M5 12h14" /></svg>;
}
function Obnovit() {
  return <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M21 12a9 9 0 1 1-3-6.7M21 3v6h-6" /></svg>;
}
