import { useEffect, useState, type ReactNode } from "react";
import type {
  OtkazNaEkrane,
  Pomeha,
  Rezhim,
  Server,
  Statistika,
  StatusOtvet,
} from "../protokol";
import { glavnoeDeystvie, podpis } from "./podpisi";
import { tekstOtkaza } from "./otkazy";
import { type PodpiskaNaEkrane, type SpisokServerov, type ZamerZaderzhki } from "./Servery";
import type { PravilaOtvet } from "./Pravila";
import type { Marshrut } from "../trafik";
import sphere from "../assets/affory-sphere.png";
import { Knopka, Poisk, PROCHERK, Segment } from "./ui";
import { IkVniz } from "../ikonki";
import { KnopkaSpravki, SpravkaProtokolov } from "./SpravkaProtokolov";
import { KatalogServerov, gruppyServerov } from "./KatalogServerov";
import { slovoPosleChisla } from "../chisla";
import { useSohranenieZhurnala } from "./sohranenieZhurnala";
import { formatSkorosti, useSkorostTrafika } from "./skorostTrafika";
import "./Glavnyy.css";

// Pure over props. No subscription, no bridge, no runtime import: App.tsx
// owns the wiring and hands the whole StatusOtvet down in one piece. Half
// the state arriving in two events is exactly how two halves start to
// disagree, so there is one prop, not five.
//
// Перерисован 16.09.2026 по макетам владельца: две колонки, слева состояние
// туннеля, справа набор серверов, внизу служебная строка на всю ширину.
// Отдельной кнопки «Отключить» под сферой нет: сфера и есть выключатель.
// Свечение меняется только внутри ядра; оболочка остаётся неподвижной.

export interface GlavnyyProps {
  podpiski?: PodpiskaNaEkrane[];
  naObnovitPodpisku?: (id: string) => void;
  obnovlenieIdet?: boolean;
  obnovlyaemyePodpiski?: string[];
  pravila?: PravilaOtvet | null;
  zaderzhki?: ZamerZaderzhki[];
  zanyato?: boolean;
  naProverit?: () => void;
  proverkaIdet?: boolean;
  naVyborServera?: (id: string, podpiska?: string) => void;
  naTrafik?: (r: Marshrut) => void;
  naPravila?: () => void;
  skorost?: ReactNode;
  status: StatusOtvet;
  /** `null` is "no data yet" and renders as a dash, never as zero. */
  statistika?: Statistika | null;
  /** Known servers, to name the carrier instead of showing its id. */
  servery?: Server[];
  /** The whole listServers answer, when the tab has it; `null` means the
   *  list is simply not here, which is no longer a reason to show an id. */
  spisok?: SpisokServerov | null;
  spisokOtkaz?: OtkazNaEkrane | null;
  naDeystvie?: () => void;
  naServery?: () => void;
  /** Раздел «Свой VPN»: второй путь к первому серверу из пустого списка. */
  naSvoyServer?: () => void;
  naRezhim?: (r: Rezhim) => void;
  /** Ведёт к карточке обновления в настройках. Без него плашки в подвале нет
   *  вовсе: подпись, которая никуда не ведёт, хуже её отсутствия. */
  kObnovleniyu?: () => void;
  /** Обновление УЖЕ идёт. Плашка тогда не зовёт начинать заново: человек
   *  нажал «Установить» в настройках и ушёл на главную, а загрузка длится
   *  минуту. */
  podmenaIdet?: boolean;
  /** «Сохранить журнал» под «служба не отвечает» (1.10.0): ошибка без
   *  плашки отказа, и журнал при ней нужнее всего. */
  naSohranitZhurnal?: () => Promise<string>;
}

function imyaServera(
  id: string | undefined,
  servery: Server[],
): string | undefined {
  if (!id) return undefined;
  return servery.find((s) => s.id === id)?.imya ?? id;
}

function transportServera(
  id: string | undefined,
  servery: Server[],
): string | undefined {
  return servery.find((s) => s.id === id)?.transport;
}

function nesushchiy(s: StatusOtvet, servery: Server[]): string {
  // The core names the carrier; our choice is a different question. In auto
  // mode vybran_id is empty by construction and that is not "none chosen".
  //
  // The frame's own name wins over the list: in auto mode the carrier changes
  // between two listServers answers, and the list is then the stale one.
  const imya =
    s.nesushchiy_imya ||
    imyaServera(s.nesushchiy_id, servery) ||
    imyaServera(s.vybran_id, servery);
  if (imya) return imya;
  return s.rezhim_marshruta === "avto"
    ? "сервер выберется автоматически"
    : "сервер не выбран";
}

function chislo(v: number | undefined, ed: string): string {
  return v === undefined ? PROCHERK : `${v} ${ed}`;
}

/** Bytes for humans: unit grows with the number, one decimal, Russian comma.
 *  Raw "471172330 Б" on the strip was unreadable (owner, 03.09.2026). */
export function obyom(v: number | undefined): string {
  if (v === undefined) return PROCHERK;
  if (v < 1024) return `${v} Б`;
  const edinicy = ["КБ", "МБ", "ГБ", "ТБ"];
  let z = v / 1024;
  let i = 0;
  while (z >= 1024 && i < edinicy.length - 1) {
    z /= 1024;
    i++;
  }
  return `${z.toFixed(1).replace(".", ",")} ${edinicy[i]}`;
}

/** "1 ч 05 мин" since the tunnel came up; a dash when it is not up. */
export function vSeti(
  podnyatS: string | undefined,
  seychas: number = Date.now(),
): string {
  if (!podnyatS) return PROCHERK;
  const t = Date.parse(podnyatS);
  if (Number.isNaN(t)) return PROCHERK;
  const min = Math.max(0, Math.floor((seychas - t) / 60000));
  const ch = Math.floor(min / 60);
  if (ch === 0) return `${min} мин`;
  return `${ch} ч ${String(min % 60).padStart(2, "0")} мин`;
}

/** Что происходит во время восстановления: причина прошлого отказа и когда
 *  следующая попытка. Без неё «восстанавливается» висело минутами молча, и
 *  не было видно, идёт ли что-то вообще (1.9.1). */
export function hodVosstanovleniya(
  oshibka: StatusOtvet["oshibka"],
  sledPopytka: string | undefined,
  seychas: number = Date.now(),
): string {
  const prichina = oshibka ? (tekstOtkaza[oshibka.kod]?.tekst ?? oshibka.tekst) : "";
  const t = sledPopytka ? Date.parse(sledPopytka) : NaN;
  let kogda = "пробую подключиться";
  if (!Number.isNaN(t)) {
    const s = Math.max(1, Math.ceil((t - seychas) / 1000));
    kogda = s < 60 ? `снова через ${s} с` : `снова через ${Math.ceil(s / 60)} мин`;
  }
  return prichina ? `${prichina}, ${kogda}` : kogda;
}

/** Фраза о чужой программе, которая ломает VPN прямо сейчас (1.10.0). Окно
 *  её не закрывает: чужие процессы не наши, и решать человеку. Строка уходит
 *  сама, когда служба перестала видеть программу. */
export function tekstPomehi(p: Pomeha): string {
  switch (p.vid) {
    case "obhod":
      return `Работает ${p.imya}: он перехватывает те же соединения, что и VPN. Выключи его на время работы Affory`;
    case "vpn":
      return `Включён ${p.imya}: два VPN сразу не работают. Отключи его и подключись снова`;
  }
  return `Работает ${p.imya}: он мешает VPN. Выключи его на время работы Affory`;
}

/** Строка о сбитых часах (1.10.0). Отказ серверов она не утверждает: многие
 *  серверы пускают и со сбитыми на сутки часами (опыт в госте 21.10.2026), и
 *  при живом VPN такая строка гнала бы чинить исправное. Сокращения «мин»,
 *  «ч», «сут.» не склоняются, поэтому без slovoPosleChisla. */
export function tekstChasov(sekund: number): string {
  const kuda = sekund < 0 ? "отстают" : "спешат";
  const s = Math.abs(sekund);
  const skolko = s < 3600 ? `${Math.floor(s / 60)} мин` : s < 86400 ? `${Math.floor(s / 3600)} ч` : `${Math.floor(s / 86400)} сут.`;
  return `Часы компьютера ${kuda} на ${skolko}. Если подключение не удаётся, включи в настройках Windows «Установить время автоматически»`;
}

export function Glavnyy({
  podpiski = [],
  naObnovitPodpisku,
  obnovlenieIdet = false,
  obnovlyaemyePodpiski = [],
  status,
  statistika = null,
  servery,
  spisok = null,
  spisokOtkaz = null,
  naDeystvie,
  naServery,
  naSvoyServer,
  naRezhim,
  pravila = null,
  zaderzhki = [],
  zanyato = false,
  naVyborServera,
  naTrafik,
  skorost,
  naProverit,
  proverkaIdet = false,
  naPravila,
  kObnovleniyu,
  podmenaIdet = false,
  naSohranitZhurnal,
}: GlavnyyProps) {
  const zhurnal = useSohranenieZhurnala(naSohranitZhurnal);
  const izvestnye = servery ?? spisok?.servery ?? [];
  const podnyat = status.sostoyanie === "podnyat";
  const skorosti = useSkorostTrafika(statistika, podnyat, `${status.podnyat_s ?? ""}:${status.nesushchiy_id ?? ""}`);
  const busy =
    status.sostoyanie === "podnimaetsya" ||
    status.sostoyanie === "vosstanavlivaetsya";
  const molchit = status.sostoyanie === "sluzhba-molchit";
  const rezhim = status.rezhim_marshruta ?? "avto";
  // `||`, а не `??`: пустая строка это тоже «никто не несёт», и через `??`
  // она прошла бы дальше выбранным сервером, оставив экран без имени.
  const selected = status.nesushchiy_id || status.vybran_id || spisok?.vybran;
  const loading = spisok === null && servery === undefined && !spisokOtkaz;
  const route = pravila?.trafik?.po_umolchaniyu ?? "vpn";
  const [query, setQuery] = useState("");
  const [spravka, zadatSpravku] = useState(false);
  const [seychas, zadatSeychas] = useState(() => Date.now());
  useEffect(() => {
    if (!podnyat) return;
    const timer = setInterval(() => zadatSeychas(Date.now()), 30000);
    return () => clearInterval(timer);
  }, [podnyat]);
  const vosst = status.sostoyanie === "vosstanavlivaetsya";
  // Отсчёт до следующей попытки идёт по секундам, поэтому часы тикают чаще,
  // но только пока он на экране.
  useEffect(() => {
    if (!vosst || !status.sled_popytka) return;
    zadatSeychas(Date.now());
    const timer = setInterval(() => zadatSeychas(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [vosst, status.sled_popytka]);
  const action = busy
    ? "Отменить подключение"
    : (glavnoeDeystvie[status.sostoyanie] ?? "Отключить");
  const imya = podnyat
    ? nesushchiy(status, izvestnye)
    : rezhim === "avto"
      ? "Сервер выберется сам"
      : (imyaServera(selected, izvestnye) ?? "Сервер не выбран");
  const transport = podnyat ? transportServera(status.nesushchiy_id, izvestnye) : undefined;
  const gruppy = gruppyServerov(izvestnye, podpiski, spisok?.podpiska_uzel);
  const vsego = gruppy.reduce((n, g) => n + g.servery.length, 0);
  return (
    <section
      className="flex min-h-0 flex-1 flex-col"
      data-state={status.sostoyanie}
      aria-label="Подключение"
    >
      <div className="flex min-h-0 flex-1">
        {/* Левая колонка: состояние туннеля */}
        <div className="affory-connection border-border flex w-[42%] min-w-[380px] shrink-0 flex-col items-center overflow-y-auto border-r px-8 py-6">
          <div className="affory-identity flex w-full flex-1 flex-col items-center justify-center">
          <button
            type="button"
            className="affory-core group relative flex aspect-square w-full max-w-[260px] shrink-0 items-center justify-center rounded-3xl"
            aria-label={action}
            aria-pressed={podnyat}
            disabled={!naDeystvie || (zanyato && !busy)}
            data-testid="glavnoe-deystvie"
            onClick={naDeystvie}
          >
            <span
              aria-hidden
              className={`absolute inset-[22%] rounded-full blur-2xl transition-colors duration-500 ${
                podnyat ? "bg-accent/20" : "bg-accent/8"
              }`}
            />
            <img
              src={sphere}
              alt=""
              draggable={false}
              className={`relative h-full w-full select-none object-contain transition-all duration-500 ${
                podnyat || busy ? "opacity-100" : "opacity-45 saturate-0"
              }`}
            />
            <span className="affory-core-light" aria-hidden />
          </button>
          <div className="affory-status mt-2 flex flex-wrap items-baseline justify-center gap-x-2.5 gap-y-1 text-center">
            <span className="text-foreground text-base font-medium first-letter:uppercase" role="status" data-testid="sostoyanie">
              {podpis[status.sostoyanie]}
            </span>
            <span className={podnyat ? "text-fg-secondary text-sm" : "sr-only"}>
              <span className="sr-only">В сети </span>
              <span data-testid="v-seti">{vSeti(podnyat ? status.podnyat_s : undefined, seychas)}</span>
            </span>
          </div>
          {vosst && (
            <p className="text-fg-secondary mt-1 text-center text-sm" data-testid="hod-vosstanovleniya">
              {hodVosstanovleniya(status.oshibka, status.sled_popytka, seychas)}
            </p>
          )}
          {molchit && zhurnal.sohranit && (
            <div className="mt-1 flex flex-col items-center gap-1" data-testid="molchit-zhurnal">
              <button
                type="button"
                data-testid="molchit-sohranit-zhurnal"
                disabled={zhurnal.idet}
                onClick={() => void zhurnal.sohranit?.()}
                className="text-accent-ink hover:underline text-sm font-medium disabled:opacity-60"
              >
                {zhurnal.idet ? "Сохраняю журнал…" : "Сохранить журнал"}
              </button>
              {zhurnal.itog && (
                <p role="status" className="text-fg-secondary text-center text-sm" data-testid="molchit-itog-zhurnala">{zhurnal.itog}</p>
              )}
            </div>
          )}
          <p className="affory-carrier text-foreground mt-2 w-full break-all text-center text-[17px] font-medium" data-testid="nesushchiy">
            <span className="affory-server-name">{imya}</span>
            {transport && <span className="affory-protocol text-fg-muted">{transport}</span>}
          </p>
          {/* Ручной режим, выбранный сервер молчит, и VPN везёт другой. Сказать
              это обязательно: молча отдать человеку другую страну нельзя. */}
          {podnyat && status.vremenno_vmesto && (
            <p className="text-fg-secondary mt-1 text-center text-sm" data-testid="vremenno-vmesto">
              {status.vremenno_vmesto} не отвечает, VPN временно идёт через этот сервер
            </p>
          )}
          {/* При любом состоянии, а не только поднятом: о zapret надо знать и
              до подключения. Подключение строка не запрещает (решение
              владельца 21.10.2026). */}
          {status.pomehi?.map((p) => (
            <p key={p.imya} role="status" className="text-warn mt-1 text-center text-sm" data-testid="pomeha">
              {tekstPomehi(p)}
            </p>
          ))}
          {status.chasy_rashozhdenie ? (
            <p role="status" className="text-warn mt-1 text-center text-sm" data-testid="chasy">
              {tekstChasov(status.chasy_rashozhdenie)}
            </p>
          ) : null}
          </div>

          <div className="affory-routing mt-5 w-full max-w-[380px]">
            <Segment
              aria-label="Куда идёт трафик"
              rastyanut
              ton="tihiy"
              aktiven={!(molchit || busy || zanyato || !pravila?.trafik || !naTrafik)}
              vybrano={route}
              naVybor={(r) => naTrafik?.(r as Marshrut)}
              znacheniya={[
                { z: "vpn", podpis: "Всё через VPN" },
                { z: "direct", podpis: "Только выбранное" },
              ]}
            />
            <p className="text-fg-muted mt-2.5 text-center text-[13px]">
              {status.kill_switch ? "Защита сети включена: всё идёт через VPN, прямые маршруты не действуют" : route === "vpn"
                ? "Через VPN, кроме прямых маршрутов"
                : "Через VPN только то, что в правилах"}
            </p>
            {/* Отсюда до списка исключений был один путь: догадаться про
                раздел «Правила» в полосе наверху. Подчёркивания нет, ссылка
                опознаётся цветом (правило интерфейса). */}
            {naPravila && (
              <p className="mt-1.5 text-center">
                <button
                  type="button"
                  data-testid="k-pravilam"
                  onClick={naPravila}
                  className="text-accent-ink hover:brightness-125 text-[13px] font-medium"
                >
                  Настроить правила
                </button>
              </p>
            )}
          </div>

          <div className="affory-traffic border-border mt-6 w-full max-w-[420px] border-t pt-5" data-testid="statistika">
            <dl className="grid grid-cols-2">
              <div className="border-border border-r pr-3">
                <Cifra nazvanie="↓ Приём" znachenie={formatSkorosti(skorosti.priem)} testId="skorost-priema" />
                <div className="affory-total text-fg-muted mt-2"><dt>Получено</dt><dd className="text-fg-secondary">{obyom(podnyat ? statistika?.prinyato : undefined)}</dd></div>
              </div>
              <div className="pl-4">
                <Cifra nazvanie="↑ Отдача" znachenie={formatSkorosti(skorosti.otdacha)} testId="skorost-otdachi" />
                <div className="affory-total text-fg-muted mt-2"><dt>Отправлено</dt><dd className="text-fg-secondary">{obyom(podnyat ? statistika?.otdano : undefined)}</dd></div>
              </div>
            </dl>
            {/* Задержка это итог блока, а не третья колонка: она одна на оба
                направления. Поэтому строка во всю ширину под своей чертой, в
                том же ритме, что «Получено» и «Отправлено» внутри колонок.
                По центру и без черты она читалась как подпись, случайно
                оставшаяся под таблицей (владелец, 19.09.2026). */}
            <dl
              className="affory-latency border-border mt-4 flex items-baseline justify-between border-t pt-3 text-[13px]"
              /* Пояснение нужно, потому что число сравнивают с чужими: с пингом
                 в игре и в Discord. Оно и правда про то же - один круг, а не
                 весь запрос с рукопожатием (21.09.2026). */
              title="Сколько идёт один круг до интернета через VPN. То же, что пинг в играх и в Discord."
            >
              <dt className="text-fg-muted">Задержка</dt>
              <dd className="text-foreground font-medium" data-testid="zaderzhka">{podnyat ? chislo(statistika?.zaderzhka_ms, "мс") : PROCHERK}</dd>
            </dl>
          </div>
        </div>

        {/* Правая колонка: набор серверов и замер полосы */}
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-8 py-6">
            <header className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                {/* Значок справки стоит здесь, а не только на экране
                    «Управлять»: имя протокола человек читает именно в этом
                    списке, и вопрос «что из этого брать» возникает тут. */}
                <div className="flex items-center gap-2">
                  <h2 className="text-foreground text-[22px] font-semibold leading-tight">Серверы</h2>
                  <KnopkaSpravki onClick={() => zadatSpravku(true)} />
                </div>
                <p className="text-fg-muted mt-1 text-[13px]">
                  {spisok
                    ? `${vsego} ${slovoPosleChisla(vsego, "сервер", "сервера", "серверов")}${podpiski.length ? ` · ${podpiski.length} ${slovoPosleChisla(podpiski.length, "подписка", "подписки", "подписок")}` : " в списке"}`
                    : spisokOtkaz
                      ? "Список не прочитался"
                      // «Читаю список» при молчащей службе это обещание работы,
                      // которой не идёт: читать не у кого.
                      : molchit ? "Служба не отвечает" : "Читаю список"}
                </p>
              </div>
              <Knopka rang="vtoraya" bolshaya onClick={naServery}>Управлять</Knopka>
            </header>

            <div className="flex flex-col gap-2">
              <Segment
                aria-label="Как выбирается сервер"
                rastyanut
                ton="tihiy"
                aktiven={!(molchit || busy || zanyato)}
                vybrano={rezhim}
                naVybor={(r) => naRezhim?.(r as Rezhim)}
                znacheniya={[
                  { z: "avto", podpis: "Автоматически" },
                  { z: "ruchnoy", podpis: "Вручную" },
                ]}
              />
              {/* Подписи под переключателем нет намеренно (22.09.2026): она
                  пересказывала сам переключатель и нажатие на строку списка.
                  Если она когда-нибудь вернётся, обещать «на следующем
                  подключении» ей нельзя: службе хватает clash_api, и выбор
                  применяется на живом ядре сразу. */}
            </div>

            <div className="flex items-center gap-3">
              <Poisk
                id="af-server-search"
                aria-label="Поиск сервера"
                placeholder="Найти сервер..."
                znachenie={query}
                naVvod={setQuery}
              />
              <Knopka
                rang="vtoraya"
                bolshaya
                zhdyot={proverkaIdet}
                aktiven={!(molchit || busy || zanyato || loading || !izvestnye.length || !naProverit)}
                onClick={naProverit}
              >
                {proverkaIdet ? "Проверяю отклик" : "Проверить серверы"}
              </Knopka>
            </div>

            {!loading && !spisokOtkaz && gruppy.length > 0 && <KatalogServerov
              servery={izvestnye} podpiski={podpiski} uzel={spisok?.podpiska_uzel} zapros={query}
              zaderzhki={zaderzhki} nesushchiy={status.nesushchiy_id} vybran={selected} podnyat={podnyat}
              disabled={molchit || busy || zanyato} naVybor={naVyborServera}
              naObnovit={naObnovitPodpisku} obnovlyaetsya={obnovlenieIdet} obnovlyaemyePodpiski={obnovlyaemyePodpiski}
            />}
            <ul>
              {loading && (
                <li className="text-fg-muted border-border rounded-lg border px-4 py-6 text-center text-[13px]" role="status">
                  {molchit ? "Служба не отвечает, список серверов недоступен" : "Читаю список серверов"}
                </li>
              )}
              {spisokOtkaz && (
                <li className="text-fg-muted border-border rounded-lg border px-4 py-6 text-center text-[13px]">
                  Список серверов недоступен. Причина и повторная загрузка указаны выше
                </li>
              )}
              {!loading && !spisokOtkaz && vsego === 0 && (
                <li className="border-border flex flex-col items-center gap-2 rounded-lg border border-dashed px-4 py-10 text-center">
                  <span className="text-foreground text-sm font-medium">
                    {query ? "Ничего не найдено" : "Серверов пока нет"}
                  </span>
                  <span className="text-fg-muted text-[13px]">
                    {query
                      ? "Попробуй другое имя или адрес"
                      : "Вставь ссылку на сервер или подписку"}
                  </span>
                  {!query && (
                    <div className="mt-1 flex flex-wrap justify-center gap-2">
                      <Knopka rang="glavnaya" onClick={naServery}>
                        Добавить сервер
                      </Knopka>
                      {naSvoyServer && (
                        <Knopka rang="vtoraya" testId="glavnyy-svoy-server" onClick={naSvoyServer}>
                          Поставить на свой сервер
                        </Knopka>
                      )}
                    </div>
                  )}
                </li>
              )}
            </ul>
          </div>

          {skorost}
        </div>
      </div>

      <footer className="border-border bg-rail text-fg-muted flex h-11 shrink-0 items-center justify-between border-t px-6 text-[13px]">
        <span>
          Адрес выхода{" "}
          <b className="text-fg-secondary font-medium">{podnyat ? statistika?.adres_vyhoda || PROCHERK : PROCHERK}</b>
        </span>
        <span className="flex items-center gap-3">
          {/* Новая версия рядом с номером своей (D3, 23.09.2026, просьба
              владельца). В настройках находку видно только тому, кто туда
              зашёл и раскрыл раздел, а трей говорит о ней один раз и молча
              забывает. Подвал главного человек видит всегда. */}
          {status.obnovlenie && kObnovleniyu && (
            <button
              type="button"
              data-testid="est-obnovlenie"
              onClick={kObnovleniyu}
              title={podmenaIdet ? "Показать ход обновления" : "Открыть настройки и обновиться"}
              className="bg-accent text-foreground flex items-center gap-1.5 rounded-md px-2.5 py-[3px] text-[12px] font-medium hover:brightness-110"
            >
              <IkVniz className="h-3.5 w-3.5" />
              {podmenaIdet ? `Обновляюсь до ${status.obnovlenie.versiya}` : `Есть версия ${status.obnovlenie.versiya}`}
            </button>
          )}
          {/* Без номера, а не со словом «dev»: версия приходит от службы, и её
              отсутствие значит «служба молчит», что и так написано выше. Во
              время обновления служба молчит намеренно, и подпись «Affory dev»
              прочли 13.09.2026 как подмену сборкой разработчика. */}
          <span>
            Affory{" "}
            {status.versiya_programmy && <b className="text-fg-secondary font-medium">{status.versiya_programmy}</b>}
          </span>
        </span>
      </footer>

      {spravka && (
        <SpravkaProtokolov
          zakryt={() => zadatSpravku(false)}
          svoi={izvestnye.map((s) => s.transport)}
        />
      )}
    </section>
  );
}

function Cifra({
  nazvanie,
  znachenie,
  testId,
}: {
  nazvanie: string;
  znachenie: string;
  testId?: string;
}) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-fg-muted text-[13px]">{nazvanie}</dt>
      <dd className="affory-rate text-foreground font-semibold" data-testid={testId}>
        {znachenie.endsWith(" Мбит/с")
          ? <>{znachenie.replace(" Мбит/с", "")} <span className="affory-rate-unit">Мбит/с</span></>
          : znachenie}
      </dd>
    </div>
  );
}
