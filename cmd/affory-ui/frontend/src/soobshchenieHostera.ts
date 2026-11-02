// Разбор сообщения хостера (Ж3 плана «Свой VPN»).
//
// Хостер присылает доступ письмом или в Telegram, и каждый своими словами:
// «IP-адрес: …», «Login - root», «🔑 Пароль — …», «ssh root@… -p 2222». Человек
// вставляет сообщение целиком, а окно раскладывает его по полям мастера. Текст
// разбирается здесь и никуда не уходит: служба получает только поля.
//
// Разбор строками. В строке метка и значение через двоеточие, тире или дефис
// с пробелами; эмодзи и значки перед меткой отбрасываются. Метка без значения
// берёт его со следующей строки. Чего разобрать не вышло, того просто нет в
// итоге: поля человек правит руками, а неверный пароль хуже пустого поля.

export interface DostupIzSoobshcheniya {
  adres?: string;
  port?: number;
  polzovatel?: string;
  parol?: string;
}

type Pole = keyof DostupIzSoobshcheniya;
type PoleMetki = Pole | "polzovatel-i-parol";

// Слова меток ищутся с начала слова: «port» не ловит «support», «user» не
// ловит «superuser». «pass» и «pwd» только целым словом: «Passport» не пароль.
// Адрес по «ip» только в начале метки: «Private IP» и «Failover IP» не он.
// Порядок важен: «Root password» и «SSH port» содержат и другие слова.
const S = String.raw`(?<![\p{L}\p{N}])`;
const METKI: { pole: Pole; slova: RegExp }[] = [
  { pole: "parol", slova: new RegExp(String.raw`${S}(?:пароль|password|passwd|(?:pass|pwd)(?![\p{L}\p{N}]))`, "u") },
  { pole: "port", slova: new RegExp(`${S}(?:порт|port)`, "u") },
  { pole: "polzovatel", slova: new RegExp(`${S}(?:логин|пользовател|login|user)`, "u") },
  { pole: "adres", slova: new RegExp(`^ip|${S}(?:адрес|address|host|хост|сервер|server)`, "u") },
];

const IMYA = "[a-z_][a-z0-9_.-]{0,31}";
const POLZOVATEL = new RegExp(`^${IMYA}$`, "i");

// «Логин / пароль: root / Xy7#»: оба поля одной строкой. «User password» сюда
// не попадает: между словами нет разделителя. Пара в обратном порядке
// («Пароль/логин») и метка на три поля («Host/login/password») не
// разбираются вовсе: где что, пришлось бы гадать.
const RAZDELITEL = String.raw`[/,&+]|\sи\s|\sand\s`;
const MEZHDU = String.raw`\S*\s*(?:${RAZDELITEL})\s*`;
const LOGIN_I_PAROL = new RegExp(`(?:логин|login|user|пользовател)${MEZHDU}(?:пароль|password|pass)`);
const PAROL_I_LOGIN = new RegExp(`(?:пароль|password|pass)${MEZHDU}(?:логин|login|user|пользовател)`);
// Третье поле через разделитель: «/IP», «Host/». «Логин и пароль от сервера»
// и «Логин/Login и пароль/Password» остаются парой.
const ADRES_ILI_PORT = "ip|адрес|address|host|хост|сервер|server|порт|port";
const TRETE_POLE = new RegExp(
  String.raw`(?:${RAZDELITEL})\s*(?:${ADRES_ILI_PORT})|${S}(?:${ADRES_ILI_PORT})\S*\s*(?:${RAZDELITEL})`,
  "u",
);
const IMYA_I_PAROL = new RegExp(String.raw`^(${IMYA})(?:\s*[/|,;:]\s*|\s+)(\S+)$`, "i");

// Заглушка вместо значения: «Пароль: -», «Пароль: нет» при входе по ключу.
const ZAGLUSHKA = /^(?:[^\p{L}\p{N}]+|нет|none|n\/a|null|nil|empty|пусто)$/iu;

// Невидимые символы из копии письма: пробелы нулевой ширины (U+200B-U+200D,
// U+2060), метка порядка байтов U+FEFF и мягкий перенос U+00AD. Иначе уехали
// бы в пароль. Собраны кодами, чтобы в исходнике их было видно.
const NEVIDIMYE = new RegExp(`[${String.fromCodePoint(0x200b, 0x200c, 0x200d, 0x2060, 0xfeff, 0xad)}]`, "g");

// Доступ не к серверу: панель хостера, кабинет, почта, сброс пароля, система
// спасения, а ещё шлюз, DNS, резолвер, NTP, IPMI и IP-KVM. «Пароль от
// панели» иначе встал бы паролем root, «DNS-сервер» адресом, а «Hosting:
// сайт хостера» именем узла.
// Здесь подстрока нарочно: лишнее совпадение даёт пустое поле, а не чужое.
const CHUZHIE =
  /панел|panel|кабинет|account|аккаунт|billing|биллинг|vnc|консол|console|web|сайт|mail|почт|reset|сброс|восстанов|rescue|(?<!s)ftp|mysql|баз[аы]|шлюз|gateway|dns|resolv|резолв|ntp|маск|mask|ipmi|ip[\s-]?kvm|kvm[\s-]?ip|nameserver|hosting|хостинг/;

const IPV4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/;
// Точка после адреса это конец фразы, а не пятая часть.
const IPV4_V_TEKSTE = /(?<![\d.])(?:\d{1,3}\.){3}\d{1,3}(?!\d|\.\d)/g;
const IMYA_UZLA = /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/i;

/** IPv4, которым сервер виден из интернета. Частные (10/8, 172.16/12,
 *  192.168/16, 100.64/10, 127/8, 169.254/16, 0/8) и групповые с
 *  зарезервированными (от 224.0.0.0, в том числе маска 255.255.255.0) до
 *  сервера не доводят: «Private IP: 10.0.0.5», «Subnet: 255.255.255.0». */
function publichnyyIPv4(a: string): boolean {
  if (!IPV4.test(a)) return false;
  const [x, y] = a.split(".").map(Number);
  return !(
    x === 0 ||
    x === 10 ||
    x === 127 ||
    x >= 224 ||
    (x === 172 && y >= 16 && y <= 31) ||
    (x === 192 && y === 168) ||
    (x === 100 && y >= 64 && y <= 127) ||
    (x === 169 && y === 254)
  );
}

/** IPv6, которым сервер виден из интернета: не локальный канала (fe80::/10),
 *  не уникальный локальный (fc00::/7), не групповой (ff00::/8) и не петля. */
function publichnyyIPv6(a: string): boolean {
  const s = a.toLowerCase();
  return !/^(?:fe[89ab][0-9a-f]|f[cd][0-9a-f]{2}|ff[0-9a-f]{2}):/.test(s) && s !== "::1" && s !== "::";
}

/** У IPv6 не меньше двух двоеточий: «203.0.113.10:2200» это адрес с портом. */
function eIPv6(s: string): boolean {
  const dvoetochiy = s.split(":").length - 1;
  return dvoetochiy >= 2 && dvoetochiy <= 7 && /^[0-9a-f:.]+$/i.test(s) && (s.match(/::/g)?.length ?? 0) <= 1;
}

/** Метка без значков и номера пункта в начале: «🖥 Сервер», «• IP», «1) Логин»,
 *  «2. IP». */
function chistayaMetka(s: string): string {
  return s
    .replace(/^(?:[^\p{L}\p{N}]+|\d{1,3}[.)])+/u, "")
    .replace(/[\s.]+$/, "")
    .toLowerCase();
}

/** Значение без `кода` Telegram и «ёлочек» вокруг. Прямые кавычки не
 *  снимаются: в сгенерированном пароле они бывают его частью. */
function chistoeZnachenie(s: string): string {
  const t = s.trim();
  const m = /^(?:`(.*)`|«(.*)»)$/s.exec(t);
  return m ? (m[1] ?? m[2]).trim() : t;
}

function port(s: string): number | undefined {
  if (!/^\d{1,5}$/.test(s)) return undefined;
  const n = Number(s);
  return n >= 1 && n <= 65535 ? n : undefined;
}

type Adres = { adres: string; port?: number };

/** Адрес с портом или без: 203.0.113.10, 203.0.113.10:2222, [2001:db8::1]:22,
 *  vps.example.net. Имя без точки («VPS-1») это название, а не адрес. Маска у
 *  IPv4 («203.0.113.10/24») отбрасывается; IPv6 с маской это подсеть, а не
 *  адрес сервера, она не разбирается. */
function adresIPort(s: string): Adres | undefined {
  const v = (s.split(/\s+/)[0] ?? "").replace(/[.,;]+$/, "").replace(/^((?:\d{1,3}\.){3}\d{1,3})\/\d{1,2}$/, "$1");
  const skobki = /^\[([0-9a-f:.]+)\](?::(\d+))?$/i.exec(v);
  if (skobki && eIPv6(skobki[1])) return { adres: skobki[1], port: skobki[2] ? port(skobki[2]) : undefined };
  if (eIPv6(v)) return { adres: v };
  const [hostChast, portChast, lishnee] = v.split(":");
  if (lishnee !== undefined) return undefined;
  if (!IPV4.test(hostChast) && !IMYA_UZLA.test(hostChast)) return undefined;
  return { adres: hostChast, port: portChast === undefined ? undefined : port(portChast) };
}

/** Делит строку на метку и значение. Дефис без пробелов вокруг разделителем не
 *  считается: «IP-адрес» это одно слово. Значение пустое у метки, чьё значение
 *  стоит следующей строкой («Пароль:»). */
function razdelit(stroka: string): [string, string] | undefined {
  const m = /^(.*?)\s*(?::|：|=|\s[-–—]\s|[–—])\s*(.*)$/.exec(stroka);
  if (m && m[1].trim() !== "") return [m[1], m[2]];
  // «IP 203.0.113.10», «Пароль Xy7#kL2p»: метка словом, значение одним словом
  // через пробел. Фраза вида «Password reset link …» сюда не проходит.
  const p = /^(\S+)\s+(\S+)$/.exec(stroka);
  return p ? [p[1], p[2]] : undefined;
}

/** Поле по метке. Чужая метка (панель, шлюз) или незнакомая - undefined. */
function poleMetki(metka: string): PoleMetki | undefined {
  if (metka === "" || CHUZHIE.test(metka) || PAROL_I_LOGIN.test(metka)) return undefined;
  if (LOGIN_I_PAROL.test(metka)) return TRETE_POLE.test(metka) ? undefined : "polzovatel-i-parol";
  return METKI.find((m) => m.slova.test(metka))?.pole;
}

/** Разбор сообщения целиком. Адреса копятся по видам до конца: IPv4 важнее
 *  имени узла и IPv6, у VPS обычно есть всё, а ключи ставятся на IPv4. Порт
 *  из самого адреса («host:2083») берётся только у выбранного адреса: у
 *  другого он может быть портом панели. Порт из метки и «ssh -p» это порт SSH,
 *  он берётся сразу. */
export function razobratSoobshchenie(tekst: string): DostupIzSoobshcheniya {
  const itog: DostupIzSoobshcheniya = {};
  const vidy: { v4?: Adres; imya?: Adres; v6?: Adres } = {};
  // IPv4 из строк не про шлюз, DNS и панель: запас, если меток нет.
  const bezMetki = new Set<string>();

  const prinyatPort = (s: string) => {
    const p = port(s);
    if (p !== undefined) itog.port ??= p;
  };
  const prinyatAdres = (a: Adres | undefined) => {
    if (!a) return;
    if (a.adres.includes(":")) {
      if (publichnyyIPv6(a.adres)) vidy.v6 ??= a;
    } else if (!IPV4.test(a.adres)) vidy.imya ??= a;
    else if (publichnyyIPv4(a.adres)) vidy.v4 ??= a;
  };
  // Имена в Linux по обычаю строчные: «Логин: Root» это root. adduser в
  // Debian и Ubuntu заглавные по умолчанию не пропускает (NAME_REGEX).
  const prinyatImya = (s: string) => {
    if (POLZOVATEL.test(s)) itog.polzovatel ??= s.toLowerCase();
  };
  const prinyat = (pole: PoleMetki, syroe: string) => {
    const znachenie = chistoeZnachenie(syroe);
    if (znachenie === "" || ZAGLUSHKA.test(znachenie)) return;
    switch (pole) {
      case "parol":
        // Пробел без `кода` вокруг значит фразу: «будет отправлен в SMS».
        if (!/\s/.test(znachenie) || znachenie !== syroe.trim()) itog.parol ??= znachenie;
        break;
      case "port":
        // «2222 (SSH)», «2222/tcp».
        prinyatPort(znachenie.split(/\s+/)[0].replace(/\/tcp$/i, ""));
        break;
      case "polzovatel":
        prinyatImya(znachenie);
        break;
      case "adres":
        prinyatAdres(adresIPort(znachenie));
        break;
      case "polzovatel-i-parol": {
        // Ровно имя и пароль, иначе не угадывать.
        const m = IMYA_I_PAROL.exec(znachenie);
        if (m && !ZAGLUSHKA.test(m[2])) {
          prinyatImya(m[1]);
          itog.parol ??= m[2];
        }
        break;
      }
    }
  };

  let zhdyot: PoleMetki | undefined;
  const chistyy = tekst.replace(NEVIDIMYE, "");
  for (const syraya of chistyy.split(/\r?\n/)) {
    const stroka = syraya.trim();
    if (stroka === "") continue;
    const chuzhaya = CHUZHIE.test(stroka.toLowerCase());
    if (!chuzhaya) for (const a of stroka.match(IPV4_V_TEKSTE) ?? []) if (publichnyyIPv4(a)) bezMetki.add(a);

    const ssh = /(?:^|[^\p{L}\p{N}])ssh(?:\s*[:：=]\s*|\s+[-–—]\s+|\s+)(.*)$/iu.exec(stroka);
    // Перед ssh чужая метка: «Консоль восстановления: ssh rescue@…».
    if (ssh && !CHUZHIE.test(stroka.slice(0, ssh.index).toLowerCase())) {
      // «ssh root@203.0.113.10 -p 2222», «ssh -p2222 root@203.0.113.10»,
      // «SSH: root@203.0.113.10», «ssh://root@203.0.113.10:2222»,
      // «ssh -l root 203.0.113.10».
      const chasti = ssh[1].split(/\s+/);
      let imyaKlyuchom: string | undefined;
      let goloeSlovo: Adres | undefined;
      for (let i = 0; i < chasti.length; i++) {
        const c = chasti[i];
        // Ключ со значением отдельным словом: «-p 2222», «-l root», а ещё
        // «-i key.pem» и «-J jump@…», чьё значение не адрес сервера.
        if (/^-[bcDEeFIiJLlmOoPpQRSWw]$/.test(c)) {
          const znachenie = chasti[++i];
          if (znachenie === undefined) break;
          if (c === "-p") prinyatPort(znachenie);
          else if (c === "-l") imyaKlyuchom = znachenie;
          continue;
        }
        const p = /^-p(\d+)$/.exec(c);
        if (p) {
          prinyatPort(p[1]);
          continue;
        }
        if (c.startsWith("-")) continue;
        const at = /^(?:\/\/)?([^@\s/]+)@(.+)$/.exec(c);
        if (!at) {
          goloeSlovo ??= adresIPort(c);
          continue;
        }
        prinyatImya(at[1]);
        prinyatAdres(adresIPort(at[2]));
      }
      // Адрес отдельным словом берётся только при имени ключом: иначе это
      // мог быть любой адрес из фразы.
      if (imyaKlyuchom !== undefined && POLZOVATEL.test(imyaKlyuchom)) {
        prinyatImya(imyaKlyuchom);
        prinyatAdres(goloeSlovo);
      }
    }

    const para = razdelit(stroka);
    const pole = para ? poleMetki(chistayaMetka(para[0])) : undefined;
    if (zhdyot !== undefined) {
      const byla = zhdyot;
      zhdyot = undefined;
      // Строка без своей метки и есть значение прошлой.
      if (pole === undefined && !chuzhaya) {
        prinyat(byla, stroka);
        continue;
      }
      // «Пароль:» и ниже «Pass:word» или «User:abc» одним словом: то ли
      // метка, то ли само значение, не гадать. С пробелом это метка:
      // «Данные сервера:», ниже «IP: …». Пустая метка («Логин:») ждёт своего.
      if (para && para[1].trim() !== "" && !/\s/.test(stroka)) continue;
    }
    if (!para || pole === undefined) continue;
    if (para[1].trim() === "") zhdyot = pole;
    else prinyat(pole, para[1]);
  }

  // «Your server 203.0.113.10 is ready.»: адрес без метки, если он в тексте
  // один. Он важнее подписанного имени узла: имя из письма хостера часто
  // выбрано при заказе и в DNS не записано. Чужие адреса (шлюз, DNS,
  // резолвер, частные) в запас не попадают.
  const bezMetkiOdin: Adres | undefined = bezMetki.size === 1 ? { adres: [...bezMetki][0] } : undefined;
  const vybran = vidy.v4 ?? bezMetkiOdin ?? vidy.imya ?? vidy.v6;
  if (vybran !== undefined) {
    itog.adres = vybran.adres;
    if (vybran.port !== undefined) itog.port ??= vybran.port;
  }
  return itog;
}
