// Разбор того, что человек вставил в поле сайта.
//
// До 22.09.2026 поле принимало ровно одно готовое имя домена: адрес из
// адресной строки браузера («https://example.org/watch?v=1») служба отвергала
// целиком, кириллица не принималась вовсе, а список сайтов добавлялся по
// одному. При этом служба права в своей строгости: в правило уезжает имя,
// которое увидит ядро, поэтому чинить надо ввод, а не проверку.
//
// Здесь ровно приведение ввода к тому, что примет служба (`reDomen` в
// cmd/affory-svc/komandy_pravil.go): ASCII, до 253 знаков, метки до 63, не
// IP-адрес. Ничего не отправляется и не сохраняется.

/** Одно имя, готовое к отправке, вместе с тем, что для него набрали. */
export interface GotovyyDomen {
  /** То, что уедет в правило: нижний регистр, punycode для кириллицы. */
  domen: string;
  /** Как это выглядело во вводе. Показывается рядом, когда отличается:
   *  «xn--e1afmkfd.xn--p1ai» без «пример.рф» человек не узнаёт. */
  ishodnyy: string;
}

export interface OtkazStrokiDomena {
  vvod: string;
  prichina: string;
}

export interface RazborDomenov {
  gotovye: GotovyyDomen[];
  otkazy: OtkazStrokiDomena[];
}

/** Разделители ввода: перенос строки, пробел, запятая, точка с запятой.
 *  Вставка списка из буфера в однострочное поле приходит через пробелы, а
 *  руками пишут через запятую. */
const RAZDELITELI = /[\s,;]+/;

const METKA = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/;

/** Похоже ли на адрес, а не на имя. IPv6 приходит из URL в скобках, IPv4
 *  отличается от имени тем, что состоит только из цифр и точек. CIDR не
 *  доживает до сюда: слэш уезжает в путь URL, а остаток перестаёт быть
 *  числом. Всё это отдельная сущность, и прятать её под именем сайта нельзя:
 *  правило домена работает по имени, которое ядро видит в запросе. */
function adresANeImya(host: string): boolean {
  return host.startsWith("[") || /^[\d.]+$/.test(host);
}

/** Одно значение ввода. Возвращает имя для правила или причину отказа. */
export function normalizovatDomen(vvod: string): { domen: string } | { prichina: string } {
  const syroy = vvod.trim().replace(/[,;]+$/, "");
  if (!syroy) return { prichina: "пустая строка" };
  let host: string;
  try {
    // Схема нужна конструктору URL, а не правилу: он же делает IDNA, поэтому
    // «пример.рф» превращается в punycode тем же кодом, что разбирает адрес.
    const url = new URL(syroy.includes("://") ? syroy : `https://${syroy}`);
    if (url.protocol !== "https:" && url.protocol !== "http:") return { prichina: "адрес не http и не https" };
    if (url.username || url.password) return { prichina: "адрес с логином и паролем не годится" };
    host = url.hostname.toLowerCase().replace(/\.+$/, "");
  } catch {
    return { prichina: "не похоже на имя сайта" };
  }
  if (!host) return { prichina: "не похоже на имя сайта" };
  if (adresANeImya(host)) return { prichina: "это адрес, а не имя сайта: правила сайтов работают по имени" };
  if (host.length > 253) return { prichina: "имя длиннее 253 знаков" };
  const metki = host.split(".");
  if (metki.length < 2) return { prichina: "нужно полное имя, например example.com" };
  if (metki.some((m) => m.length > 63)) return { prichina: "часть имени длиннее 63 знаков" };
  if (!metki.every((m) => METKA.test(m))) return { prichina: "не похоже на имя сайта" };
  return { domen: host };
}

/** Весь ввод разом. Повторы внутри ввода схлопываются молча: человек дважды
 *  вставил один сайт, а не ошибся. Порядок сохраняется - список правил
 *  читается сверху вниз, и переставлять его за человека незачем. */
export function razobratVvodDomenov(vvod: string): RazborDomenov {
  const gotovye: GotovyyDomen[] = [];
  const otkazy: OtkazStrokiDomena[] = [];
  const vidno = new Set<string>();
  for (const kusok of vvod.split(RAZDELITELI)) {
    if (!kusok.trim()) continue;
    const itog = normalizovatDomen(kusok);
    if ("prichina" in itog) {
      otkazy.push({ vvod: kusok, prichina: itog.prichina });
      continue;
    }
    if (vidno.has(itog.domen)) continue;
    vidno.add(itog.domen);
    gotovye.push({ domen: itog.domen, ishodnyy: kusok.trim() });
  }
  return { gotovye, otkazy };
}
