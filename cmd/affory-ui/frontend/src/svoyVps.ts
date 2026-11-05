import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from "react";
import type { Kadr } from "./most";
import type { SvoyaTrevoga } from "./protokol";
import { tekstOshibki } from "./ponyatno";
import { KOD_OBOLOCHKI } from "./ekrany/otkazy";

// Раздел «Свой VPN» (план 2026-11-01, Ж2): ответы команд svoyVps*, их разбор
// и связь окна со службой. Каждая команда дольше срока канала идёт в службе
// заданием: ответ сразу отдаёт начальный снимок, ход приходит событием
// svoy-vps-hod (в нём только номер, вид, фаза и шаг), полный снимок отдаёт
// svoyVpsHod. Формы повторяют cmd/affory-svc/svoy_vps*.go руками, как
// protokol.ts повторяет kadr.go.

export type Faza = "idyot" | "gotovo" | "oshibka" | "otmeneno";

/** Ключ в ходе подбора и проверки. `vid` пуст, пока ключ проверяется. */
export interface KlyuchHoda {
  transport: string;
  port: number;
  vid?: string;
  otkaz?: string;
  /** Ключ не провёз, а его порт на этом компьютере перехватывает эта
   *  программа (Avast на почтовых портах). */
  pomeha?: string;
}

/** Кандидат подбора, выпавший до проверки. */
export interface OtsevHoda {
  proto: string;
  port: number;
  prichina: string;
  process?: string;
}

export interface ZamechanieVps {
  kod: string;
  stop: boolean;
  tekst: string;
}

/** Шаг «Проверка» мастера. */
export interface OsmotrVps {
  put: string;
  root: boolean;
  klyuch_hosta_tip: string;
  klyuch_hosta: string;
  otpechatok: string;
  sistema?: string;
  zamechaniya: ZamechanieVps[];
  mozhno_stavit: boolean;
}

/** Ключи записаны: снимок несёт их ещё до перепроверки из сети. */
export interface UstanovkaVps {
  server: string;
  klyuchi: KlyuchHoda[];
  ids: string[];
  /** Виды ключей (reality-tcp, hy2), что не прошли из сети этого компьютера. */
  ne_iz_seti: string[];
}

export interface OtkazVps {
  kod: string;
  tekst: string;
  klyuch_ostalsya?: boolean;
  podozhdat?: number;
  otpechatok?: string;
  /** Ключи не прошли из сети человека: это же лежит в записи о сервере. */
  iz_seti?: boolean;
}

export interface PortVps {
  vid: string;
  port: number;
  slushaet: boolean;
}

export interface ProverkaVps {
  versiya?: string;
  programma_otkaz?: string;
  yadro_zhivo: boolean;
  oshibka_yadra?: string;
  porty: PortVps[];
  sert_do?: number;
  sert_trevoga?: boolean;
  sert_otkaz?: string;
  maska: string;
  maska_otkaz?: string;
  pamyat_mb: number;
  disk_mb: number;
}

export interface ParolVps {
  vyklyuchen: boolean;
  /** После включения: vklyuchen, vyklyuchen_chuzhim или neizvestno. */
  snaruzhi?: string;
  fayly?: string[];
}

/** Итог обновления программы сервера: номера до и после. uzhe - эта
 *  программа уже стояла. ne_proshli - виды ключей, что провозили из сети до
 *  обновления, а после него нет. */
export interface ObnovlenieVps {
  bylo?: string;
  versiya: string;
  uzhe: boolean;
  ne_proshli: string[];
}

/** Смена маски (МА2): svoya - что ввёл человек, brend_soglasen - согласен
 *  на известное имя, imya и istochnik - что встало на сервер. */
export interface MaskaVps {
  svoya?: string;
  brend_soglasen?: boolean;
  imya?: string;
  istochnik?: string;
}

/** Снимок задания своего сервера (HodVps службы). */
export interface HodVps {
  id: number;
  vid: string;
  faza: Faza;
  shag?: string;
  server?: string;
  planovaya?: boolean;
  /** Адрес сервера установки: у окна, открытого посреди неё, формы нет. */
  adres?: string;
  put?: string;
  klyuch?: KlyuchHoda;
  proverki: KlyuchHoda[];
  otseyany: OtsevHoda[];
  krug?: number;
  sleduyushchiy?: number;
  /** Круг детектора кончился плохо, а повторы ещё будут: что он показал и
   *  когда повторы (время ISO). */
  krug_tekst?: string;
  povtory: string[];
  osmotr?: OsmotrVps;
  ustanovlen?: UstanovkaVps;
  snyat?: { parol_vernulsya: boolean };
  otpushchen?: { klyucha_ne_bylo: boolean };
  parol?: ParolVps;
  obnovlenie?: ObnovlenieVps;
  maska?: MaskaVps;
  proverka?: ProverkaVps;
  otkaz?: OtkazVps;
  preduprezhdeniya: OtkazVps[];
  nachalo?: string;
  konec?: string;
}

/** Запись о своём сервере для карточки (SvoyServerDlyaEkrana службы). */
export interface SvoyServer {
  id: string;
  imya: string;
  adres: string;
  port_ssh: number;
  polzovatel: string;
  parol_vyklyuchen: boolean;
  versiya?: string;
  maska: string;
  /** Откуда маска: sosedi, zapas, brend или svoya. Пусто - сервер 1.11.0
   *  или взят под управление. */
  maska_istochnik?: string;
  /** Программа сервера не достала маску три проверки подряд (МА3): чем
   *  кончилась последняя. */
  maska_ne_otvechaet?: string;
  reality?: number;
  hy2?: number;
  porty_ruchnye: boolean;
  /** 80, 443 или net: сертификата нет, hysteria2 не стоит. */
  prodlenie?: string;
  servery: string[];
  postavlen?: string;
  sert_do?: number;
  sert_trevoga: boolean;
  sert_otkaz?: string;
  sert_proveren?: string;
  proveren?: string;
  proverka_otkaz?: string;
  /** Виды ключей, что не прошли из сети этого компьютера при последней
   *  установке или подборе, и почему. Проверка сервера по кнопке гоняет из
   *  сети ключи видов с работающим портом и убирает провёзшие и те, чьего
   *  ключа больше нет. */
  ne_iz_seti: string[];
  set_tekst?: string;
  klyuch_upravleniya: boolean;
  klyuch_ne_chitaetsya?: string;
  otpechatok?: string;
  novyy_otpechatok?: string;
  /** Эта сборка ставит программу сервера новее той, что на нём: её номер. */
  novaya_versiya?: string;
}

// Разбор. Тело приезжает с провода, и служба старее окна может прислать
// другую форму, поэтому проверяется каждое поле. Запись без того, по чему
// её узнают (id, адрес, номер задания), отбрасывается целиком; необязательное
// поле не той формы становится отсутствием, а не нулём.

type Obekt = Record<string, unknown>;

function obekt(v: unknown): Obekt | null {
  return v !== null && typeof v === "object" && !Array.isArray(v) ? (v as Obekt) : null;
}

function stroka(v: unknown): string | undefined {
  return typeof v === "string" && v !== "" ? v : undefined;
}

function chislo(v: unknown): number | undefined {
  return typeof v === "number" && Number.isFinite(v) ? v : undefined;
}

function spisok<T>(v: unknown, razobrat: (x: unknown) => T | null): T[] {
  return Array.isArray(v) ? v.flatMap((x) => { const t = razobrat(x); return t === null ? [] : [t]; }) : [];
}

// Нулевое время Go («0001-01-01T00:00:00Z») это «не было», а не первый год.
function vremya(v: unknown): string | undefined {
  const s = stroka(v);
  if (s === undefined) return undefined;
  const t = Date.parse(s);
  return Number.isFinite(t) && new Date(t).getUTCFullYear() > 2000 ? s : undefined;
}

function klyuchHodaIz(v: unknown): KlyuchHoda | null {
  const o = obekt(v);
  const transport = stroka(o?.transport);
  const port = chislo(o?.port);
  if (!o || transport === undefined || port === undefined) return null;
  return { transport, port, vid: stroka(o.vid), otkaz: stroka(o.otkaz) };
}

function otsevIz(v: unknown): OtsevHoda | null {
  const o = obekt(v);
  const proto = stroka(o?.proto);
  const port = chislo(o?.port);
  const prichina = stroka(o?.prichina);
  if (!o || proto === undefined || port === undefined || prichina === undefined) return null;
  return { proto, port, prichina, process: stroka(o.process) };
}

function otkazVpsIz(v: unknown): OtkazVps | null {
  const o = obekt(v);
  const kod = stroka(o?.kod);
  if (!o || kod === undefined) return null;
  return {
    kod,
    tekst: stroka(o.tekst) ?? "",
    klyuch_ostalsya: o.klyuch_ostalsya === true || undefined,
    podozhdat: chislo(o.podozhdat),
    otpechatok: stroka(o.otpechatok),
    iz_seti: o.iz_seti === true || undefined,
  };
}

function osmotrIz(v: unknown): OsmotrVps | null {
  const o = obekt(v);
  if (!o) return null;
  const zamechaniya = spisok(o.zamechaniya, (x) => {
    const z = obekt(x);
    const kod = stroka(z?.kod);
    const tekst = stroka(z?.tekst);
    return z && kod !== undefined && tekst !== undefined ? { kod, stop: z.stop === true, tekst } : null;
  });
  const klyuch_hosta = stroka(o.klyuch_hosta);
  const klyuch_hosta_tip = stroka(o.klyuch_hosta_tip);
  // Без ключа хоста установка не войдёт: она входит, только если сервер
  // предъявит ключ с осмотра.
  if (klyuch_hosta === undefined || klyuch_hosta_tip === undefined) return null;
  return {
    put: stroka(o.put) ?? "",
    root: o.root === true,
    klyuch_hosta_tip,
    klyuch_hosta,
    otpechatok: stroka(o.otpechatok) ?? "",
    sistema: stroka(o.sistema),
    zamechaniya,
    mozhno_stavit: o.mozhno_stavit === true,
  };
}

function ustanovkaIz(v: unknown): UstanovkaVps | null {
  const o = obekt(v);
  const server = stroka(o?.server);
  if (!o || server === undefined) return null;
  return {
    server,
    klyuchi: spisok(o.klyuchi, klyuchHodaIz),
    ids: spisok(o.ids, (x) => stroka(x) ?? null),
    ne_iz_seti: spisok(o.ne_iz_seti, (x) => stroka(x) ?? null),
  };
}

function proverkaIz(v: unknown): ProverkaVps | null {
  const o = obekt(v);
  if (!o) return null;
  return {
    versiya: stroka(o.versiya),
    programma_otkaz: stroka(o.programma_otkaz),
    yadro_zhivo: o.yadro_zhivo === true,
    oshibka_yadra: stroka(o.oshibka_yadra),
    porty: spisok(o.porty, (x) => {
      const p = obekt(x);
      const vid = stroka(p?.vid);
      const port = chislo(p?.port);
      return p && vid !== undefined && port !== undefined ? { vid, port, slushaet: p.slushaet === true } : null;
    }),
    sert_do: chislo(o.sert_do),
    sert_trevoga: o.sert_trevoga === true || undefined,
    sert_otkaz: stroka(o.sert_otkaz),
    maska: stroka(o.maska) ?? "ne-provereno",
    maska_otkaz: stroka(o.maska_otkaz),
    // -1 у службы значит «не узнали».
    pamyat_mb: chislo(o.pamyat_mb) ?? -1,
    disk_mb: chislo(o.disk_mb) ?? -1,
  };
}

const FAZY: readonly string[] = ["idyot", "gotovo", "oshibka", "otmeneno"];

function faza(v: unknown): Faza | undefined {
  return typeof v === "string" && FAZY.includes(v) ? (v as Faza) : undefined;
}

/** Полный снимок задания. `{"faza":"net"}` (заданий не было) и чужая форма
 *  дают null. */
export function hodIz(v: unknown): HodVps | null {
  const o = obekt(v);
  const id = chislo(o?.id);
  const vid = stroka(o?.vid);
  const f = faza(o?.faza);
  if (!o || id === undefined || vid === undefined || f === undefined) return null;
  const snyat = obekt(o.snyat);
  const otpushchen = obekt(o.otpushchen);
  const parol = obekt(o.parol);
  const obnovlenie = obekt(o.obnovlenie);
  const novaya = stroka(obnovlenie?.versiya);
  const maska = obekt(o.maska);
  return {
    id,
    vid,
    faza: f,
    shag: stroka(o.shag),
    server: stroka(o.server),
    planovaya: o.planovaya === true || undefined,
    adres: stroka(o.adres),
    put: stroka(o.put),
    klyuch: klyuchHodaIz(o.klyuch) ?? undefined,
    proverki: spisok(o.proverki, klyuchHodaIz),
    otseyany: spisok(o.otseyany, otsevIz),
    krug: chislo(o.krug),
    sleduyushchiy: chislo(o.sleduyushchiy),
    krug_tekst: stroka(o.krug_tekst),
    povtory: spisok(o.povtory, (x) => vremya(x) ?? null),
    osmotr: osmotrIz(o.osmotr) ?? undefined,
    ustanovlen: ustanovkaIz(o.ustanovlen) ?? undefined,
    snyat: snyat ? { parol_vernulsya: snyat.parol_vernulsya === true } : undefined,
    otpushchen: otpushchen ? { klyucha_ne_bylo: otpushchen.klyucha_ne_bylo === true } : undefined,
    parol: parol ? {
      vyklyuchen: parol.vyklyuchen === true,
      snaruzhi: stroka(parol.snaruzhi),
      fayly: spisok(parol.fayly, (x) => stroka(x) ?? null),
    } : undefined,
    obnovlenie: obnovlenie && novaya !== undefined
      ? {
        bylo: stroka(obnovlenie.bylo), versiya: novaya, uzhe: obnovlenie.uzhe === true,
        ne_proshli: spisok(obnovlenie.ne_proshli, (x) => stroka(x) ?? null),
      }
      : undefined,
    maska: maska ? {
      svoya: stroka(maska.svoya), brend_soglasen: maska.brend_soglasen === true || undefined,
      imya: stroka(maska.imya), istochnik: stroka(maska.istochnik),
    } : undefined,
    proverka: proverkaIz(o.proverka) ?? undefined,
    otkaz: otkazVpsIz(o.otkaz) ?? undefined,
    preduprezhdeniya: spisok(o.preduprezhdeniya, otkazVpsIz),
    nachalo: vremya(o.nachalo),
    konec: vremya(o.konec),
  };
}

/** Событие svoy-vps-hod: номер, вид, фаза и шаг, без подробностей. */
export function sobytieHodaIz(v: unknown): Pick<HodVps, "id" | "vid" | "faza" | "shag" | "server" | "planovaya"> | null {
  const o = obekt(v);
  const id = chislo(o?.id);
  const vid = stroka(o?.vid);
  const f = faza(o?.faza);
  if (!o || id === undefined || vid === undefined || f === undefined) return null;
  return { id, vid, faza: f, shag: stroka(o.shag), server: stroka(o.server), planovaya: o.planovaya === true || undefined };
}

function serverIz(v: unknown): SvoyServer | null {
  const o = obekt(v);
  const id = stroka(o?.id);
  const adres = stroka(o?.adres);
  if (!o || id === undefined || adres === undefined) return null;
  return {
    id,
    imya: stroka(o.imya) ?? "",
    adres,
    port_ssh: chislo(o.port_ssh) ?? 22,
    polzovatel: stroka(o.polzovatel) ?? "",
    parol_vyklyuchen: o.parol_vyklyuchen === true,
    versiya: stroka(o.versiya),
    maska: stroka(o.maska) ?? "",
    maska_istochnik: stroka(o.maska_istochnik),
    maska_ne_otvechaet: stroka(o.maska_ne_otvechaet),
    reality: chislo(o.reality),
    hy2: chislo(o.hy2),
    porty_ruchnye: o.porty_ruchnye === true,
    prodlenie: stroka(o.prodlenie),
    servery: spisok(o.servery, (x) => stroka(x) ?? null),
    postavlen: vremya(o.postavlen),
    sert_do: chislo(o.sert_do),
    sert_trevoga: o.sert_trevoga === true,
    sert_otkaz: stroka(o.sert_otkaz),
    sert_proveren: vremya(o.sert_proveren),
    proveren: vremya(o.proveren),
    proverka_otkaz: stroka(o.proverka_otkaz),
    ne_iz_seti: spisok(o.ne_iz_seti, (x) => stroka(x) ?? null),
    set_tekst: stroka(o.set_tekst),
    klyuch_upravleniya: o.klyuch_upravleniya === true,
    klyuch_ne_chitaetsya: stroka(o.klyuch_ne_chitaetsya),
    otpechatok: stroka(o.otpechatok),
    novyy_otpechatok: stroka(o.novyy_otpechatok),
    novaya_versiya: stroka(o.novaya_versiya),
  };
}

/** Ответ svoyVpsSpisok. null - формы нет вовсе: служба старее окна. */
export function spisokIz(v: unknown): SvoyServer[] | null {
  const o = obekt(v);
  return o && Array.isArray(o.servery) ? spisok(o.servery, serverIz) : null;
}

// Состояние сервера для карточки и метки раздела.

export type SostoyanieServera = "horosho" | "vnimanie" | "slomano";

export interface OcenkaServera {
  sostoyanie: SostoyanieServera;
  prichina?: string;
  sovet?: string;
  /** Причина взята из записи о сети человека: строка дела её не повторяет. */
  izSeti?: boolean;
  /** Причина это отказ последней проверки: строка упавшей проверки говорит
   *  то же самое и несёт кнопку, плашка карточки её не повторяет. */
  izProverki?: boolean;
  /** Маска перестала отвечать: плашка несёт «Сменить маску». */
  maska?: boolean;
}

// Хвост тревоги ведёт в этот самый раздел, а в карточке он лишний.
const HVOSTY_TREVOGI = [". Проверь сервер в разделе «Свой VPN»", ". Обнови её в разделе «Свой VPN»"];

/** Последнее законченное дело вида `vid` над сервером. */
export function posledneeDelo(hody: HodVps[], server: string, vid: string): HodVps | undefined {
  let itog: HodVps | undefined;
  for (const h of hody) {
    if (h.server === server && h.vid === vid && h.faza !== "idyot" && (!itog || h.id > itog.id)) itog = h;
  }
  return itog;
}

/** Последняя удачная проверка сервера, если после неё его не ставили, не
 *  подбирали порты и не брали под управление: запись отмечает и эти визиты, и
 *  то, что видела проверка до них, о нынешнем сервере не скажет. Отметки нет
 *  вовсе - принят новый ключ сервера, его могли переустановить, и прежняя
 *  проверка тоже не в счёт. */
export function svezhayaProverka(s: SvoyServer, hody: HodVps[]): ProverkaVps | undefined {
  const h = posledneeDelo(hody, s.id, "proverka");
  if (h?.faza !== "gotovo") return undefined;
  if (h.konec && (!s.proveren || Date.parse(h.konec) < Date.parse(s.proveren))) return undefined;
  return h.proverka;
}

/** Порты, что проверка видела у сервера на нынешних его портах: проверка до
 *  подбора портов видела прежние, и о нынешних она не скажет. */
export function portyNyneshnie(s: SvoyServer, p: ProverkaVps | undefined): PortVps[] {
  return p?.porty.filter((x) => x.port === (x.vid === "hy2" ? s.hy2 : s.reality)) ?? [];
}

function imyaVida(t: string): string {
  return t === "hy2" ? "hysteria2" : "reality";
}

// Пункт меню подбора не спрашивает, поэтому о смене ключей говорит совет.
const HVOST_PODBORA = "Если порты сменятся, ключи на телефоне придётся добавить заново";
const SOVET_PODBORA = `Подбери порты заново: подбор ставит их на сервер ещё раз. ${HVOST_PODBORA}`;
const SOVET_PEREZAPUSKA = `Подбери порты заново: подбор перезапустит VPN на сервере. ${HVOST_PODBORA}`;

/** Три значения карточки (план, §4): работает, нужно внимание, не работает.
 *  «Не работает» только по свежей проверке, которая видела VPN на сервере
 *  мёртвым: запись о сервере сама этого не помнит. */
export function ocenitServer(s: SvoyServer, trevogi: SvoyaTrevoga[], hody: HodVps[]): OcenkaServera {
  const p = svezhayaProverka(s, hody);
  const proverka = p && !p.programma_otkaz ? p : undefined;
  const porty = portyNyneshnie(s, proverka);
  const molchat = porty.filter((x) => !x.slushaet);
  if (proverka) {
    if (!proverka.yadro_zhivo) {
      return {
        sostoyanie: "slomano",
        prichina: "VPN на сервере не запущен",
        // Подбор заново ставит пакет, и программа сервера перезапускает VPN
        // (server.Primenit). Текст ядра человеку не прочесть, он в отчёте.
        sovet: proverka.oshibka_yadra
          ? `${SOVET_PEREZAPUSKA}. Что ответил сервер, есть в отчёте: меню «…», «Скопировать отчёт»`
          : SOVET_PEREZAPUSKA,
      };
    }
    if (porty.length > 0 && molchat.length === porty.length) {
      return { sostoyanie: "slomano", prichina: "VPN на сервере не принимает подключения", sovet: SOVET_PODBORA };
    }
  }
  const trevoga = trevogi.find((t) => t.id === s.id);
  if (trevoga) {
    const hvost = HVOSTY_TREVOGI.find((x) => trevoga.tekst.endsWith(x));
    const tekst = hvost ? trevoga.tekst.slice(0, -hvost.length) : trevoga.tekst;
    return { sostoyanie: "vnimanie", prichina: tekst.charAt(0).toUpperCase() + tekst.slice(1) };
  }
  if (s.novyy_otpechatok) return { sostoyanie: "vnimanie" };
  if (s.proverka_otkaz) {
    return { sostoyanie: "vnimanie", prichina: "Последняя проверка не прошла", sovet: s.proverka_otkaz.charAt(0).toUpperCase() + s.proverka_otkaz.slice(1), izProverki: true };
  }
  // Ключ, мёртвый на самом сервере, главнее того, что он не открывается из
  // сети: чинит его подбор портов, а не другой адрес.
  if (molchat.length > 0) {
    return { sostoyanie: "vnimanie", prichina: `На сервере не работает ${molchat.map((x) => imyaVida(x.vid)).join(" и ")}`, sovet: SOVET_PODBORA };
  }
  // Сайт маски пропал: ключ reality не подключится, откуда бы ни шёл, и это
  // главнее того, что он не открывается из сети человека.
  if (s.maska_ne_otvechaet && s.reality !== undefined) {
    return {
      sostoyanie: "vnimanie",
      prichina: "Маска перестала отвечать",
      sovet: `${s.maska_ne_otvechaet.charAt(0).toUpperCase() + s.maska_ne_otvechaet.slice(1)}. С ней ключ reality не подключится, смени маску`,
      maska: true,
    };
  }
  // Сервер может быть здоров, а ключи из сети человека не проходят: это тоже
  // «нужно внимание». Снимает его запись: служба убирает вид, когда его ключ
  // провезёт проверку из сети или ключа больше нет.
  if (s.ne_iz_seti.length > 0) {
    const vidy = s.ne_iz_seti.map(imyaVida);
    return {
      sostoyanie: "vnimanie",
      prichina: `Из твоей сети не ${vidy.length > 1 ? "открываются" : "открывается"} ${vidy.join(" и ")}`,
      sovet: s.set_tekst ? s.set_tekst.charAt(0).toUpperCase() + s.set_tekst.slice(1) : undefined,
      izSeti: true,
    };
  }
  return { sostoyanie: "horosho" };
}

/** Метка раздела в полосе: худшее из состояний своих серверов. */
export function metkaRazdela(servery: SvoyServer[] | null, trevogi: SvoyaTrevoga[], hody: HodVps[]): "warn" | "danger" | undefined {
  let metka: "warn" | "danger" | undefined;
  // Тревога из статуса приходит и тогда, когда списка ещё нет.
  if (trevogi.length > 0) metka = "warn";
  for (const s of servery ?? []) {
    const o = ocenitServer(s, trevogi, hody).sostoyanie;
    if (o === "slomano") return "danger";
    if (o === "vnimanie") metka = "warn";
  }
  return metka;
}

// Мастер. Живёт в App, а не на экране: вкладку переключают посреди мастера,
// и набранное не должно пропадать вместе с экраном.

/** Ключ человека: окно прочло файл само, служба получает содержимое. */
export interface KlyuchFayla {
  imya: string;
  base64: string;
}

export interface Forma {
  soobshchenie: string;
  adres: string;
  /** Порт SSH строкой поля; пусто значит 22. */
  portSshPolya: string;
  polzovatel: string;
  sposob: "parol" | "klyuch";
  parol: string;
  klyuch: KlyuchFayla | null;
  parolKlyucha: string;
  portReality: string;
  portHy2: string;
  /** Название сервера; пусто значит прежнее или «Мой сервер». */
  imya: string;
  /** Адрес сервера из раздела, если мастер открыт его «Поставить заново»:
   *  только так служба ставит поверх записи. */
  poverhZapisi: string;
}

export const PUSTAYA_FORMA: Forma = {
  soobshchenie: "",
  adres: "",
  portSshPolya: "",
  polzovatel: "root",
  sposob: "parol",
  parol: "",
  klyuch: null,
  parolKlyucha: "",
  portReality: "",
  portHy2: "",
  imya: "",
  poverhZapisi: "",
};

/** Шаг мастера. `cel` у доступа: поставить VPN или взять под управление
 *  сервер, где он уже стоит. `forma` у установки пуста, когда окно открылось
 *  посреди неё: пароля в памяти уже нет, повтор идёт через доступ.
 *  `neZhdat`: ключи записаны, и человек не стал ждать перепроверки из сети. */
export type Master =
  | { shag: "dostup"; cel: "postavit" | "vzyat"; forma: Forma; otkaz?: OtkazVps }
  | { shag: "proverka"; forma: Forma; id: number; pustoy: boolean; otkaz?: OtkazVps }
  | { shag: "ustanovka"; forma: Forma | null; id: number; neZhdat?: boolean };

/** Команда удалась, а служба предупреждает: переименование записало набор,
 *  а правила брандмауэра отстали. */
export type ItogKomandy = { hod: HodVps } | { telo: unknown; preduprezhdenie?: OtkazVps } | { otkaz: OtkazVps };

/** Предел названия своего сервера, тот же, что у службы (dlinaImeni): Happ
 *  показывает из имени ключа 30 знаков, хвост «, hysteria2» занимает 11.
 *  Считается по знакам Юникода, как в службе. */
export const DLINA_IMENI = 19;

const IMYA_PO_UMOLCHANIYU = "Мой сервер";

/** Дела, что пишут запись о сервере целиком, с названием из начала дела
 *  (pishutZapis службы): переименование посреди них служба не пустит. */
const PISHUT_ZAPIS = new Set(["ustanovka", "podbor", "vzyat", "maska"]);

/** Идёт дело, которое затёрло бы новое название сервера id. Дело, чей сервер
 *  в снимке ещё не назван, могло начаться над ним же. */
export function imyaZanyato(hody: HodVps[], id: string): boolean {
  return hody.some((h) => h.faza === "idyot" && PISHUT_ZAPIS.has(h.vid) && (!h.server || h.server === id));
}

/** Название, которое служба даст серверу с пустым полем: «Мой сервер», а
 *  если оно занято другим своим сервером, «Мой сервер 2» и дальше
 *  (imyaPoUmolchaniyu службы). Установка поверх записи с адресом poverh
 *  оставляет её прежнее название, а сама запись занятым не считается. */
export function imyaPoUmolchaniyu(servery: SvoyServer[], poverh = ""): string {
  const bylo = poverh === "" ? undefined : servery.find((s) => s.adres === poverh);
  if (bylo?.imya) return bylo.imya;
  const zanyato = new Set(servery.filter((s) => s !== bylo).map((s) => s.imya));
  let imya = IMYA_PO_UMOLCHANIYU;
  for (let i = 2; zanyato.has(imya); i++) imya = `${IMYA_PO_UMOLCHANIYU} ${i}`;
  return imya;
}

export interface SvoyVps {
  /** null - список ещё не пришёл или не прочитался (тогда spisokOtkaz). */
  servery: SvoyServer[] | null;
  spisokOtkaz: OtkazVps | null;
  /** Последние снимки дел, по одному на номер, по возрастанию номера. */
  hody: HodVps[];
  master: Master | null;
  zadatMaster: Dispatch<SetStateAction<Master | null>>;
  /** Дела, итог которых человек закрыл в карточке. */
  skryty: number[];
  skryt: (id: number) => void;
  /** Команда службы. Задание отдаёт снимок, прочие команды тело. Отказ
   *  службы и сбой канала приходят отказом: экран рисует его на месте. */
  komanda: (imya: string, telo: unknown) => Promise<ItogKomandy>;
  obnovit: () => void;
  /** Адрес сервера по id, и того, что из списка уже ушёл. */
  adresServera: (id: string) => string | undefined;
}

type Zvat = (imya: string, telo?: unknown) => Promise<Kadr>;
type NaSobytie = (obrabotchik: (kadr: Kadr) => void) => () => void;

// Сколько снимков держать: мастер и карточки ищут дело по номеру, а плановая
// проверка может встать между установкой и взглядом человека на её итог.
const HODOV_V_PAMYATI = 8;

// Какие дела меняют набор ключей: по их удачному концу окну пора перечитать
// и общий список серверов.
const MENYAYUT_KLYUCHI = new Set(["ustanovka", "vzyat", "podbor", "snos", "otpusk", "maska"]);

// Номер установки, итог которой человек уже видел. Окно, поднятое заново после
// упавшей установки, иначе молчало бы о ней, а показанное каждый раз заново
// висело бы до перезапуска службы.
const KLYUCH_VIDENNOGO = "affory.svoy-vps.uvidennaya-ustanovka";

function prochitatUvidennuyu(): number {
  try {
    return Number(localStorage.getItem(KLYUCH_VIDENNOGO) ?? "0") || 0;
  } catch {
    return 0;
  }
}

export function zapomnitUvidennuyu(id: number): void {
  try {
    localStorage.setItem(KLYUCH_VIDENNOGO, String(id));
  } catch (e: unknown) {
    // Не записалось: окно, поднятое заново, покажет итог ещё раз. Это лучше,
    // чем промолчать о нём.
    console.warn("номер увиденной установки не записан", e);
  }
}

// Упавшую установку окно возвращает при запуске не дольше суток: старше она
// уже не новость.
const SROK_VOZVRATA_MS = 24 * 3600 * 1000;

function vstavitHod(hody: HodVps[], h: HodVps): HodVps[] {
  const bez = hody.filter((x) => x.id !== h.id);
  bez.push(h);
  bez.sort((a, b) => a.id - b.id);
  return bez.slice(-HODOV_V_PAMYATI);
}

export function useSvoyVps({ zvat, naSobytie, naSboy, naSmenuKlyuchey, aktiven }: {
  zvat: Zvat;
  naSobytie: NaSobytie;
  /** Исключение канала: App решает, оборвалась ли связь. */
  naSboy: (e: unknown) => void;
  naSmenuKlyuchey: () => void;
  /** Служба на связи. Пока её нет, спрашивать не у кого. */
  aktiven: boolean;
}): SvoyVps {
  const [servery, zadatServery] = useState<SvoyServer[] | null>(null);
  const [spisokOtkaz, zadatSpisokOtkaz] = useState<OtkazVps | null>(null);
  const [hody, zadatHody] = useState<HodVps[]>([]);
  const [master, zadatMaster] = useState<Master | null>(null);
  const [skryty, zadatSkryty] = useState<number[]>([]);
  const nomerSpiska = useRef(0);

  // Установка удалась: пароль и ключ мастеру больше не нужны. Стираются здесь,
  // а не в экране: экран смонтирован только на своей вкладке, а мастер живёт
  // всю сессию окна. Адрес остаётся для шапки.
  useEffect(() => {
    zadatMaster((m) => {
      if (m?.shag !== "ustanovka" || !m.forma) return m;
      if (m.forma.parol === "" && m.forma.klyuch === null && m.forma.parolKlyucha === "") return m;
      if (hody.find((h) => h.id === m.id)?.faza !== "gotovo") return m;
      return { ...m, forma: { ...PUSTAYA_FORMA, adres: m.forma.adres } };
    });
  }, [hody, master]);

  // Адреса всех серверов, что были в списке: итог сноса и «забыть» называет
  // сервер, которого в списке уже нет.
  const adresa = useRef(new Map<string, string>());
  const zhiv = useRef(true);
  useEffect(() => {
    zhiv.current = true;
    return () => { zhiv.current = false; };
  }, []);

  // Обработчики снаружи меняются с каждым рисованием App: через ссылку, чтобы
  // подписка на события не переподписывалась каждый раз.
  const vneshnie = useRef({ naSboy, naSmenuKlyuchey });
  vneshnie.current = { naSboy, naSmenuKlyuchey };

  const obnovitSpisok = useCallback(async () => {
    const nomer = ++nomerSpiska.current;
    try {
      const k = await zvat("svoyVpsSpisok", {});
      if (!zhiv.current || nomer !== nomerSpiska.current) return;
      if (k.oshibka) {
        zadatSpisokOtkaz({ kod: k.oshibka.kod, tekst: k.oshibka.tekst });
        return;
      }
      const s = spisokIz(k.telo);
      if (s === null) {
        zadatSpisokOtkaz({ kod: KOD_OBOLOCHKI, tekst: "служба ответила без списка своих серверов: она старее окна, обнови программу" });
        return;
      }
      zadatSpisokOtkaz(null);
      zadatServery(s);
      for (const x of s) adresa.current.set(x.id, x.adres);
    } catch (e: unknown) {
      if (!zhiv.current || nomer !== nomerSpiska.current) return;
      vneshnie.current.naSboy(e);
      zadatSpisokOtkaz({ kod: KOD_OBOLOCHKI, tekst: `список своих серверов не прочитан: ${tekstOshibki(e)}` });
    }
  }, [zvat]);

  // Снимок принимается, если он не старше показанного. Исключение одно:
  // по концу плановой проверки служба снова отдаёт итог прошлого дела
  // человека, у которого номер меньше.
  const prinyatHod = useCallback((h: HodVps) => {
    zadatHody((pr) => {
      const byl = pr.find((x) => x.id === h.id);
      // Опоздавший ответ про то же дело не возвращает законченное в «идёт».
      if (byl && byl.faza !== "idyot" && h.faza === "idyot") return pr;
      return vstavitHod(pr, h);
    });
  }, []);

  // Ответ команды несёт снимок с начала дела. Снимок по событию, успевший
  // раньше, свежее: ответ его не трогает.
  const prinyatNachalo = useCallback((h: HodVps) => {
    zadatHody((pr) => (pr.some((x) => x.id === h.id) ? pr : vstavitHod(pr, h)));
  }, []);

  // Снимок по событию. Пока один запрос в пути, события копятся в один
  // следующий: событий на установке десятки, а снимок нужен последний.
  const zaprosHoda = useRef({ idyot: false, eshchyo: false });
  const pervyyHod = useRef(true);
  // Дела, чьи ключи уже записаны и перечитаны до конца дела.
  const klyuchiProchitany = useRef(new Set<number>());
  const sprositHod = useCallback(async () => {
    const z = zaprosHoda.current;
    if (z.idyot) {
      z.eshchyo = true;
      return;
    }
    z.idyot = true;
    try {
      do {
        z.eshchyo = false;
        let k: Kadr;
        try {
          k = await zvat("svoyVpsHod", {});
        } catch (e: unknown) {
          vneshnie.current.naSboy(e);
          return;
        }
        if (!zhiv.current) return;
        // Отказ здесь значит службу без этой команды: показывать нечего.
        // Заданий с запуска службы не было: возвращать мастер не к чему.
        const h = k.oshibka ? null : hodIz(k.telo);
        if (!h) {
          pervyyHod.current = false;
          continue;
        }
        prinyatHod(h);
        // Ключи записаны до перепроверки из сети (И7): список и общий набор
        // перечитываются сейчас, а не в конце дела, которого человек может
        // не ждать.
        if (h.faza === "idyot" && h.ustanovlen && !klyuchiProchitany.current.has(h.id)) {
          klyuchiProchitany.current.add(h.id);
          void obnovitSpisok();
          vneshnie.current.naSmenuKlyuchey();
        }
        // Первый снимок после запуска окна: установка, начатая до него,
        // возвращает мастер на свой шаг. Та, перепроверки которой человек
        // не стал ждать, видна в карточке.
        if (pervyyHod.current) {
          pervyyHod.current = false;
          const svezhaya = h.konec === undefined || Date.now() - Date.parse(h.konec) < SROK_VOZVRATA_MS;
          const uvidennaya = h.id === prochitatUvidennuyu();
          const vernut = h.faza === "idyot" ? !(h.ustanovlen && uvidennaya) : h.faza !== "gotovo" && !uvidennaya;
          if (h.vid === "ustanovka" && svezhaya && vernut) {
            zadatMaster((m) => m ?? { shag: "ustanovka", forma: null, id: h.id });
          }
        }
      } while (z.eshchyo);
    } finally {
      z.idyot = false;
    }
  }, [zvat, prinyatHod, obnovitSpisok]);

  const obnovit = useCallback(() => {
    void obnovitSpisok();
    void sprositHod();
  }, [obnovitSpisok, sprositHod]);

  // Служба на связи: при запуске окна и после возврата канала спросить всё
  // заново. Событий, пропущенных без связи, никто не повторит.
  useEffect(() => {
    if (aktiven) obnovit();
  }, [aktiven, obnovit]);

  useEffect(() => naSobytie((kadr) => {
    if (kadr.imya !== "svoy-vps-hod") return;
    const s = sobytieHodaIz(kadr.telo);
    if (!s) return;
    void sprositHod();
    if (s.faza === "idyot") return;
    // Дело кончилось: запись о сервере могла смениться при любом итоге
    // (отметка проверки, ожидание нового ключа хоста).
    void obnovitSpisok();
    if (s.faza === "gotovo" && MENYAYUT_KLYUCHI.has(s.vid)) vneshnie.current.naSmenuKlyuchey();
  }), [naSobytie, sprositHod, obnovitSpisok]);

  const komanda = useCallback(async (imya: string, telo: unknown): Promise<ItogKomandy> => {
    let k: Kadr;
    try {
      k = await zvat(imya, telo);
    } catch (e: unknown) {
      vneshnie.current.naSboy(e);
      return { otkaz: { kod: KOD_OBOLOCHKI, tekst: tekstOshibki(e) } };
    }
    // Эти команды служба делает сразу, без задания: события хода не будет, и
    // запись, а у «забыть» и переименования и ключи общего списка,
    // перечитываются отсюда. И при отказе тоже: «забыть» могло успеть стереть
    // ключ управления, а запись набора могла пройти до отказа.
    const sraznu = imya === "svoyVpsZabyt" || imya === "svoyVpsPrinyatKlyuchHosta" || imya === "svoyVpsPereimenovat";
    if (sraznu) {
      void obnovitSpisok();
      if (imya !== "svoyVpsPrinyatKlyuchHosta") vneshnie.current.naSmenuKlyuchey();
    }
    if (k.oshibka) return { otkaz: { kod: k.oshibka.kod, tekst: k.oshibka.tekst } };
    if (sraznu) {
      const preduprezhdenie = otkazVpsIz(obekt(k.telo)?.preduprezhdenie);
      return preduprezhdenie ? { telo: k.telo, preduprezhdenie } : { telo: k.telo };
    }
    if (imya.startsWith("svoyVps")) {
      const h = hodIz(k.telo);
      if (h) {
        // Отмена отвечает снимком уже после отмены, он не начальный.
        if (imya === "svoyVpsOtmenit") prinyatHod(h);
        else prinyatNachalo(h);
        return { hod: h };
      }
    }
    return { telo: k.telo };
  }, [zvat, prinyatHod, prinyatNachalo, obnovitSpisok]);

  const skryt = useCallback((id: number) => zadatSkryty((s) => (s.includes(id) ? s : [...s, id])), []);
  const adresServera = useCallback((id: string) => adresa.current.get(id), []);

  return { servery, spisokOtkaz, hody, master, zadatMaster, skryty, skryt, komanda, obnovit, adresServera };
}
