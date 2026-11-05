// Подставной рантайм Wails: съёмка снимков README и обход интерфейса.
//
// Подменяется именно @wailsio/runtime, а не most.ts: мост тогда работает ровно
// тем кодом, что уезжает в выпуск, и снимок показывает настоящий экран, а не
// его копию. Alias живёт в vite.stend.config.ts.
//
// Данные подставные и намеренно узнаваемые: адреса из RFC 5737, ключей нет
// нигде. Настоящие адреса в кадр не попадают никогда.
//
// 19.09.2026 заглушка перестала быть статичной. До этого `connect` и
// `disconnect` возвращали один и тот же `podnyat`, то есть окно нельзя было
// провести ни по одному переходу: нажатие ничего не меняло. Теперь состояние
// живёт здесь и команды его двигают, а СЛУЧАЙ задаётся параметром адреса:
//
//     ?sluchay=podnyat      туннель поднят (по умолчанию)
//     ?sluchay=vyklyuchen   выключен, ждёт нажатия
//     ?sluchay=molchit      служба не отвечает
//     ?sluchay=otkaz        подъём не удался, окно показывает объяснение
//     ?sluchay=pusto        подписки нет, список серверов пуст
//     ?sluchay=novyy        службы нет вовсе: экран первого запуска
//     ?sluchay=novyy-otkaz  тот же экран, но установку отклонили
//     ?sluchay=reklama-otkaz      туннель поднят, последнее обновление списка рекламы не удалось
//     ?sluchay=reklama-gotovitsya туннель поднят, выбран «Расширенный», действует «Базовый»
//     ?sluchay=sertifikat   выключен, «Проверить» даёт два сервера с причиной от ядра
//
// Три последних в госте не воспроизводятся без нарочной поломки продукта, и
// именно поэтому экраны отказов не проверял никто.

// Подставляется конфигом стенда из файла VERSIYA в корне дерева. Числом здесь
// он уже стоял и уже соврал: снимки README показывали 1.2.0 в дни 1.3.2.
declare const __VERSIYA_STENDA__: string;
const VERSIYA = typeof __VERSIYA_STENDA__ === "string" ? __VERSIYA_STENDA__ : "dev";

function parametr(imya: string, poumolchaniyu: string): string {
  try {
    return new URLSearchParams(location.search).get(imya) ?? poumolchaniyu;
  } catch {
    return poumolchaniyu;
  }
}

import type { Server } from "../protokol";
import { DLINA_IMENI } from "../svoyVps";

const SLUCHAY = parametr("sluchay", "podnyat");
/** Случаи рекламы живут на поднятом туннеле: строки вкладки про подключение
 *  без него показывали бы другую ветку. */
const REKLAMA_SLUCHAY = SLUCHAY === "reklama-otkaz" || SLUCHAY === "reklama-gotovitsya";
const PODNYAT = SLUCHAY === "podnyat" || REKLAMA_SLUCHAY;

/** Первый запуск: службы в системе нет, канал молчит, и единственное, что
 *  делает окно, это кнопка установки. Флаг живой - подставная установка его
 *  снимает, и окно само уходит на обычный экран, как в жизни после UAC. */
let sluzhbaEst = SLUCHAY !== "novyy" && SLUCHAY !== "novyy-otkaz";

/** Номер выпуска, который находит подставная проверка: следующий минор от своей
 *  версии. Числом здесь стоять нельзя - оно разошлось бы с VERSIYA ровно так,
 *  как уже разошлись снимки README, и стенд предлагал бы обновиться назад. */
function sleduyushchayaVersiya(): string {
  const ch = VERSIYA.split(".").map((n) => Number.parseInt(n, 10));
  if (ch.length !== 3 || ch.some((n) => !Number.isFinite(n))) return "1.0.0";
  return `${ch[0]}.${ch[1] + 1}.0`;
}

// Тип берётся у протокола, а не выводится: без него поле, которого нет ни у
// одной записи образца (vne_avto), не присвоить, а образец врал бы формой
// ответа службы.
const osnovnyeServery: Server[] = [
  { id: "nl", imya: "Нидерланды · Амстердам", transport: "reality-tcp", host: "203.0.113.11", port: 443, iz_podpiski: true },
  // Транспорты пишутся ровно теми словами, какими их называет служба
  // (`izvestnyeTransporty` в internal/genkonfig/vhod.go). До 19.09.2026 здесь
  // стояли «hysteria2», «vless-ws» и «vless-tcp», которых служба не отдаёт
  // никогда, и стенд показывал экран, какого в жизни не бывает.
  { id: "de", imya: "Германия · Франкфурт", transport: "hy2", host: "203.0.113.12", port: 443, iz_podpiski: true, s_pinom: true },
  { id: "fi", imya: "Финляндия · Хельсинки", transport: "ws", host: "203.0.113.13", port: 443, iz_podpiski: true },
  { id: "se", imya: "Швеция · Стокгольм", transport: "anytls", host: "203.0.113.14", port: 443, iz_podpiski: true },
  // tuic стоит в подписке первым, и справка открывает им список своих
  // протоколов: без него снимок справки описывал бы протокол, которого на
  // соседнем снимке нет (21.09.2026).
  { id: "pl", imya: "Польша · Варшава", transport: "tuic", host: "203.0.113.16", port: 10443, iz_podpiski: true },
  { id: "svoy", imya: "Свой сервер", transport: "trojan", host: "203.0.113.15", port: 443, iz_podpiski: false },
];

const zapasnyeServery: Server[] = [{ id: "reserve-1", imya: "Запасной сервер", host: "203.0.113.20", port: 443, transport: "trojan", iz_podpiski: true }];
let aktivnayaPodpiska = "osn";
let servery = [...osnovnyeServery];

// Отказ подъёма берётся настоящим кодом и настоящим текстом службы: экран
// разбирает код и рисует по нему свой разговор, и выдуманный код провёл бы
// обход мимо той ветки, которую он же и проверяет.
const OTKAZ = {
  kod: "server-auth-failed",
  tekst: "VPN включился, но данные через него не пошли. Попыток: 2, по 15 секунд каждая. Причина: сервер не принял ключ (код ответа 502)",
};

const status: Record<string, unknown> = {
  sostoyanie: PODNYAT ? "podnyat"
    : SLUCHAY === "molchit" ? "sluzhba-molchit"
    : SLUCHAY === "otkaz" ? "ne-neset"
    : "vyklyuchen",
  vybran_id: "nl",
  // Пустой строки тут быть не может: оба поля в протоколе с omitempty, то
  // есть служба их просто НЕ ШЛЁТ, пока никто ничего не несёт. Пустая строка
  // стояла здесь до 23.09.2026 и показывала на выключенном окне «Сервер не
  // выбран» при выбранном сервере - экран, которого в жизни не бывает.
  nesushchiy_id: PODNYAT ? "nl" : undefined,
  nesushchiy_imya: PODNYAT ? "Нидерланды · Амстердам" : undefined,
  rezhim_marshruta: "ruchnoy",
  trafik_po_umolchaniyu: "vpn",
  kill_switch: false,
  avtozapusk: true,
  podklyuchat_pri_starte: false,
  zhurnal: false,
  diagnostika: false,
  port_proksi: 10809,
  versiya_programmy: VERSIYA,
  versiya_sluzhby: VERSIYA,
  podnyat_s: new Date(Date.now() - 82 * 60 * 1000).toISOString(),
  ...(SLUCHAY === "otkaz" ? { oshibka: OTKAZ } : {}),
  // Строка о перехватчике на главном экране (1.10.0), для снимков и судей.
  ...(SLUCHAY === "pomehi" ? { pomehi: [{ imya: "zapret", vid: "obhod" }] } : {}),
  // Лежащий список: тот же паспорт, что у встроенного (internal/reklama).
  reklama: {
    uroven: "light",
    pravil: 44757,
    versiya: "2026.0928.0846.00",
    sobran: "2026-09-28T08:46:00Z",
    proveren: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(),
    deystvuet: true,
    ...(SLUCHAY === "reklama-otkaz" ? { otkaz: "список не скачан: код ответа 503" } : {}),
  },
};

/** Переходы состояния. Подъём идёт ЧЕРЕЗ promezhutochnoe «podnimaetsya»:
 *  окно рисует его отдельной подписью, и переход сразу в «podnyat» оставил
 *  бы эту ветку непройденной. Задержка маленькая, обходу хватает. */
function perevesti(komanda: string): void {
  if (SLUCHAY === "molchit") return;
  if (komanda === "connect") {
    status.sostoyanie = "podnimaetsya";
    status.oshibka = undefined;
    setTimeout(() => {
      if (SLUCHAY === "otkaz") {
        status.sostoyanie = "ne-neset";
        status.oshibka = OTKAZ;
      } else {
        status.sostoyanie = "podnyat";
        status.nesushchiy_id = String(status.vybran_id ?? "nl");
        status.nesushchiy_imya = servery.find((s) => s.id === status.vybran_id)?.imya ?? "";
      }
      izvestit("kanal", JSON.stringify({ tip: "sobytie", id: 0, imya: "status", telo: status }));
    }, 300);
  }
  if (komanda === "disconnect") {
    status.sostoyanie = "vyklyuchen";
    status.nesushchiy_id = undefined;
    status.nesushchiy_imya = undefined;
    status.oshibka = undefined;
  }
}

const pravila = {
  // protsessy и domeny верхнего уровня это ПРЕЖНИЙ формат (списки строк), а
  // нынешние правила живут в trafik. Перепутать их значит уронить экран.
  protsessy: [] as string[],
  domeny: [] as string[],
  // Три поля из teloPravil, которых у заглушки не было: экран читает их и
  // решает, показывать ли «правила ждут переподъёма» и выключатель ru-списка.
  trebuet_podyoma: false,
  spisok_izmenyon: false,
  bez_ru_spiska: false,
  // Как шлёт служба: объект всегда целиком. Случаи рекламы включают её сразу,
  // иначе вкладка не показала бы ни одной строки состояния.
  reklama: {
    vkl: REKLAMA_SLUCHAY,
    uroven: (SLUCHAY === "reklama-gotovitsya" ? "multi" : "light") as "light" | "multi",
    razresheno: [] as string[],
  },
  trafik: {
    po_umolchaniyu: "vpn",
    prilozheniya: [
      { put: "C:\\Program Files\\Mozilla Firefox\\firefox.exe", imya: "firefox.exe", potomki: true, marshrut: "direct" },
      // Свои программы, а не программы сервисов: правило на steam.exe служба
      // снимает при обновлении, этой программой теперь управляет карточка.
      { put: "C:\\Program Files\\Epic Games\\Fortnite\\FortniteClient-Win64-Shipping.exe", imya: "FortniteClient-Win64-Shipping.exe", potomki: true, marshrut: "direct" },
      { put: "C:\\Program Files\\qBittorrent\\qbittorrent.exe", imya: "qbittorrent.exe", potomki: false, marshrut: "vpn" },
    ],
    domeny: [
      { domen: "gosuslugi.ru", marshrut: "direct" },
      { domen: "reddit.com", marshrut: "vpn" },
    ],
    // Явные маршруты сервисов: два ведут мимо туннеля, счётчик вкладки
    // показывает шесть из восьми.
    servisy: [
      { id: "telegram", marshrut: "direct" },
      { id: "spotify", marshrut: "direct" },
    ],
  },
  // Каталог повторяет встроенный (internal/katalog/servisy.json): те же
  // сервисы, те же домены и те же имена клиентов, иначе снимок врёт о
  // содержимом выпуска. Пять последних живут без доменов: набора в iplist для
  // них нет, и маршрут держится на программе.
  katalog: {
    versiya: "2026.09.08",
    istochnik: "https://iplist.opencck.org/ru/",
    servisy: [
      { id: "youtube", imya: "YouTube", domeny: ["youtube.com", "youtu.be", "googlevideo.com"], programmy: [], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "discord", imya: "Discord", domeny: ["discord.com", "discord.gg", "discord.media"], programmy: ["Discord.exe", "DiscordPTB.exe", "DiscordCanary.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "chatgpt", imya: "ChatGPT", domeny: ["chatgpt.com", "openai.com", "oaistatic.com"], programmy: ["ChatGPT.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "instagram", imya: "Instagram", domeny: ["instagram.com", "cdninstagram.com"], programmy: [], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "claude", imya: "Claude", domeny: ["claude.ai", "claude.com", "anthropic.com"], programmy: ["Claude.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "telegram", imya: "Telegram", domeny: ["telegram.org", "telegram.me", "t.me"], programmy: ["Telegram.exe", "Telegram Desktop.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "spotify", imya: "Spotify", domeny: ["spotify.com", "scdn.co", "spotifycdn.com"], programmy: ["Spotify.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "soundcloud", imya: "SoundCloud", domeny: ["soundcloud.com", "sndcdn.com", "snd.sc"], programmy: ["SoundCloud.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "steam", imya: "Steam", domeny: [], programmy: ["steam.exe"] },
      { id: "epicgames", imya: "Epic Games", domeny: [], programmy: ["EpicGamesLauncher.exe"] },
      { id: "battlenet", imya: "Battle.net", domeny: [], programmy: ["Battle.net.exe"] },
      { id: "riot", imya: "Riot Client", domeny: [], programmy: ["RiotClientServices.exe"] },
      { id: "ubisoft", imya: "Ubisoft Connect", domeny: [], programmy: ["UbisoftConnect.exe", "upc.exe"] },
    ],
  },
};

// Замеры приходят ответом на measureDelays, то есть на нажатие «Проверить
// серверы»: съёмка нажимает её сама. Пинг одним числом, того же порядка, что
// «Задержка» главного экрана (38 мс в снимке статистики ниже).
// Случай sertifikat (02.10.2026): у двух серверов отказ с причиной, которую
// назвало ядро, как при истёкшем сертификате и чужом имени.
const zamery = {
  zamery: SLUCHAY === "sertifikat" ? [
    { id: "nl", ping_ms: null, ping_prichina: "сертификат не действует",
      ping_otkaz: "сертификат сервера просрочен или ещё не начал действовать: если часы на компьютере верные, его должен продлить владелец сервера" },
    { id: "de", ping_ms: null, ping_prichina: "сертификат на другое имя",
      ping_otkaz: "у сервера сертификат на другое имя: адрес в ключе мог устареть, или связь перехватывают по пути" },
    { id: "fi", ping_ms: 61 },
    { id: "pl", ping_ms: 52 },
    { id: "se", ping_ms: null, ping_otkaz: "сервер не ответил вовремя" },
    { id: "svoy", ping_ms: 64 },
  ] : [
    { id: "nl", ping_ms: 46 },
    { id: "de", ping_ms: 38 },
    { id: "fi", ping_ms: 61 },
    { id: "pl", ping_ms: 52 },
    { id: "se", ping_ms: 57 },
    { id: "svoy", ping_ms: 64 },
  ],
};

/** Снимок замера скорости. Живёт переменной, потому что экран ОПРАШИВАЕТ его
 *  раз в такт: одно и то же тело на все опросы оставило бы замер вечно
 *  идущим, а кнопку «Отмена» вечно нажатой. */
let skorost: Record<string, unknown> = { id: 0, phase: "idle", path: "vpn", provider: "", name: "", attempt: 0 };

function stroka(x: unknown): string {
  return typeof x === "string" ? x : "";
}

/** Тумблер настроек. Ответ это статус, и поле в нём обязано смениться: иначе
 *  обход прочитает неподвижный тумблер как дефект окна. */
function tumbler(pole: string): (vhod: Record<string, unknown>) => unknown {
  return (vhod) => {
    status[pole] = vhod.vkl === true;
    return status;
  };
}

function vybrat(vhod: Record<string, unknown>): unknown {
  const id = stroka(vhod.id) || stroka(vhod.server);
  if (id) {
    status.vybran_id = id;
    if (status.sostoyanie === "podnyat") {
      status.nesushchiy_id = id;
      status.nesushchiy_imya = servery.find((s) => s.id === id)?.imya ?? "";
    }
  }
  return status;
}

// Ответы команд, по одному на каждую команду службы.
//
// ---------------------------------------------------------------- свой VPN
//
// Записи и задания своего сервера (1.11.0) живут здесь, как в службе
// (cmd/affory-svc/svoy_vps.go): одно задание за раз, ответ команды сразу
// отдаёт снимок, ход идёт событием svoy-vps-hod, полный снимок отдаёт
// svoyVpsHod. Случай раздела задаётся своим параметром:
//
//     ?svoy=pusto            своих серверов нет (по умолчанию)
//     ?svoy=server           один сервер, работает
//     ?svoy=slomano          проверка видит VPN на сервере мёртвым
//     ?svoy=bez-klyucha      ключ управления остался на другом компьютере
//     ?svoy=klyuch-hosta     сервер предъявил другой ключ хоста
//     ?svoy=ustanovka        установка идёт с запуска окна
//     ?svoy=ustanovka-otkaz  установка встала на подборе портов
//     ?svoy=net-ustanovki    сервер в разделе, а установки на нём нет до новой
//
// Адрес 203.0.113.66 в мастере даёт отказ входа. Пароль и ключ из тела
// заглушка не читает и не хранит.

const SVOY = parametr("svoy", "pusto");
const SVOY_ID = "svoy-5f1c0e2a9b7d4c31";
const SVOY_ADRES = "203.0.113.40";
const CHAS_MS = 3600 * 1000;
// ?obnovlenie=est: окно ставит программу сервера новее, чем на нём;
// otkaz - и обновление не выходит.
const OBNOVLENIE = parametr("obnovlenie", "net");
// ?maska=mertva: сторож программы сервера не достаёт маску (МА3); 1-11 -
// сервер 1.11.0 с прежней известной маской (МА4).
const MASKA = parametr("maska", "zhiva");

/** Отказ команды тем же кадром, что у службы: полем oshibka, а не
 *  исключением моста. */
class OtkazStenda extends Error {
  constructor(readonly kod: string, tekst: string) {
    super(tekst);
  }
}

type Zapis = Record<string, unknown>;

function svoyZapis(adres: string, reality: number, hy2: number, imya = "Мой сервер"): Zapis {
  return {
    id: SVOY_ID, imya, adres, port_ssh: 22, polzovatel: "root",
    klyuch_hosta_tip: "ssh-ed25519", maska: "www.example.net", maska_istochnik: "sosedi", versiya: "0.6.0",
    reality, hy2, prodlenie: "80",
    servery: [`${SVOY_ID}-reality`, `${SVOY_ID}-hy2`],
    postavlen: new Date(Date.now() - 72 * CHAS_MS).toISOString(),
    sert_do: Math.floor((Date.now() + 80 * 24 * CHAS_MS) / 1000),
    sert_proveren: new Date(Date.now() - 2 * CHAS_MS).toISOString(),
    proveren: new Date(Date.now() - 2 * CHAS_MS).toISOString(),
    klyuch_upravleniya: SVOY !== "bez-klyucha",
    otpechatok: "SHA256:c3RlbmQtaG9zdC1rZXktMDE",
    ...(SVOY === "klyuch-hosta" ? { novyy_otpechatok: "SHA256:c3RlbmQtaG9zdC1rZXktMDI" } : {}),
    ...(OBNOVLENIE !== "net" ? { novaya_versiya: "0.7.0" } : {}),
    ...(MASKA === "mertva" ? { maska_ne_otvechaet: "сайт отвечает без h2" } : {}),
    ...(MASKA === "1-11" ? { maska: "dl.google.com", maska_istochnik: undefined, versiya: "0.7.0" } : {}),
  };
}

/** Ключи группы в общем списке: имена и транспорты, как у konfig.go. */
function klyuchiSvoego(adres: string, reality: number, hy2: number, imya = "Мой сервер"): Server[] {
  return [
    { id: `${SVOY_ID}-reality`, imya: `${imya}, reality`, transport: "reality-tcp", host: adres, port: reality, iz_podpiski: false, svoy: true },
    { id: `${SVOY_ID}-hy2`, imya: `${imya}, hysteria2`, transport: "hy2", host: adres, port: hy2, iz_podpiski: false, svoy: true },
  ];
}

const S_SERVEROM = !["pusto", "ustanovka", "ustanovka-otkaz"].includes(SVOY);
let svoi: Zapis[] = S_SERVEROM ? [svoyZapis(SVOY_ADRES, 2087, 5443)] : [];
if (S_SERVEROM) servery.push(...klyuchiSvoego(SVOY_ADRES, 2087, 5443));

function ubratSvoego(id: string): void {
  svoi = svoi.filter((z) => z.id !== id);
  servery = servery.filter((s) => !s.id.startsWith(`${id}-`));
}

let hodVps: Zapis | null = null;
let shagiVps: ReturnType<typeof setTimeout>[] = [];
let ustanovkaEst = SVOY !== "net-ustanovki";
const OTKAZ_NET_USTANOVKI = { kod: "vps-reinstall", tekst: "на сервере нет установки Affory: поставь его заново, ключи VPN при этом сменятся" };

function izvestitHod(): void {
  if (!hodVps) return;
  const { id, vid, faza, shag, server, planovaya } = hodVps;
  izvestit("kanal", JSON.stringify({ tip: "sobytie", id: 0, imya: "svoy-vps-hod", telo: { id, vid, faza, shag, server, planovaya } }));
}

/** Новое задание. Шаги идут по таймеру, каждый правит снимок и шлёт событие,
 *  последний ставит фазу. */
function nachatDelo(vid: string, server: string | undefined, shagi: ((h: Zapis) => void)[], shagMs = 700): Zapis {
  if (hodVps && hodVps.faza === "idyot") throw new OtkazStenda("vps-busy", "служба уже занята другим делом со своим сервером, дождись его конца");
  hodVps = { id: Date.now(), vid, faza: "idyot", shag: vid === "ustanovka" ? "zagruzka" : "vhod", server, proverki: [], otseyany: [], preduprezhdeniya: [], nachalo: new Date().toISOString() };
  shagiVps = shagi.map((shag, i) => setTimeout(() => {
    if (!hodVps || hodVps.faza !== "idyot") return;
    shag(hodVps);
    if (hodVps.faza !== "idyot") hodVps.konec = new Date().toISOString();
    izvestitHod();
  }, (i + 1) * shagMs));
  return { ...hodVps };
}

function zakonchit(h: Zapis, faza: string, dobavit: Zapis = {}): void {
  Object.assign(h, dobavit, { faza });
  delete h.klyuch;
}

const PUT_PRYAMO = "напрямую через Ethernet";

/** Ход подбора: два порта не проходят, по одному на вид проходит. */
function shagiPodbora(adres: string, otkaz: boolean): ((h: Zapis) => void)[] {
  const proverki = (h: Zapis) => h.proverki as Zapis[];
  return [
    (h) => { h.shag = "podbor"; h.otseyany = [{ proto: "tcp", port: 22, prichina: "ssh" }, { proto: "tcp", port: 80, prichina: "sert" }]; h.klyuch = { transport: "reality-tcp", port: 443 }; },
    (h) => { proverki(h).push({ transport: "reality-tcp", port: 443, vid: "net-otveta", otkaz: "за 10 секунд ни байта" }); h.klyuch = { transport: "reality-tcp", port: 2087 }; },
    (h) => { proverki(h).push({ transport: "reality-tcp", port: 2087, vid: "vezyot" }); h.klyuch = { transport: "hy2", port: 443 }; },
    otkaz
      ? (h) => {
          proverki(h).push({ transport: "hy2", port: 443, vid: "zamerz", otkaz: "встал на 300 КБ" });
          zakonchit(h, "oshibka", { otkaz: { kod: "vps-ports-failed", tekst: `ни один порт hysteria2 не прошёл из этой сети до ${adres}` } });
        }
      : (h) => { proverki(h).push({ transport: "hy2", port: 443, vid: "zamerz", otkaz: "встал на 300 КБ" }); h.klyuch = { transport: "hy2", port: 5443 }; },
    (h) => { proverki(h).push({ transport: "hy2", port: 5443, vid: "vezyot" }); delete h.klyuch; h.shag = "paket"; },
    (h) => { h.shag = "porty"; },
    (h) => { h.shag = "proverka"; h.klyuch = { transport: "reality-tcp", port: 2087 }; },
  ];
}

function ustanovlen(): Zapis {
  return {
    server: SVOY_ID,
    klyuchi: [{ transport: "reality-tcp", port: 2087, vid: "vezyot" }, { transport: "hy2", port: 5443, vid: "vezyot" }],
    ids: [`${SVOY_ID}-reality`, `${SVOY_ID}-hy2`],
  };
}

function poId(v: Record<string, unknown>): Zapis {
  const z = svoi.find((x) => x.id === stroka(v.id));
  if (!z) throw new OtkazStenda("request-invalid", "свой сервер с таким id не найден");
  return z;
}

const SVOY_OTVETY: Record<string, (vhod: Record<string, unknown>) => unknown> = {
  svoyVpsSpisok: () => ({ servery: svoi }),
  svoyVpsHod: () => (hodVps ? { ...hodVps } : { faza: "net" }),
  svoyVpsOtmenit: () => {
    if (hodVps && hodVps.faza === "idyot") {
      for (const t of shagiVps) clearTimeout(t);
      zakonchit(hodVps, "otmeneno");
      hodVps.konec = new Date().toISOString();
      izvestitHod();
    }
    return hodVps ? { ...hodVps } : { faza: "net" };
  },
  svoyVpsOsmotret: (v) => {
    const adres = stroka(v.adres);
    return nachatDelo("osmotr", undefined, [
      (h) => {
        if (adres === "203.0.113.66") {
          zakonchit(h, "oshibka", { otkaz: { kod: "vps-login-failed", tekst: "сервер не принял пароль для root" } });
          return;
        }
        h.put = PUT_PRYAMO;
        zakonchit(h, "gotovo", {
          osmotr: {
            put: PUT_PRYAMO, root: true, klyuch_hosta_tip: "ssh-ed25519", klyuch_hosta: "c3RlbmQ=",
            otpechatok: "SHA256:c3RlbmQtaG9zdC1rZXktMDE", sistema: "Ubuntu 24.04, x86_64, KVM, 1 ГБ памяти",
            zamechaniya: [{ kod: "net-bbr", stop: false, tekst: "ядро без BBR: Affory включит его сам, если позволит система" }],
            mozhno_stavit: true,
          },
        });
      },
    ], 1200);
  },
  svoyVpsUstanovit: (v) => {
    const adres = stroka(v.adres);
    const imya = stroka(v.imya).trim() || "Мой сервер";
    const posle = (h: Zapis) => {
      zakonchit(h, "gotovo", { ustanovlen: ustanovlen() });
      ustanovkaEst = true;
      svoi = [...svoi.filter((z) => z.id !== SVOY_ID), { ...svoyZapis(adres, 2087, 5443, imya), proveren: new Date().toISOString(), klyuch_upravleniya: true }];
      servery = [...servery.filter((s) => !s.id.startsWith(`${SVOY_ID}-`)), ...klyuchiSvoego(adres, 2087, 5443, imya)];
    };
    const h = nachatDelo("ustanovka", SVOY_ID, [
      (x) => { x.shag = "vhod"; x.adres = adres; },
      (x) => { x.shag = "osmotr"; x.put = PUT_PRYAMO; },
      (x) => { x.shag = "klyuch"; },
      (x) => { x.shag = "programma"; },
      (x) => { x.shag = "maska"; },
      (x) => { x.shag = "sertifikat"; },
      ...shagiPodbora(adres, SVOY === "ustanovka-otkaz"),
      posle,
    ]);
    return h;
  },
  svoyVpsProverit: (v) => {
    const z = poId(v);
    return nachatDelo("proverka", stroka(z.id), [
      (h) => { h.shag = "osmotr"; h.put = PUT_PRYAMO; },
      (h) => {
        if (!ustanovkaEst) {
          zakonchit(h, "oshibka", { otkaz: OTKAZ_NET_USTANOVKI });
          // Служба пишет причину неудачи в запись (deloProverki).
          Object.assign(z, { proveren: new Date().toISOString(), proverka_otkaz: OTKAZ_NET_USTANOVKI.tekst });
          return;
        }
        const zhivo = SVOY !== "slomano";
        zakonchit(h, "gotovo", {
          proverka: {
            versiya: "0.6.0", yadro_zhivo: zhivo,
            ...(zhivo ? {} : { oshibka_yadra: "sing-box вышел с кодом 1: порт 2087 уже занят" }),
            porty: [{ vid: "reality", port: z.reality, slushaet: zhivo }, { vid: "hy2", port: z.hy2, slushaet: zhivo }],
            sert_do: z.sert_do, maska: "godna", pamyat_mb: 612, disk_mb: 18234,
          },
        });
        z.proveren = new Date().toISOString();
      },
    ], 900);
  },
  svoyVpsPodobratPorty: (v) => {
    const z = poId(v);
    const reality = typeof v.reality === "number" ? v.reality : 2087;
    const hy2 = typeof v.hy2 === "number" ? v.hy2 : 5443;
    if (!ustanovkaEst) return nachatDelo("podbor", stroka(z.id), [(h) => zakonchit(h, "oshibka", { otkaz: OTKAZ_NET_USTANOVKI })]);
    return nachatDelo("podbor", stroka(z.id), [
      ...shagiPodbora(stroka(z.adres), false),
      (h) => {
        zakonchit(h, "gotovo", { ustanovlen: { ...ustanovlen(), klyuchi: [{ transport: "reality-tcp", port: reality, vid: "vezyot" }, { transport: "hy2", port: hy2, vid: "vezyot" }] } });
        // Подбор, как и служба (zapisatSvoy), отмечает визит: прежняя проверка не в счёт.
        Object.assign(z, { reality, hy2, porty_ruchnye: typeof v.reality === "number" || typeof v.hy2 === "number", proveren: new Date().toISOString() });
        servery = [...servery.filter((s) => !s.id.startsWith(`${z.id}-`)), ...klyuchiSvoego(stroka(z.adres), reality, hy2)];
      },
    ]);
  },
  // Своя маска: известное имя без согласия - vps-mask-known, suckless.org
  // не годится, остальное встаёт.
  svoyVpsMaska: (v) => {
    const z = poId(v);
    const svoya = stroka(v.svoya).trim().toLowerCase();
    const izvestnaya = ["dl.google.com", "www.adobe.com", "www.microsoft.com"].includes(svoya);
    return nachatDelo("maska", stroka(z.id), [
      (h) => { h.shag = "maska"; h.maska = { svoya: svoya || undefined }; },
      (h) => {
        if (svoya === "suckless.org") {
          zakonchit(h, "oshibka", { otkaz: { kod: "vps-mask-failed", tekst: "маска suckless.org не годится: сайт не отвечает h2. На сервере прежняя маска" } });
          return;
        }
        if (izvestnaya && v.brend_soglasen !== true) {
          zakonchit(h, "oshibka", { otkaz: { kod: "vps-mask-known",
            tekst: `маска ${svoya} - известный сайт: на мобильном интернете такие имена глушат, и ключ reality с ним может не открыться. На сервере прежняя маска` } });
          return;
        }
        h.shag = "paket";
      },
      (h) => {
        const imya = svoya || "shop.example.org";
        const istochnik = !svoya ? "sosedi" : izvestnaya ? "brend" : "svoya";
        zakonchit(h, "gotovo", { put: PUT_PRYAMO, maska: { svoya: svoya || undefined, imya, istochnik },
          ustanovlen: { ...ustanovlen(), klyuchi: [{ transport: "reality-tcp", port: z.reality }, { transport: "hy2", port: z.hy2 }] } });
        Object.assign(z, { maska: imya, maska_istochnik: istochnik, maska_ne_otvechaet: undefined, proveren: new Date().toISOString() });
      },
    ], svoya ? 900 : 1500);
  },
  svoyVpsSnesti: (v) => {
    const z = poId(v);
    return nachatDelo("snos", stroka(z.id), [
      (h) => { h.shag = "osmotr"; h.put = PUT_PRYAMO; },
      (h) => {
        zakonchit(h, "gotovo", { snyat: { parol_vernulsya: z.parol_vyklyuchen === true } });
        ubratSvoego(stroka(z.id));
      },
    ], 1000);
  },
  svoyVpsOtpustit: (v) => {
    const z = poId(v);
    return nachatDelo("otpusk", stroka(z.id), [
      (h) => {
        zakonchit(h, "gotovo", { put: PUT_PRYAMO, otpushchen: { klyucha_ne_bylo: false } });
        ubratSvoego(stroka(z.id));
      },
    ], 1000);
  },
  svoyVpsZabyt: (v) => {
    if (hodVps && hodVps.faza === "idyot") throw new OtkazStenda("vps-busy", "сервер нельзя забыть, пока идёт дело с ним");
    const z = poId(v);
    ubratSvoego(stroka(z.id));
    return { id: z.id };
  },
  svoyVpsPereimenovat: (v) => {
    if (hodVps && hodVps.faza === "idyot") throw new OtkazStenda("vps-busy", "со своим сервером уже идёт дело: дождись его конца или отмени");
    const z = poId(v);
    const imya = Array.from(stroka(v.imya).trim()).slice(0, DLINA_IMENI).join("");
    if (imya === "") throw new OtkazStenda("request-invalid", "название пустое");
    z.imya = imya;
    servery = servery.map((s) => (s.id === `${z.id}-reality` ? { ...s, imya: `${imya}, reality` } : s.id === `${z.id}-hy2` ? { ...s, imya: `${imya}, hysteria2` } : s));
    return { id: z.id, imya };
  },
  svoyVpsVzyat: (v) => {
    const z = svoi.find((x) => x.adres === stroka(v.adres));
    return nachatDelo("vzyat", z ? stroka(z.id) : SVOY_ID, [
      (h) => {
        if (!ustanovkaEst) {
          zakonchit(h, "oshibka", { otkaz: OTKAZ_NET_USTANOVKI });
          return;
        }
        zakonchit(h, "gotovo", { put: PUT_PRYAMO });
        if (z) Object.assign(z, { klyuch_upravleniya: true, proveren: new Date().toISOString() });
      },
    ], 1000);
  },
  svoyVpsParol: (v) => {
    const z = poId(v);
    const vyklyuchit = v.vyklyuchit === true;
    return nachatDelo("parol", stroka(z.id), [
      (h) => {
        zakonchit(h, "gotovo", { put: PUT_PRYAMO, parol: vyklyuchit ? { vyklyuchen: true } : { vyklyuchen: false, snaruzhi: "vklyuchen" } });
        z.parol_vyklyuchen = vyklyuchit;
      },
    ], 900);
  },
  svoyVpsObnovit: (v) => {
    const z = poId(v);
    const novaya = stroka(z.novaya_versiya);
    if (!novaya) throw new OtkazStenda("request-invalid", `на сервере уже программа ${stroka(z.versiya)}, новее в этой версии Affory нет`);
    return nachatDelo("obnovlenie", stroka(z.id), [
      (h) => { h.shag = "osmotr"; },
      (h) => { h.shag = "programma"; },
      (h) => { h.shag = "perezapusk"; },
      (h) => { h.shag = "proverka"; h.klyuch = { transport: "reality-tcp", port: 2087 }; },
      OBNOVLENIE === "otkaz"
        ? (h) => {
            zakonchit(h, "oshibka", { otkaz: { kod: "vps-update-failed",
              tekst: "программа сервера не обновилась: с ней ни один ключ не провёз из твоей сети. Прежняя программа возвращена" } });
          }
        : (h) => {
            zakonchit(h, "gotovo", { put: PUT_PRYAMO, obnovlenie: { bylo: stroka(z.versiya), versiya: novaya, ne_proshli: [] } });
            Object.assign(z, { versiya: novaya, proveren: new Date().toISOString() });
            delete z.novaya_versiya;
            delete z.proverka_otkaz;
          },
    ], 900);
  },
  svoyVpsPrinyatKlyuchHosta: (v) => {
    const z = poId(v);
    if (stroka(v.otpechatok) !== stroka(z.novyy_otpechatok)) throw new OtkazStenda("vps-host-key-changed", "сервер предъявил уже другой ключ");
    z.otpechatok = z.novyy_otpechatok;
    delete z.novyy_otpechatok;
    // Как служба: сервер могли переустановить, отметка визита и пароля сброшены.
    z.parol_vyklyuchen = false;
    delete z.proveren;
    delete z.proverka_otkaz;
    return { id: z.id };
  },
};

// Установка, начатая до запуска окна: мастер обязан вернуться к ней сам.
if (SVOY === "ustanovka" || SVOY === "ustanovka-otkaz") {
  SVOY_OTVETY.svoyVpsUstanovit({ adres: SVOY_ADRES });
}

// Форма ответа повторяет службу поле в поле. Окно разбирает тело и падает
// ЦЕЛИКОМ, если поля нет: первый же прогон обхода 19.09.2026 уронил раздел
// «Настройки» нажатием «Проверить», потому что прежний `default` возвращал
// пустое тело, а экран читает `proverka.punkty.map`. Служба на checkLeaks
// отдаёт четыре пункта ВСЕГДА (internal/set/utechki.go), то есть виноват был
// стенд. Незнакомых команд с тех пор нет молча, см. probely ниже.
const OTVETY: Record<string, (vhod: Record<string, unknown>) => unknown> = {
  status: () => status,
  // Имя поля у connect это `server`, а не `id` (dispetcher.go). Заглушка
  // читала `id`, и выбор сервера при подключении не доезжал никуда.
  connect: (v) => {
    const id = stroka(v.server) || stroka(v.id);
    if (id) { status.vybran_id = id; status.rezhim_marshruta = "ruchnoy"; }
    perevesti("connect");
    return status;
  },
  disconnect: () => {
    perevesti("disconnect");
    return status;
  },
  setServer: vybrat,
  switch: vybrat,
  // Статус ВЛОЖЕН, см. statusIz в App.tsx: плоский статус тут читался бы
  // окном, но разошёлся бы с ответом службы.
  setRouteMode: (v) => {
    if (stroka(v.rezhim)) status.rezhim_marshruta = stroka(v.rezhim);
    return { status, trebuet_podyoma: false };
  },
  hello: () => ({ pozzhe: {} }),

  setKillSwitch: tumbler("kill_switch"),
  setAutostart: tumbler("avtozapusk"),
  setConnectOnStart: tumbler("podklyuchat_pri_starte"),
  setJournal: tumbler("zhurnal"),
  setDiagnostics: tumbler("diagnostika"),
  clearJournal: () => status,
  // Проверка НАХОДИТ выпуск: до 23.09.2026 заглушка отдавала свой же статус, и
  // ни плашку в подвале главного, ни карточку с кнопкой «Установить» обход
  // увидеть не мог - их рисует только находка.
  checkUpdate: () => {
    status.obnovlenie = { versiya: sleduyushchayaVersiya(), razmer: 26934390, provereno: new Date().toISOString() };
    status.obnovlenie_provereno = new Date().toISOString();
    return status;
  },
  downloadUpdate: () => ({ zapushchena: true, srok_s: 90 }),
  installUpdate: () => ({ zapushchena: true, srok_s: 90 }),
  setBandwidth: () => status,
  subscribeStats: (v) => ({ vkl: v.vkl === true }),

  listServers: () => {
    // Пустой набор это не «список из нуля строк», а другой экран целиком:
    // окно предлагает завести подписку. Ветка не гонялась ни разу, потому
    // что в госте подписка есть всегда.
    if (SLUCHAY === "pusto") {
      return { servery: [], vybran: "", podpiska_zadana: false, podpiska_uzel: "" };
    }
    return { servery, versii: Object.fromEntries(servery.map(s => [s.id, `demo-v1-${s.id}`])), vybran: status.vybran_id, podpiska_zadana: true, podpiska_uzel: aktivnayaPodpiska === "osn" ? "panel.example" : "reserve.example" };
  },
  // Область автовыбора (A5). Заглушка правит СВОЙ список и отдаёт его целиком,
  // как служба: без этого меню правой кнопки на стенде нажималось бы вхолостую.
  setAutoMember: (v) => {
    const id = stroka(v.id);
    const srv = servery.find((s) => s.id === id);
    if (srv) srv.vne_avto = v.uchastvuet !== true;
    return { servery, versii: Object.fromEntries(servery.map(s => [s.id, `demo-v1-${s.id}`])), vybran: status.vybran_id, podpiska_zadana: true, podpiska_uzel: aktivnayaPodpiska === "osn" ? "panel.example" : "reserve.example" };
  },
  addServer: () => ({ server: servery[servery.length - 1] }),
  // Пачка (26.09.2026): стенд считает строки, а не разбирает их. Строка без
  // «://» идёт в отказ, чтобы итог с пропущенными было видно на снимке.
  addServers: (v) => {
    const stroki = stroka(v.tekst).split(/\r\n|\r|\n/);
    const otkazy = stroki
      .map((s, i) => ({ s: s.trim(), stroka: i + 1 }))
      .filter(({ s }) => s !== "" && !s.toLowerCase().startsWith("http") && !s.includes("://"))
      .map(({ stroka: n }) => ({ stroka: n, prichina: "ссылка не разобрана: нет схемы" }));
    const dobavleno = stroki.filter((s) => s.includes("://") && !s.trim().toLowerCase().startsWith("http")).length;
    return { dobavleno, obnovleno: 0, uzhe_bylo: 0, otkazy };
  },
  // С 02.10.2026 выгрузка отдаёт и строки по серверам, а ids выбирает один
  // сервер: так меню правой кнопки берёт ключ одного.
  exportServers: (v) => {
    const ids = Array.isArray(v.ids) ? v.ids.filter((x): x is string => typeof x === "string") : [];
    const otobrany = ids.length ? servery.filter((s) => ids.includes(s.id)) : servery;
    const klyuchi = otobrany.map((s) => ({
      id: s.id, imya: s.imya, transport: s.transport, adres: `${s.host}:${s.port}`,
      ssylka: `hy2://demo-parol@${s.host}:${s.port}?sni=demo.example#${encodeURIComponent(s.imya)}`,
    }));
    const tekst = klyuchi.map((k) => k.ssylka).join("\n");
    return { tekst, base64: btoa(String.fromCharCode(...new TextEncoder().encode(tekst))), vsego: klyuchi.length, servery: klyuchi, propushcheny: [] };
  },
  removeServer: () => ({ ostalos: servery.length }),
  listRules: () => pravila,
  inspectApplication: (v) => {
    const root=pravila.trafik.prilozheniya.find(a=>a.put.toLowerCase()===stroka(v.put).toLowerCase());
    if(!root)throw new Error("Правило приложения изменилось. Обнови список правил");
    const launcher=root.put.toLowerCase().includes("steam");
    const child=launcher?"C:\\Games\\Demo\\game.exe":root.put;
    const winner=pravila.trafik.prilozheniya.find(a=>a.put.toLowerCase()===child.toLowerCase()) || root;
    const included=!launcher || root.potomki;
    return {pravilo:root,reviziya_pravil:"demo",trebuet_podyoma:false,vremya:new Date().toISOString(),fayl:root.put.includes("Missing")?"net":"est",samo:launcher?0:1,vsego:included?1:0,
      zapushchennye:included?[{pid:24680,created:"134000000000000001",put:child,imya:child.split("\\").pop(),cherez:launcher?[root.put,child]:[root.put],marshrut:winner.marshrut,pravilo_put:winner.put,pravilo_imya:winner.imya,pereopredelen:winner.put!==root.put}]:[],
      neizvestno:1,neizvestnye:["C:\\Apps\\Separate.exe"],ogranichen:false};
  },
  // Пустой список стоял здесь до 23.09.2026, и проверка охвата всегда падала
  // в ветку «подходящих соединений не обнаружено»: сам список соединений в
  // окне не видел никто. Фильтр учитывается, иначе проба любого домена
  // показывала бы чужие строки.
  listConnections: (v) => {
    const domen = stroka(v.domen).toLowerCase();
    const put = stroka(v.put).toLowerCase();
    // Значения ровно той формы, какую отдаёт clash_api: выход это первое
    // звено цепочки («srv-nl», «direct»), правило это строка sing-box вида
    // «domain_suffix=x => srv-nl», путь процесса полный. Своя форма показала
    // бы «не удалось определить» на каждой строке.
    const hrom = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe";
    const vse = [
      { Id: "1", Host: "youtube.com", Adres: "203.0.113.60", Port: 443, Protsess: hrom, Vyhod: "srv-nl", Pravilo: "rule_set=service-youtube => srv-nl", Nachalo: new Date(Date.now() - 42 * 1000).toISOString() },
      { Id: "2", Host: "rr3---sn-5hne6nsy.googlevideo.com", Adres: "203.0.113.61", Port: 443, Protsess: hrom, Vyhod: "srv-nl", Pravilo: "domain_suffix=googlevideo.com => srv-nl", Nachalo: new Date(Date.now() - 9 * 1000).toISOString() },
      { Id: "3", Host: "gosuslugi.ru", Adres: "198.51.100.7", Port: 443, Protsess: hrom, Vyhod: "direct", Pravilo: "domain_suffix=gosuslugi.ru => direct", Nachalo: new Date(Date.now() - 4 * 60 * 1000).toISOString() },
      { Id: "4", Host: "", Adres: "203.0.113.62", Port: 443, Protsess: "C:\\Users\\home\\AppData\\Local\\Discord\\app-1.0.9187\\Discord.exe", Vyhod: "srv-nl", Pravilo: "final => vybor", Nachalo: new Date(Date.now() - 77 * 1000).toISOString() },
    ];
    const podhodit = (s: typeof vse[number]) => {
      if (domen) return s.Host === domen || s.Host.endsWith(`.${domen}`);
      if (put) return put.endsWith(`\\${s.Protsess.toLowerCase()}`);
      return true;
    };
    return { yadro: status.sostoyanie === "podnyat", vremya: new Date().toISOString(), ogranichen: false, soedineniya: vse.filter(podhodit) };
  },
  setRules: (v) => {
    if (v.trafik && typeof v.trafik === "object") {
      pravila.trafik = { ...pravila.trafik, ...(v.trafik as Record<string, unknown>) } as typeof pravila.trafik;
    }
    // Оба поля служба хранит рядом с trafik. Без них стенд терял
    // выключатель российского списка после первого же применения.
    if (typeof v.bez_ru_spiska === "boolean") pravila.bez_ru_spiska = v.bez_ru_spiska;
    if (v.reklama && typeof v.reklama === "object") {
      pravila.reklama = { ...pravila.reklama, ...(v.reklama as Record<string, unknown>) } as typeof pravila.reklama;
    }
    return pravila;
  },
  listSubscriptions: () => ({ podpiski: SLUCHAY === "pusto" ? [] : [
    { id: "osn", uzel: "panel.example", aktivnaya: aktivnayaPodpiska === "osn", serverov: 5, servery: osnovnyeServery.filter(s => s.iz_podpiski), obnovlena: new Date(Date.now()-50*60000).toISOString() },
    { id: "reserve", uzel: "reserve.example", aktivnaya: aktivnayaPodpiska === "reserve", serverov: 1, servery: zapasnyeServery },
  ] }),
  addSubscription: (v) => ({ id: stroka(v.id) || "osn", aktivnaya: true, serverov: servery.length, otkazy: [] }),
  removeSubscription: (v) => ({ udalena: stroka(v.id) }),
  setActiveSubscription: (v) => {
    aktivnayaPodpiska = stroka(v.id) || "osn";
    servery = aktivnayaPodpiska === "osn" ? [...osnovnyeServery] : [...zapasnyeServery, ...osnovnyeServery.filter(s => !s.iz_podpiski)];
    return { aktivnaya: aktivnayaPodpiska, serverov: servery.length, otkazy: [] };
  },
  setSubscription: () => ({ zadana: true, serverov: servery.length, otkazy: [] }),
  refreshSubscription: () => ({ serverov: servery.length, otkazy: [] }),

  measureDelays: () => ({ zamery: zamery.zamery.filter(z => servery.some(s => s.id === z.id)).map(z => ({ ...z, versiya: `demo-v1-${z.id}` })) }),
  measureBandwidth: () => ({
    mbit_vniz: 318.4, bayt_vniz: 2_140_000_000, potokov: 4, sovet_vniz: 286,
    cherez_tunnel: true,
    mbit_vverh: 96.2, bayt_vverh: 620_000_000, sovet_vverh: 86,
  }),
  getServerHealth: () => ({
    vremya: Math.floor(Date.now() / 1000) - 240, vozrast_s: 240, ustarel: false, trevogi: [],
    dney_do_konca_sertifikata_maski: 74, dney_s_obnovleniya_xray: 12,
    hy2_aktiven: true, dney_do_konca_sertifikata_hy2: 74,
  }),
  checkExitIp: () => ({ adres: "203.0.113.24", cherez: "tunnel" }),
  // Четыре пункта теми же словами и теми же итогами, что у службы: экран
  // разбирает `itog` и рисует по нему три разных строки.
  checkLeaks: () => ({
    adres_vyhoda: "203.0.113.24",
    punkty: [
      { imya: "адрес выхода", itog: "ok", tekst: "через VPN виден 203.0.113.24, это адрес сервера" },
      { imya: "IPv6", itog: "ok", tekst: "исходящий IPv6 закрыт или его у машины нет: мимо VPN по нему не уйти" },
      { imya: "поиск сайтов по имени", itog: "ne_vidim", tekst: "эти запросы забирает на себя VPN. Куда они уходят дальше, программа со своей стороны увидеть не может: это проверяется отдельно, на испытательной машине" },
      { imya: "браузер со своим поиском адресов (DoH)", itog: "ne_vidim", tekst: "такой браузер ищет адреса сам, своим DoH, и общую настройку обходит. Правила по сайтам на него не действуют, и эта проверка его тоже не видит" },
    ],
  }),
  // Пять слоёв теми же видами и подписями, что у службы (proverka_seti.go).
  // Один слой СЛОМАН намеренно: список, где всё зелено, не показывает, как
  // окно рисует отказ, и обход проходил бы мимо этой половины разметки.
  checkNetwork: () => ({
    vremya: new Date().toISOString(),
    sloi: [
      { vid: "tunnel", podpis: "VPN на этом компьютере", proshlo: true, podrobno: "Affory, адрес 172.19.0.1", ms: 2 },
      { vid: "yadro", podpis: "Связь с сервером", proshlo: true, podrobno: "ответ за 41 мс", ms: 41 },
      { vid: "imya", podpis: "Имена сайтов через VPN", proshlo: true, podrobno: "cp.cloudflare.com это 104.16.132.229", ms: 28 },
      { vid: "mestnyy-dns", podpis: "Имена сайтов мимо VPN", proshlo: false, podrobno: "yandex.ru не находится по имени: нет ответа за отведённый срок; через 192.168.0.1 открываются российские сайты и всё, что идёт мимо VPN", ms: 3001 },
      { vid: "udp", podpis: "Голос и видео", proshlo: true, podrobno: "получено 10 из 10, среднее 24 мс, разброс 6 мс", ms: 1140 },
    ],
  }),
  startSpeedTest: (v) => {
    skorost = { id: Date.now(), phase: "download", path: "vpn", provider: stroka(v.provider) || "librespeed", name: "LibreSpeed · Helsinki", attempt: 1 };
    setTimeout(() => {
      skorost = {
        ...skorost, phase: "complete",
        result: { name: "LibreSpeed · Helsinki", download_mbps: 284.6, upload_mbps: 92.1, attempts: [{ name: "LibreSpeed · Helsinki" }] },
      };
    }, 600);
    return skorost;
  },
  speedTestStatus: () => skorost,
  cancelSpeedTest: () => {
    skorost = { ...skorost, phase: "cancelled", reason: "замер отменён" };
    return skorost;
  },

  exportProfile: () => ({ profil: "QUZGT1JZLVBST0ZJTA==" }),
  importProfile: () => ({ prinyato: true }),
  ...SVOY_OTVETY,
};

/** Команды, которых заглушка не знает.
 *
 *  Пустой ответ на такую команду однажды уронил окно и выглядел дефектом
 *  продукта. Теперь незнакомая команда получает ОТКАЗ - окно рисует его
 *  штатно, тем же путём, что отказ службы, - а её имя остаётся здесь. Обход
 *  читает список в конце и объявляет НЕГОДЕН стенда, а не провал продукта:
 *  ровно то разделение, ради которого затевался переход с UIA. */
const probely: string[] = [];
(globalThis as unknown as Record<string, unknown>).__probelyStenda = probely;

type Obrabotchik = (sobytie: { data: unknown }) => void;
const podpischiki = new Map<string, Obrabotchik[]>();

function izvestit(imya: string, data: unknown): void {
  for (const o of podpischiki.get(imya) ?? []) o({ data });
}

// Пути настоящего вида: у Discord в нём номер сборки, у Steam диск установки.
// Ровно поэтому путь и не угадывается в самом окне.
const ZAPUSHCHENNYE = [
  { imya: "Discord", put: "C:\\Users\\home\\AppData\\Local\\Discord\\app-1.0.9187\\Discord.exe" },
  { imya: "Steam", put: "C:\\Program Files (x86)\\Steam\\steam.exe" },
  { imya: "Telegram", put: "C:\\Users\\home\\AppData\\Roaming\\Telegram Desktop\\Telegram.exe" },
  { imya: "Браузер", put: "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" },
];

export const Call = {
  async ByName(imya: string, ...args: unknown[]): Promise<unknown> {
    if (imya === "main.most.VybratPrilozhenie") throw new Error("В браузерном стенде выбор файла недоступен. В установленном Affory откроется окно выбора приложения.");
    // Файла ключа в браузере нет: стенду нужно имя и непустое содержимое.
    if (imya === "main.most.VybratKlyuchSsh") return { imya: "id_ed25519", base64: "c3RlbmQ=" };
    if (imya === "main.most.OtkrytPapkuZhurnalov") throw new Error("В браузерном стенде Проводник недоступен. В установленном Affory кнопка открывает папку журналов.");
    if (imya === "main.most.OtkrytNastroykiVremeni") throw new Error("В браузерном стенде настроек Windows нет. В установленном Affory кнопка открывает настройки даты и времени.");
    if (imya === "main.most.SluzhbaUstanovlena") return sluzhbaEst;
    if (imya === "main.most.UstanovitSluzhbu") {
      if (SLUCHAY === "novyy-otkaz") throw new Error("права не выданы: запрос отклонён");
      // Служба появляется не мгновенно: окно должно успеть показать полосу
      // хода, иначе эта ветка так и останется непройденной.
      setTimeout(() => { sluzhbaEst = true; }, 4000);
      return null;
    }
    // Запущенные программы стенда. Без них карточки сервисов с клиентом
    // выглядели бы «не запущено» на всех снимках, а у Steam маршрут вовсе
    // нельзя было бы выбрать: путь берётся среди запущенных.
    if (imya === "main.most.SpisokProtsessov") return ZAPUSHCHENNYE;
    // Настоящий код рисует оболочка на Go (qr_vyvod.go), в браузере её нет.
    // Стенду нужен только размер и место картинки, поэтому узор, а не QR.
    if (imya === "main.most.KodyQr") {
      const uzor = "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 29 29'><rect width='29' height='29' fill='white'/><path d='M4 4h7v7H4zM18 4h7v7h-7zM4 18h7v7H4zM13 13h3v3h-3zM18 18h2v2h-2zM22 20h3v3h-3z' fill='black'/></svg>";
      return ["data:image/svg+xml;utf8," + encodeURIComponent(uzor)];
    }
    // QR с экрана: снимать в браузере нечего, стенду нужен блок подтверждения
    // с тем, что показала бы оболочка.
    if (imya === "main.most.NaytiQrNaEkrane") {
      return {
        klyuchi: [{ imya: "Германия · Франкфурт", transport: "reality-grpc", adres: "203.0.113.31:110" }],
        podpiski: ["panel.example.net"],
        negodnyh: 1,
      };
    }
    if (imya === "main.most.DobavitNaydennoeQr") return "добавлено 1; подписка добавлена про запас";
    if (imya === "main.most.ZabytQr") return null;
    // Диалога сохранения в браузере нет: стенду нужна строка итога.
    if (imya === "main.most.SohranitDiagnostiku") return "сохранено: affory-zhurnal-2026-09-28-134000.txt, 1,2 МБ";
    // Журнала у стенда нет: строка уходит в консоль, где её и ищут.
    if (imya === "main.most.ZapisatVZhurnal") { console.info("журнал окна:", args[0]); return null; }
    if (imya === "main.most.OpisanieVersii") return "Affory 1.10.0, Windows 11 (26200)";
    if (imya === "main.most.Zvat") {
      // Канала нет, пока нет службы: окно ловит это исключением и спрашивает
      // диспетчер, установлена ли она вообще.
      if (!sluzhbaEst) throw new Error("канал недоступен: нет службы");
      const komanda = String(args[0]);
      // Тело запроса приезжает вторым доводом строкой JSON (см. most.ts).
      // Раньше заглушка его выбрасывала, и выбор сервера не доезжал никуда:
      // окно щёлкало по списку, а стенд показывал прежний.
      let vhod: Record<string, unknown> = {};
      try { vhod = JSON.parse(String(args[1] ?? "{}")) as Record<string, unknown>; } catch { vhod = {}; }
      const otvetchik = OTVETY[komanda];
      if (!otvetchik) {
        if (!probely.includes(komanda)) probely.push(komanda);
        return JSON.stringify({
          tip: "otvet", id: 1, imya: komanda,
          oshibka: { kod: "stend-ne-znaet-komandu", tekst: `подставной мост не знает команду ${komanda}` },
        });
      }
      try {
        return JSON.stringify({ tip: "otvet", id: 1, imya: komanda, telo: otvetchik(vhod) });
      } catch (e) {
        if (!(e instanceof OtkazStenda)) throw e;
        return JSON.stringify({ tip: "otvet", id: 1, imya: komanda, oshibka: { kod: e.kod, tekst: e.message } });
      }
    }
    return null;
  },
};

export const Events = {
  On(imya: string, o: Obrabotchik): () => void {
    const bylo = podpischiki.get(imya) ?? [];
    podpischiki.set(imya, [...bylo, o]);
    // Стартовое значение подаётся В МОМЕНТ ПОДПИСКИ, а не по таймеру.
    //
    // Раньше подача шла четырежды - на 80, 250, 600 и 1200 мс, чтобы точно
    // попасть после useEffect окна. Для снимков это работало, а обход ломало
    // насмерть: человек жмёт «Правила» на 400-й миллисекунде, а на 600-й
    // заглушка возвращает экран на стартовый, и вкладка «не открывается».
    // Разбор 19.09.2026 занял бы куда больше, если бы Playwright не показал
    // дерево на момент отказа: там стояло «tab Подключение [selected]».
    //
    // Подписчик виден прямо здесь, поэтому гадать про сроки незачем.
    if (!bylo.length) queueMicrotask(() => podat(imya, o));
    return () => podpischiki.set(imya, (podpischiki.get(imya) ?? []).filter((x) => x !== o));
  },
};

// ToggleMaximise зовёт most.ts при нажатии «Развернуть». Без него нажатие
// роняло бы окно исключением, а не разворачивало его.
export const Window = { Minimise: async () => {}, ToggleMaximise: async () => {}, Close: async () => {} };
export const Browser = { OpenURL: async () => {} };
export const Clipboard = { Text: async () => "", SetText: async (_tekst: string) => {} };

// Экран выбирается параметром адреса, вкладку окно берёт из события трея.
// Цифры под сферой приходят тем же путём, что и в жизни: событием kanal, где
// data это СТРОКА JSON, а не объект (см. naSobytie в most.ts).
//
// Счётчики растут с каждым кадром: скорость под сферой окно считает по
// разнице двух снимков, и с застывшими числами снимок README показывал
// прочерк вместо скорости (02.10.2026). Прирост за секунду идёт по кругу из
// четырёх значений, чтобы цифра жила, но кадр съёмки был предсказуемым.
const PRIROST_VNIZ = [5_400_000, 5_150_000, 5_600_000, 5_300_000];
const PRIROST_VVERH = [420_000, 390_000, 450_000, 410_000];
let takt = 0;
let prinyato = 2_362_232_012;
let otdano = 184_090_624;
function kadrStats(): string {
  takt++;
  prinyato += PRIROST_VNIZ[takt % PRIROST_VNIZ.length];
  otdano += PRIROST_VVERH[takt % PRIROST_VVERH.length];
  return JSON.stringify({
    tip: "sobytie", id: 0, imya: "stats",
    telo: { adres_vyhoda: "203.0.113.24", zaderzhka_ms: 38, prinyato, otdano },
  });
}

/** Что подать подписчику события `imya` сразу при подписке. */
function podat(imya: string, o: Obrabotchik): void {
  if (imya === "okno") o({ data: true });
  if (imya === "vkladka") o({ data: parametr("ekran", "podklyuchenie") });
  if (imya === "kanal") o({ data: kadrStats() });
}

// Цифры под сферой идут потоком, как в жизни. Вкладка и признак окна
// потоком НЕ идут: они подаются один раз при подписке, см. Events.On.
setInterval(() => izvestit("kanal", kadrStats()), 1000);
