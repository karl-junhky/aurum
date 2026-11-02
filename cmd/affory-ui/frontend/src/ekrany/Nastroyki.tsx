import { useEffect, useRef, useState } from "react";
import type { HodObnovleniya, OtkazNaEkrane, ProverkaSeti, RezultatProverki, ShagObnovleniya, Sostoyanie, StatusOtvet } from "../protokol";
import { otlozhenaDo } from "./Pravila";
import { Udalenie } from "./Udalenie";
import { Gruppa, Knopka, Neudacha, Panel, Pole, Polosa, Ryad, RyadRazdela, Tumbler } from "./ui";
import {
  IkArhiv, IkDiagnostika, IkMonitor, IkObnovit,
  IkProfil, IkProksi, IkPusk, IkShchit, IkStrelkaVpravo,
} from "../ikonki";
import { obyom } from "./Glavnyy";
import { Zhurnaly } from "./Zhurnaly";
import { prichinaPryamogoTrafika, type PravilaTrafika } from "../trafik";

// Settings tab. 4.8 brought the two launch switches, 4.7 the uninstall, 4.11
// the rest: the §9.2 defaults shown rather than implied, the "all traffic"
// mode with its warning, the local proxy port, and the three deferred
// checks (leaks, exit address, update) disabled with the wave from hello.
// Rare settings live here in collapsible sections; there is no second page,
// потому что страница, которую надо искать, это страница, которой не
// пользуются.

export interface NastroykiProps {
  trafik?: PravilaTrafika | null;
  estChernovikPravil?: boolean;
  naPravila?: () => void;
  obnovitPravila?: () => void;
  naPapkuZhurnalov?: () => Promise<void>;
  /** Сохранение диагностики файлом, итог строкой, пустая при отмене. */
  naDiagnostiku?: () => Promise<string>;
  status: StatusOtvet;
  /** Deferred commands with wave numbers, from hello. `null` until it answers. */
  otlozheno: Record<string, number> | null;
  /** Sends a service command; the answer folds into `status` upstream. */
  naKomandu: (komanda: string, telo: unknown) => void;
  naUdalenie: (steretKlyuchi: boolean) => void;
  /** Whether the service answers at all. Absent means "read it off the
   *  status", which is what the screen did before the shell told it. */
  svyaz?: "est" | "net";
  /** Reconnects to the service. The one action a screen without a service
   *  can still offer. */
  povtorit?: () => void;
  /** Last checkLeaks answer; `null` until the button is pressed. */
  proverka?: RezultatProverki | null;
  /** Why the last check brought nothing. App used to write `proverka` only
   *  on success, so a failed check left the PREVIOUS one on screen as if it
   *  were current (03.09.2026). */
  proverkaOtkaz?: OtkazNaEkrane | null;
  /** Слои сети из последней проверки (A3). null до первого нажатия. */
  proverkaSeti?: ProverkaSeti | null;
  /** Writes the whole set to a file, and reads it back. Administrator only
   *  (owner 03.09.2026: secrets leaving the machine stay behind the UAC).
   *  The PASSWORD comes from here, the path from the shell's own dialog: the
   *  screen is not allowed near the bridge, and the bridge never sees the
   *  password. */
  vyvestiProfil?: (parol: string) => void;
  vvestiProfil?: (parol: string) => void;
  /** Outcome of the last attempt in the person's words. */
  itogProfilya?: string | null;
  /** Wave 6.5: opens the native archive dialog; the path goes to installUpdate from App. */
  naObnovlenie?: () => void;
  /** Last checkExitIp answer. Shown in the row: a press must change something on screen. */
  adresVyhoda?: AdresVyhoda | null;
  /** Последний замер полосы. Кнопка тратит настоящий трафик человека, и
   *  молчание после неё читается как «ничего не делает». */
  zamerPolosy?: ZamerPolosy | null;
  /** Шаг идущего обновления. `null` значит «не идёт», и это не то же самое,
   *  что «идёт, но неизвестно что»: пустая карточка во время загрузки и была
   *  жалобой 13.09.2026. */
  hodObnovleniya?: HodObnovleniya | null;
  /** Растёт, когда трей просит показать обновление. Числом, а не флагом:
   *  второе нажатие обязано подсветить карточку снова. */
  vestiKObnovleniyu?: number;
  /** Идёт ли длинная команда прямо сейчас: замер полосы, проверка утечек,
   *  проверка адреса, проверка версии. Кнопка без вертушки читается как
   *  зависшая программа, и человек жмёт её второй раз. */
  zanyatyeKomandy?: Record<string, boolean>;
}

/** Замер полосы, обе стороны. Число и его отсутствие здесь разные вещи:
 *  `null` значит «не измерено», а ноль значил бы «канала нет» и уехал бы в
 *  объявление, где ноль это Brutal с нулевой оценкой канала. */
export interface ZamerPolosy {
  mbitVniz: number | null;
  sovetVniz: number | null;
  mbitVverh: number | null;
  sovetVverh: number | null;
  otkazVverh: string;
  cherezTunnel: boolean;
  vremya: string;
}

/** Мбит/с одним числом. Десятые оставлены: разница между 87 и 87.4 не важна,
 *  а между 0.4 и 0 важна очень. */
function mbit(v: number | null): string {
  return v === null ? "не измерено" : `${v.toFixed(1)} Мбит/с`;
}

export interface AdresVyhoda {
  adres: string;
  cherez: "tunnel" | "napryamuyu";
  vremya: string;
}

function tekstAdresa(a: AdresVyhoda, podnyat: boolean): string {
  if (a.cherez !== "tunnel") return `напрямую, VPN не подключён: ${a.adres}, ${a.vremya}`;
  // The measurement was true when it was taken. After a disconnect the line
  // "через туннель" is a claim about NOW, and it is a false one (03.09.2026).
  return podnyat
    ? `через VPN: ${a.adres}, ${a.vremya}`
    : `измерено в ${a.vremya}, VPN с тех пор отключён: ${a.adres}`;
}

/** "12:03" for the moment an answer landed. The service does not stamp
 *  checkLeaks, and the screen must not pass a stale report off as fresh. */
function chasyMinuty(): string {
  return new Date().toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
}

const ITOG: Record<string, string> = {
  ok: "в порядке",
  utechka: "утечка",
  ne_izmereno: "не измерено",
  ne_vidim: "не проверяется",
};

type Razdel = "hysteria" | "diagnostika" | "profil";

export function Nastroyki({
  trafik, estChernovikPravil=false, naPravila, obnovitPravila,
  status, otlozheno, svyaz, povtorit, naKomandu, naUdalenie,
  proverka = null, proverkaOtkaz = null, proverkaSeti = null, naObnovlenie, adresVyhoda = null, zamerPolosy = null,
  vyvestiProfil, vvestiProfil, itogProfilya = null, zanyatyeKomandy = {},
  hodObnovleniya = null, vestiKObnovleniyu = 0, naPapkuZhurnalov, naDiagnostiku,
}: NastroykiProps) {
  // The password lives exactly as long as this screen does, goes into the body
  // of one command and nowhere else: not a file name, not an argument, not a
  // log line. It is deliberately NOT lifted to App.
  const [parolProfilya, zadatParolProfilya] = useState("");
  // Обновление идёт, пока последний шаг не отказ: отказ гасит полосу, а его
  // причина приезжает отдельным отказом команды, как у всех прочих кнопок.
  const idyot = hodObnovleniya !== null && hodObnovleniya.shag !== "otkaz";
  const ostalosPodmeny = otschetPodmeny(hodObnovleniya);
  const ryadObnovleniya = useRef<HTMLDivElement>(null);
  // Трей привёл человека сюда: карточку надо не просто показать, а показать
  // так, чтобы он её нашёл. Прокрутка и подсветка на две секунды.
  useEffect(() => {
    if (!vestiKObnovleniyu) return;
    const uzel = ryadObnovleniya.current;
    if (!uzel) return;
    uzel.scrollIntoView({ block: "center", behavior: "smooth" });
    uzel.classList.add("af-privlech");
    const t = setTimeout(() => uzel.classList.remove("af-privlech"), 2000);
    return () => clearTimeout(t);
  }, [vestiKObnovleniyu]);
  const [dialog, zadatDialog] = useState(false);
  // The mode question waits for an answer: switching re-raises the tunnel
  // and drops every connection, which is not what a toggle usually does.
  const [vopros, zadatVopros] = useState<boolean | null>(null);
  // Разделы открываются независимо друг от друга: аккордеон, который
  // закрывает предыдущий, заставляет человека открывать один и тот же раздел
  // по второму разу, стоит ему заглянуть в соседний.
  const [otkryty, zadatOtkryty] = useState<Razdel[]>([]);
  // Полоса живёт строками, а не числами: пустое поле это «не введено», а не
  // ноль, и ноль отсюда уходит в службу только по кнопке «Снять».
  const [vverh, zadatVverh] = useState("");
  const [vniz, zadatVniz] = useState("");
  const [mishen, zadatMishen] = useState("");
  const [mishenVverh, zadatMishenVverh] = useState("");
  const objavlena = (status.polosa_vverh ?? 0) > 0 && (status.polosa_vniz ?? 0) > 0;
  // Пара или ничего: Brutal управляет обоими направлениями сразу, и одно число
  // означало бы объявленный ноль во втором.
  const paraGodna = Number(vverh) > 0 && Number(vniz) > 0;
  const svyazNet = svyaz !== undefined ? svyaz === "net" : status.sostoyanie === "sluzhba-molchit";
  const aktiven = !svyazNet;
  const zhdyomHello = otlozheno === null;
  // Every grey button says why it is grey. The three checks used to go grey
  // in silence while the explanation beside them showed its usual text.
  const pochemuSero = (svoya: string | undefined): string | undefined =>
    svyazNet ? "служба не отвечает" : zhdyomHello ? "служба ещё не ответила" : svoya;
  const mozhnoZvat = !svyazNet && !zhdyomHello;
  const podnyat = status.sostoyanie === "podnyat";
  const killSwitch = status.kill_switch ?? false;
  const nahodka = status.obnovlenie ?? null;
  // Прямые маршруты включению защиты больше не мешают (23.09.2026): под
  // защитой они не применяются, а набор человека остаётся как есть. Осталась
  // ровно одна помеха - незавершённый черновик правил: применять его придётся
  // всё равно, и делать это посреди смены режима нельзя.
  const pochemuNelzyaVklyuchit=estChernovikPravil?"Сначала примени или отмени черновик правил":null;
  // Что именно уснёт под защитой. Не запрет, а предупреждение до нажатия.
  const usnutPodZashchitoy=trafik?prichinaPryamogoTrafika(trafik):null;
  // Disabling protection stays available even when rule inspection failed.
  const rezhimZanyat=zanyatyeKomandy.setKillSwitch===true || zanyatyeKomandy.setRules===true;
  const mozhnoRezhim = mozhnoZvat && !rezhimZanyat && (killSwitch || !pochemuNelzyaVklyuchit);
  // The screen remembers WHICH tunnel state produced the answer it shows: a
  // leak report taken over a raised tunnel says nothing about a dropped one.
  const [snyato, zadatSnyato] = useState<{ sostoyanie: Sostoyanie; vremya: string } | null>(null);
  useEffect(() => {
    if (proverka) zadatSnyato({ sostoyanie: status.sostoyanie, vremya: chasyMinuty() });
    else zadatSnyato(null);
    // The tunnel state is captured at the moment of the answer on purpose:
    // adding it to the deps would re-stamp a stale report as fresh.

  }, [proverka]);
  const ustarela = proverka !== null && snyato !== null && snyato.sostoyanie !== status.sostoyanie;
  const pokazatPunkty = proverka !== null && !proverkaOtkaz && !ustarela;

  const utechki = otlozhenaDo(otlozheno, "checkLeaks");
  const adresOtlozhen = otlozhenaDo(otlozheno, "checkExitIp");
  const obnovlenie = otlozhenaDo(otlozheno, "installUpdate");
  const zhdyot = (k: string) => zanyatyeKomandy[k] === true;
  const perekluchit = (r: Razdel) =>
    zadatOtkryty(otkryty.includes(r) ? otkryty.filter((x) => x !== r) : [...otkryty, r]);

  return (
    <section aria-label="Настройки" className="mx-auto flex w-full max-w-[940px] flex-col gap-7 px-8 py-7">
      {/* No summary line: versiya_sluzhby is the protocol number, and "служба 1"
          under the title told the human nothing (owner, 02.09.2026). */}
      <header className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-foreground text-[26px] font-semibold leading-tight">Настройки</h2>
          <p className="text-fg-muted mt-1.5 text-sm">
            {svyazNet ? "Служба не отвечает, настройки только для чтения" : "Настрой поведение программы под свои задачи"}
          </p>
        </div>
        {svyazNet && povtorit && (
          <Knopka rang="vtoraya" bolshaya testId="povtorit-svyaz" onClick={povtorit}>Повторить</Knopka>
        )}
      </header>

      <Gruppa nazvanie="Запуск" poyasnenie="Автозапуск и автоматическое подключение">
        <Panel>
          <Ryad
            znachok={<IkMonitor className="h-[18px] w-[18px]" />}
            nazvanie="Запускать при входе в Windows"
            poyasnenie="Программа поднимается в трее, окно не открывается"
            aktiven={aktiven}
          >
            <Tumbler
              testId="avtozapusk"
              podpis="запускать при входе в Windows"
              vkl={status.avtozapusk ?? false}
              aktiven={aktiven}
              naSmenu={(vkl) => naKomandu("setAutostart", { vkl })}
            />
          </Ryad>
          <Ryad
            znachok={<IkPusk className="h-[18px] w-[18px]" />}
            nazvanie="Подключаться при старте"
            poyasnenie="VPN поднимает служба, до входа в систему"
            aktiven={aktiven}
          >
            <Tumbler
              testId="pri-starte"
              podpis="подключаться при старте"
              vkl={status.podklyuchat_pri_starte ?? false}
              aktiven={aktiven}
              naSmenu={(vkl) => naKomandu("setConnectOnStart", { vkl })}
            />
          </Ryad>
        </Panel>
      </Gruppa>

      <Gruppa nazvanie="Защита" poyasnenie="Безопасность и стабильность соединения">
        {vopros !== null ? (
          <section role="dialog" aria-label="Смена режима" className="border-border bg-surface flex flex-col gap-3 rounded-xl border p-5">
            <h4 className="text-foreground text-sm font-semibold">
              {vopros ? "Включить блокировку сети при обрыве VPN" : "Выключить блокировку сети при обрыве VPN"}
            </h4>
            <p className="text-fg-secondary text-[13px] leading-relaxed">
              VPN переподключится под новый режим, все соединения разорвутся и поднимутся заново.
              Загрузки и звонки оборвутся
            </p>
            {vopros && pochemuNelzyaVklyuchit && <p role="alert" className="text-warn text-[13px]">{pochemuNelzyaVklyuchit}. Включение защиты недоступно</p>}
            {vopros && !pochemuNelzyaVklyuchit && usnutPodZashchitoy && <p className="text-fg-secondary text-[13px] leading-relaxed" data-testid="usnut-pravila">{usnutPodZashchitoy}. Пока защита включена, они не действуют - весь трафик идёт через VPN. Правила остаются на месте и заработают снова, когда выключишь защиту</p>}
            <div className="flex gap-2">
              {/* За кнопкой стоит расстановка правил брандмауэра, а это
                  секунды, а не мгновение. */}
              <Knopka rang="glavnaya" bolshaya testId="podtverdit-rezhim" zhdyot={zhdyot("setKillSwitch")}
                      aktiven={mozhnoZvat && !rezhimZanyat && !(vopros && !!pochemuNelzyaVklyuchit)}
                      onClick={() => { if(!mozhnoZvat || rezhimZanyat || (vopros && pochemuNelzyaVklyuchit))return; naKomandu("setKillSwitch", { vkl: vopros }); zadatVopros(null); }}>
                {vopros ? "Включить" : "Выключить"}
              </Knopka>
              <Knopka rang="tekst" bolshaya testId="otmena-rezhima" onClick={() => zadatVopros(null)}>Отмена</Knopka>
            </div>
          </section>
        ) : (
          <Panel>
            <Ryad
              znachok={<IkShchit className="h-[18px] w-[18px]" />}
              testId="ves-trafik-ryad"
              nazvanie="Блокировать сеть при обрыве VPN"
              poyasnenie={
                // Состав прямых правил назван и ДО включения: при выключенном
                // VPN подтверждения нет вовсе, и человек узнал бы об уснувших
                // правилах только по их молчанию.
                !killSwitch && pochemuNelzyaVklyuchit ? pochemuNelzyaVklyuchit
                  : usnutPodZashchitoy ? `${usnutPodZashchitoy}. ${killSwitch ? "Сейчас они не действуют: всё идёт через VPN" : "Под защитой они не будут действовать"}`
                  : "Действует во время подключения, после отключения сеть освобождается"
              }
              aktiven={mozhnoRezhim}
            >
              <Tumbler
                testId="ves-trafik"
                podpis="весь трафик только через VPN"
                vkl={killSwitch}
                aktiven={mozhnoRezhim}
                naSmenu={(vkl) => { if(!mozhnoZvat || rezhimZanyat || (vkl && pochemuNelzyaVklyuchit))return; if (podnyat) zadatVopros(vkl); else naKomandu("setKillSwitch", { vkl }); }}
              />
            </Ryad>
            <Ryad
              znachok={<IkProksi className="h-[18px] w-[18px]" />}
              testId="proksi"
              nazvanie="Локальный прокси"
              poyasnenie="Адрес для программ, которые ходят через прокси сами"
              aktiven={aktiven}
            >
              {/* Адрес выделяется целиком по щелчку и копируется клавишами.
                  Своя кнопка «Скопировать» здесь была лишней: она умела ровно
                  то же, что Ctrl+C, и занимала место в строке (владелец,
                  16.09.2026). */}
              <span className="text-fg-secondary select-all text-[13px]">
                {status.port_proksi
                  ? `127.0.0.1:${status.port_proksi} · HTTP и SOCKS`
                  : podnyat
                    ? "не поднят: порт занят другой программой, VPN это не задевает"
                    : "поднимается вместе с VPN"}
              </span>
            </Ryad>
          </Panel>
        )}
        {/* Карточкой, как всё вокруг: голый абзац под панелью читался выпавшим
            из вёрстки, а кнопка под ним стояла сиротой. Остался один случай -
            незавершённый черновик правил; прямые маршруты защите не мешают. */}
        {!killSwitch && !trafik && <div className="border-border bg-surface flex flex-col gap-2 rounded-xl border px-4 py-3 text-[13px]">
          {/* Про черновик говорит сам ряд защиты: два одинаковых объяснения на
              одном экране человек читает как две разные помехи. */}
          <p className="text-fg-secondary leading-relaxed">Правила ещё не прочитаны. Обнови их, чтобы увидеть, что изменится под защитой</p>
          <div className="flex flex-wrap gap-2">
            {naPravila && <Knopka rang="tekst" onClick={naPravila}>Открыть правила</Knopka>}
            {!trafik && obnovitPravila && <Knopka rang="vtoraya" aktiven={mozhnoZvat} onClick={obnovitPravila}>Обновить правила</Knopka>}
          </div>
        </div>}

        {/* Раскрывающиеся разделы отделены линией от карточек выше: без неё
            голые строки прилипали к панели и читались её продолжением.
            Своей карточки им не дать - внутри уже есть панель, а карточка в
            карточке это разнобой похуже. */}
        <div className="border-border mt-3 flex flex-col border-t pt-1">
          {/* Полоса канала. Нужна ровно одному протоколу, hysteria2, у которого
              объявление полосы и ЕСТЬ переключатель Brutal: отдельного флага нет.
              Поэтому здесь не «ограничить скорость», а «сказать протоколу, какой
              канал он делит».

              Источников два. Ссылка сервера может нести upmbps и downmbps сама,
              и наша подписка так и делает для входа hy2-brutal. Эти поля её
              перебивают: ссылку пишет держатель сервера, а тут человек говорит
              про СВОЙ домашний канал, которым Brutal и управляет. Пустые поля
              значат «взять из ссылки», а не «BBR». */}
          <RyadRazdela
            testId="razdel-hysteria"
            znachok={<IkStrelkaVpravo className="h-4 w-4" />}
            nazvanie="Параметры Hysteria2 и ручной замер"
            poyasnenie="Объявленная полоса канала и проверка скорости"
            otkryt={otkryty.includes("hysteria")}
            naZhmyh={() => perekluchit("hysteria")}
            deti={
              <Panel testId="polosa">
                <Ryad
                  nazvanie="Полоса канала"
                  poyasnenie={
                    objavlena
                      ? `${status.polosa_vverh} вверх · ${status.polosa_vniz} вниз, Мбит · hysteria2 держит эту скорость`
                      : "Автоматически из ссылки сервера. С ручными значениями hysteria2 держит заданную скорость, даже когда сеть теряет данные; завышенные могут ухудшить соединение."
                  }
                  aktiven={aktiven}
                  lomat
                >
                  <div className="flex items-center gap-2">
                    <Pole
                      testId="polosa-vverh"
                      tip="text"
                      znachenie={vverh}
                      naVvod={(z) => zadatVverh(z.replace(/[^0-9]/g, ""))}
                      placeholder="вверх"
                      aktiven={aktiven}
                      aria-label="полоса вверх, Мбит"
                      className="w-24"
                    />
                    <Pole
                      testId="polosa-vniz"
                      tip="text"
                      znachenie={vniz}
                      naVvod={(z) => zadatVniz(z.replace(/[^0-9]/g, ""))}
                      placeholder="вниз"
                      aktiven={aktiven}
                      aria-label="полоса вниз, Мбит"
                      className="w-24"
                    />
                    <Knopka
                      rang="vtoraya"
                      testId="sohranit-polosu"
                      zhdyot={zhdyot("setBandwidth")}
                      aktiven={aktiven && paraGodna}
                      onClick={() => naKomandu("setBandwidth", { vverh: Number(vverh), vniz: Number(vniz) })}
                    >
                      Сохранить
                    </Knopka>
                    {objavlena && (
                      <Knopka
                        rang="vtoraya"
                        testId="snyat-polosu"
                        zhdyot={zhdyot("setBandwidth")}
                        aktiven={aktiven}
                        onClick={() => {
                          zadatVverh("");
                          zadatVniz("");
                          naKomandu("setBandwidth", { vverh: 0, vniz: 0 });
                        }}
                      >
                        Снять
                      </Knopka>
                    )}
                  </div>
                </Ryad>
                {/* Замер вместо ввода. Число для Brutal обязано быть измерением: он
                    шлёт ровно с объявленной скоростью, и цену завышения платит канал
                    человека, который своей полосы не знает.

                    Мишень задаётся ЯВНО и умолчания не имеет. Клиент не ходит
                    самовольно на чужой хост и не тратит трафик мобильного тарифа без
                    спроса: минута скачивания это десятки мегабайт. */}
                <Ryad
                  nazvanie="Измерить полосу"
                  poyasnenie={
                    zhdyot("measureBandwidth")
                      ? "Идёт замер: качаю файл и засекаю время, это десятки мегабайт и до полуминуты"
                      : "Для собственного сервера измерений. Обычная страница сайта не принимает тестовую загрузку."
                  }
                  aktiven={aktiven}
                  lomat
                >
                  <div className="flex items-center gap-2">
                    <Pole
                      testId="polosa-mishen"
                      tip="text"
                      znachenie={mishen}
                      naVvod={zadatMishen}
                      placeholder="https://адрес/большой-файл"
                      aktiven={aktiven}
                      aria-label="адрес файла для замера приёма"
                      className="w-56"
                    />
                    {/* Вторая мишень отдельная, потому что направления живут по
                        разным адресам: у speed.cloudflare.com это __down и __up.
                        Пустое поле значит «отдачу не мерим», и замер приёма всё
                        равно состоится. */}
                    <Pole
                      testId="polosa-mishen-vverh"
                      tip="text"
                      znachenie={mishenVverh}
                      naVvod={zadatMishenVverh}
                      placeholder="https://адрес/приём-заливки"
                      aktiven={aktiven}
                      aria-label="адрес для замера отдачи"
                      className="w-56"
                    />
                    <Knopka
                      rang="vtoraya"
                      testId="izmerit-polosu"
                      zhdyot={zhdyot("measureBandwidth")}
                      aktiven={aktiven && mishen.trim() !== ""}
                      onClick={() => {
                        const adres = mishen.trim();
                        if (adres === "") return;
                        naKomandu("measureBandwidth", {
                          adres, adres_vverh: mishenVverh.trim(), potokov: 4, sekund: 10,
                        });
                      }}
                    >
                      {zhdyot("measureBandwidth") ? "Меряю" : "Измерить"}
                    </Knopka>
                  </div>
                </Ryad>
                {/* Результат прямо под кнопкой. Замер тратит десятки мегабайт, и
                    молчание после него человек читает как сломанную кнопку, а
                    значит жмёт ещё раз (решено 03.09.2026). */}
                {zamerPolosy && (
                  <div className="border-border border-t p-4" data-testid="zamer-polosy">
                    <p className="text-sm">
                      приём {mbit(zamerPolosy.mbitVniz)} · отдача {mbit(zamerPolosy.mbitVverh)}
                      {" · "}
                      {zamerPolosy.cherezTunnel ? "через VPN" : "напрямую, VPN не подключён"}
                      {" · "}
                      {zamerPolosy.vremya}
                    </p>
                    {zamerPolosy.otkazVverh !== "" && (
                      <p className="text-fg-muted mt-1 text-xs" data-testid="zamer-otkaz-vverh">
                        отдача не измерена: {zamerPolosy.otkazVverh}
                      </p>
                    )}
                    <div className="mt-3 flex items-center gap-2">
                      {/* Объявляется СОВЕТ, а не замер. Запас вниз и есть весь смысл:
                          Brutal шлёт ровно с объявленной скоростью, и завышение
                          стоило на стенде 10% полосы и 30% задержки. */}
                      <Knopka
                        rang="vtoraya"
                        testId="obyavit-polosu"
                        zhdyot={zhdyot("setBandwidth")}
                        aktiven={aktiven && zamerPolosy.sovetVniz !== null && zamerPolosy.sovetVverh !== null}
                        onClick={() => {
                          if (zamerPolosy.sovetVniz === null || zamerPolosy.sovetVverh === null) return;
                          zadatVverh(String(zamerPolosy.sovetVverh));
                          zadatVniz(String(zamerPolosy.sovetVniz));
                          naKomandu("setBandwidth", { vverh: zamerPolosy.sovetVverh, vniz: zamerPolosy.sovetVniz });
                        }}
                      >
                        {zamerPolosy.sovetVverh !== null && zamerPolosy.sovetVniz !== null ? `Применить ${zamerPolosy.sovetVverh} / ${zamerPolosy.sovetVniz} Мбит/с к Hysteria2` : "Применить к Hysteria2"}
                      </Knopka>
                      {zamerPolosy.sovetVverh === null && (
                        <span className="text-fg-muted text-xs">
                          Для применения нужен успешный замер в обе стороны.
                        </span>
                      )}
                    </div>
                  </div>
                )}
              </Panel>
            }
          />

          <RyadRazdela
            testId="razdel-diagnostika"
            znachok={<IkDiagnostika className="h-4 w-4" />}
            nazvanie="Диагностика и журналы"
            poyasnenie="Проверка соединения, запись диагностики и папка с журналами"
            otkryt={otkryty.includes("diagnostika")}
            naZhmyh={() => perekluchit("diagnostika")}
            deti={
              <Panel testId="proverka">
                <Ryad
                  nazvanie="Утечки"
                  poyasnenie={
                    pochemuSero(utechki) ??
                    (zhdyot("checkLeaks")
                      ? "Смотрю, чей адрес видит интернет и через кого находятся сайты"
                      : "Чей адрес видит интернет и через кого находятся сайты")
                  }
                  aktiven={aktiven && !utechki}
                >
                  <Knopka
                    rang="vtoraya"
                    testId="proverit-utechki"
                    zhdyot={zhdyot("checkLeaks")}
                    aktiven={mozhnoZvat && !utechki}
                    onClick={() => naKomandu("checkLeaks", {})}
                  >
                    {zhdyot("checkLeaks") ? "Проверяю" : "Проверить"}
                  </Knopka>
                </Ryad>
                {proverkaOtkaz && (
                  <div className="border-border border-t p-4">
                    <Neudacha
                      testId="otkaz-proverki"
                      kod={proverkaOtkaz.kod}
                      tekst={proverkaOtkaz.tekst}
                      deystvie={mozhnoZvat && !utechki ? () => naKomandu("checkLeaks", {}) : undefined}
                      podpisDeystviya="Повторить"
                    />
                  </div>
                )}
                {ustarela && !proverkaOtkaz && (
                  <Ryad
                    testId="proverka-ustarela"
                    nazvanie="Результат устарел"
                    poyasnenie="Состояние VPN сменилось после проверки, проверь заново"
                    aktiven={false}
                  />
                )}
                {pokazatPunkty && snyato && (
                  <div className="border-border border-t px-4 py-3">
                    <p className="text-fg-muted mb-1 text-xs" data-testid="proverka-snyata">проверено в {snyato.vremya}</p>
                    <ul data-testid="punkty" className="flex max-h-64 flex-col gap-1 overflow-y-auto text-sm">
                      {proverka.punkty.map((p) => (
                        <li key={p.imya} data-itog={p.itog} className={p.itog === "utechka" ? "text-warn break-words" : p.itog === "ok" ? "break-words" : "text-fg-muted break-words"}>
                          <span className="font-medium">{p.imya}</span>
                          {" · "}
                          {ITOG[p.itog] ?? p.itog}
                          {" · "}
                          <span className="text-fg-muted">{p.tekst}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                <Ryad
                  testId="proverka-seti"
                  nazvanie="Что работает, а что нет"
                  poyasnenie={
                    zhdyot("checkNetwork")
                      ? "Проверяю по очереди: VPN, сервер, имена сайтов через VPN и мимо него, голос и видео"
                      : "Каждая часть отдельно: если что-то одно сломано, видно, что именно"
                  }
                  aktiven={aktiven}
                >
                  <Knopka
                    rang="vtoraya"
                    testId="proverit-set"
                    zhdyot={zhdyot("checkNetwork")}
                    aktiven={mozhnoZvat}
                    onClick={() => naKomandu("checkNetwork", {})}
                  >
                    {zhdyot("checkNetwork") ? "Проверяю" : "Проверить"}
                  </Knopka>
                </Ryad>
                {proverkaSeti && proverkaSeti.sloi.length > 0 && (
                  <div className="border-border border-t px-4 py-3">
                    <ul data-testid="sloi-seti" className="flex flex-col gap-1 text-sm">
                      {proverkaSeti.sloi.map((sl) => (
                        <li key={sl.vid} data-vid={sl.vid} data-proshlo={sl.proshlo}
                            className={sl.proshlo ? "break-words" : "text-warn break-words"}>
                          <span className="font-medium">{sl.podpis}</span>
                          {" · "}
                          {sl.proshlo ? "работает" : "не отвечает"}
                          {" · "}
                          <span className="text-fg-muted">{sl.podrobno}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                <Ryad
                  testId="adres-vyhoda"
                  nazvanie="Адрес выхода"
                  poyasnenie={
                    pochemuSero(adresOtlozhen) ??
                    (zhdyot("checkExitIp")
                      ? "Спрашиваю, каким адресом тебя видит интернет"
                      : adresVyhoda
                        ? tekstAdresa(adresVyhoda, podnyat)
                        : "По кнопке и при смене сервера, сам по себе не обновляется")
                  }
                  aktiven={aktiven && !adresOtlozhen}
                >
                  <Knopka
                    rang="vtoraya"
                    testId="proverit-adres"
                    zhdyot={zhdyot("checkExitIp")}
                    aktiven={mozhnoZvat && !adresOtlozhen}
                    onClick={() => naKomandu("checkExitIp", {})}
                  >
                    {zhdyot("checkExitIp") ? "Спрашиваю" : "Проверить"}
                  </Knopka>
                </Ryad>
                <Zhurnaly status={status} disabled={!mozhnoZvat} naKomandu={naKomandu}
                  naPapku={naPapkuZhurnalov} naDiagnostiku={naDiagnostiku} zanyatyeKomandy={zanyatyeKomandy}/>
              </Panel>
            }
          />
        </div>
      </Gruppa>

      <Gruppa nazvanie="Обновление" poyasnenie="Версия программы и способы её обновить">
        <Panel>
          {/* The service checks the update server daily on its own; this row
              shows what it found and lets the human act. The archive-from-disk
              path below stays as the second way in. */}
          <div ref={ryadObnovleniya}>
          <Ryad
            znachok={<IkObnovit className="h-[18px] w-[18px]" />}
            testId="obnovlenie"
            nazvanie={
              idyot
                ? zagolovokHoda(hodObnovleniya, status.versiya_programmy)
                : nahodka
                  ? `Есть ${nahodka.versiya}, ${obyom(nahodka.razmer)}`
                  : versiyaStrokoy(status.versiya_programmy)
            }
            poyasnenie={
              idyot
                ? <PolosaObnovleniya hod={hodObnovleniya} ostalosS={ostalosPodmeny} />
                : pochemuSero(undefined) ??
                  (zhdyot("checkUpdate")
                    ? "Смотрю сервер обновлений"
                    : nahodka
                      ? "Служба скачает архив, проверит его целость и перезапустится; если за 20 секунд не выйдет, останется прежняя версия"
                      : // Отказ важнее отметки: пока он стоит, «новее нет» это не
                        // ответ, а молчание сервера, и мёртвая проверка неделю
                        // выглядела здоровой (23.09.2026).
                        status.obnovlenie_otkaz
                        ? `Последняя проверка не удалась: ${status.obnovlenie_otkaz}`
                        : status.obnovlenie_provereno
                          ? `Проверено ${vremya(status.obnovlenie_provereno)}, новее нет; проверяется раз в сутки`
                          : "Ещё не проверялось; служба проверяет раз в сутки")
            }
            aktiven={aktiven || idyot}
          >
            {/* Пока обновление идёт, кнопок нет вовсе. Второе нажатие
                отправило бы вторую загрузку службе, которой на шаге подмены
                уже не существует. */}
            {idyot ? null : nahodka ? (
              // Между нажатием и первым событием хода проходит время: служба
              // успевает сходить на сервер обновлений. Без вертушки эта пауза
              // выглядит как нажатие, которое ничего не сделало.
              <Knopka rang="glavnaya" testId="ustanovit-obnovlenie" zhdyot={zhdyot("downloadUpdate")}
                      aktiven={mozhnoZvat} onClick={() => naKomandu("downloadUpdate", {})}>
                {zhdyot("downloadUpdate") ? "Начинаю" : "Установить"}
              </Knopka>
            ) : (
              <Knopka rang="vtoraya" testId="proverit-versiyu" zhdyot={zhdyot("checkUpdate")} aktiven={mozhnoZvat} onClick={() => naKomandu("checkUpdate", {})}>
                {zhdyot("checkUpdate") ? "Смотрю сервер" : "Проверить"}
              </Knopka>
            )}
          </Ryad>
          </div>
          <Ryad
            znachok={<IkArhiv className="h-[18px] w-[18px]" />}
            testId="arhiv-sborki"
            nazvanie="Архив сборки"
            poyasnenie={pochemuSero(obnovlenie) ?? "Архив с файлом .sha256 рядом; второй путь, когда сервер обновлений недоступен"}
            aktiven={aktiven && !obnovlenie}
          >
            {/* После выбора файла служба считает sha256 архива, и это не
                мгновенно. Диалог к тому времени уже закрыт, экран снова
                неподвижен, и человек ждёт у пустого места. */}
            <Knopka rang="vtoraya" testId="proverit-obnovlenie" zhdyot={zhdyot("installUpdate")}
                    aktiven={mozhnoZvat && !obnovlenie} onClick={() => (naObnovlenie ? naObnovlenie() : naKomandu("installUpdate", {}))}>
              {zhdyot("installUpdate") ? "Проверяю архив" : "Выбрать архив"}
            </Knopka>
          </Ryad>
        </Panel>
      </Gruppa>

      <div className="flex flex-col">
        <RyadRazdela
          testId="razdel-profil"
          znachok={<IkProfil className="h-4 w-4" />}
          nazvanie="Профиль и обслуживание"
          poyasnenie="Перенос профиля одним файлом и удаление программы"
          otkryt={otkryty.includes("profil")}
          naZhmyh={() => perekluchit("profil")}
          deti={
            <div className="flex flex-col gap-3">
              {(vyvestiProfil || vvestiProfil) && (
                <Panel>
                  <Ryad
                    nazvanie="Пароль профиля"
                    poyasnenie={pochemuSero(undefined) ?? "Нужен и на вывод, и на ввод; нигде не сохраняется и в журнал не пишется"}
                    aktiven={mozhnoZvat}
                  >
                    <input
                      type="password"
                      data-testid="parol-profilya"
                      aria-label="пароль профиля"
                      autoComplete="off"
                      value={parolProfilya}
                      onChange={(e) => zadatParolProfilya(e.target.value)}
                      className="bg-elevated border-border text-foreground placeholder:text-fg-faint hover:border-border-hover focus:border-border-active h-9 min-w-0 rounded-md border px-3 text-[13px]"
                    />
                  </Ryad>
                  {vyvestiProfil && (
                    <Ryad
                      nazvanie="Вывести профиль"
                      poyasnenie={pochemuSero(undefined) ?? "Серверы, подписка и правила одним файлом; ключи внутри, поэтому файл хранить как пароль"}
                      aktiven={mozhnoZvat}
                    >
                      <Knopka
                        rang="vtoraya"
                        testId="vyvesti-profil"
                        zhdyot={zhdyot("exportProfile")}
                        aktiven={mozhnoZvat && parolProfilya !== ""}
                        onClick={() => vyvestiProfil(parolProfilya)}
                      >
                        {zhdyot("exportProfile") ? "Пишу" : "Вывести"}
                      </Knopka>
                    </Ryad>
                  )}
                  {vvestiProfil && (
                    <Ryad
                      nazvanie="Ввести профиль"
                      poyasnenie={pochemuSero(undefined) ?? "Заменит серверы, подписку и правила целиком"}
                      aktiven={mozhnoZvat}
                    >
                      <Knopka
                        rang="vtoraya"
                        testId="vvesti-profil"
                        zhdyot={zhdyot("importProfile")}
                        aktiven={mozhnoZvat && parolProfilya !== ""}
                        onClick={() => vvestiProfil(parolProfilya)}
                      >
                        {zhdyot("importProfile") ? "Читаю" : "Ввести"}
                      </Knopka>
                    </Ryad>
                  )}
                  {itogProfilya && (
                    // The outcome belongs next to the button that produced it: the
                    // banner is for refusals, and a success has no banner at all.
                    <Ryad testId="itog-profilya" nazvanie={itogProfilya} aktiven={false} lomat />
                  )}
                </Panel>
              )}

              {dialog ? (
                <Udalenie naUdalenie={naUdalenie} naOtmenu={() => zadatDialog(false)} />
              ) : (
                <Panel>
                  <Ryad nazvanie="Удалить программу" poyasnenie="Служба, сетевое подключение VPN и файлы; ключи по выбору">
                    <Knopka rang="opasnaya" testId="otkryt-udalenie" onClick={() => zadatDialog(true)}>
                      Удалить
                    </Knopka>
                  </Ryad>
                </Panel>
              )}
            </div>
          }
        />
      </div>
    </section>
  );
}

/** "03.09.2026, 13:00" from an ISO stamp; the raw string when it does not parse. */
function vremya(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString("ru-RU", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

/** Версия программы строкой. Отсутствие версии это молчащая служба или сборка
 *  из дерева, и ни то, ни другое не называется словом «dev»: 13.09.2026 окно
 *  подписало им минуту обновления, и это прочли как «перебросило на dev». */
function versiyaStrokoy(versiya?: string): string {
  return versiya ? `Программа ${versiya}` : "Версия неизвестна";
}

const PODPISI_SHAGOV: Record<ShagObnovleniya, string> = {
  skachivanie: "скачивание",
  sverka: "проверка целости архива",
  raspakovka: "распаковка архива",
  podmena: "замена файлов, служба перезапускается",
  otkaz: "не удалось",
};

/** Заголовок строки во время обновления. Номер выпуска, а не своя версия:
 *  своя в эту минуту уже ничего не значит. */
function zagolovokHoda(hod: HodObnovleniya | null, svoya?: string): string {
  const kuda = hod?.versiya ?? svoya;
  return kuda ? `Обновление до ${kuda}` : "Обновление";
}

/** Доля загрузки в процентах; null, когда считать не из чего. */
function dolyaHoda(hod: HodObnovleniya | null): number | null {
  if (!hod || hod.shag !== "skachivanie") return null;
  const vsego = hod.vsego ?? 0;
  const skachano = hod.skachano ?? 0;
  if (vsego <= 0) return null;
  return Math.min(100, Math.round((skachano / vsego) * 100));
}

/** Полоса и подпись под ней. Полоса без известной доли остаётся бегущей: на
 *  сверке и распаковке считать нечего, а замереть на месте она не должна. */
function PolosaObnovleniya({ hod, ostalosS }: { hod: HodObnovleniya | null; ostalosS: number | null }) {
  if (!hod) return null;
  const dolya = dolyaHoda(hod);
  const podpis = PODPISI_SHAGOV[hod.shag];
  const hvost =
    hod.shag === "skachivanie" && dolya !== null
      ? ` ${dolya} %`
      : hod.shag === "podmena" && ostalosS !== null
        ? `, осталось ${ostalosS} с`
        : "";
  return (
    <span className="flex flex-col gap-1.5">
      <span>{podpis + hvost}</span>
      <Polosa dolya={dolya} podpis="ход обновления" />
    </span>
  );
}

/** Отсчёт секунд на шаге подмены. Ведёт его ОКНО, а не служба: службы в эту
 *  минуту нет, сказать ей нечем, и молчаливое ожидание без счётчика читается
 *  как зависшая программа. */
function otschetPodmeny(hod: HodObnovleniya | null): number | null {
  const srok = hod?.shag === "podmena" ? (hod.srok_s ?? 0) : 0;
  const [ostalos, zadat] = useState(srok);
  useEffect(() => {
    if (srok <= 0) return;
    zadat(srok);
    const t = setInterval(() => zadat((o) => (o > 0 ? o - 1 : 0)), 1000);
    return () => clearInterval(t);
  }, [srok]);
  return srok > 0 ? ostalos : null;
}
