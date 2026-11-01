// Mirror of internal/protokol/kadr.go, kept by hand. Generated bindings would
// tie the screens to the shell's type layout; this file ties them to the wire.
// When kadr.go changes, this changes with it, and Glavnyy.test.tsx lists the
// seven states verbatim so a drift here fails a test, not a user.

/** protokol.Versiya. The bridge already says hello with it on connect; the
 *  screen repeats hello to learn which commands are deferred (`pozzhe`). */
export const VERSIYA_PROTOKOLA = 1;

export type Sostoyanie =
  | "sluzhba-molchit"
  | "vyklyuchen"
  | "podnimaetsya"
  | "podnyat"
  | "ne-neset"
  | "vosstanavlivaetsya"
  | "otkaz";

export type Rezhim = "avto" | "ruchnoy";

export interface Oshibka {
  kod: string;
  tekst: string;
}

/** A refusal as a SCREEN receives it. Narrower than Oshibka on purpose: a
 *  tab may know the code and nothing else (the list simply did not read),
 *  and §9.1 names the code, so the text is optional here and required on
 *  the wire. */
export interface OtkazNaEkrane {
  kod: string;
  tekst?: string;
}

/** ssylki.OtkazStroki: одна строка подписки, которая не разобралась. Номер
 *  строки и причина, и НИКОГДА сама ссылка: в ней uuid и ключи, а служба уже
 *  вычищает их из текста отказа (razbor.go, bezSsylki). */
export interface OtkazStroki {
  stroka: number;
  prichina: string;
}

/** protokol.StatusOtvet. Everything but `sostoyanie` is optional on the
 *  screen side: an older service omits fields, and the screen must still
 *  render rather than crash on a missing key. */
export interface StatusOtvet {
	trafik_po_umolchaniyu?: "vpn" | "direct";
  sostoyanie: Sostoyanie;
  vybran_id?: string;
  nesushchiy_id?: string;
  /** Name of the carrier, sent next to its id (kadr.go NesushchiyImya). In
   *  auto mode the carrier changes without any command from the human, so
   *  the frame carries the name and the tray already reads it; the screen
   *  used to ignore the field and fall back to the bare id. */
  nesushchiy_imya?: string;
  /** Name of the chosen server that stopped answering while the core carries
   *  another one in its place (kadr.go VremennoVmesto, manual mode). */
  vremenno_vmesto?: string;
  rezhim_marshruta?: Rezhim;
  kill_switch?: boolean;
  avtozapusk?: boolean;
  /** Second decision, separate from autostart (§9.2 defaults differ). */
  podklyuchat_pri_starte?: boolean;
  zhurnal?: boolean;
  diagnostika?: boolean;
  /** Local mixed proxy port next to the tunnel; absent when it is not up. */
  port_proksi?: number;
  /** Полоса канала в мегабитах, объявленная человеком. Отсутствие означает «не
   *  измерена»: ядро тогда считает полосу само (BBR), и это рабочее состояние,
   *  а не пустое поле. */
  polosa_vverh?: number;
  polosa_vniz?: number;
  versiya_sluzhby?: string;
  podnyat_s?: string;
  /** Когда восстановление попробует снова (kadr.go SledPopytka, с 1.9.1). Есть
   *  только на паузе между попытками; во время самой попытки поля нет. */
  sled_popytka?: string;
  /** Release build version; absent on dev builds. */
  versiya_programmy?: string;
  /** A newer version seen on the update server (service checks daily). */
  obnovlenie?: Obnovlenie;
  obnovlenie_provereno?: string;
  /** Чем кончилась последняя проверка, если не удалась. Пусто значит «удалась».
   *  Отметка выше ставится только при удаче, и без этой причины мёртвая
   *  проверка выглядела на экране здоровой (23.09.2026). */
  obnovlenie_otkaz?: string;
  oshibka?: Oshibka;
  /** Список блокировки рекламы (28.09.2026). Нет поля значит «списка нет и
   *  отказов не было», в том числе у службы прошлой версии. */
  reklama?: ReklamaSostoyanie;
  /** Чужие программы, которые ломают VPN прямо сейчас (kadr.go Pomehi,
   *  1.10.0): перехватчик пакетов или чужой VPN с маршрутом по умолчанию.
   *  Антивирусов здесь нет, их служба называет в отказе. */
  pomehi?: Pomeha[];
  /** На сколько секунд часы компьютера впереди точного времени, отрицательное
   *  значит «отстают» (kadr.go ChasyRashozhdenie, 1.10.0). Поле есть только
   *  при расхождении больше двух минут. */
  chasy_rashozhdenie?: number;
}

/** protokol.Pomeha. vid выбирает фразу: obhod просят выключить, vpn
 *  отключить. Незнакомый vid от службы новее говорит общей фразой. */
export interface Pomeha {
  imya: string;
  vid: string;
}

/** protokol.ReklamaSostoyanie: список, который ЛЕЖИТ В ФАЙЛЕ, а не выбранный
 *  человеком. Пока новый уровень готовится, они расходятся. */
export interface ReklamaSostoyanie {
  uroven: string;
  pravil: number;
  versiya?: string;
  sobran?: string;
  proveren?: string;
  vstroennyy?: boolean;
  /** Всегда в кадре: false значит «конфиг живого ядра блока не несёт». */
  deystvuet: boolean;
  /** Голая причина последнего отказа, фразу вокруг строит окно. */
  otkaz?: string;
}

/** Live numbers under the main object (§8.3). `null` on the screen means
 *  "no data", and the fallback contract forbids drawing that as zero. */
export interface Statistika {
  adres_vyhoda: string;
  /** Absent until the core has a measurement; never zero for "unknown". */
  zaderzhka_ms?: number;
  otdano?: number;
  prinyato?: number;
}

/** One line of checkLeaks. `ne_vidim` is the honest answer for what the
 *  service cannot observe (browser DoH, packets on the wire). */
export interface PunktProverki {
  imya: string;
  itog: "ok" | "utechka" | "ne_izmereno" | "ne_vidim";
  tekst: string;
}

export interface RezultatProverki {
  punkty: PunktProverki[];
  adres_vyhoda?: string;
}

/** Один слой проверки сети (A3). Подпись приходит от службы: слой знает про
 *  себя больше, чем окно, и переводить его обратно в человеческие слова тут
 *  значило бы держать второй словарь, который разойдётся с первым. */
export interface SloyProverkiSeti {
  vid: string;
  podpis: string;
  proshlo: boolean;
  podrobno: string;
  ms: number;
}
export interface ProverkaSeti {
  vremya: string;
  sloi: SloyProverkiSeti[];
}

/** protokol.Server as the service projects it for the screen (dlyaEkrana):
 *  identity, transport, address and the two marks. Keys never travel. */
export interface Server {
  id: string;
  imya: string;
  transport: string;
  host: string;
  port: number;
  iz_podpiski: boolean;
  /** Пропал из подписки, но ядро ещё держит его кандидатом: уйдёт сам после переподключения. */
  uderzhan?: boolean;
  /** Ключ своего сервера из раздела «Свой VPN». */
  svoy?: boolean;
  nebezopasnyy_ignorirovan?: boolean;
  /** В ссылке был отпечаток сертификата, а ядро сверяет только ключ (П4 аудита 1.6.1). */
  otpechatok_ne_proveryaetsya?: boolean;
  /** hy2 with pinSHA256: the pin stays in the service, only the mark travels. */
  s_pinom?: boolean;
  /** Человек убрал сервер из автовыбора (A5). Выбрать его руками по-прежнему
   *  можно: из группы автомата он ушёл, из списка нет. */
  vne_avto?: boolean;
}

export interface Obnovlenie {
  versiya: string;
  razmer: number;
  provereno: string;
}

/** Шаги события obnovlenie-hod. Ответ на downloadUpdate приходит ОДИН и в самом
 *  конце, а между нажатием и им лежат два десятка мегабайт и подмена файлов.
 *  Без этих шагов окно всю минуту показывало ровно ничего. */
export type ShagObnovleniya = "skachivanie" | "sverka" | "raspakovka" | "podmena" | "otkaz";

export interface HodObnovleniya {
  shag: ShagObnovleniya;
  /** Номер выпуска, ради которого всё затеяно. Своя версия в это время уже
   *  ничего не значит, и подписывать полосу надо этим. */
  versiya?: string;
  skachano?: number;
  vsego?: number;
  /** Сколько секунд служба даёт новой версии на ответ, прежде чем откатиться. */
  srok_s?: number;
  tekst?: string;
}

/** getServerHealth (wave 6.4): the VPS snapshot found next to the subscription. */
export interface Zdorovie {
  vremya: number;
  vozrast_s: number;
  ustarel: boolean;
  trevogi: string[];
  dney_do_konca_sertifikata_maski?: number;
  dney_s_obnovleniya_xray?: number;
  xray_versiya?: string;
  hy2_aktiven?: boolean;
  /** Let's Encrypt on a bare IP lives 6 days; absent when the server runs no hy2. */
  dney_do_konca_sertifikata_hy2?: number;
}
