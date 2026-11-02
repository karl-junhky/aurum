import { aktualnyeZamery } from "./zamery";
import type { ChernovikPravil } from "./trafik";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import "./desktop.css";
import { Glavnyy } from "./ekrany/Glavnyy";
import { perenestiIdServerov } from "./ekrany/KatalogServerov";
import { Skorost } from "./ekrany/Skorost";
import { useSkorost } from "./skorost";
import { Karkas } from "./ekrany/Karkas";
import { Nastroyki, type AdresVyhoda, type ZamerPolosy } from "./ekrany/Nastroyki";
import { Otkaz } from "./ekrany/Otkaz";
import { KOD_OBOLOCHKI, KOD_SROKA, type Deystvie } from "./ekrany/otkazy";
import { PervyyZapusk, type SostoyanieUstanovki } from "./ekrany/PervyyZapusk";
import { Pravila, type PravilaOtvet } from "./ekrany/Pravila";
import {
  dobavitNaydennoeQr, kodyQr, naytiQrNaEkrane, opisanieVersii, otkrytNastroykiVremeni, otkrytPapkuZhurnalov, prochitatSoedineniya, prochitatOhvatPrilozheniya, perezapustitOkno, perezapustitSPravami, prochitatProfil, skopirovatTekst, sohranitDiagnostiku, sohranitProfil, spisokProtsessov, startovayaVkladka,
  tekstBufera, vybratArhiv, vybratKlyuchSsh, vybratKudaSohranit, vybratOtkuda, vybratPrilozhenie, zabytQr, zapisatVZhurnal, type Zapushchennyy,
} from "./most";
import { Servery, svodkaQrIz, type PodpiskaNaEkrane, type SpisokServerov, type ZamerZaderzhki } from "./ekrany/Servery";
import { SvoyVpn } from "./ekrany/SvoyVpn";
import { metkaRazdela, useSvoyVps } from "./svoyVps";
import { VKLADKI, type Vkladka } from "./ekrany/vkladki";
import {
  KanalNedostupen, otkrytGitHub, naSobytie, naVidimostOkna, naVkladku, oknoRazvernut, oknoSvernut, oknoZakryt, sluzhbaUstanovlena,
  udalitProgrammu, ustanovitSluzhbu, zvat, type Kadr,
} from "./most";
import { naladitVstavku } from "./vstavka";
import { VERSIYA_PROTOKOLA, type HodObnovleniya, type OtkazStroki, type ProverkaSeti, type Rezhim, type RezultatProverki, type Statistika, type StatusOtvet } from "./protokol";
import { tekstOshibki } from "./ponyatno";

// The only place that talks to most.ts. Screens get whole StatusOtvet values
// as props and never touch the bridge, so a shell swap is most.ts plus here.

const MOLCHIT: StatusOtvet = { sostoyanie: "sluzhba-molchit" };

/** State of the pipe as the window knows it. `zhdyom` is "not asked yet" and
 *  is NOT a refusal: before this existed every launch opened on "служба не
 *  отвечает" for the whole first round trip, healthy service included. */
export type Svyaz = "zhdyom" | "est" | "net";

/** How often the window re-asks status. The tray already polls every five
 *  seconds (trey.go); two halves of one program counting differently is how
 *  a green tray and a window saying "служба не отвечает" ended up on screen
 *  at the same time. */
export const PERIOD_OPROSA_MS = 5000;

/** Код сбоя самой оболочки. Живёт в otkazy.ts рядом со своей фразой для
 *  экрана, а здесь только пробрасывается дальше: два объявления одной строки
 *  однажды разойдутся, и разойдутся молча. */
export { KOD_OBOLOCHKI };

export interface AppProps {
  /** Poll period override. Tests shorten it; nothing else does. */
  periodOprosaMs?: number;
}

/** A refusal worth a screen: from a command answer, from status.oshibka, or
 *  from the shell itself when the pipe is squatted. */
export interface Otkazano {
  kod: string;
  tekst?: string;
  vinovnik?: string;
  /** Command to repeat for `povtorit`. */
  komanda?: string;
  /** Tab the refusal was raised on. A refusal with no tab belongs to the
   *  window as a whole (a list that would not load, a dead pipe) and is
   *  drawn everywhere; one raised by a button belongs where the button is. */
  vkladka?: Vkladka;
}

/** Identity of a refusal for the "closed stays closed" rule: the same code
 *  with the same text is the same standing refusal, a different one is news. */
function klyuchOtkaza(o: { kod: string; tekst?: string } | null): string | null {
  return o === null ? null : o.kod + " | " + (o.tekst ?? "");
}

// kanal.Podklyuchitsya reports a squatted pipe as "pipe-squatted: владелец
// канала <name>". The name is diagnostics for a person, not a trust decision.
function otkazIzKanala(soobshchenie: string): Otkazano | null {
  if (!soobshchenie.includes("pipe-squatted")) return null;
  const m = /владелец канала (.+)$/.exec(soobshchenie);
  return { kod: "pipe-squatted", vinovnik: m?.[1]?.trim() };
}

function statusIz(kadr: Kadr): StatusOtvet | null {
  const t = kadr.telo;
  if (t && typeof t === "object" && "sostoyanie" in t) return t as StatusOtvet;
  // setRouteMode answers {status, trebuet_podyoma}: the status is nested.
  if (t && typeof t === "object" && "status" in t) return statusIz({ ...kadr, telo: (t as { status: unknown }).status });
  return null;
}

// otkazyIz достаёт список непонятых строк из тела ответа подписки.
//
// Проверяется КАЖДОЕ поле, а не только наличие массива: тело приезжает с
// провода, и служба постарше может прислать другую форму. Запись без номера
// строки или без причины бесполезна на экране, поэтому она отбрасывается здесь,
// а не рисуется пустой строкой.
function otkazyIz(telo: unknown): OtkazStroki[] {
  if (!telo || typeof telo !== "object") return [];
  const syrye = (telo as { otkazy?: unknown }).otkazy;
  if (!Array.isArray(syrye)) return [];
  return syrye.flatMap((o) => {
    if (!o || typeof o !== "object") return [];
    const { stroka, prichina } = o as { stroka?: unknown; prichina?: unknown };
    if (typeof stroka !== "number" || typeof prichina !== "string" || prichina === "") return [];
    return [{ stroka, prichina }];
  });
}

// zameryIz достаёт замеры задержки. Как и otkazyIz, проверяет КАЖДОЕ поле.
//
// Число и отсутствие числа здесь разные вещи, и путать их нельзя: `null`
// означает «не измерено», а ноль означал бы «мгновенно» и поставил бы узел
// первым по задержке. Поэтому нечисло не превращается в ноль, а остаётся
// отсутствием, и рядом с ним живёт текст отказа.
function zameryIz(telo: unknown): ZamerZaderzhki[] {
  if (!telo || typeof telo !== "object") return [];
  const syrye = (telo as { zamery?: unknown }).zamery;
  if (!Array.isArray(syrye)) return [];
  return syrye.flatMap((z) => {
    if (!z || typeof z !== "object") return [];
    const o = z as Record<string, unknown>;
    if (typeof o.id !== "string" || o.id === "") return [];
    const chislo = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v : null);
    const tekst = (v: unknown) => (typeof v === "string" ? v : "");
    return [{
      id: o.id,
      versiya: tekst(o.versiya) || undefined,
      ping_ms: chislo(o.ping_ms),
      ping_otkaz: tekst(o.ping_otkaz),
      ping_prichina: tekst(o.ping_prichina) || undefined,
    }];
  });
}

// polosaIz достаёт замер полосы. Нечисло остаётся отсутствием, а не нулём.
//
// Ноль тут не безобидное умолчание: объявленный ноль это Brutal с нулевой
// оценкой канала, то есть протокол, которому сказали «канала нет». Поэтому
// «не измерено» доезжает до экрана как отсутствие числа, а не как ноль.
function polosaIz(telo: unknown, vremya: string): ZamerPolosy | null {
  if (!telo || typeof telo !== "object") return null;
  const o = telo as Record<string, unknown>;
  const chislo = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v : null);
  const celoe = (v: unknown) => (typeof v === "number" && Number.isInteger(v) && v > 0 ? v : null);
  return {
    mbitVniz: chislo(o.mbit_vniz),
    sovetVniz: celoe(o.sovet_vniz),
    mbitVverh: chislo(o.mbit_vverh),
    sovetVverh: celoe(o.sovet_vverh),
    otkazVverh: typeof o.otkaz_vverh === "string" ? o.otkaz_vverh : "",
    cherezTunnel: o.cherez_tunnel === true,
    vremya,
  };
}

// Commands after which the server list on screen is stale.
const MENYAYUT_SPISOK = new Set(["addServer", "addServers", "removeServer", "setSubscription", "refreshSubscription", "setServer", "setRouteMode", "setAutoMember", "connect",
  "addSubscription", "removeSubscription", "setActiveSubscription"]);
// Команды, после которых меняется САМ список подписок. Переключение активной
// сюда входит: строка «активна» переезжает на другую запись.
const MENYAYUT_PODPISKI = new Set(["setSubscription", "addSubscription", "removeSubscription", "setActiveSubscription", "refreshSubscription"]);
// Same for the rules list.
const MENYAYUT_PRAVILA = new Set(["setRules"]);
// Команды подъёма. Если их срок ответа истёк, служба подъём ВЕДЁТ дальше, и
// итог приедет событием state: отказ из него встанет плашкой сам. Плашка на
// истёкшем сроке объявляла провал работающего подключения (жалоба 15.10.2026).
const ZHDUT_ITOGA = new Set(["connect", "setServer"]);

export function App({ periodOprosaMs = PERIOD_OPROSA_MS }: AppProps = {}) {
  const [busyCommand, setBusyCommand] = useState<string | null>(null);
  // Какие команды сейчас в полёте. Это НЕ то же, что busyCommand: тот гасит
  // управление целиком и потому держит только пять команд, меняющих состояние
  // туннеля. Вертушка на кнопке нужна КАЖДОЙ команде, за которой стоит
  // ожидание, и гасить ради неё весь экран нельзя: замер полосы идёт
  // полминуты.
  //
  // Счётчик, а не флаг: одна и та же команда может уйти дважды (нажали, пока
  // шла проверка по таймеру), и первый вернувшийся ответ снял бы вертушку у
  // ещё живого второго вызова.
  const [iduchshie, zadatIduchshie] = useState<Record<string, number>>({});
  const otmetit = useCallback((komanda: string, shag: 1 | -1) => {
    zadatIduchshie((p) => {
      const stalo = (p[komanda] ?? 0) + shag;
      if (stalo > 0) return { ...p, [komanda]: stalo };
      const { [komanda]: _ushla, ...ostalnye } = p;
      return ostalnye;
    });
  }, []);
  const zanyatyeKomandy = useMemo(
    () => Object.fromEntries(Object.keys(iduchshie).map((k) => [k, true])),
    [iduchshie],
  );
  // null means "no answer yet". The screens still want a whole StatusOtvet,
  // so `naEkrane` below substitutes MOLCHIT once we know the pipe is dead.
  const [status, zadatStatus] = useState<StatusOtvet | null>(null);
  // Видно ли окно. Решено 10.09.2026: в трее программа проводит
  // подавляющую часть времени, и опрашивать ядро ради счётчиков, которых никто
  // не видит, незачем. Начальное true: окно, стартовавшее без ключа --trey,
  // уже на экране, а ушедшее в трей получит false первым же событием.
  const [oknoVidno, zadatOknoVidno] = useState(true);
  // Та же видимость для стартовых запросов. Через ссылку, а не зависимостью:
  // иначе каждый показ и уход в трей перезапускали эффект монтирования и
  // заново спрашивали статус, список серверов и hello (О5 аудита 1.6.1).
  const oknoVidnoRef = useRef(true);
  oknoVidnoRef.current = oknoVidno;
  const [svyaz, zadatSvyaz] = useState<Svyaz>("zhdyom");
  // Замер скорости опрашивается только при видимом окне: в трее смотреть на
  // полосу некому (О4 аудита 1.6.1). Вернувшееся окно спросит состояние сразу.
  const speed = useSkorost(zvat, svyaz === "est" && oknoVidno);
  // null until the service actually answers subscribeStats with numbers.
  // It answers not-implemented until wave 6, so this stays null on purpose.
  const [statistika, zadatStat] = useState<Statistika | null>(null);
  const [adresVyhoda, zadatAdresVyhoda] = useState<AdresVyhoda | null>(null);
  const [zamerPolosy, zadatZamerPolosy] = useState<ZamerPolosy | null>(null);
  // Last checkLeaks answer; lives with the settings tab, null until asked.
  const [proverka, zadatProverku] = useState<RezultatProverki | null>(null);
  // Why the last check brought nothing. Without it a failed checkLeaks left the
  // PREVIOUS report on screen, and it read as the current one.
  const [proverkaOtkaz, zadatProverkaOtkaz] = useState<Otkazano | null>(null);
  // Проверка слоёв сети (A3): туннель, сервер, имена, резолвер сети и UDP
  // порознь. null до первого нажатия.
  const [proverkaSeti, zadatProverkuSeti] = useState<ProverkaSeti | null>(null);

  // Строки последней подписки, которые клиент не понял. Живут отдельно от
  // списка серверов намеренно: список это то, что получилось, а это то, что не
  // получилось, и второе исчезало молча.
  const [otkazyPodpiski, zadatOtkazyPodpiski] = useState<OtkazStroki[]>([]);
  const [podpiski, zadatPodpiski] = useState<PodpiskaNaEkrane[]>([]);
  const [zaderzhki, zadatZaderzhki] = useState<ZamerZaderzhki[]>([]);
  const posledniySpisok = useRef<SpisokServerov | null>(null);
  const nomerSpiska = useRef(0);

  const [otkaz, zadatOtkaz] = useState<Otkazano | null>(null);
  // What the person closed by hand. status.oshibka is a STANDING refusal and
  // used to survive every zadatOtkaz(null): while the service held one, the
  // banner could not be dismissed at all.
  const [zakryto, zadatZakryto] = useState<string | null>(null);
  // Three states, not two: waiting, ready, refused. `null` in the data used
  // to mean both "not asked yet" and "asked and refused", and the screen drew
  // the second as the first for ever.
  const [spisok, zadatSpisok] = useState<SpisokServerov | null>(null);
  const [spisokOtkaz, zadatSpisokOtkaz] = useState<Otkazano | null>(null);
  // Deferred commands with wave numbers, from hello. The screens over those
  // commands take the wave from here, never from their own strings.
  const [otlozheno, zadatOtlozheno] = useState<Record<string, number> | null>(null);
  const [pravila, zadatPravila] = useState<PravilaOtvet | null>(null);
  const [pravilaOtkaz, zadatPravilaOtkaz] = useState<Otkazano | null>(null);
  const [pravilaChitayutsya,zadatPravilaChitayutsya]=useState(false);
  const nomerPravil=useRef(0);
  // setRules answered trebuet_podyoma: the change waits for the next connect,
  // and the rules tab says so until then.
  const [pravilaZhdut, zadatPravilaZhdut] = useState(false);
  // Running processes for the rules form; null until the shell answers.
  const [zapushchennye, zadatZapushchennye] = useState<Zapushchennyy[] | null>(null);
  // Три состояния вместо одного `null`: идёт чтение, оболочка отказала, ответ
  // получен. До этого пустой ответ и неудача давали в форме одну строку «Не
  // найдено», то есть провал выглядел ровно как отсутствие запущенных программ.
  const [protsessyChitayutsya, zadatProtsessyChitayutsya] = useState(false);
  const [protsessyOtkaz, zadatProtsessyOtkaz] = useState("");
  const nomerProtsessov = useRef(0);
  const [chernovikPravil, zadatChernovikPravil] = useState<ChernovikPravil | null>(null);
  const [vkladka, zadatVkladku] = useState<Vkladka>("podklyuchenie");
  // Шаг идущего обновления и зов трея к нему. Оба живут здесь, а не на экране
  // настроек: события приходят в оболочку, а экран может быть не показан вовсе.
  const [hodObnovleniya, zadatHod] = useState<HodObnovleniya | null>(null);
  /** Версия, которую называла служба, пока это окно рисовало её интерфейс.
   *  Сменилась значит на диске уже другая программа, а процесс всё тот же. */
  const svoyaVersiya = useRef<string | null>(null);
  /** Подмена началась: после неё окно обязано и полосу погасить, и уйти. */
  const podmenaNachalas = useRef(false);
  const [vestiKObnovleniyu, zadatVesti] = useState(0);
  // Last profile outcome in the person's words. A press with no visible
  // result reads as "the button does nothing" (owner, 03.09.2026).
  const [itogProfilya, zadatItogProfilya] = useState<string | null>(null);
  // Read inside vypolnit without making it a dependency: rebuilding vypolnit
  // on every tab switch would restart the mount effect and re-subscribe.
  const vkladkaSeychas = useRef<Vkladka>(vkladka);
  useEffect(() => {
    vkladkaSeychas.current = vkladka;
  }, [vkladka]);
  // null: the service exists (answering or not). Otherwise the first-run
  // screen owns the whole window until status answers.
  const [ustanovka, zadatUstanovku] = useState<{ sostoyanie: SostoyanieUstanovki; prichina?: string } | null>(null);

  // What the banner is showing right now, for the dismiss below: naDeystvie
  // is built long before `pokazat` is computed, and a stale copy of the
  // refusal is exactly what "closed stays closed" cannot afford.
  const pokazannoe = useRef<Otkazano | null>(null);

  /** Real dismissal: the cross and every acknowledgement button. Clearing
   *  `otkaz` alone did nothing while the service held a standing refusal in
   *  status.oshibka, and the button read as broken. */
  const ubratOtkaz = useCallback(() => {
    zadatZakryto(klyuchOtkaza(pokazannoe.current));
    zadatOtkaz(null);
    zadatSpisokOtkaz(null);
    zadatPravilaOtkaz(null);
  }, []);

  // Nothing is swallowed. A dead pipe is the status path's story and gets no
  // banner of its own; everything else reaches the person as its own text,
  // because a shell failure with no §9.1 code is still a failure.
  const zhaloba = useCallback((chto: string, e: unknown) => {
    if (e instanceof KanalNedostupen) {
      zadatSvyaz("net");
      return;
    }
    zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: `${chto}: ${e instanceof Error ? e.message : String(e)}` });
  }, []);

  // Номер запроса, как у listServers и listRules: форма зовёт это и по
  // открытию, и по кнопке, и по возврату фокуса в окно, поэтому поздний ответ
  // прошлого запроса обязан молчать, а не перетирать свежий список.
  const obnovitProtsessy = useCallback(() => {
    const nomer = ++nomerProtsessov.current;
    zadatProtsessyChitayutsya(true);
    void spisokProtsessov().then((spisok) => {
      if (nomer !== nomerProtsessov.current) return;
      zadatZapushchennye(spisok);
      zadatProtsessyOtkaz("");
      zadatProtsessyChitayutsya(false);
    }).catch((e: unknown) => {
      if (nomer !== nomerProtsessov.current) return;
      zadatProtsessyChitayutsya(false);
      // Мёртвая труба это история экрана связи; своего баннера ей тут не нужно.
      if (e instanceof KanalNedostupen) {
        zadatSvyaz("net");
        return;
      }
      // Прошлый снимок НЕ стирается: форма покажет его с пометкой, что
      // обновить не удалось. Стереть значит обменять устаревший список на
      // пустой. Общий баннер тоже не поднимается: путь можно ввести руками
      // или выбрать на ПК, и список запущенных программ этому не мешает.
      zadatProtsessyOtkaz(tekstOshibki(e));
    });
  }, []);

  const obnovitSpisok = useCallback(async () => {
    const nomer = ++nomerSpiska.current;
    try {
      const kadr = await zvat("listServers");
      if (nomer !== nomerSpiska.current) return;
      if (kadr.oshibka) {
        // secrets-unreadable is a real answer here (komandy_serverov.go) and
        // it has its own screen in §9.1. Dropping it left an empty list.
        zadatSpisokOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst });
        return;
      }
      zadatSpisokOtkaz(null);
      if (kadr.telo && typeof kadr.telo === "object") {
        const next = kadr.telo as SpisokServerov;
        posledniySpisok.current = next;
        zadatSpisok(next);
        zadatZaderzhki(prev => aktualnyeZamery(prev, next));
      }
    } catch (e: unknown) {
      zhaloba("listServers", e);
    }
  }, [zhaloba]);

  // Подписки отдельной командой, а не полями listServers: их список нужен
  // экрану серверов целиком, а listServers говорит только про активную.
  const obnovitPodpiski = useCallback(async () => {
    try {
      const kadr = await zvat("listSubscriptions");
      // Отказ здесь МОЛЧАЛИВ намеренно: служба старее окна этой команды не
      // знает, и баннер «команда не существует» сказал бы человеку про наш
      // порядок обновления, а не про его подписки. Экран в этом случае рисует
      // одну строку по полям listServers.
      if (kadr.oshibka) return;
      const telo = kadr.telo as { podpiski?: PodpiskaNaEkrane[] } | undefined;
      zadatPodpiski(Array.isArray(telo?.podpiski) ? telo.podpiski : []);
    } catch (e: unknown) {
      // Исключение это НЕ «команды нет», а оборванный канал: такое молчать
      // нельзя, иначе экран подписок выглядит пустым при живой подписке.
      zhaloba("listSubscriptions", e);
    }
  }, [zhaloba]);

  // Свой VPN (план 2026-11-01, Ж): список, ход дел и мастер живут здесь, а не
  // в экране, чтобы установка, начатая до ухода на другую вкладку, не теряла
  // свой шаг. Отказы команд экран рисует сам, сюда уходит только обрыв канала.
  const svoy = useSvoyVps({
    zvat,
    naSobytie,
    naSboy: (e) => {
      if (e instanceof KanalNedostupen) zadatSvyaz("net");
    },
    // Установка, подбор и снос меняют ключи в общем списке серверов.
    naSmenuKlyuchey: () => {
      void obnovitSpisok();
      void obnovitPodpiski();
    },
    aktiven: svyaz === "est",
  });

  const zagruzitOtlozhennye = useCallback(async () => {
    try {
      const kadr = await zvat("hello", { protocol: VERSIYA_PROTOKOLA });
      if (kadr.oshibka) {
        // protocol-mismatch: the two halves are different versions, §9.1.
        zadatOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst });
        return;
      }
      const t = kadr.telo as { pozzhe?: Record<string, number> } | undefined;
      zadatOtlozheno(t?.pozzhe ?? {});
    } catch (e: unknown) {
      // A failed hello left `otlozheno` null for ever: Rules stayed on
      // "служба ещё не ответила" and every Settings control stayed grey.
      zhaloba("hello", e);
    }
  }, [zhaloba]);

  const obnovitPravila = useCallback(async () => {
    const nomer=++nomerPravil.current;
    zadatPravilaChitayutsya(true);
    try {
      const kadr = await zvat("listRules");
      if(nomer!==nomerPravil.current)return;
      if (kadr.oshibka) {
        zadatPravilaOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst });
        return;
      }
      zadatPravilaOtkaz(null);
      if (kadr.telo && typeof kadr.telo === "object") zadatPravila(kadr.telo as PravilaOtvet);
      else throw new Error("Служба не вернула правила");
    } catch (e: unknown) {
      if(nomer!==nomerPravil.current)return;
      zadatPravilaOtkaz({kod:KOD_OBOLOCHKI,tekst:`Не удалось прочитать правила: ${e instanceof Error?e.message:String(e)}`});
      zhaloba("listRules", e);
    } finally {
      if(nomer===nomerPravil.current)zadatPravilaChitayutsya(false);
    }
  }, [zhaloba]);

  // Runs a command and folds its answer into state. Every command goes
  // through here so a refusal frame becomes a refusal screen in one place.
  const vypolnit = useCallback(async (komanda: string, telo: unknown = {}, naOtvet?: (telo: unknown) => void, naOtkaz?: (o: { kod: string; tekst: string }) => void) => {
    const blocksControls = ["connect", "disconnect", "setRules", "setRouteMode", "setServer", "setActiveSubscription"].includes(komanda);
    if (blocksControls) setBusyCommand(komanda);
    const idPodpiski = komanda === "refreshSubscription" && typeof telo === "object" && telo !== null && "id" in telo && typeof telo.id === "string" ? telo.id : "";
    const ozhidanie = idPodpiski ? `${komanda}:${idPodpiski}` : komanda;
    otmetit(ozhidanie, 1);
    try {
      const kadr = await zvat(komanda, telo);
      // A frame came back, refusal or not: the pipe is alive.
      zadatSvyaz("est");
      if (kadr.oshibka) {
        // Экран, который сам показывает отказ на месте, забирает его себе, и
        // баннер не дублирует его чужими словами.
        if (kadr.oshibka.kod === KOD_SROKA && ZHDUT_ITOGA.has(komanda)) zadatOtkaz(null);
        else if (naOtkaz) naOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst });
        else zadatOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst, komanda, vkladka: vkladkaSeychas.current });
      } else {
        zadatOtkaz(null);
        naOtvet?.(kadr.telo);
      }
      const s = statusIz(kadr);
      if (s) zadatStatus(s);
      zadatUstanovku(null);
      // Even a refused addServer may have changed the list (a subscription
      // that saved but did not download), so reload after both outcomes.
      if (MENYAYUT_SPISOK.has(komanda)) void obnovitSpisok();
      // Обе команды подписки отдают строки, которые не разобрались. Пустой
      // ответ ОБНУЛЯЕТ прежний список: старые отказы после удачного обновления
      // это разговор о позапрошлой подписке.
      if (komanda === "setSubscription" || komanda === "refreshSubscription" || komanda === "addSubscription" || komanda === "setActiveSubscription") {
        zadatOtkazyPodpiski(otkazyIz(kadr.telo));
      }
      if (MENYAYUT_PODPISKI.has(komanda)) void obnovitPodpiski();
      // Замеры задержки. Ответ ЗАМЕЩАЕТ прежний целиком: показывать замер
      // позапрошлого прохода рядом со свежим значит врать про оба.
      if (komanda === "measureDelays" && !kadr.oshibka) {
        zadatZaderzhki(aktualnyeZamery(zameryIz(kadr.telo), posledniySpisok.current));
      }
      // Замер полосы. Отказ УНОСИТ прошлые числа: они были правдой про прошлый
      // проход, а на экране читались бы как ответ на нажатие, которое только
      // что не удалось. Ровно этим болела проверка утечек до 03.09.2026.
      if (komanda === "measureBandwidth") {
        if (kadr.oshibka) {
          zadatZamerPolosy(null);
        } else {
          const vremya = new Date().toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
          zadatZamerPolosy(polosaIz(kadr.telo, vremya));
        }
      }
      if (MENYAYUT_PRAVILA.has(komanda)) {
        const t = kadr.telo;
        if (!kadr.oshibka && t && typeof t === "object" && "trafik" in t) zadatPravila(t as PravilaOtvet);
        await obnovitPravila();
        if (!kadr.oshibka && t && typeof t === "object") zadatPravilaZhdut((t as { trebuet_podyoma?: unknown }).trebuet_podyoma === true);
      }
      // A fresh connect reads the rules from disk: nothing waits any more.
      if (komanda === "connect") zadatPravilaZhdut(false);
      if (komanda === "checkLeaks") {
        if (kadr.oshibka) {
          // The old report is not an answer about now: it goes out with the
          // refusal that replaced it.
          zadatProverku(null);
          zadatProverkaOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst });
        } else {
          const r = kadr.telo as RezultatProverki;
          zadatProverku(r);
          zadatProverkaOtkaz(null);
          if (r.adres_vyhoda) zadatStat((s) => ({ ...(s ?? {}), adres_vyhoda: r.adres_vyhoda as string }));
        }
      }
      if (komanda === "checkNetwork") {
        // Прошлый список слоёв не отвечает за сейчас: он уходит вместе с
        // отказом, который его заменил. Та же грабля, что уже была с утечками.
        zadatProverkuSeti(kadr.oshibka ? null : (kadr.telo as ProverkaSeti));
      }
      if (!kadr.oshibka && komanda === "checkExitIp") {
        const t = kadr.telo as { adres?: string; cherez?: string };
        // Direct measurement is not the tunnel's exit: keep it off the main object.
        if (t.adres && t.cherez === "tunnel") zadatStat((s) => ({ ...(s ?? {}), adres_vyhoda: t.adres as string }));
        // The settings row shows the answer itself: a press with no visible
        // result read as "the button does nothing" (owner, 03.09.2026).
        if (t.adres) {
          const vremya = new Date().toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
          zadatAdresVyhoda({ adres: t.adres, cherez: t.cherez === "tunnel" ? "tunnel" : "napryamuyu", vremya });
        }
      }
      return !kadr.oshibka;
    } catch (e: unknown) {
      // A closed pipe is a state, not an exception to hide. Anything else is
      // a broken frame, and rethrowing it here bought an unhandled rejection
      // and a dead button with no trace on screen.
      if (!(e instanceof KanalNedostupen)) {
        zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: `${komanda}: ${e instanceof Error ? e.message : String(e)}`, komanda, vkladka: vkladkaSeychas.current });
        return;
      }
      zadatSvyaz("net");
      zadatStatus(MOLCHIT);
      const squat = otkazIzKanala(e.message);
      zadatOtkaz(squat);
      // "No service" and "service not answering" are different screens
      // (§9.1: no-admin vs pipe-squatted vs sluzhba-molchit). Ask the SCM.
      if (!squat) {
        const est = await sluzhbaUstanovlena().catch(() => true);
        if (!est) zadatUstanovku((u) => u ?? { sostoyanie: "net-sluzhby" });
      }
      return false;
    } finally {
      if (blocksControls) setBusyCommand(current => current === komanda ? null : current);
      otmetit(ozhidanie, -1);
    }
  }, [obnovitSpisok, obnovitPravila, otmetit]);

  const naUstanovku = useCallback(() => {
    zadatUstanovku({ sostoyanie: "ustanavlivaetsya" });
    ustanovitSluzhbu()
      .then(() => {
        // UAC answered, install running. Poll until the service speaks;
        // vypolnit clears `ustanovka` on the first real answer.
        let popytok = 0;
        const t = setInterval(() => {
          popytok += 1;
          zvat("status")
            .then(() => {
              // The service speaks: stop polling and let the normal path
              // fold the answer in and drop the first-run screen.
              clearInterval(t);
              void vypolnit("status");
            })
            .catch(() => {
              if (popytok >= 30) {
                clearInterval(t);
                zadatUstanovku({ sostoyanie: "otkaz", prichina: "служба не ответила за минуту после установки" });
              }
            });
        }, 2000);
      })
      .catch((e: unknown) => {
        zadatUstanovku({ sostoyanie: "otkaz", prichina: tekstOshibki(e) });
      });
  }, [vypolnit]);

  const oprosit = useCallback(() => vypolnit("status"), [vypolnit]);

  // The repeating poll must NOT go through vypolnit: that one clears the
  // refusal banner on every answered frame, and a banner that dies five
  // seconds after it appears is the same as no banner at all.
  const oprositTiho = useCallback(async () => {
    try {
      const kadr = await zvat("status");
      zadatSvyaz("est");
      const s = statusIz(kadr);
      if (s) zadatStatus(s);
      if (!kadr.oshibka) zadatUstanovku(null);
    } catch (e: unknown) {
      if (!(e instanceof KanalNedostupen)) {
        // A broken frame is not a dead pipe: say so instead of polling on
        // forever against a shell that answers rubbish.
        zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: e instanceof Error ? e.message : String(e) });
        return;
      }
      zadatSvyaz("net");
      zadatStatus(MOLCHIT);
    }
  }, []);

  /** Main action of the `sluzhba-molchit` state: ask again, out loud. Until
   *  03.09.2026 that state had no button at all, so a window that had lost
   *  the service could only be closed and started again. */
  const povtorit = useCallback(() => void vypolnit("status"), [vypolnit]);

  // What each spec §9.1 action does here. Actions with no home yet in this
  // wave (install the service, elevate, update) only clear the screen, and
  // the fallback contract records them as such until 4.6 and 4.11 land.
  const naDeystvie = useCallback((d: Deystvie) => {
    switch (d) {
      case "povtorit":
        void vypolnit(otkaz?.komanda ?? "status");
        break;
      case "perepodklyuchitsya":
        void vypolnit("disconnect").then(() => vypolnit("connect"));
        break;
      case "otkryt-servery":
        zadatVkladku("servery");
        zadatOtkaz(null);
        break;
      case "obnovit-podpisku":
        void vypolnit("refreshSubscription");
        break;
      // Права: окно перезапускается повышенным. Служба смотрит на токен
      // того, кто пришёл в канал, поэтому повысить одну команду нельзя.
      // До 03.09.2026 эта ветка просто гасила баннер, и человек, поставивший
      // программу и перезапустивший её, не мог ни добавить сервер, ни удалить,
      // ни обновить подписку: живой прогон в госте это показал.
      case "zaprosit-prava":
        // Declining UAC used to clear the banner, so refusing the prompt
        // looked exactly like being granted the rights (guest run 03.09.2026).
        // Вкладка уходит с собой: новое окно открывается там, где человек
        // нажал кнопку, а не на «Подключении» (живой прогон 13.09.2026).
        void perezapustitSPravami(vkladkaSeychas.current).catch((e: unknown) =>
          zadatOtkaz({
            kod: "admin-required",
            tekst: e instanceof Error ? e.message : String(e),
            vkladka: vkladkaSeychas.current,
          }),
        );
        break;
      // Часы сбиты: окно открывает настройки Windows, а отказ остаётся на
      // экране, пока человек не поправит время и не нажмёт «Подключить».
      // Отказ открытия говорит своим текстом поверх того же отказа.
      case "nastroit-vremya":
        void otkrytNastroykiVremeni().catch((e: unknown) =>
          zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: e instanceof Error ? e.message : String(e) }),
        );
        break;
      // Отказы своего сервера разбирает его раздел: там отпечаток, карточка
      // и вход для захвата.
      case "prinyat-klyuch-hosta":
      case "vzyat-pod-upravlenie":
        zadatVkladku("svoy-vpn");
        ubratOtkaz();
        break;
      case "proverit-set":
      case "pokazat-vinovnika":
      case "nichego":
      case "postavit-sluzhbu":
      case "obnovit-programmu":
      case "pereustanovit-os":
      case "otkryt-port-u-hostera":
      case "poprosit-drugoy-adres":
        ubratOtkaz();
        break;
    }
  }, [ubratOtkaz, vypolnit, otkaz]);

  useEffect(() => naVidimostOkna(zadatOknoVidno), []);

  // Окно, поднятое с правами, открывается на той вкладке, с которой их просили.
  // Один раз при старте: дальше вкладку выбирает человек.
  useEffect(() => {
    void startovayaVkladka().then((v) => {
      if ((VKLADKI as readonly string[]).includes(v)) zadatVkladku(v as Vkladka);
    });
  }, []);

  // Подписка на статистику уходит в службу по ОДНОЙ команде за раз.
  //
  // Служба выполняет каждую команду соединения своим потоком (obsluzhivanie.go),
  // и две команды, посланные разом, применяются в любом порядке. Возврат окна
  // слал отписку и две подписки почти одновременно, и отписка, выполненная
  // последней, оставляла окно без цифр до следующего сворачивания. Жалоба
  // 26.09.2026: «окно с замером скорости иногда умирает само по себе».
  //
  // Следующая команда уходит только после ответа на прежнюю и несёт ПОСЛЕДНЕЕ
  // желание, промежуточные схлопываются. Счётчик, а не сравнение с посланным:
  // после возврата канала подписку надо послать заново и с тем же желанием,
  // потому что новое соединение службы о ней не знает.
  const podpiska = useRef({ vkl: false, zaprosheno: 0, poslano: 0, idyot: false });
  const soobshchitPodpisku = useCallback((vkl: boolean) => {
    const p = podpiska.current;
    p.vkl = vkl;
    p.zaprosheno += 1;
    if (p.idyot) return;
    p.idyot = true;
    void (async () => {
      while (p.poslano < p.zaprosheno) {
        p.poslano = p.zaprosheno;
        try {
          await zvat("subscribeStats", { vkl: p.vkl });
        } catch (e: unknown) {
          zhaloba("subscribeStats", e);
        }
      }
      p.idyot = false;
    })();
  }, [zhaloba]);

  // Everything the window asks for at startup, in one place: the reconnect
  // path below repeats exactly this list and nothing else.
  const sprositVsyo = useCallback(() => {
    void obnovitSpisok();
    void zagruzitOtlozhennye();
    // Подписка на статистику живёт с ЭТИМ экраном и только пока он виден.
    // Намерение было записано тут комментарием с самого начала, а отписка
    // висела на размонтировании компонента, которого при уходе в трей не
    // случается никогда: окно прячется, а не закрывается. Отдельный эффект
    // ниже включает и выключает её по видимости.
    soobshchitPodpisku(oknoVidnoRef.current);
    // listRules is NOT here: it is fetched by the effect below, once hello
    // has said the command exists. Asking a deferred command for data would
    // turn its refusal into a banner about a wave that has not landed yet.
  }, [obnovitSpisok, zagruzitOtlozhennye, soobshchitPodpisku]);

  useEffect(() => {
    void oprosit();
    sprositVsyo();
    const otpisatsya = naSobytie((kadr) => {
      if (kadr.imya === "state") {
        const s = statusIz(kadr);
        if (s) zadatStatus(s);
      } else if (kadr.imya === "stats") {
        // Numbers come only while subscribed; the service omits zaderzhka_ms
        // when the core has not measured yet, and the screen draws a dash.
        zadatStat(kadr.telo as Statistika);
      } else if (kadr.imya === "obnovlenie-hod") {
        const h = kadr.telo as HodObnovleniya;
        // Отказ гасит полосу: причина приезжает отдельным отказом команды, как
        // у любой другой кнопки, и держать после неё полосу значит врать.
        if (h.shag === "podmena") podmenaNachalas.current = true;
        zadatHod(h.shag === "otkaz" ? null : h);
      } else if (kadr.imya === "kanal-zakryt") {
        zadatSvyaz("net");
        zadatStatus(MOLCHIT);
        zadatStat(null);
      }
    });
    return () => {
      otpisatsya();
      soobshchitPodpisku(false);
    };
  }, [oprosit, sprositVsyo, soobshchitPodpisku]);

  // Подписка на статистику следует за видимостью окна. Отдельным эффектом, а
  // не внутри sprositVsyo: тот зовётся ещё и на возврате канала, а тут вопрос
  // ровно один, видно окно или нет.
  const pervayaVidimost = useRef(true);
  useEffect(() => {
    if (pervayaVidimost.current) {
      // Первый проход уже сделан монтированием: sprositVsyo подписался сам.
      pervayaVidimost.current = false;
      return;
    }
    soobshchitPodpisku(oknoVidno);
    // Вернувшееся окно спрашивает статус СРАЗУ, а не через период: человек
    // развернул программу, чтобы посмотреть, и снимок пятисекундной давности
    // это не то, за чем он её разворачивал.
    if (oknoVidno) void oprositTiho();
  }, [oknoVidno, oprositTiho, soobshchitPodpisku]);

  // Обновление меняет файл программы, а не этот процесс. Windows держит
  // открытый образ, подмена отодвигает его в .ubrat, и окно продолжает рисовать
  // ПРЕЖНИЙ код: живой прогон в госте 13.09.2026 показал службу 1.1.2 при окне
  // сборки 0.9.9 и том же pid. Со стороны это «программа не обновилась».
  //
  // Признак сменившейся программы один: служба называет номер, отличный от
  // того, с которым это окно живёт. Он же гасит полосу, которую иначе гасить
  // некому: последнее событие приходит ДО остановки службы.
  useEffect(() => {
    const nomer = status?.versiya_programmy;
    if (!nomer) return; // служба молчит или это сборка без номера
    if (svoyaVersiya.current === null) {
      svoyaVersiya.current = nomer;
      return;
    }
    if (svoyaVersiya.current === nomer) {
      // Тот же номер после подмены это откат или тот же выпуск из архива:
      // уходить некуда, но полоса обязана погаснуть.
      if (podmenaNachalas.current) {
        podmenaNachalas.current = false;
        zadatHod(null);
      }
      return;
    }
    svoyaVersiya.current = nomer;
    podmenaNachalas.current = false;
    zadatHod(null);
    void perezapustitOkno().catch((e: unknown) => zhaloba("Новое окно не открылось", e));
  }, [status?.versiya_programmy, zhaloba]);

  // Строка версии под отказом. Её отдаёт окно, а не служба: служба, которая
  // молчит, это как раз тот отказ, под которым строка нужнее всего.
  const [versiyaOkna, zadatVersiyuOkna] = useState("");
  useEffect(() => {
    let zhivo = true;
    opisanieVersii().then(
      (v) => { if (zhivo) zadatVersiyuOkna(v); },
      (e: unknown) => zapisatVZhurnal(`строка версии не получена: ${e instanceof Error ? e.message : String(e)}`),
    );
    return () => { zhivo = false; };
  }, []);

  // Трей просит открыть вкладку. Пункт «Обновить до X» открывает окно и зовёт
  // сюда: до 13.09.2026 он просто показывал окно на брошенной вкладке, и это
  // читалось как «ничего не происходит».
  useEffect(() => {
    return naVkladku((imya) => {
      if (!(VKLADKI as readonly string[]).includes(imya)) return;
      zadatVkladku(imya as Vkladka);
      // Счётчиком, а не флагом: второе нажатие в трее обязано подсветить
      // карточку снова, даже если вкладка уже открыта.
      if (imya === "nastroyki") zadatVesti((n) => n + 1);
    });
  }, []);

  // The tray recovers from a service restart on its own and the window did
  // not: status was asked once, at mount. Five seconds is the tray's period.
  //
  // У свёрнутого окна отставать нечему, поэтому опрос встаёт вместе с ним.
  // Побочно это убирает спусковой крючок пересборки меню в трее: до
  // 10.09.2026 каждый ответ status доходил до SetMenu.
  useEffect(() => {
    if (!oknoVidno) return;
    const t = window.setInterval(() => void oprositTiho(), periodOprosaMs);
    return () => window.clearInterval(t);
  }, [oprositTiho, periodOprosaMs, oknoVidno]);

  // On the way back from a dead pipe everything has to be asked again: the
  // service drops the stats subscription when a client disconnects
  // (komandy.go) and emits `state` only on a CHANGE, so a service that came
  // back in the same state tells a fresh subscriber nothing at all.
  const proshlayaSvyaz = useRef<Svyaz>("zhdyom");
  useEffect(() => {
    const bylo = proshlayaSvyaz.current;
    proshlayaSvyaz.current = svyaz;
    if (bylo !== "net" || svyaz !== "est") return;
    sprositVsyo();
    obnovitProtsessy();
  }, [svyaz, sprositVsyo, obnovitProtsessy]);

  // listRules is fetched only once the service says it exists: asking a
  // deferred command for data would turn its refusal into a banner.
  useEffect(() => {
    if ((vkladka === "pravila" || vkladka === "podklyuchenie" || vkladka === "nastroyki") && otlozheno !== null && !("listRules" in otlozheno)) void obnovitPravila();
  }, [vkladka, otlozheno, obnovitPravila]);

  // Refresh after returning from another app; yesterday's snapshot is not clairvoyant.
  useEffect(() => {
    if (vkladka !== "pravila") return;
    obnovitProtsessy();
    window.addEventListener("focus", obnovitProtsessy);
    return () => window.removeEventListener("focus", obnovitProtsessy);
  }, [vkladka, obnovitProtsessy]);

  // The service refreshes the subscription on its own schedule; opening the
  // tab is the cheap moment to catch up with that.
  useEffect(() => {
    if (vkladka === "servery" || vkladka === "podklyuchenie") {
      void obnovitSpisok();
      void obnovitPodpiski();
    }
  }, [vkladka, obnovitSpisok, obnovitPodpiski]);

  // most.ts now throws instead of turning a broken clipboard into "": the
  // screen still wants a string, so the reason goes to the banner and the
  // empty string means only "the clipboard was empty".
  const chitatBufer = useCallback(async (): Promise<string> => {
    try {
      return await tekstBufera();
    } catch (e: unknown) {
      zhaloba("буфер обмена", e);
      return "";
    }
  }, [zhaloba]);

  // Пароль из вставленного письма хостера. Очищается, только если в буфере
  // ещё он: скопированное после этого человеком стирать нельзя.
  const ochistitBufer = useCallback(async (parol: string): Promise<boolean> => {
    if (await tekstBufera() !== parol) return false;
    await skopirovatTekst("");
    return true;
  }, []);

  // Подстраховка Ctrl+V: WebView2 у Wails идёт с выключенными браузерными
  // акселераторами, и на рабочей машине вставка с клавиатуры не доходила до
  // поля вовсе. Слушатель ничего не делает там, где родная вставка работает.
  useEffect(() => naladitVstavku(chitatBufer), [chitatBufer]);

  /** Export: the path comes from the native dialog, the password from the
   *  person. `exportProfile` needs an administrator, and its refusal is a
   *  banner like any other. */
  const vyvestiProfil = useCallback(async (parol: string) => {
    zadatItogProfilya(null);
    // Отметка ставится ПОСЛЕ диалога выбора файла: пока он открыт, окно и так
    // занято своим, а вертушка под ним крутилась бы впустую. Ждать человек
    // начинает с той секунды, когда файл назван.
    let otmecheno = false;
    try {
      const put = await vybratKudaSohranit();
      if (!put) return;
      otmetit("exportProfile", 1);
      otmecheno = true;
      const kadr = await zvat("exportProfile", { parol });
      if (kadr.oshibka) {
        zadatOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst, komanda: "exportProfile", vkladka: "nastroyki" });
        return;
      }
      const telo = kadr.telo as { profil?: string } | undefined;
      if (!telo?.profil) {
        zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: "служба ответила без профиля", vkladka: "nastroyki" });
        return;
      }
      await sohranitProfil(put, telo.profil);
      zadatOtkaz(null);
      // The path is not a secret, the password is. Naming the file is the
      // only way the person can tell a save from a silent no-op.
      zadatItogProfilya("профиль записан: " + put);
    } catch (e: unknown) {
      zhaloba("exportProfile", e);
    } finally {
      if (otmecheno) otmetit("exportProfile", -1);
    }
  }, [zhaloba, otmetit]);

  /** Import replaces the whole server list, so the list is re-read after. */
  const vvestiProfil = useCallback(async (parol: string) => {
    zadatItogProfilya(null);
    let otmecheno = false;
    try {
      const put = await vybratOtkuda();
      if (!put) return;
      otmetit("importProfile", 1);
      otmecheno = true;
      const profil = await prochitatProfil(put);
      const kadr = await zvat("importProfile", { parol, profil });
      if (kadr.oshibka) {
        zadatOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst, komanda: "importProfile", vkladka: "nastroyki" });
        return;
      }
      zadatOtkaz(null);
      zadatItogProfilya("профиль принят, список серверов заменён");
      void obnovitSpisok();
      void oprosit();
    } catch (e: unknown) {
      zhaloba("importProfile", e);
    } finally {
      if (otmecheno) otmetit("importProfile", -1);
    }
  }, [obnovitSpisok, oprosit, zhaloba, otmetit]);

  const naRezhim = useCallback((rezhim: Rezhim) => void vypolnit("setRouteMode", { rezhim }), [vypolnit]);
  const naServery = useCallback(() => zadatVkladku("servery"), []);
  // Плашка из подвала главного ведёт туда же, куда пункт трея: на настройки, с
  // подсветкой карточки. Счётчик тот же, поэтому второе нажатие подсвечивает
  // снова, даже когда вкладка уже открыта.
  const kObnovleniyu = useCallback(() => {
    zadatVkladku("nastroyki");
    zadatVesti((n) => n + 1);
  }, []);

  // Screens take a whole StatusOtvet and have no "not asked yet" case: while
  // svyaz is `zhdyom` they are not drawn at all, so this stand-in is only ever
  // read once the pipe answered or died.
  const naEkrane: StatusOtvet = status ?? MOLCHIT;

  const naGlavnoe = useCallback(() => {
    // The silent-service state has one honest action, and it is not
    // "disconnect": there is nothing on the other end to disconnect from.
    if (naEkrane.sostoyanie === "sluzhba-molchit") {
      povtorit();
      return;
    }
    const cmd = naEkrane.sostoyanie === "vyklyuchen" || naEkrane.sostoyanie === "otkaz"
      ? "connect"
      : "disconnect";
    void vypolnit(cmd);
  }, [naEkrane.sostoyanie, povtorit, vypolnit]);

  // status.oshibka is the service's standing refusal (e.g. why the last
  // connect failed); a fresh command answer with oshibka takes precedence.
  // A list that refused sits between the two and carries its own retry: the
  // person must not have to guess which tab failed to load.
  let pokazat: Otkazano | null = otkaz;
  let naPovtor: (() => void) | undefined;
  if (!pokazat && spisokOtkaz) {
    pokazat = spisokOtkaz;
    naPovtor = () => void obnovitSpisok();
  }
  if (!pokazat && pravilaOtkaz) {
    pokazat = pravilaOtkaz;
    naPovtor = () => void obnovitPravila();
  }
  // Во время восстановления причина уже стоит строкой под статусом, и попытки
  // ещё идут. Красная плашка поверх читалась как «всё, сдалось» (1.9.1).
  if (!pokazat && naEkrane.oshibka && naEkrane.sostoyanie !== "vosstanavlivaetsya") {
    const izStatusa = { kod: naEkrane.oshibka.kod, tekst: naEkrane.oshibka.tekst };
    if (klyuchOtkaza(izStatusa) !== zakryto) pokazat = izStatusa;
  }
  // A refusal raised by a button stays on that button's tab.
  if (pokazat?.vkladka && pokazat.vkladka !== vkladka) pokazat = null;
  pokazannoe.current = pokazat;
  // Каждый показанный отказ уходит в журнал окна дословно (1.10.0): прежде
  // то, что видел человек, в журналах не оставалось нигде, и жалобу «выскочила
  // ошибка» сверять было не с чем.
  const klyuchPokazannogo = klyuchOtkaza(pokazat);
  useEffect(() => {
    if (klyuchPokazannogo !== null) zapisatVZhurnal(`отказ на экране: ${klyuchPokazannogo}`);
  }, [klyuchPokazannogo]);



  return (
    <main className="affory-desktop bg-background text-foreground h-screen" data-testid="oboloshka">
      <Karkas naGitHub={() => { void otkrytGitHub().catch((e: unknown) => zhaloba("Не удалось открыть GitHub", e)); }} vkladka={vkladka} naVkladku={zadatVkladku} naSvernut={oknoSvernut} naRazvernut={oknoRazvernut} naZakryt={oknoZakryt} zablokirovany={ustanovka !== null} metkaSvoegoVpn={metkaRazdela(svoy.servery, naEkrane.svoi_trevogi ?? [], svoy.hody)}>
        {/* A refusal is shown on whichever tab is open: a declined UAC on
            Settings must not wait for the human to walk back to Connection. */}
        {pokazat && !ustanovka && (
          // Закреплён сверху НАМЕРЕННО: баннер живёт в той же прокрутке, что и
          // содержимое вкладки, и человек, стоящий внизу настроек у карточки
          // обновления, отказ наверху не видел вовсе. Замер в госте 13.09.2026:
          // баннер на Y от -72 до -50 при окне от 12, то есть за кромкой. С
          // экрана это читалось как «нажал, и ничего не произошло».
          <div data-testid="otkaz-obertka" className="bg-background sticky top-0 z-20 px-8 pt-6">
            <Otkaz
              key={klyuchPokazannogo}
              kod={pokazat.kod}
              tekst={pokazat.tekst}
              vinovnik={pokazat.vinovnik}
              naDeystvie={naDeystvie}
              naPovtor={naPovtor}
              naZakrytie={ubratOtkaz}
              naSohranitZhurnal={sohranitDiagnostiku}
              naSkopirovat={skopirovatTekst}
              versiya={versiyaOkna}
            />
          </div>
        )}
        {svyaz === "zhdyom" ? (
          // Neutral, not a refusal: nothing is known yet, and a screen that
          // accuses the service of silence before the first answer is a lie
          // the person cannot tell from a real outage.
          <div className="text-fg-muted flex h-full items-center justify-center text-sm" data-testid="zhdyom">
            ожидание ответа службы
          </div>
        ) : ustanovka ? (
          // §9.2: until the service exists nothing else is active, tabs
          // included in spirit; the tab strip stays so the window can close.
          <PervyyZapusk sostoyanie={ustanovka.sostoyanie} prichina={ustanovka.prichina} naUstanovku={naUstanovku} />
        ) : vkladka === "podklyuchenie" ? (
          <Glavnyy
            naSohranitZhurnal={sohranitDiagnostiku}
            podpiski={podpiski}
            naObnovitPodpisku={id => void vypolnit("refreshSubscription", { id })}
            obnovlenieIdet={zanyatyeKomandy["refreshSubscription"] === true}
            obnovlyaemyePodpiski={Object.keys(zanyatyeKomandy).filter(k => k.startsWith("refreshSubscription:")).map(k => k.slice("refreshSubscription:".length))}
            naProverit={() => void vypolnit("measureDelays", {})}
            // measureDelays в пятёрку busyCommand не входит и входить не
            // должен: замер задержек не трогает туннель. Вертушка ему всё
            // равно нужна, поэтому берётся из списка идущих.
            proverkaIdet={zanyatyeKomandy["measureDelays"] === true}
            skorost={<Skorost {...speed} disabled={busyCommand !== null || (naEkrane.sostoyanie !== "vyklyuchen" && naEkrane.sostoyanie !== "podnyat")} />}
            pravila={pravila}
            zaderzhki={zaderzhki}
            zanyato={busyCommand !== null}
            naPravila={() => zadatVkladku("pravila")}
            naTrafik={r => { if (pravila?.trafik) void vypolnit("setRules", { trafik: { ...pravila.trafik, po_umolchaniyu: r }, bez_ru_spiska: pravila.bez_ru_spiska }); }}
            naVyborServera={(id, podpiska) => { void (async () => {
              let tekushchiyId = id;
              if (podpiska && !podpiski.some(p => p.id === podpiska && p.aktivnaya)) {
                if (!await vypolnit("setActiveSubscription", { id: podpiska }, telo => {
                  if (typeof telo !== "object" || telo === null || !("server_ids" in telo)) return;
                  const ids = telo.server_ids;
                  if (typeof ids !== "object" || ids === null || Array.isArray(ids)) return;
                  const zameny = Object.fromEntries(Object.entries(ids).filter((pair): pair is [string, string] => typeof pair[1] === "string"));
                  tekushchiyId = zameny[id] ?? id;
                  perenestiIdServerov(podpiska, zameny);
                })) return;
              }
              await vypolnit("connect", { server: tekushchiyId });
            })(); }}
            spisokOtkaz={spisokOtkaz}
            status={naEkrane}
            statistika={statistika}
            servery={spisok?.servery}
            spisok={spisok}
            naDeystvie={naGlavnoe}
            naServery={naServery}
            naSvoyServer={() => zadatVkladku("svoy-vpn")}
            naRezhim={naRezhim}
            kObnovleniyu={kObnovleniyu}
            podmenaIdet={hodObnovleniya !== null}
          />
        ) : vkladka === "servery" ? (
          // The refusal is drawn BY the tab now, not instead of it: the screen
          // names the reason, offers the repeat, and still lets a server be
          // added by link. Hiding the whole tab was a dead end (03.09.2026).
          <Servery
            status={naEkrane}
            spisok={spisok}
            spisokOtkaz={spisokOtkaz}
            obnovitSpisok={() => void obnovitSpisok()}
            naKomandu={(komanda, telo, naOtvet, naOtkaz) => void vypolnit(komanda, telo, naOtvet, naOtkaz)}
            skopirovat={skopirovatTekst}
            kodyQr={kodyQr}
            zanyatyeKomandy={zanyatyeKomandy}
            otkazyPodpiski={otkazyPodpiski}
            podpiski={podpiski}
            zaderzhki={zaderzhki}
            chitatBufer={chitatBufer}
            // The shell adds the server or the subscription itself, so both
            // lists are reloaded here: vypolnit never saw that command.
            qr={{
              nayti: async () => svodkaQrIz(await naytiQrNaEkrane()),
              dobavit: async () => {
                const itog = await dobavitNaydennoeQr();
                void obnovitSpisok();
                void obnovitPodpiski();
                return itog;
              },
              zabyt: zabytQr,
            }}
          />
        ) : vkladka === "svoy-vpn" ? (
          <SvoyVpn
            svoy={svoy}
            trevogi={naEkrane.svoi_trevogi ?? []}
            aktiven={svyaz === "est"}
            naPodklyuchit={(id) => {
              zadatVkladku("podklyuchenie");
              void vypolnit("connect", { server: id });
            }}
            naServery={naServery}
            skopirovat={skopirovatTekst}
            kodyQr={kodyQr}
            vybratKlyuch={vybratKlyuchSsh}
            ochistitBufer={ochistitBufer}
            versiya={versiyaOkna}
          />
        ) : vkladka === "pravila" ? (
          <Pravila
            proveritPrilozhenie={prochitatOhvatPrilozheniya}
            proveritSoedineniya={prochitatSoedineniya}
            chernovik={chernovikPravil}
            naChernovik={zadatChernovikPravil}
            zanyato={busyCommand !== null}
            zanyatyeKomandy={zanyatyeKomandy}
            status={naEkrane}
            otlozheno={otlozheno}
            pravila={pravila}
            pravilaOtkaz={pravilaOtkaz}
            obnovitPravila={() => void obnovitPravila()}
            zhdutPodyoma={pravilaZhdut && naEkrane.sostoyanie !== "vyklyuchen"}
            zapushchennye={zapushchennye}
            protsessyChitayutsya={protsessyChitayutsya}
            protsessyOtkaz={protsessyOtkaz}
            obnovitProtsessy={obnovitProtsessy}
            naVyborPrilozheniya={vybratPrilozhenie}
            naKomandu={(komanda, telo) => vypolnit(komanda, telo)}
          />
        ) : vkladka === "nastroyki" ? (
          <Nastroyki
            trafik={pravilaOtkaz || pravilaChitayutsya?null:pravila?.trafik}
            estChernovikPravil={!!chernovikPravil}
            naPravila={()=>zadatVkladku("pravila")}
            obnovitPravila={()=>void obnovitPravila()}
            naPapkuZhurnalov={otkrytPapkuZhurnalov}
            naDiagnostiku={sohranitDiagnostiku}
            hodObnovleniya={hodObnovleniya}
            vestiKObnovleniyu={vestiKObnovleniyu}
            adresVyhoda={adresVyhoda}
            zamerPolosy={zamerPolosy}
            status={naEkrane}
            otlozheno={otlozheno}
            svyaz={svyaz}
            povtorit={() => void sprositVsyo()}
            proverka={proverka}
            proverkaOtkaz={proverkaOtkaz}
            proverkaSeti={proverkaSeti}
            // Какие команды сейчас в полёте. Кнопка, за которой стоит
            // секунда ожидания, обязана сказать об этом сама: молчание после
            // нажатия человек читает как зависшую программу и жмёт второй раз.
            //
            // Раньше сюда уходил ОДИН busyCommand, а он держит только пять
            // команд, гасящих управление. Замер полосы, проверка утечек, адрес
            // выхода и проверка обновления в те пять не входят, и вертушка на
            // их кнопках, написанная в этом файле, не загоралась никогда.
            zanyatyeKomandy={zanyatyeKomandy}
            vyvestiProfil={(parol) => void vyvestiProfil(parol)}
            vvestiProfil={(parol) => void vvestiProfil(parol)}
            itogProfilya={itogProfilya}
            naKomandu={(komanda, telo) => void vypolnit(komanda, telo)}
            naObnovlenie={() =>
              // The dialog lives on the Go side; "" means nothing chosen and
              // nothing is sent. The service verifies sha256, not the window.
              vybratArhiv()
                .then((put) => { if (put) void vypolnit("installUpdate", { path: put }); })
                .catch((e: unknown) => zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: `Архив не выбран: ${e instanceof Error ? e.message : String(e)}` }))
            }
            naUdalenie={(steret) =>
              // A declined UAC is a refusal like any other: the human sees the
              // system's own text and the program stays where it was.
              // Код оболочки, а не admin-required: кнопка того отказа
              // перезапускает ОКНО с правами, а удаление так и не повторяет.
              udalitProgrammu(steret).catch((e: unknown) =>
                zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: e instanceof Error ? e.message : String(e) }),
              )
            }
          />
        ) : null}
      </Karkas>
    </main>
  );
}
