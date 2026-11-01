import type { Dostup, Etap, Punkt, PodborPortov, SvoyServer } from "./SvoyVpn";

// Показательные данные шаблона. Адреса только из документационных сетей
// (203.0.113.0/24, RFC 5737): репозиторий публичный.

export const DOSTUP: Dostup = {
  soobshchenie: [
    "Your server is ready",
    "",
    "IP address: 203.0.113.10",
    "Username: root",
    "Password: primer-parolya",
    "SSH port: 22",
  ].join("\n"),
  adres: "203.0.113.10",
  portSsh: "22",
  polzovatel: "root",
  sposob: "parol",
  parol: "primer-parolya",
  klyuchFayl: "",
  parolKlyucha: "",
  portReality: "",
  portHy2: "",
};

export const OSMOTR: Punkt[] = [
  { imya: "Вход", itog: "da", podrobno: "по паролю, порт 22" },
  { imya: "Права", itog: "da", podrobno: "root" },
  { imya: "Путь", itog: "da", podrobno: "напрямую из твоей сети" },
  { imya: "Система", itog: "da", podrobno: "Ubuntu 24.04, KVM, 1 ГБ памяти" },
  { imya: "Место", itog: "da", podrobno: "свободно 18 ГБ" },
  { imya: "Порты", itog: "da", podrobno: "80 и 443 свободны" },
  { imya: "Чистота", itog: "da", podrobno: "чужих программ на портах нет" },
];

export const OSMOTR_OTKAZ: Punkt[] = [
  { imya: "Вход", itog: "da", podrobno: "по паролю, порт 22" },
  { imya: "Права", itog: "da", podrobno: "root" },
  { imya: "Путь", itog: "vnimanie", podrobno: "напрямую сервер молчит, вход через другой сервер" },
  { imya: "Система", itog: "net", podrobno: "Ubuntu 22.04, OpenVZ" },
  { imya: "Место", itog: "da", podrobno: "свободно 9 ГБ" },
  { imya: "Порты", itog: "zhdyot", podrobno: "не проверялись" },
  { imya: "Чистота", itog: "zhdyot", podrobno: "не проверялась" },
];

export const OTPECHATOK = "SHA256:q3Vt1mXo8bRkEw2hZpL9cJ4sNfA7yUeG0dTiKvBnM5w";

export const ETAPY: Etap[] = [
  { id: "programma", imya: "Программа сервера", itog: "da", podrobno: "поставлена, vpn-uzel 0.6.0" },
  { id: "sertifikat", imya: "Сертификат", itog: "da", podrobno: "выпущен на адрес 203.0.113.10" },
  { id: "porty", imya: "Подбор портов", itog: "idyot", podrobno: "по одному порту из твоей сети" },
  { id: "klyuchi", imya: "Проверка ключей", itog: "zhdyot", podrobno: "2 МБ через каждый ключ" },
];

export const ETAPY_OTKAZ: Etap[] = [
  { id: "programma", imya: "Программа сервера", itog: "da", podrobno: "поставлена, vpn-uzel 0.6.0" },
  { id: "sertifikat", imya: "Сертификат", itog: "da", podrobno: "выпущен на адрес 203.0.113.10" },
  { id: "porty", imya: "Подбор портов", itog: "net", podrobno: "из твоей сети не прошёл ни один" },
  { id: "klyuchi", imya: "Проверка ключей", itog: "zhdyot", podrobno: "не начиналась" },
];

export const PODBOR: PodborPortov = {
  reality: [
    { port: 443, itog: "ne-proshyol", prichina: "соединение молчит" },
    { port: 8443, itog: "probuyu" },
    { port: 2053, itog: "v-ocheredi" },
    { port: 9443, itog: "v-ocheredi" },
    { port: 465, itog: "v-ocheredi" },
    { port: 993, itog: "v-ocheredi" },
  ],
  hy2: [
    { port: 443, itog: "proshyol" },
  ],
  seychas: "443 по TCP не прошёл: соединение молчит. Пробую 8443",
};

export const PODBOR_OTKAZ: PodborPortov = {
  reality: [443, 8443, 2053, 9443, 465, 993].map((port) => ({ port, itog: "ne-proshyol" as const, prichina: "соединение молчит" })),
  hy2: [443, 8443, 11444, 5443, 2053].map((port) => ({ port, itog: "ne-proshyol" as const, prichina: "ответа нет" })),
  seychas: "Каждый порт проверен трижды: сразу, через 2 и через 10 минут",
};

const KLYUCHI = [
  { imya: "reality", proto: "TCP", port: 8443, proverka: "проверен: 2 МБ за 1,1 с" },
  { imya: "hysteria2", proto: "UDP", port: 443, proverka: "проверен: 2 МБ за 0,8 с" },
] as const;

export const SERVER: SvoyServer = {
  adres: "203.0.113.10",
  sostoyanie: "horosho",
  klyuchi: [...KLYUCHI],
  sertifikatDo: "7 ноября",
  trafik: "41,2 ГБ",
  vhodPoParolyu: true,
  programma: "vpn-uzel 0.6.0",
  provereno: "5 минут назад",
};

export const SERVER_VNIMANIE: SvoyServer = {
  ...SERVER,
  sostoyanie: "vnimanie",
  prichina: "Сертификат не продлился: порт 80 занят другой программой",
  sovet: "Ключи работают ещё 3 дня. Освободи 80 на сервере и нажми «Проверить»",
};

export const SERVER_SLOMANO: SvoyServer = {
  ...SERVER,
  sostoyanie: "slomano",
  prichina: "Твоя сеть не пускает к адресу сервера",
  sovet: "Напрямую сервер молчит на всех портах, а через другой сервер отвечает. Попроси у хостера другой адрес",
  klyuchi: KLYUCHI.map((k) => ({ ...k, proverka: "напрямую из твоей сети не открылся" })),
};

export const SERVER_BEZ_KLYUCHA: SvoyServer = { ...SERVER, bezKlyuchaUpravleniya: true };

// VPN при этом может работать, но менять сервер нельзя: отсюда «нужно
// внимание», а не «работает» рядом с красной плашкой.
export const SERVER_KLYUCH_HOSTA: SvoyServer = { ...SERVER, sostoyanie: "vnimanie", klyuchHostaSmenilsya: true };
