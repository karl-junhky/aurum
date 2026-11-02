import { useEffect, useRef, useState } from "react";
import type { PravilaProps } from "./Pravila";
import { imyaMarshruta, prichinaPryamogoTrafika, snimokPravil, zadatMarshrutServisa, type VyborServisa, type ChernovikPravil, type Marshrut, type PravilaTrafika } from "../trafik";
import { perekrytiyaServisa } from "../ohvat";
import { Flazhok, Knopka, Poisk, Pole, SegmentStolbik, Svorachivaemyy, Tumbler } from "./ui";
import { IkPlyus, IkSayt, IkSsylka, IkTreugolnik } from "../ikonki";
import { IkonkaServisa } from "./IkonkaServisa";
import { Vybor } from "./Vybor";
import { slovoPosleChisla } from "../chisla";
import { razobratVvodDomenov } from "../domeny";
import { naydennyePuti } from "../programmy";
import { sleduyushchayaVkladka } from "./klavishi-vkladok";
import { OhvatPravil } from "./OhvatPravil";
import { Reklama } from "./Reklama";
import { tekstOshibki } from "../ponyatno";

// Раздел правил: слева режим по умолчанию и счёт правил, справа вкладки (три,
// и четвёртая «Реклама» у службы, которая её знает). Боковая область и вкладки
// стоят на одном месте во всех видах, поэтому переключение вкладки ничего на
// экране не двигает.

export function VyborTrafika({
  value,
  disabled,
  onChange,
}: {
  value: Marshrut;
  disabled?: boolean;
  onChange: (r: Marshrut) => void;
}) {
  return (
    <SegmentStolbik
      aria-label="Трафик по умолчанию"
      aktiven={!disabled}
      vybrano={value}
      naVybor={onChange}
      znacheniya={[
        { z: "vpn", podpis: "Всё через VPN" },
        { z: "direct", podpis: "Только выбранное" },
      ]}
    />
  );
}

function RouteSelect({
  value,
  label,
  disabled,
  onChange,
}: {
  value: Marshrut;
  label: string;
  disabled?: boolean;
  onChange: (v: Marshrut) => void;
}) {
  return (
    <Vybor
      label={label}
      value={value}
      disabled={disabled}
      options={[{ value: "vpn", label: "Через VPN" }, { value: "direct", label: "Напрямую" }]}
      onChange={onChange}
    />
  );
}

/** Путь под ширину колонки: начало, многоточие, хвост с именем файла. Предел
 *  в знаках, а не в пикселях: мерить ширину в каждой строке таблицы значит
 *  мерить её на каждый кадр, а колонка тут фиксированная. */
export function sokratitPut(put: string, predel = 58): string {
  if (put.length <= predel) return put;
  return `${put.slice(0, 13)}...${put.slice(-(predel - 16))}`;
}

// Колонки таблиц задаются ОДНОЙ строкой на вид и переиспользуются шапкой и
// строками: две раскладки рядом это два места, где колонки разъезжаются.
const SETKA_PRILOZHENIY = "grid grid-cols-[minmax(0,1fr)_150px_80px] min-[1100px]:grid-cols-[minmax(0,1fr)_190px_150px_110px] items-center gap-x-4 gap-y-2 min-w-0 flex-1";
const SETKA_SAYTOV = "grid grid-cols-[minmax(0,1fr)_150px_110px] items-center gap-x-4 min-w-0 flex-1";

/** Строка списка правил: отметка для группового действия слева, дальше своя
 *  сетка колонок. Отметка ВНЕ сетки намеренно: приложения на узком окне
 *  переносят имя на отдельную строку, и лишняя колонка в самой сетке уводила
 *  бы флажок охвата под чужой заголовок. */
const STROKA_S_VYBOROM = "flex items-center gap-3";

/** Столько правил сайтов принимает служба (cmd/affory-svc/trafik.go). Набор
 *  сверх предела отвергается ЦЕЛИКОМ, поэтому счёт ведётся и здесь. */
const PREDEL_DOMENOV = 1024;

/** Сколько разобранных имён показывать в предпросмотре. Вставляют и по сотне;
 *  сотня строк под полем прячет и кнопку, и список правил. */
const POKAZAT_V_PREDPROSMOTRE = 8;

export function Marshruty({
  proveritPrilozhenie,
  proveritSoedineniya,
  status,
  pravila,
  trafik: sohranennyyTrafik,
  chernovik,
  naChernovik,
  zapushchennye,
  protsessyChitayutsya = false,
  protsessyOtkaz = "",
  obnovitProtsessy,
  naVyborPrilozheniya,
  naKomandu,
  zanyato = false,
}: PravilaProps & { trafik: PravilaTrafika }) {
  const [localDraft, setLocalDraft] = useState<ChernovikPravil | null>(null);
  const draft = chernovik === undefined ? localDraft : chernovik;
  const setDraft = naChernovik ?? setLocalDraft;
  const trafik = draft?.trafik ?? sohranennyyTrafik;
  const bezRu = draft?.bezRu ?? (pravila?.bez_ru_spiska === true);
  // Реклама едет тем же черновиком. undefined значит «служба её не знает»:
  // тогда нет ни вкладки, ни поля в setRules.
  const reklama = draft ? draft.reklama : pravila?.reklama;
  const base = snimokPravil(sohranennyyTrafik, pravila?.bez_ru_spiska === true, pravila?.reklama);
  const dirty = snimokPravil(trafik, bezRu, reklama) !== base;
  const conflict = !!draft && dirty && draft.baza !== base;
  // Прямые маршруты при включённой защите больше не конфликт, а ПОЯСНЕНИЕ
  // (23.09.2026): применить их можно, они сохраняются, но пока защита включена
  // не действуют - весь трафик идёт через VPN. Отсекает их служба при сборке
  // конфига, набор человека при этом не меняется.
  const pryamyeSpyat=status.kill_switch?prichinaPryamogoTrafika(trafik):null;
  const [applying, setApplying] = useState(false);
  const inFlight = useRef(false);
  const [notice, setNotice] = useState("");
  const [applyError, setApplyError] = useState("");
  const [tab, setTab] = useState<"services" | "apps" | "sites" | "reklama">("services");
  const [adding, setAdding] = useState(false);
  // Кнопки полосы вкладок и кнопка «Добавить»: первым нужен фокус при ходьбе
  // стрелками, второй принимает фокус обратно, когда исчезает та кнопка, на
  // которой он стоял, - удалённая строка или ставшее ненужным применение.
  const knopkiVkladok = useRef<(HTMLButtonElement | null)[]>([]);
  const knopkaDobavit = useRef<HTMLButtonElement | null>(null);
  const pervoePoleFormy = useRef<HTMLInputElement | null>(null);
  useEffect(() => {
    // И на вкладке сервисов: карточка с клиентом берёт его путь среди
    // запущенных программ, а без списка она не знает, есть ли он вообще.
    if ((adding && tab === "apps") || tab === "services") obnovitProtsessy?.();
  }, [adding, tab, obnovitProtsessy]);
  // Курсор в первое поле открытой формы: иначе после нажатия «Добавить» надо
  // было ещё дойти до поля табом через всю полосу вкладок.
  useEffect(() => {
    if (adding) pervoePoleFormy.current?.focus();
  }, [adding, tab]);
  const [query, setQuery] = useState("");
  const [path, setPath] = useState("");
  const [picking, setPicking] = useState(false);
  const [pickerError, setPickerError] = useState("");
  const pickerEpoch = useRef(0);
  useEffect(() => () => { pickerEpoch.current++; }, []);
  const [domain, setDomain] = useState("");
  // Ввод сайтов разбирается на каждый набранный знак: предпросмотр показывает
  // ровно то, что уедет в правило, а punycode кириллицы иначе появлялся бы в
  // списке уже после применения и читался как чужая строка.
  const razborDomenov = razobratVvodDomenov(domain);
  const [descendants, setDescendants] = useState(true);
  // Умолчание новой формы это ИСКЛЮЧЕНИЕ из общего режима: при «Всё через VPN»
  // человек заводит правило, чтобы что-то пустить напрямую. Защита на это
  // умолчание больше не влияет: правило сохранится и заработает, когда её
  // выключат.
  const [route, setRoute] = useState<Marshrut>(
    trafik.po_umolchaniyu === "vpn" ? "direct" : "vpn",
  );
  const [remove, setRemove] = useState<string | null>(null);
  // Поиск и фильтр по сохранённым правилам: списки упираются в 256 приложений
  // и 1024 сайта, и на второй сотне скролл перестаёт быть способом что-то
  // найти. Выбор строк живёт рядом с ними: групповое действие применяется
  // ровно к тому, что человек видит и отметил.
  const [poiskPravil, setPoiskPravil] = useState("");
  const [filtrMarshruta, setFiltrMarshruta] = useState<"vse" | Marshrut>("vse");
  const [vybrannye, setVybrannye] = useState<string[]>([]);
  const [raskryto, setRaskryto] = useState<Record<string, boolean>>({});
  const [vesSpisok, setVesSpisok] = useState(false);
  const disabled =
    applying ||
    zanyato ||
    picking ||
    status.sostoyanie === "sluzhba-molchit" ||
    status.sostoyanie === "podnimaetsya" ||
    status.sostoyanie === "vosstanavlivaetsya";
  const save = (next: PravilaTrafika, nextBezRu = bezRu, nextReklama = reklama) => {
    if (disabled || snimokPravil(next, nextBezRu, nextReklama) === snimokPravil(trafik, bezRu, reklama)) return false;
    setApplyError("");
    setNotice("");
    setDraft(snimokPravil(next, nextBezRu, nextReklama) === base ? null : {trafik:next,bezRu:nextBezRu,reklama:nextReklama,baza:draft?.baza ?? base,reviziya:draft?.reviziya ?? pravila?.reviziya_pravil});
    return true;
  };
  const apply = async () => {
    if (disabled || inFlight.current || !dirty || conflict) return;
    inFlight.current = true;
    setApplying(true); setApplyError(""); setNotice("");
    try {
      const revision = pravila?.reviziya_pravil ?? draft?.reviziya;
      // reklama только если служба её прислала: служба прошлой версии поля не
      // знает, и тело для неё остаётся прежним.
      const body = {trafik,bez_ru_spiska:bezRu,...(reklama !== undefined ? {reklama} : {}),...(revision ? {reviziya_pravil:revision} : {})};
      const ok = await naKomandu("setRules",body);
      if (ok !== true) { setApplyError("Не удалось применить правила. Черновик сохранён; причина указана в сообщении службы."); return; }
      setDraft(null); setPath(""); setDomain(""); setAdding(false); setVybrannye([]);
      // Кнопка «Применить изменения» после успеха гаснет вместе со всей
      // полосой черновика, поэтому фокус с неё уходит к действию, с которого
      // редактирование и начинается.
      knopkaDobavit.current?.focus();
      setNotice(status.sostoyanie === "vyklyuchen" ? "Правила сохранены" : "Правила применены");
    } catch (error: unknown) {
      setApplyError(tekstOshibki(error));
    } finally {
      inFlight.current = false; setApplying(false);
    }
  };
  const apps = trafik.prilozheniya ?? [],
    domains = trafik.domeny ?? [],
    services = trafik.servisy ?? [];
  const katalog = pravila?.katalog?.servisy ?? [];
  const ruleCount = apps.length + domains.length + services.length;
  // Что показано на вкладке после поиска и фильтра. Ключ строки это путь для
  // приложения и домен для сайта: он же ключ выбора, он же ключ удаления.
  const zapros = poiskPravil.trim().toLowerCase();
  const vidimyePrilozheniya = apps.filter(
    (a) => (filtrMarshruta === "vse" || a.marshrut === filtrMarshruta) &&
      (zapros === "" || `${a.imya} ${a.put}`.toLowerCase().includes(zapros)),
  );
  const vidimyeDomeny = domains.filter(
    (d) => (filtrMarshruta === "vse" || d.marshrut === filtrMarshruta) &&
      (zapros === "" || d.domen.includes(zapros)),
  );
  const vidimyeKlyuchi = tab === "apps" ? vidimyePrilozheniya.map((a) => a.put) : tab === "sites" ? vidimyeDomeny.map((d) => d.domen) : [];
  // Отмеченное, но скрытое фильтром, в групповое действие НЕ идёт: человек
  // отмечает то, что видит, и «удалить выбранные» после смены фильтра иначе
  // унесло бы строки, которых на экране уже нет.
  const otmecheno = vybrannye.filter((k) => vidimyeKlyuchi.includes(k));
  const vsegoNaVkladke = tab === "apps" ? apps.length : tab === "sites" ? domains.length : 0;
  const otfiltrovano = zapros !== "" || filtrMarshruta !== "vse";
  /** Групповая правка выбранных строк: один черновик, одно применение. */
  const gruppoy = (deystvie: "vpn" | "direct" | "udalit") => {
    if (disabled || otmecheno.length === 0) return;
    const naboru = new Set(otmecheno);
    const izmenyon = tab === "apps"
      ? { ...trafik, prilozheniya: deystvie === "udalit"
          ? apps.filter((a) => !naboru.has(a.put))
          : apps.map((a) => (naboru.has(a.put) ? { ...a, marshrut: deystvie } : a)) }
      : { ...trafik, domeny: deystvie === "udalit"
          ? domains.filter((d) => !naboru.has(d.domen))
          : domains.map((d) => (naboru.has(d.domen) ? { ...d, marshrut: deystvie } : d)) };
    const skolko = otmecheno.length;
    if (!save(izmenyon)) return;
    setVybrannye([]);
    setRemove(null);
    if (deystvie === "udalit") {
      // Строки вместе со своими кнопками сейчас исчезнут.
      knopkaDobavit.current?.focus();
      setNotice(`${skolko} ${slovoPosleChisla(skolko, "правило", "правила", "правил")} удалено в черновике. Нажми «Применить изменения», когда закончишь редактирование.`);
      return;
    }
    setNotice(`Маршрут изменён у ${skolko} ${slovoPosleChisla(skolko, "правила", "правил", "правил")} в черновике. Нажми «Применить изменения», когда закончишь редактирование.`);
  };
  const candidates = (zapushchennye ?? []).filter((p) =>
    `${p.imya} ${p.put}`.toLowerCase().includes(query.toLowerCase()),
  );
  // Разные ответы вместо одного «Не найдено»: идёт чтение, чтение провалилось,
  // запущенных программ действительно ноль, запрос ничего не отобрал, ответа
  // ещё не было вовсе. Ноль в непустом ответе это ИЗМЕРЕННЫЙ ноль, а провал и
  // отсутствие ответа мерили не программы человека, а нашу связь с оболочкой.
  //
  // Список на руках старше отказа: он остаётся с пометкой сверху, потому что
  // устаревший снимок полезнее пустоты, а путь можно ввести и руками.
  const spisokEst = !!zapushchennye;
  const sostoyanieSpiska: "chitaetsya" | "otkaz" | "pusto" | "bezSovpadeniy" | "netOtveta" | "est" =
    candidates.length > 0 ? "est"
    : spisokEst ? (zapushchennye.length > 0 ? "bezSovpadeniy" : "pusto")
    : protsessyChitayutsya ? "chitaetsya"
    : protsessyOtkaz !== "" ? "otkaz"
    : "netOtveta";
  const changeTab = (next: typeof tab) => {
    pickerEpoch.current++;
    setPicking(false);
    setPickerError("");
    setTab(next);
    setAdding(false);
    setRemove(null);
    // Поиск, фильтр и отметки принадлежат списку, а не окну: перенести их на
    // соседнюю вкладку значит показать там пустой список и объяснять, почему.
    setPoiskPravil("");
    setFiltrMarshruta("vse");
    setVybrannye([]);
  };
  const browse = async () => {
    if (disabled || !naVyborPrilozheniya) return;
    const epoch = ++pickerEpoch.current;
    setPicking(true);
    setPickerError("");
    try {
      const chosen = await naVyborPrilozheniya();
      if (epoch === pickerEpoch.current && chosen) setPath(chosen);
    } catch (error: unknown) {
      if (epoch === pickerEpoch.current)
        setPickerError(tekstOshibki(error));
    } finally {
      if (epoch === pickerEpoch.current) setPicking(false);
    }
  };
  const add = () => {
    if (disabled) return;
    // Пустой ввод отсекается здесь, а не только гашением кнопки: по Enter из
    // поля поиска форма отправляется в обход кнопки, и правило с пустым путём
    // раньше доехало бы до черновика.
    if ((tab === "apps" ? path.trim() : domain.trim()) === "") return;
    let changed = false;
    let skolko = 1;
    if (tab === "apps") {
      const app = {
        put: path.trim(),
        imya: path.trim().split(/[/\\]/).pop() ?? "Приложение",
        potomki: descendants,
        marshrut: route,
      };
      const existing = apps.findIndex(p => p.put.toLowerCase() === app.put.toLowerCase());
      changed = save({
        ...trafik,
        prilozheniya: existing < 0 ? [...apps,app] : apps.map((previous,i) => i===existing ? app : previous),
      });
    } else {
      const novye = razborDomenov.gotovye.map((g) => g.domen);
      if (novye.length === 0) return;
      const itog = [
        ...domains.filter((d) => !novye.includes(d.domen)),
        ...novye.map((domen) => ({ domen, marshrut: route })),
      ];
      // Предел службы (cmd/affory-svc/trafik.go): весь набор отвергается
      // целиком, поэтому упереться в него лучше здесь, чем получить отказ на
      // применение и гадать, какая из вставленных строк лишняя.
      if (itog.length > PREDEL_DOMENOV) {
        setNotice(`Правил сайтов может быть не больше ${PREDEL_DOMENOV}: сейчас ${domains.length}, а в этом вводе ещё ${novye.length}.`);
        return;
      }
      changed = save({
        ...trafik,
        domeny: itog,
      });
      skolko = novye.length;
    }
    setAdding(false);
    // Форма закрылась вместе с полем, в котором стоял курсор. Без этого фокус
    // падал на body, и следующий Tab начинал обход окна заново.
    knopkaDobavit.current?.focus();
    setNotice(
      !changed ? "Такое правило уже есть в списке."
      : skolko > 1 ? `${skolko} ${slovoPosleChisla(skolko, "правило", "правила", "правил")} добавлено в черновик. Нажми «Применить изменения», когда закончишь редактирование.`
      : "Правило добавлено в черновик. Нажми «Применить изменения», когда закончишь редактирование.",
    );
  };

  // Что включено на вкладке, видно НЕ ЗАХОДЯ на неё. Прежде счёт был только у
  // приложений и сайтов, а «российские сайты напрямую» не показывал вообще
  // никто: чтобы узнать про них, надо было догадаться открыть «Сайты».
  const ruSpisokVkl = !bezRu && !status.kill_switch;
  // Tabs count visible entries; the service summary separately counts configured
  // routes and explicit overrides. Neither counter claims a live network result.
  const servisovCherezVPN = katalog.filter(
    (s) => (services.find((r) => r.id === s.id)?.marshrut ?? trafik.po_umolchaniyu) === "vpn",
  ).length;
  const servisovYavno=katalog.filter(s=>services.some(r=>r.id===s.id)).length;
  const pereopredeleniya=new Map(katalog.map(s=>{
    const route=services.find(r=>r.id===s.id)?.marshrut ?? trafik.po_umolchaniyu;
    return [s.id,perekrytiyaServisa(s.domeny,domains,route)];
  }));
  const perekryvayushchihDomenov=new Set([...pereopredeleniya.values()].flat().map(d=>d.domen)).size;
  const VKLADKI: { v: typeof tab; podpis: string; schyot: number; poyasnenie?: string; vklyucheno?: string }[] = [
    {
      v: "services",
      podpis: "Сервисы",
      schyot: katalog.length,
      poyasnenie: `${katalog.length} сервисов в каталоге`,
    },
    { v: "apps", podpis: "Приложения", schyot: apps.length, poyasnenie: `${apps.length} правил приложений` },
    {
      v: "sites",
      podpis: "Сайты",
      schyot: domains.length,
      poyasnenie: `${domains.length} правил сайтов`,
      vklyucheno: ruSpisokVkl ? "Российские сайты идут напрямую" : undefined,
    },
    // Только у службы, которая знает блокировку: у прошлой версии вкладка
    // обещала бы то, чего служба не сделает.
    ...(reklama !== undefined ? [{
      v: "reklama" as const,
      podpis: "Реклама",
      schyot: reklama.razresheno.length,
      poyasnenie: `${reklama.razresheno.length} ${slovoPosleChisla(reklama.razresheno.length, "исключение", "исключения", "исключений")}`,
      vklyucheno: reklama.vkl ? "Реклама и трекеры блокируются" : undefined,
    }] : []),
  ];

  return (
    <section className="flex min-h-0 flex-1" aria-label="Правила">
      <aside className="border-border flex w-[288px] shrink-0 flex-col gap-4 overflow-y-auto border-r px-6 py-6">
        <h2 className="text-foreground text-[22px] font-semibold leading-tight">Куда идёт трафик</h2>
        <VyborTrafika
          value={trafik.po_umolchaniyu}
          disabled={disabled}
          onChange={(value) => save({ ...trafik, po_umolchaniyu: value })}
        />
        <p className="text-fg-muted text-sm leading-relaxed">
          {/* Под защитой это место говорит и про неё, и про спящие правила:
              двумя абзацами в одной колонке (второй стоял в самом низу) одно
              и то же читалось как два разных сообщения. */}
          {status.kill_switch?(pryamyeSpyat
            ?"Защита сети включена: весь трафик идёт через VPN. Прямые маршруты сейчас не действуют и заработают снова после её выключения"
            :"Защита сети включена: весь трафик идёт через VPN"):trafik.po_umolchaniyu === "vpn"
            ? "VPN для всего интернета. Добавь приложения и сайты, которые должны работать напрямую"
            : "Прямой интернет. Через VPN идёт только то, что добавлено в правила"}
        </p>
        <div className="border-border mt-1 border-t pt-4" data-testid="svodka-pravil">
          <p className="flex items-baseline gap-2">
            <span className="text-accent-ink text-[26px] font-semibold leading-none">{ruleCount}</span>
            <span className="text-fg-secondary text-sm">
              {slovoPosleChisla(ruleCount, "отдельное правило", "отдельных правила", "отдельных правил")}
            </span>
          </p>
          <p className="text-fg-muted mt-2 text-[13px] leading-relaxed">
            Отдельные правила сохраняются при смене режима
          </p>
          {/* Сеткой, а не flex-wrap: три пары в 240 мм колонки не помещались, и
              «Сервисы» переносились на вторую строку, ломая ряд. Числа стоят
              своей колонкой и выровнены, поэтому читаются столбиком. */}
          <dl className="text-fg-muted mt-2 grid grid-cols-[1fr_auto] gap-x-3 gap-y-1 text-[13px] leading-relaxed">
            <dt>Приложения</dt>
            <dd className="text-fg-secondary tabular-nums">{apps.length}</dd>
            <dt>Сайты</dt>
            <dd className="text-fg-secondary tabular-nums">{domains.length}</dd>
            <dt>Сервисы</dt>
            <dd className="text-fg-secondary tabular-nums">{services.length}</dd>
          </dl>
        </div>
        {status.sostoyanie === "vyklyuchen" && (
          <p className="text-fg-muted text-[13px] leading-relaxed">
            Правила начнут работать после нажатия на сферу
          </p>
        )}
        {(zanyato || applying) && (
          <p role="status" className="text-fg-secondary text-[13px]">Применяю правила</p>
        )}
        {dirty && <div className="border-border flex flex-col gap-3 border-t pt-4" aria-label="Несохранённые правила">
          <p className="text-fg-secondary text-sm">Есть неприменённые изменения</p>
          {conflict ? <p role="alert" className="text-warn text-[13px]">Сохранённые правила изменились. Отмени черновик и проверь актуальный набор перед редактированием.</p>
            : <p className="text-fg-muted text-[13px]">{status.sostoyanie === "vyklyuchen" ? "Весь набор сохранится одним действием." : "Применение всего набора вызовет одно короткое переподключение VPN."}</p>}
          <Knopka rang="glavnaya" aktiven={!disabled && !conflict} zhdyot={applying} onClick={() => void apply()}>Применить изменения</Knopka>
          <Knopka rang="vtoraya" aktiven={!disabled} onClick={() => {setDraft(null);setPath("");setDomain("");setAdding(false);setApplyError("");setNotice("Черновик отменён");}}>Отменить изменения</Knopka>
        </div>}
        {notice && <p role="status" className="text-fg-secondary text-[13px]">{notice}</p>}
        {applyError && <p role="alert" className="text-danger text-[13px]">{applyError}</p>}
        {pravila?.trebuet_podyoma && (
          <p role="status" className="text-warn text-[13px] leading-relaxed">
            Сохранено. Переподключись, чтобы правила заработали
          </p>
        )}
      </aside>

      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto">
        <nav role="tablist" aria-label="Вид правил" className="border-border flex shrink-0 items-stretch gap-1 border-b px-8">
          {VKLADKI.map(({ v, podpis, schyot, poyasnenie, vklyucheno }, nomer) => {
            const on = v === tab;
            return (
              <button
                key={v}
                ref={(el) => { knopkiVkladok.current[nomer] = el; }}
                type="button"
                role="tab"
                aria-selected={on}
                // В порядок Tab попадает только выбранная вкладка: иначе путь с
                // клавиатуры до содержимого правил шёл через все три.
                tabIndex={on ? 0 : -1}
                onClick={() => changeTab(v)}
                onKeyDown={(e) => {
                  const kuda = sleduyushchayaVkladka(e.key, nomer, VKLADKI.length);
                  if (kuda === null) return;
                  e.preventDefault();
                  changeTab(VKLADKI[kuda].v);
                  knopkiVkladok.current[kuda]?.focus();
                }}
                className={`relative flex h-12 items-center gap-2 px-3 text-sm font-medium transition-colors ${
                  on ? "text-foreground" : "text-fg-muted hover:text-fg-secondary"
                }`}
              >
                {podpis}
                {/* Число элементов на вкладке, а не число соединений через VPN. */}
                {schyot > 0 && (
                  <span
                    title={poyasnenie}
                    className={`text-[13px] font-normal ${on ? "text-accent-ink" : "text-fg-faint"}`}
                  >
                    {schyot}
                  </span>
                )}
                {/* Точка значит «здесь включено то, чего в счёте правил нет».
                    Она названа словами, а не оставлена загадкой: имя читает и
                    подсказка, и чтение с экрана. */}
                {vklyucheno && (
                  <span
                    className="bg-accent-ink h-1.5 w-1.5 shrink-0 rounded-full"
                    data-testid={`vklyucheno-${v}`}
                    title={vklyucheno}
                    aria-label={vklyucheno}
                    role="img"
                  />
                )}
                {on && <span aria-hidden className="bg-accent-ink absolute inset-x-0 bottom-0 h-[2px] rounded-t" />}
              </button>
            );
          })}
        </nav>

        <div className="min-w-0 flex-1 px-8 py-6">
          {tab === "services" && (
            <div className="flex flex-col gap-5">
              <header>
                <h3 className="text-foreground text-[17px] font-semibold leading-tight">Популярные сервисы</h3>
                <p className="text-fg-muted mt-1 text-[13px]">Маршрут сайтов сервиса и его приложения. «По общему режиму» следует настройке слева.</p>
              </header>
              {(katalog.length>0 || services.length>0) && <div className="flex flex-wrap items-center justify-between gap-3">
                {katalog.length>0 && <div className="text-fg-muted text-[13px]" aria-label="Маршруты сервисов">
                  <p>По настройкам: через VPN {servisovCherezVPN}, напрямую {katalog.length-servisovCherezVPN}</p>
                  <p>Заданы отдельно: {servisovYavno} · по общему режиму: {katalog.length-servisovYavno}</p>
                  {perekryvayushchihDomenov>0 && <p className="text-warn">Другой маршрут задан в правилах сайтов: {perekryvayushchihDomenov}</p>}
                </div>}
                <Knopka rang="vtoraya" aktiven={!disabled && services.length>0} onClick={()=>save({...trafik,servisy:[]})}>Сбросить маршруты сервисов</Knopka>
              </div>}

              {katalog.length === 0 && (
                // Пустой каталог рисовал пустоту: заголовок, подпись и полэкрана
                // ничего. Служба старее окна не шлёт каталог вовсе, и человек
                // видел сломанную вкладку вместо объяснения.
                <div className="border-border flex flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-10 text-center" data-testid="net-katalog">
                  <span className="text-foreground text-sm font-medium">Каталог сервисов не пришёл</span>
                  <span className="text-fg-muted max-w-[440px] text-[13px] leading-relaxed">
                    Список готовых наборов доменов обновляется вместе с программой.
                    Правила приложений и сайтов работают и без него
                  </span>
                </div>
              )}

              <div className="grid grid-cols-1 gap-3 min-[1100px]:grid-cols-2">
                {katalog.map((service) => {
                  const explicit = services.find((r) => r.id === service.id);
                  const effective = explicit?.marshrut ?? trafik.po_umolchaniyu;
                  const vkl = effective === "vpn";
                  const otkryt = raskryto[service.id] ?? false;
                  // Клиент сервиса. Имена файлов знает каталог, путь - только
                  // машина, поэтому он ищется среди запущенных программ.
                  const estKlient = (service.programmy?.length ?? 0) > 0;
                  const putiKlienta = naydennyePuti(service.programmy, zapushchennye);
                  const klientVPravile = explicit?.programmy?.length ?? 0;
                  // Карточка без доменов держится на одном клиенте. Пока его не
                  // запустили, выбирать ей маршрут нечему: правило вышло бы
                  // пустым, а на экране выглядело бы работающим.
                  const nechemuMarshrut = estKlient && service.domeny.length === 0
                    && putiKlienta.length === 0 && klientVPravile === 0;
                  return (
                    <article key={service.id} className="border-border bg-surface flex flex-col overflow-hidden rounded-xl border">
                      <div className="flex flex-wrap items-center gap-3 px-4 py-3.5">
                        <span className={`border-border flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border ${vkl ? "text-foreground" : "text-fg-faint"}`}>
                          <IkonkaServisa id={service.id} imya={service.imya} />
                        </span>
                        <span className="flex min-w-[80px] flex-1 flex-col gap-0.5">
                          <span className="text-foreground truncate text-sm font-medium">{service.imya}</span>
                          <span className="text-fg-muted truncate text-[13px]">
                            {vkl ? "Через VPN" : "Напрямую"}
                          </span>
                        </span>
                        <Vybor<VyborServisa> label={`Маршрут сервиса ${service.imya}`} value={explicit?.marshrut ?? "inherit"} disabled={disabled || nechemuMarshrut}
                          options={[{value:"inherit",label:"По общему режиму"},{value:"vpn",label:"Через VPN"},{value:"direct",label:"Напрямую"}]}
                          onChange={value=>save(zadatMarshrutServisa(trafik,service.id,value,putiKlienta))}/>
                      </div>
                      {/* Строка про клиента говорит ровно тогда, когда человеку
                          есть что с ней делать. На карточке, которую он не
                          включал, предупреждение это шум: оранжевая строка на
                          каждой второй карточке перестаёт читаться вовсе. */}
                      {estKlient && explicit && klientVPravile===0 && (
                        <p className="text-warn px-4 pb-2 text-[12px]" data-testid={`klient-${service.id}`}>
                          {putiKlienta.length>0
                            ? "Приложение запущено: выбери маршрут заново, и правило накроет его тоже"
                            : "Приложение не запущено: правило накроет только сайты"}
                        </p>
                      )}
                      {estKlient && klientVPravile>0 && (
                        <p className="text-fg-muted px-4 pb-2 text-[12px]" data-testid={`klient-${service.id}`}>
                          Правило накрывает и приложение, и программы, которые оно запускает
                        </p>
                      )}
                      {estKlient && !explicit && nechemuMarshrut && (
                        <p className="text-fg-muted px-4 pb-2 text-[12px]" data-testid={`klient-${service.id}`}>
                          {/* Одной строкой: у лаунчеров эта подпись стоит на
                              четырёх карточках подряд, и длинная фраза
                              превращала ряд в простыню одинакового текста. */}
                          Запусти приложение, чтобы выбрать маршрут
                        </p>
                      )}
                      {(pereopredeleniya.get(service.id)?.length ?? 0)>0 && <p className="text-warn px-4 pb-2 text-[12px]">Есть другой маршрут в правилах сайтов</p>}

                      {/* mt-auto прижимает строку к низу: соседка по ряду
                          растягивает карточку предупреждением, и без этого
                          разделитель вставал посреди пустоты. */}
                      <button
                        type="button"
                        aria-expanded={otkryt}
                        onClick={() => setRaskryto({ ...raskryto, [service.id]: !otkryt })}
                        className="group border-border hover:bg-surface-hover mt-auto flex items-center gap-2 border-t px-4 py-2.5 text-left transition-colors"
                      >
                        <IkTreugolnik className={`text-fg-muted h-3 w-3 shrink-0 transition-transform ${otkryt ? "rotate-90" : ""}`} />
                        <span className="text-fg-muted group-hover:text-fg-secondary text-[13px]">
                          {service.domeny.length>0
                            ? `${service.domeny.length} ${slovoPosleChisla(service.domeny.length, "домен", "домена", "доменов")} с поддоменами`
                            : "Только приложение, без сайтов"}
                          {estKlient && service.domeny.length>0 ? " и приложение" : ""}
                        </span>
                      </button>

                      {otkryt && (
                        <ul className="border-border flex flex-col gap-1 border-t px-4 py-3">
                          {/* Пути, а не имена файлов: человек видит, какая
                              именно копия программы накрыта, и замечает чужую. */}
                          {(explicit?.programmy ?? putiKlienta).map((put) => (
                            <li key={put} className="text-fg-secondary break-all text-[13px]" title={put}>{sokratitPut(put)}</li>
                          ))}
                          {service.domeny.map((d) => (
                            <li key={d} className="text-fg-secondary break-all text-[13px]">{d}</li>
                          ))}
                          {/* Пустой раскрытый список читается как поломка. Так
                              выглядит карточка без сайтов, клиент которой не
                              запущен: показывать нечего, и это надо сказать. */}
                          {service.domeny.length===0 && (explicit?.programmy ?? putiKlienta).length===0 && (
                            <li className="text-fg-muted text-[13px]">Путь к приложению появится, когда оно будет запущено</li>
                          )}
                          {pereopredeleniya.get(service.id)?.map(d=><li key={`override-${d.domen}`} className="text-warn break-all text-[12px]">Правило {d.domen}: {imyaMarshruta(d.marshrut).toLowerCase()}</li>)}
                        </ul>
                      )}
                    </article>
                  );
                })}
              </div>

              <p className="text-fg-muted text-[13px]">
                Здесь задан маршрут сайтов сервиса и его приложения. Своё правило приложения или отдельного сайта может его изменить.
              </p>
              <p className="text-fg-muted text-[13px]">
                Своё приложение, которого здесь нет? Добавь его во вкладке «Приложения»
              </p>

              <div className="flex flex-col">
                <Svorachivaemyy
                  zagolovok="Источник и особенности правил"
                  deti={
                    <div className="flex flex-col gap-2">
                      <p>
                        Каталог OpenCCK, версия {pravila?.katalog?.versiya ?? "неизвестна"}. Обновляется
                        вместе с программой. Правило накрывает домен и все его поддомены.
                      </p>
                      <p>
                        Сначала действует правило приложения, затем отдельного сайта, затем сервиса,
                        затем общий режим. Программы, которые ходят через прокси Affory, идут через VPN раньше твоих правил.
                        Проверить приложение вместе с сайтом можно на вкладках «Приложения» и «Сайты».
                      </p>
                      <p>
                        Некоторые браузеры прячут имя сайта, например с включённым безопасным DNS. Для такого случая
                        добавь приложение целиком во вкладке «Приложения».
                      </p>
                    </div>
                  }
                />
                <Svorachivaemyy
                  zagolovok="Дополнительно"
                  deti={
                    <div className="flex flex-col gap-3">
                      <div className="flex flex-wrap gap-2">
                        <Knopka rang="vtoraya" onClick={() => setVesSpisok(!vesSpisok)}>
                          {vesSpisok ? "Скрыть весь список доменов" : "Показать весь список доменов"}
                        </Knopka>
                      </div>
                      {vesSpisok && (
                        <ul className="border-border bg-elevated flex flex-col gap-1 rounded-lg border px-4 py-3">
                          {katalog.flatMap((s) => s.domeny.map((d) => ({ d, s }))).map(({ d, s }) => {
                            const cherez = (services.find((r) => r.id === s.id)?.marshrut ?? trafik.po_umolchaniyu) === "vpn";
                            return (
                              <li key={`${s.id}-${d}`} className="flex items-center justify-between gap-4 text-[13px]">
                                <span className="text-fg-secondary">{d}</span>
                                <span className="text-fg-muted">{cherez ? "через VPN" : "напрямую"}</span>
                              </li>
                            );
                          })}
                        </ul>
                      )}
                    </div>
                  }
                />
              </div>
            </div>
          )}

          {tab === "reklama" && reklama && pravila?.reklama && (
            <Reklama
              reklama={reklama}
              sohranyonnoe={pravila.reklama}
              status={status}
              disabled={disabled}
              naSmenu={(next) => save(trafik, bezRu, next)}
              soobshchit={setNotice}
            />
          )}

          {(tab === "apps" || tab === "sites") && (
            <div className="flex flex-col gap-5">
              {tab === "sites" && (
                <div className="border-border bg-surface flex items-center gap-4 rounded-xl border px-4 py-3" aria-label="Российские сайты">
                  <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="text-foreground text-sm font-medium">Российские сайты напрямую</span>
                    <span className="text-fg-muted text-[13px] leading-relaxed">
                      {/* Строка про «чтобы включить» показывалась и на ВКЛЮЧЁННОМ
                          списке: под блокировкой он не выключен, а не действует,
                          и звать включать уже включённое значит врать. */}
                      {status.kill_switch
                        ? "Список не действует, пока включена защита сети. Выключить её можно в настройках, раздел «Защита»"
                        : "Сайты из российского списка открываются без VPN, если нет более важного правила"}
                    </span>
                  </span>
                  <Tumbler
                    testId="ru-spisok"
                    podpis="Российские сайты напрямую"
                    aktiven={!disabled && !status.kill_switch}
                    vkl={!bezRu}
                    naSmenu={(v) => save(trafik, !v)}
                  />
                </div>
              )}

              <header className="flex items-start justify-between gap-4">
                <div>
                  <h3 className="text-foreground text-[17px] font-semibold leading-tight">
                    {tab === "apps" ? "Правила приложений" : "Правила сайтов"}
                  </h3>
                  <p className="text-fg-muted mt-1 text-[13px]">
                    {tab === "apps"
                      ? "Приложение и программы, которые оно запускает"
                      : "Домен и все его поддомены"}
                  </p>
                </div>
                <Knopka
                  rang="glavnaya"
                  bolshaya
                  priv={knopkaDobavit}
                  aria-expanded={adding}
                  aktiven={!disabled}
                  onClick={() => {
                    pickerEpoch.current++;
                    setPickerError("");
                    setPicking(false);
                    if (!adding) setRoute(trafik.po_umolchaniyu === "vpn" ? "direct" : "vpn");
                    setAdding(!adding);
                  }}
                >
                  <IkPlyus className="h-4 w-4" />
                  {adding ? "Закрыть" : "Добавить"}
                </Knopka>
              </header>

              {adding && (
                // Настоящая форма, а не набор полей: Enter в поле пути или
                // домена добавляет правило в черновик, как ждёт любой, кто
                // заполнял поле с клавиатуры. Вводит только черновик, поэтому
                // случайный Enter ничего не применяет.
                <form
                  onSubmit={(e) => { e.preventDefault(); add(); }}
                  className="border-border bg-surface flex flex-col gap-3 rounded-xl border p-4">
                  {tab === "apps" ? (
                    <>
                      <div className="flex items-center gap-3">
                        <Poisk
                          aria-label="Поиск приложения"
                          priv={pervoePoleFormy}
                          znachenie={query}
                          naVvod={setQuery}
                          placeholder="Найти среди запущенных"
                        />
                        {/* Список запрашивается заново и по открытию формы, и
                            по возврату фокуса в окно. Кнопка тут для случая,
                            когда программу запустили при открытой форме: без
                            неё оставалось только закрыть форму и открыть. */}
                        <Knopka
                          rang="vtoraya"
                          zhdyot={protsessyChitayutsya}
                          aktiven={!disabled && !!obnovitProtsessy}
                          onClick={() => obnovitProtsessy?.()}
                        >
                          {protsessyChitayutsya ? "Читаю" : "Обновить"}
                        </Knopka>
                      </div>
                      {/* Отказ при списке на руках: снимок остаётся, но он
                          больше не выдаётся за свежий. */}
                      {protsessyOtkaz !== "" && spisokEst && (
                        <p role="status" className="text-warn text-[13px]">
                          Список мог устареть, обновить его не удалось: {protsessyOtkaz}
                        </p>
                      )}
                      <div className="border-border flex max-h-[220px] flex-col overflow-y-auto rounded-lg border" aria-label="Запущенные приложения">
                        {candidates.map((p) => (
                          <button
                            type="button"
                            key={p.put}
                            aria-label={`${p.imya}, ${p.put}`}
                            aria-pressed={path === p.put}
                            onClick={() => setPath(p.put)}
                            className={`flex min-w-0 flex-col gap-0.5 px-3 py-2 text-left transition-colors ${
                              path === p.put ? "bg-accent-soft" : "hover:bg-surface-hover"
                            }`}
                          >
                            <span className="text-foreground truncate text-sm font-medium">{p.imya}</span>
                            <span className="text-fg-muted truncate text-[13px]" title={p.put}>{sokratitPut(p.put)}</span>
                          </button>
                        ))}
                        {sostoyanieSpiska === "chitaetsya" && (
                          <p role="status" className="text-fg-muted px-3 py-6 text-center text-[13px]">
                            Читаю запущенные программы
                          </p>
                        )}
                        {sostoyanieSpiska === "otkaz" && (
                          <div role="alert" className="flex flex-col items-center gap-2 px-3 py-6 text-center">
                            <p className="text-danger text-[13px]">
                              Не удалось прочитать запущенные программы: {protsessyOtkaz}
                            </p>
                            <p className="text-fg-muted text-[13px]">
                              Выбери приложение на ПК или укажи путь ниже
                            </p>
                            <Knopka
                              rang="vtoraya"
                              zhdyot={protsessyChitayutsya}
                              aktiven={!disabled && !!obnovitProtsessy}
                              onClick={() => obnovitProtsessy?.()}
                            >
                              Повторить
                            </Knopka>
                          </div>
                        )}
                        {sostoyanieSpiska === "pusto" && (
                          <p className="text-fg-muted px-3 py-6 text-center text-[13px]">
                            Запущенных программ не видно. Выбери приложение на ПК или укажи путь ниже
                          </p>
                        )}
                        {sostoyanieSpiska === "bezSovpadeniy" && (
                          <p className="text-fg-muted px-3 py-6 text-center text-[13px]">
                            Совпадений нет. Выбери приложение на ПК или укажи путь ниже
                          </p>
                        )}
                        {sostoyanieSpiska === "netOtveta" && (
                          <p className="text-fg-muted px-3 py-6 text-center text-[13px]">
                            Список запущенных программ пока не получен. Выбери приложение на ПК или укажи путь ниже
                          </p>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <Pole
                          aria-label="Путь к приложению"
                          znachenie={path}
                          aktiven={!disabled}
                          naVvod={setPath}
                          placeholder="C:\Program Files\...\app.exe"
                          className="min-w-0 flex-1"
                        />
                        <Knopka
                          rang="vtoraya"
                          zhdyot={picking}
                          aktiven={!disabled && !!naVyborPrilozheniya}
                          onClick={() => void browse()}
                        >
                          {picking ? "Выбор…" : "Выбрать на ПК…"}
                        </Knopka>
                      </div>
                      {pickerError && (
                        <p className="text-danger text-[13px]" role="alert">
                          Не удалось выбрать приложение: {pickerError}
                        </p>
                      )}
                      <Flazhok
                        podpis="И запущенные им программы"
                        vkl={descendants}
                        naSmenu={setDescendants}
                      />
                    </>
                  ) : (
                    <>
                      <Pole
                        aria-label="Домен сайта"
                        priv={pervoePoleFormy}
                        znachenie={domain}
                        naVvod={setDomain}
                        placeholder="example.org, пример.рф - можно адресом и списком"
                      />
                      {domain.trim() !== "" && (
                        <div className="flex flex-col gap-1.5 text-[13px]" data-testid="razbor-domenov">
                          {razborDomenov.gotovye.length > 0 && (
                            <>
                              <p className="text-fg-muted">
                                Добавится {razborDomenov.gotovye.length} {slovoPosleChisla(razborDomenov.gotovye.length, "правило", "правила", "правил")}:
                              </p>
                              <ul className="flex flex-col gap-0.5">
                                {razborDomenov.gotovye.slice(0, POKAZAT_V_PREDPROSMOTRE).map((g) => (
                                  <li key={g.domen} className="text-fg-secondary break-all">
                                    {g.domen}
                                    {/* Набранное показывается рядом только когда
                                        разошлось с тем, что уедет в правило. */}
                                    {g.ishodnyy.toLowerCase() !== g.domen && (
                                      <span className="text-fg-muted"> · набрано {g.ishodnyy}</span>
                                    )}
                                    {domains.some((d) => d.domen === g.domen) && (
                                      <span className="text-warn"> · заменит прежний маршрут</span>
                                    )}
                                  </li>
                                ))}
                              </ul>
                              {razborDomenov.gotovye.length > POKAZAT_V_PREDPROSMOTRE && (
                                <p className="text-fg-muted">
                                  и ещё {razborDomenov.gotovye.length - POKAZAT_V_PREDPROSMOTRE}
                                </p>
                              )}
                            </>
                          )}
                          {razborDomenov.otkazy.length > 0 && (
                            <ul className="flex flex-col gap-0.5" role="alert">
                              {razborDomenov.otkazy.slice(0, POKAZAT_V_PREDPROSMOTRE).map((o) => (
                                <li key={o.vvod} className="text-danger break-all">
                                  {o.vvod}: {o.prichina}
                                </li>
                              ))}
                              {razborDomenov.otkazy.length > POKAZAT_V_PREDPROSMOTRE && (
                                <li className="text-danger">
                                  и ещё {razborDomenov.otkazy.length - POKAZAT_V_PREDPROSMOTRE} негодных
                                </li>
                              )}
                            </ul>
                          )}
                        </div>
                      )}
                    </>
                  )}
                  <div className="flex items-center justify-end gap-3">
                    <div className="w-[170px]">
                      <RouteSelect
                        label="Маршрут нового правила"
                        value={route}
                        onChange={setRoute}
                      />
                    </div>
                    <Knopka
                      rang="glavnaya"
                      bolshaya
                      tip="submit"
                      aktiven={
                        !disabled &&
                        (tab === "apps" ? path.trim() !== "" : razborDomenov.gotovye.length > 0)
                      }
                    >
                      Добавить в черновик
                    </Knopka>
                  </div>
                  {status.kill_switch && route==="direct" && <p className="text-fg-muted text-[13px] leading-relaxed">Защита сети включена: правило сохранится, но заработает после её выключения</p>}
                </form>
              )}

              {vsegoNaVkladke > 0 && (
                <div className="flex flex-col gap-3">
                  <div className="flex flex-wrap items-center gap-3">
                    <Poisk
                      aria-label={tab === "apps" ? "Поиск среди правил приложений" : "Поиск среди правил сайтов"}
                      znachenie={poiskPravil}
                      naVvod={(v) => { setPoiskPravil(v); setRemove(null); }}
                      placeholder={tab === "apps" ? "Найти правило по имени или пути" : "Найти правило по имени сайта"}
                    />
                    <div className="w-[190px]">
                      <Vybor<"vse" | Marshrut>
                        label="Показывать маршруты"
                        value={filtrMarshruta}
                        disabled={disabled}
                        options={[
                          { value: "vse", label: "Любой маршрут" },
                          { value: "vpn", label: "Только через VPN" },
                          { value: "direct", label: "Только напрямую" },
                        ]}
                        onChange={(v) => { setFiltrMarshruta(v); setRemove(null); }}
                      />
                    </div>
                  </div>
                  {otfiltrovano && (
                    <p role="status" className="text-fg-muted text-[13px]">
                      {/* Счёт показанного нужен именно при фильтре: иначе «правил
                          три» на вкладке и три строки на экране выглядят как
                          весь набор, хотя остальные просто скрыты. */}
                      Показано {vidimyeKlyuchi.length} из {vsegoNaVkladke}
                      {vidimyeKlyuchi.length === 0 && " · ни одно правило не подходит"}
                    </p>
                  )}
                  {/* Полоса группового действия появляется вместе с первой
                      отметкой: пустая полоса «выбрано 0» занимала бы место
                      всегда и не говорила бы ничего. */}
                  {otmecheno.length > 0 && (
                    <div className="border-border bg-surface flex flex-wrap items-center gap-3 rounded-xl border px-4 py-3" data-testid="gruppovye-deystviya">
                      <span className="text-foreground text-[13px] font-medium">
                        Отмечено {otmecheno.length} из {vidimyeKlyuchi.length}
                      </span>
                      <span className="flex-1" />
                      <Knopka rang="vtoraya" aktiven={!disabled} onClick={() => gruppoy("vpn")}>Через VPN</Knopka>
                      <Knopka
                        rang="vtoraya"
                        aktiven={!disabled && !status.kill_switch}
                        title={status.kill_switch ? "Защита сети включена" : undefined}
                        onClick={() => gruppoy("direct")}
                      >
                        Напрямую
                      </Knopka>
                      <Knopka rang="opasnaya" aktiven={!disabled} onClick={() => gruppoy("udalit")}>Удалить отмеченные</Knopka>
                      <Knopka rang="tekst" aktiven={!disabled} onClick={() => setVybrannye([])}>Снять отметки</Knopka>
                    </div>
                  )}
                </div>
              )}

              {(tab === "apps" ? apps.length : domains.length) === 0 ? (
                <div className="border-border flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-12 text-center">
                  <IkSsylka className="text-fg-faint h-7 w-7" />
                  <p className="text-foreground text-sm font-medium">Пока нет отдельных правил</p>
                  <p className="text-fg-muted max-w-[440px] text-[13px]">
                    Сейчас используется маршрут по умолчанию: {imyaMarshruta(trafik.po_umolchaniyu)}
                  </p>
                </div>
              ) : tab === "apps" ? (
                <div>
                  <div className={`${STROKA_S_VYBOROM} max-[1099px]:hidden border-border text-fg-muted border-b px-3 pb-2.5 text-[13px]`}>
                    <Flazhok
                      podpis="Отметить все показанные правила"
                      golos="Отметить все показанные правила"
                      skrytPodpis
                      aktiven={!disabled && vidimyeKlyuchi.length > 0}
                      vkl={vidimyeKlyuchi.length > 0 && otmecheno.length === vidimyeKlyuchi.length}
                      naSmenu={(v) => setVybrannye(v ? vidimyeKlyuchi : [])}
                    />
                    <div className={SETKA_PRILOZHENIY}>
                      <span>Приложение</span>
                      <span>Запущенные программы</span>
                      <span>Маршрут</span>
                      <span className="text-right">Действие</span>
                    </div>
                  </div>
                  <ul>
                    {vidimyePrilozheniya.map((app) => (
                      <li key={app.put} className={`${STROKA_S_VYBOROM} border-border hover:bg-surface-hover border-b px-3 py-2.5 transition-colors`}>
                        <Flazhok
                          podpis={`Отметить ${app.imya}`}
                          golos={`Отметить ${app.imya}`}
                          skrytPodpis
                          aktiven={!disabled}
                          vkl={vybrannye.includes(app.put)}
                          naSmenu={(v) => setVybrannye(v ? [...vybrannye, app.put] : vybrannye.filter((k) => k !== app.put))}
                        />
                        <div className={SETKA_PRILOZHENIY}>
                        <span className="col-span-3 min-[1100px]:col-span-1 flex min-w-0 flex-col gap-0.5">
                          <span className="text-foreground truncate text-sm font-medium">{app.imya}</span>
                          {/* Длинный путь теряет СЕРЕДИНУ, а не хвост: диск говорит, где
                              файл живёт, имя говорит, что это за файл, а папки посередине
                              человек и так не читает. Целиком в подсказке. */}
                          <span className="text-fg-muted truncate text-[13px]" title={app.put}>{sokratitPut(app.put)}</span>
                        </span>
                        <Flazhok
                          podpis="И запущенные им программы"
                          golos={`И программы, запущенные ${app.imya}`}
                          aktiven={!disabled}
                          vkl={app.potomki}
                          naSmenu={(v) =>
                            save({
                              ...trafik,
                              prilozheniya: apps.map((a) => (a.put === app.put ? { ...a, potomki: v } : a)),
                            })
                          }
                        />
                        <RouteSelect
                          label={`Маршрут ${app.imya}`}
                          value={app.marshrut}
                          disabled={disabled}
                          onChange={(r) =>
                            save({
                              ...trafik,
                              prilozheniya: apps.map((a) => (a.put === app.put ? { ...a, marshrut: r } : a)),
                            })
                          }
                        />
                        <div className="flex justify-end">
                          {/* Глазами строка читается вместе с именем слева, голосом
                              нет: десять «Удалить» подряд не различить, и второе
                              нажатие вслепую стирает не ту строку. */}
                          <Knopka
                            rang={remove === app.put ? "opasnaya" : "tekst"}
                            aria-label={remove === app.put ? `Подтвердить удаление ${app.imya}` : `Удалить правило ${app.imya}`}
                            aktiven={!disabled}
                            onClick={() => {
                              if (remove === app.put) {
                                save({ ...trafik, prilozheniya: apps.filter((a) => a.put !== app.put) });
                                // Строка вместе с кнопкой сейчас исчезнет.
                                knopkaDobavit.current?.focus();
                              } else setRemove(app.put);
                            }}
                          >
                            {remove === app.put ? "Подтвердить" : "Удалить"}
                          </Knopka>
                        </div>
                        </div>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : (
                <div>
                  <div className={`${STROKA_S_VYBOROM} border-border text-fg-muted border-b px-3 pb-2.5 text-[13px]`}>
                    <Flazhok
                      podpis="Отметить все показанные правила"
                      golos="Отметить все показанные правила"
                      skrytPodpis
                      aktiven={!disabled && vidimyeKlyuchi.length > 0}
                      vkl={vidimyeKlyuchi.length > 0 && otmecheno.length === vidimyeKlyuchi.length}
                      naSmenu={(v) => setVybrannye(v ? vidimyeKlyuchi : [])}
                    />
                    <div className={SETKA_SAYTOV}>
                      <span>Домен</span>
                      <span>Маршрут</span>
                      <span className="text-right">Действие</span>
                    </div>
                  </div>
                  <ul>
                    {vidimyeDomeny.map((d) => (
                      <li key={d.domen} className={`${STROKA_S_VYBOROM} border-border hover:bg-surface-hover border-b px-3 py-2.5 transition-colors`}>
                        <Flazhok
                          podpis={`Отметить ${d.domen}`}
                          golos={`Отметить ${d.domen}`}
                          skrytPodpis
                          aktiven={!disabled}
                          vkl={vybrannye.includes(d.domen)}
                          naSmenu={(v) => setVybrannye(v ? [...vybrannye, d.domen] : vybrannye.filter((k) => k !== d.domen))}
                        />
                        <div className={SETKA_SAYTOV}>
                        <span className="flex min-w-0 items-center gap-3">
                          <span className="border-border bg-elevated text-fg-muted flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border">
                            <IkSayt className="h-4 w-4" />
                          </span>
                          <span className="flex min-w-0 flex-col gap-0.5">
                            {/* Обрезка плюс подсказка: домен длиной в две сотни знаков
                                не должен ни выезжать за колонку, ни пропадать насовсем. */}
                            <span className="text-foreground truncate text-sm font-medium" title={d.domen}>{d.domen}</span>
                            <span className="text-fg-muted text-[13px]">Включая поддомены</span>
                          </span>
                        </span>
                        <RouteSelect
                          label={`Маршрут ${d.domen}`}
                          value={d.marshrut}
                          disabled={disabled}
                          onChange={(r) =>
                            save({
                              ...trafik,
                              domeny: domains.map((v) => (v.domen === d.domen ? { ...v, marshrut: r } : v)),
                            })
                          }
                        />
                        <div className="flex justify-end">
                          <Knopka
                            rang={remove === d.domen ? "opasnaya" : "tekst"}
                            aria-label={remove === d.domen ? `Подтвердить удаление ${d.domen}` : `Удалить правило ${d.domen}`}
                            aktiven={!disabled}
                            onClick={() => {
                              if (remove === d.domen) {
                                save({ ...trafik, domeny: domains.filter((v) => v.domen !== d.domen) });
                                knopkaDobavit.current?.focus();
                              } else setRemove(d.domen);
                            }}
                          >
                            {remove === d.domen ? "Подтвердить" : "Удалить"}
                          </Knopka>
                        </div>
                        </div>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              {/* Разделы идут одним сплошным списком: gap-5 родителя раздвигал
                  одинаковые полосы, и низ экрана читался как пустой. */}
              <div className="flex flex-col">
              <OhvatPravil key={tab} vid={tab} trafik={trafik} katalog={pravila?.katalog} chernovik={dirty} ozhidayut={pravila?.trebuet_podyoma} bezRu={bezRu} killSwitch={status.kill_switch===true} reklamaVkl={reklama?.vkl===true} disabled={disabled} proverit={proveritSoedineniya} proveritPrilozhenie={proveritPrilozhenie} vybratFayl={naVyborPrilozheniya} zamenit={(oldPath,newPath)=>{
                const key=newPath.trim().toLowerCase();
                if(apps.some(a=>a.put!==oldPath && a.put.toLowerCase()===key))throw new Error("Для этого файла уже есть правило. Измени его в списке приложений.");
                return save({...trafik,prilozheniya:apps.map(a=>a.put===oldPath?{...a,put:newPath.trim(),imya:newPath.trim().split(/[/\\]/).pop() || a.imya}:a)});
              }} naSbros={()=>save(tab==="apps"?{...trafik,prilozheniya:[]}:{...trafik,domeny:[]})}/>
                {tab === "apps" && (
                  <Svorachivaemyy
                    zagolovok="Как работает правило приложения"
                    deti={
                      <div className="flex flex-col gap-2">
                        <p>
                          Под правило попадает выбранный файл и всё, что он запускает, включая
                          запущенное уже после включения VPN: лаунчер уводит за собой игру, а
                          браузер свои вкладки.
                        </p>
                        <p>
                          Программа, открытая отдельно, под это правило не попадёт. Affory запоминает
                          запуски, пока работает служба: переподключение VPN эти сведения не стирает.
                          Если программа успела закрыться до запуска службы, сведений о её запуске
                          может не быть. В таком случае добавь саму игру или приложение отдельным правилом.
                        </p>
                        <p>
                          Если хотя бы одно приложение направлено в VPN, Affory по умолчанию ищет
                          адреса сайтов через VPN: Windows не сообщает, какое приложение спросило адрес.
                          Отдельные правила сайтов и сервисов, локальные имена и российский список проверяются раньше.
                          Поэтому маршрут соединения и поиск адреса могут различаться.
                        </p>
                        {/* Абзац из бывшего раздела «Дополнительно»: он был
                            продолжением этого же объяснения, а отдельной полосой
                            только удлинял частокол. */}
                        <p>
                          Правило по пути, а не по имени: две копии одного файла из разных папок это
                          два разных правила. Переименованный или перенесённый файл под правило
                          перестаёт попадать, путь придётся указать заново.
                        </p>
                      </div>
                    }
                  />
                )}
                {tab === "sites" && (
                  <Svorachivaemyy
                    zagolovok="Как работает правило сайта"
                    deti={
                      <p>
                        Правило накрывает домен и все его поддомены: example.com действует и на
                        api.example.com. Программа, которая обращается к адресу без имени, под правило
                        сайта не попадает, для неё есть вкладка «Приложения».
                      </p>
                    }
                  />
                )}
              </div>
            </div>
          )}

        </div>
      </div>
    </section>
  );
}
