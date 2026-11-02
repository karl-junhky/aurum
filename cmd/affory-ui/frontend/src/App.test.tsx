import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { readFile } from "node:fs/promises";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StatusOtvet } from "./protokol";

// The shell had no test at all: nine screens were covered and the wiring that
// holds them was not. Everything below drives App through a fake bridge, so a
// refusal that never reaches the screen fails here instead of in a guest VM.

const stend = vi.hoisted(() => {
  class KanalNedostupen extends Error {
    constructor(prichina: string) {
      super(prichina);
      this.name = "KanalNedostupen";
    }
  }

  interface Otkaz {
    kod: string;
    tekst: string;
  }

  const s = {
    zhiv: true,
    zaderzhkaMs: 0,
    status: { sostoyanie: "vyklyuchen" } as StatusOtvet,
    schet: new Map<string, number>(),
    otkazy: new Map<string, Otkaz>(),
    podpischiki: [] as ((kadr: unknown) => void)[],
    /** Наблюдатели видимости окна: Go сообщает сюда «свернулось / развернулось». */
    nablyudateliOkna: [] as ((vidno: boolean) => void)[],
    /** Наблюдатели вкладки: трей просит окно открыть названную. */
    nablyudateliVkladki: [] as ((vkladka: string) => void)[],
    sluzhbaEst: true,
    /** Non-null makes tekstBufera throw instead of answering "". */
    bufer: null as string | null,
    buferLomaetsya: false,
    protsessyLomayutsya: false,
    protsessy: [] as { imya: string; put: string }[],
    /** Ответы списка программ придерживаются до прямой отдачи: без этого
     *  поздний ответ прошлого запроса в тесте не воспроизвести вовсе. */
    protsessyDerzhatsya: false,
    protsessyOchered: [] as ((spisok: { imya: string; put: string }[]) => void)[],
    arhiv: "" as string,
    arhivLomaetsya: false,
    pravaLomayutsya: null as string | null,
    /** Сколько раз окно просило перезапустить себя после обновления. */
    perezapuskov: 0,
    pravaProsili: "",
    startovaya: "",
    /** Commands whose answer is not a frame at all: most.ts throws a plain
     *  Error, which is neither a refusal frame nor a dead pipe. */
    bityeKadry: new Set<string>(),
    /** Bodies of the answers, per command, when the default is not enough. */
    tela: new Map<string, unknown>(),
    /** What every command was called with, last call wins. */
    poslannoe: new Map<string, unknown>(),
    /** Every bridge call with its arguments. The profile password must show
     *  up in exactly one entry and nowhere else. */
    sled: [] as { chto: string; args: unknown[] }[],
    /** Строки журнала окна (1.10.0), отдельно от sled: журнал пишется на
     *  каждый показанный отказ, и в sled он мешал бы сверке вызовов. */
    zhurnalOkna: [] as string[],
    putSohraneniya: "",
    putZagruzki: "",
    soderzhimoeFayla: "",
    zapisano: null as { put: string; profil: string } | null,
    /** Подписка на статистику, как её видит служба: применяется в момент
     *  ОТВЕТА, а не отправки. Служба выполняет каждую команду соединения своим
     *  потоком (obsluzhivanie.go), и порядок ответов не обязан совпадать с
     *  порядком отправки. */
    podpiskaSluzhby: null as boolean | null,
    /** Задержки ответов subscribeStats по порядку вызова; кончились - сразу. */
    zaderzhkiPodpiski: [] as number[],
  };

  function telo(imya: string): unknown {
    switch (imya) {
      case "status":
      case "connect":
      case "disconnect":
      case "setRouteMode":
        return { ...s.status };
      case "hello":
        return { pozzhe: {} };
      case "listServers":
        return { servery: [], vybran: "", podpiska_zadana: false, podpiska_uzel: "" };
      case "listRules":
        // Набор маршрутов служба отдаёт ВСЕГДА, с 1.0.0: пустой ответ без
        // него означал бы службу старее окна, и проба рисовала бы экран
        // «правила не прочитаны» там, где в жизни рисуется рабочий.
        return {
          protsessy: [],
          domeny: [],
          trafik: { po_umolchaniyu: "vpn", prilozheniya: [], domeny: [], servisy: [] },
        };
      default:
        return {};
    }
  }

  async function zvat(imya: string, poslano?: unknown): Promise<unknown> {
    s.schet.set(imya, (s.schet.get(imya) ?? 0) + 1);
    s.poslannoe.set(imya, poslano);
    s.sled.push({ chto: "zvat " + imya, args: [poslano] });
    if (imya === "subscribeStats") {
      const zaderzhka = s.zaderzhkiPodpiski.shift() ?? 0;
      if (zaderzhka > 0) await new Promise((r) => setTimeout(r, zaderzhka));
      if (s.zhiv) s.podpiskaSluzhby = (poslano as { vkl?: boolean } | undefined)?.vkl ?? null;
    }
    if (s.zaderzhkaMs > 0) await new Promise((r) => setTimeout(r, s.zaderzhkaMs));
    if (!s.zhiv) throw new KanalNedostupen("канал закрыт");
    if (s.bityeKadry.has(imya)) throw new Error("кадр не разбирается");
    const o = s.otkazy.get(imya);
    if (o) return { tip: "otvet", id: 1, imya, oshibka: { ...o } };
    if (imya === "getServerHealth") {
      // No subscription in the probe, so the service has no snapshot file.
      return { tip: "otvet", id: 1, imya, oshibka: { kod: "health-snapshot-missing", tekst: "снимка нет" } };
    }
    return { tip: "otvet", id: 1, imya, telo: s.tela.get(imya) ?? telo(imya) };
  }

  return { KanalNedostupen, s, zvat };
});

vi.mock("./most", () => ({
  KanalNedostupen: stend.KanalNedostupen,
  zvat: stend.zvat,
  naSobytie: (obrabotchik: (kadr: unknown) => void) => {
    stend.s.podpischiki.push(obrabotchik);
    return () => {
      stend.s.podpischiki = stend.s.podpischiki.filter((p) => p !== obrabotchik);
    };
  },
  naVidimostOkna: (obrabotchik: (vidno: boolean) => void) => {
    stend.s.nablyudateliOkna.push(obrabotchik);
    return () => {
      stend.s.nablyudateliOkna = stend.s.nablyudateliOkna.filter((n) => n !== obrabotchik);
    };
  },
  naVkladku: (obrabotchik: (vkladka: string) => void) => {
    stend.s.nablyudateliVkladki.push(obrabotchik);
    return () => {
      stend.s.nablyudateliVkladki = stend.s.nablyudateliVkladki.filter((n) => n !== obrabotchik);
    };
  },
  oknoSvernut: () => undefined,
    oknoRazvernut: () => undefined,
  oknoZakryt: () => undefined,
  sluzhbaUstanovlena: async () => stend.s.sluzhbaEst,
  ustanovitSluzhbu: async () => undefined,
  udalitProgrammu: async () => undefined,
  perezapustitSPravami: async (vkladka: string) => {
    stend.s.pravaProsili = vkladka;
    if (stend.s.pravaLomayutsya) throw new Error(stend.s.pravaLomayutsya);
  },
  startovayaVkladka: async () => stend.s.startovaya,
  perezapustitOkno: async () => {
    stend.s.perezapuskov += 1;
  },
  vybratArhiv: async () => {
    if (stend.s.arhivLomaetsya) throw new Error("диалог выбора файла не открылся");
    return stend.s.arhiv;
  },
  vybratPrilozhenie: async () => {
    stend.s.sled.push({ chto: "vybratPrilozhenie", args: [] });
    return "C:\\Program Files\\Steam\\steam.exe";
  },
  vybratKlyuchSsh: async () => {
    stend.s.sled.push({ chto: "vybratKlyuchSsh", args: [] });
    return null;
  },
  spisokProtsessov: async () => {
    if (stend.s.protsessyLomayutsya) throw new Error("список процессов не читается");
    if (stend.s.protsessyDerzhatsya)
      return new Promise<{ imya: string; put: string }[]>((resolve) => stend.s.protsessyOchered.push(resolve));
    return [...stend.s.protsessy];
  },
  otkrytPapkuZhurnalov: async () => { stend.s.sled.push({chto:"otkrytPapkuZhurnalov",args:[]}); },
  prochitatSoedineniya: async (filter:unknown) => { stend.s.sled.push({chto:"prochitatSoedineniya",args:[filter]}); return {yadro:false,vremya:new Date().toISOString(),ogranichen:false,soedineniya:[]}; },
  prochitatOhvatPrilozheniya: async (put:string) => {stend.s.sled.push({chto:"prochitatOhvatPrilozheniya",args:[put]});return {pravilo:{put,imya:"App",potomki:true,marshrut:"vpn"},reviziya_pravil:"test",trebuet_podyoma:false,vremya:new Date().toISOString(),fayl:"est",samo:0,vsego:0,zapushchennye:[],neizvestno:0,neizvestnye:[],ogranichen:false};},
  tekstBufera: async () => {
    if (stend.s.buferLomaetsya) throw new Error("буфер обмена не прочитался");
    return stend.s.bufer ?? "";
  },
  naytiQrNaEkrane: async () => ({ klyuchi: [], podpiski: [], negodnyh: 0 }),
  dobavitNaydennoeQr: async () => "",
  zabytQr: () => {},
  sohranitDiagnostiku: async () => "",
  zapisatVZhurnal: (tekst: string) => { stend.s.zhurnalOkna.push(tekst); },
  opisanieVersii: async () => "Affory 1.10.0, Windows 11 (26200)",
  kodyQr: async (tekst: string) => { stend.s.sled.push({ chto: "kodyQr", args: [tekst] }); return ["data:image/png;base64,AAA"]; },
  skopirovatTekst: async (tekst: string) => { stend.s.sled.push({ chto: "skopirovatTekst", args: [tekst] }); },
  vybratKudaSohranit: async () => {
    stend.s.sled.push({ chto: "vybratKudaSohranit", args: [] });
    return stend.s.putSohraneniya;
  },
  vybratOtkuda: async () => {
    stend.s.sled.push({ chto: "vybratOtkuda", args: [] });
    return stend.s.putZagruzki;
  },
  sohranitProfil: async (put: string, profil: string) => {
    stend.s.sled.push({ chto: "sohranitProfil", args: [put, profil] });
    stend.s.zapisano = { put, profil };
  },
  prochitatProfil: async (put: string) => {
    stend.s.sled.push({ chto: "prochitatProfil", args: [put] });
    return stend.s.soderzhimoeFayla;
  },
}));

const { App } = await import("./App");

/** Control surface of the fake bridge for one test. */
function mostProby() {
  const s = stend.s;
  return {
    otvechatMedlenno(ms = 200) {
      s.zaderzhkaMs = ms;
    },
    /** The service goes away: every call throws and the shell reports the
     *  synthetic kanal-zakryt frame, exactly as most.go does. */
    oborvat() {
      s.zhiv = false;
      for (const p of [...s.podpischiki]) p({ tip: "sobytie", imya: "kanal-zakryt" });
    },
    /** The service is back, possibly in the very same state as before: it
     *  sends `state` only on a CHANGE, so nothing is emitted here. */
    ozhit(st: StatusOtvet) {
      s.status = st;
      s.zhiv = true;
    },
    /** The service refuses this command with the given §9.1 code. */
    otvechatOtkazom(komanda: string, kod: string, tekst = "отказ службы") {
      s.otkazy.set(komanda, { kod, tekst });
    },
    lomatKadr(komanda: string) {
      s.bityeKadry.add(komanda);
    },
    lomatBufer() {
      s.buferLomaetsya = true;
    },
    lomatProtsessy() {
      s.protsessyLomayutsya = true;
    },
    /** Дальше список программ не отвечает сам: ответы отдаются по номеру
     *  запроса, и порядок выбирает тест. */
    derzhatProtsessy() {
      s.protsessyDerzhatsya = true;
    },
    skolkoZaprosovProtsessov(): number {
      return s.protsessyOchered.length;
    },
    /** Отдаёт удержанный ответ: `nomer` это порядок запроса, с нуля. */
    otvetitProtsessami(nomer: number, spisok: { imya: string; put: string }[]) {
      const otdat = s.protsessyOchered[nomer];
      if (!otdat) throw new Error(`запроса списка программ №${nomer} не было`);
      otdat(spisok);
    },
    lomatArhiv() {
      s.arhivLomaetsya = true;
    },
    zadatStatus(st: StatusOtvet) {
      s.status = st;
    },
    otkazatVPravah(soobshchenie: string) {
      s.pravaLomayutsya = soobshchenie;
    },
    /** Вкладка, которую окно унесло с собой в запрос прав. */
    vkladkaZaprosaPrav(): string {
      return s.pravaProsili;
    },
    /** Чем окно ответит на вопрос «с какой вкладки тебя просили открыть». */
    zadatStartovuyuVkladku(v: string) {
      s.startovaya = v;
    },
    otvechatTelom(komanda: string, telo: unknown) {
      s.tela.set(komanda, telo);
    },
    zadatPutSohraneniya(put: string) {
      s.putSohraneniya = put;
    },
    zadatFayl(put: string, soderzhimoe: string) {
      s.putZagruzki = put;
      s.soderzhimoeFayla = soderzhimoe;
    },
    zapisannyyProfil() {
      return s.zapisano;
    },
    teloKomandy(komanda: string): unknown {
      return s.poslannoe.get(komanda);
    },
    /** Where a given string turned up across the whole bridge surface. */
    sledSo(chast: string): string[] {
      return s.sled.filter((z) => JSON.stringify(z.args).includes(chast)).map((z) => z.chto);
    },
    skolkoRaz(komanda: string): number {
      return s.schet.get(komanda) ?? 0;
    },
    /** Перезапуск окна после подмены: старое окно рисует старый код. */
    perezapuskovOkna(): number {
      return s.perezapuskov;
    },
    /** Подписка на статистику, какой её оставила служба после всех ответов. */
    podpiskaSluzhby(): boolean | null {
      return s.podpiskaSluzhby;
    },
    /** Следующие ответы subscribeStats придут с этими задержками, по порядку. */
    zaderzhatPodpisku(...ms: number[]) {
      s.zaderzhkiPodpiski.push(...ms);
    },
    /** Окно ушло в трей или свернулось: ровно то, что делает крестик. */
    okno(vidno: boolean) {
      for (const n of [...s.nablyudateliOkna]) n(vidno);
    },
    /** Событие службы, как оно приходит по каналу. */
    sobytie(kadr: unknown) {
      for (const p of [...s.podpischiki]) p(kadr);
    },
    /** Трей просит открыть вкладку: ровно то, что делает пункт «Обновить до X». */
    vkladka(imya: string) {
      for (const n of [...s.nablyudateliVkladki]) n(imya);
    },
  };
}

// Poll period for tests that must see several rounds without waiting 5 s.
const BYSTRO = 20;

beforeEach(() => {
  const s = stend.s;
  s.zhiv = true;
  s.zaderzhkaMs = 0;
  s.status = { sostoyanie: "vyklyuchen" };
  s.perezapuskov = 0;
  s.pravaProsili = "";
  s.startovaya = "";
  s.schet = new Map();
  s.otkazy = new Map();
  s.podpischiki = [];
  s.nablyudateliOkna = [];
  s.nablyudateliVkladki = [];
  s.sluzhbaEst = true;
  s.bufer = null;
  s.buferLomaetsya = false;
  s.protsessyLomayutsya = false;
  s.protsessy = [];
  s.protsessyDerzhatsya = false;
  s.protsessyOchered = [];
  s.arhiv = "";
  s.arhivLomaetsya = false;
  s.pravaLomayutsya = null;
  s.bityeKadry = new Set();
  s.tela = new Map();
  s.poslannoe = new Map();
  s.sled = [];
  s.zhurnalOkna = [];
  s.putSohraneniya = "";
  s.putZagruzki = "";
  s.soderzhimoeFayla = "";
  s.zapisano = null;
  s.podpiskaSluzhby = null;
  s.zaderzhkiPodpiski = [];
});

afterEach(cleanup);

/** Редкие настройки лежат в сворачиваемых разделах экрана настроек, и
 *  содержимое закрытого раздела не отрисовано. Тест, которому нужна строка
 *  внутри, раскрывает разделы так же, как это сделал бы человек. */
function raskrytRazdelyNastroek() {
  for (const zagolovok of screen.queryAllByTestId(/^razdel-/))
    if (zagolovok.getAttribute("aria-expanded") === "false") fireEvent.click(zagolovok);
}

describe("оболочка окна", () => {
  it("обновляет процессы при открытии формы и возврате в окно, сохраняя ввод", async () => {
    // Programs launch after tabs open; snapshots have yet to develop telepathy.
    mostProby().otvechatTelom("listRules", {
      protsessy: [], domeny: [],
      trafik: { po_umolchaniyu: "vpn", prilozheniya: [], domeny: [], servisy: [] },
    });
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByRole("tab", { name: "Правила" }));
    fireEvent.click(await screen.findByRole("tab", { name: /Приложения/ }));
    stend.s.protsessy = [{ imya: "local.exe", put: "C:\\Users\\Test\\AppData\\Local\\App\\local.exe" }];
    fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
    fireEvent.click(await screen.findByRole("button", { name: /^local.exe,/ }));
    fireEvent.change(screen.getByLabelText("Поиск приложения"), { target: { value: "roaming" } });
    stend.s.protsessy = [{ imya: "roaming.exe", put: "C:\\Users\\Test\\AppData\\Roaming\\App\\roaming.exe" }];
    fireEvent.focus(window);
    await screen.findByRole("button", { name: /^roaming.exe,/ });
    expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Users\\Test\\AppData\\Local\\App\\local.exe");
    expect(screen.getByLabelText("Поиск приложения")).toHaveValue("roaming");
    expect(stend.s.schet.get("setRules") ?? 0).toBe(0);
  });
  it("передаёт выбор приложения из нативного моста в форму правил", async () => {
    // Wiring deserves a test too; otherwise the perfectly tested button is furniture.
    const most = mostProby();
    most.otvechatTelom("listRules", {
      protsessy: [], domeny: [],
      trafik: { po_umolchaniyu: "vpn", prilozheniya: [], domeny: [], servisy: [] },
    });
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByRole("tab", { name: "Правила" }));
    fireEvent.click(await screen.findByRole("tab", { name: /Приложения/ }));
    fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
    fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
    await waitFor(() => expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Program Files\\Steam\\steam.exe"));
    expect(stend.s.sled.filter(call => call.chto === "vybratPrilozhenie")).toHaveLength(1);
    expect(most.skolkoRaz("setRules")).toBe(0);
  });

  // The window used to render "служба не отвечает" for the whole first round
  // trip, every single launch, even with a perfectly healthy service.
  it("на старте не обвиняет службу в молчании, пока не получил ответ", async () => {
    const most = mostProby();
    most.otvechatMedlenno(200);
    render(<App />);
    expect(screen.queryByText(/служба не отвечает/i)).toBeNull();
    await screen.findByText(/выключено/i, {}, { timeout: 3000 });
  });
});

describe("окно переживает перезапуск службы", () => {
  it("после обрыва канала сам возвращается к живой службе", async () => {
    const most = mostProby();
    render(<App periodOprosaMs={BYSTRO} />);
    await screen.findByText(/выключено/i);

    most.oborvat();
    await screen.findByText(/служба не отвечает/i);

    most.ozhit({ sostoyanie: "podnyat" });
    // The service sends `state` only on a change, so nothing arrives on its
    // own: the window has to ask again or stay a brick.
    await screen.findByText(/^подключено$/i, {}, { timeout: 3000 });
  });

  it("после восстановления канала заново спрашивает списки и подписку", async () => {
    const most = mostProby();
    render(<App periodOprosaMs={BYSTRO} />);
    await screen.findByText(/выключено/i);
    most.oborvat();
    await screen.findByText(/служба не отвечает/i);
    most.ozhit({ sostoyanie: "vyklyuchen" });
    await waitFor(
      () => {
        expect(most.skolkoRaz("hello")).toBeGreaterThan(1);
        expect(most.skolkoRaz("subscribeStats")).toBeGreaterThan(1);
        expect(most.skolkoRaz("listServers")).toBeGreaterThan(1);
      },
      { timeout: 3000 },
    );
  });

  it("при молчащей службе даёт кнопку повторить", async () => {
    const most = mostProby();
    // Long period on purpose: only the button may bring the window back, so
    // a poll cannot green this test behind the person's back.
    render(<App periodOprosaMs={100000} />);
    await screen.findByText(/выключено/i);
    most.oborvat();
    const knopka = await screen.findByRole("button", { name: /повторить|подключиться заново/i });
    most.ozhit({ sostoyanie: "vyklyuchen" });
    fireEvent.click(knopka);
    await screen.findByText(/выключено/i);
  });

  it("опрос по умолчанию раз в пять секунд, как у трея", async () => {
    const { PERIOD_OPROSA_MS } = await import("./App");
    expect(PERIOD_OPROSA_MS).toBe(5000);
  });
});

describe("ни один отказ не пропадает молча", () => {
  // Every bridge call must have a failure branch that reaches the screen.
  // The gate reads App.tsx: an empty catch here costs a dead tab, not a log line.
  it("в App.tsx нет ни одного пустого catch", async () => {
    const syroy = await readFile("src/App.tsx", "utf8");
    // Comments are stripped first: a catch whose whole body is an apology in
    // prose swallows exactly as much as a catch with nothing in it, and the
    // seven swallowing branches of 03.09.2026 were all of that kind.
    const ishodnik = syroy.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*/g, "");
    // Anchor. A gate that READS SOURCE goes green on an empty parse just as
    // happily as on clean code, and the stripping above is greedy enough to
    // eat the file if a comment marker ever appears inside a string. If these
    // three vanish, the INSTRUMENT is broken, not App.tsx.
    for (const yakor of ["useState", "catch", "zvat"]) {
      expect(ishodnik.includes(yakor), `в разобранном App.tsx нет ${yakor}: сломан разбор, а не окно`).toBe(true);
    }
    const pustye = [...ishodnik.matchAll(/catch\s*(\([^)]*\))?\s*\{\s*\}/g)];
    expect(pustye.map((m) => m[0])).toEqual([]);
  });

  // Список подписок приходит ОТДЕЛЬНОЙ командой: listServers говорит только про
  // активную, а экрану нужны все. Окно обязано спросить его при открытии
  // вкладки и перезапросить после каждого действия с подписками.
  // 26.09.2026: отказ вставки пачкой забирает форма. Баннер для кода
  // subscription-malformed говорил бы про подписку, которой во вставке нет.
  it("отказ addServers показан в форме, баннера нет, список перечитан", async () => {
    const most = mostProby();
    most.otvechatOtkazom("addServers", "subscription-malformed", "это файл настроек, а не ссылки");
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Управлять" }));
    // Список подменной службы пуст: это первый запуск, и форма открыта сама.
    fireEvent.change(await screen.findByTestId("ssylka"), { target: { value: "vless://x" } });
    const bylo = most.skolkoRaz("listServers");
    fireEvent.click(screen.getByTestId("dobavit-ssylku"));
    expect(await screen.findByTestId("ishod-vvoda")).toHaveTextContent("это файл настроек, а не ссылки");
    expect(screen.queryByTestId("otkaz")).toBeNull();
    await waitFor(() => expect(most.skolkoRaz("listServers")).toBeGreaterThan(bylo));
  });

  it("вкладка серверов спрашивает список подписок и перезапрашивает после обновления", async () => {
    const most = mostProby();
    most.otvechatTelom("listSubscriptions", {
      podpiski: [
        { id: "aaa", uzel: "panel.example.net", aktivnaya: true },
        { id: "bbb", uzel: "zapasnaya.example.net", aktivnaya: false },
      ],
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Управлять" }));
    await waitFor(() => expect(most.skolkoRaz("listSubscriptions")).toBeGreaterThan(0));

    const stroka = await screen.findByTestId("podpiska-bbb");
    expect(stroka).toHaveTextContent(/zapasnaya\.example\.net/);

    const bylo = most.skolkoRaz("listSubscriptions");
    fireEvent.click(screen.getByTestId("obnovit-podpisku-bbb"));
    await waitFor(() => expect(most.skolkoRaz("refreshSubscription")).toBe(1));
    await waitFor(() => expect(most.skolkoRaz("listSubscriptions")).toBeGreaterThan(bylo));
  });

  // Служба прежней версии этой команды не знает. Её отказ это НЕ повод для
  // баннера: он рассказал бы человеку про наш порядок обновления вместо его
  // подписок, а экран и без списка рисует строку по полям listServers.
  it("главный экран выбирает источник до подключения и останавливается на отказе источника", async () => {
    const most = mostProby();
    most.otvechatTelom("listSubscriptions", { podpiski: [
      { id: "aaa", uzel: "a.example", aktivnaya: true },
      { id: "bbb", uzel: "b.example", aktivnaya: false, servery: [{ id: "remote", imya: "Запасной узел", transport: "trojan", host: "203.0.113.1", port: 443, iz_podpiski: true }] },
    ] });
    most.otvechatOtkazom("setActiveSubscription", "subscription-unreachable");
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Подключиться к Запасной узел" }));
    await waitFor(() => expect(most.skolkoRaz("setActiveSubscription")).toBe(1));
    expect(most.skolkoRaz("connect")).toBe(0);
  });

  it("после активации подключается по новому ID профиля, а не к ручному одноимённому", async () => {
    const most=mostProby();
    most.otvechatTelom("listSubscriptions", {podpiski:[
      {id:"aaa",uzel:"a.example",aktivnaya:true},
      {id:"bbb",uzel:"b.example",aktivnaya:false,servery:[{id:"shared",imya:"Другой профиль",transport:"trojan",host:"node.example",port:443,iz_podpiski:true}]},
    ]});
    most.otvechatTelom("setActiveSubscription", {aktivnaya:"bbb",server_ids:{shared:"unique"}});
    render(<App/>);
    fireEvent.click(await screen.findByRole("button",{name:"Подключиться к Другой профиль"}));
    await waitFor(()=>expect(most.teloKomandy("connect")).toEqual({server:"unique"}));
  });

  it("обновление подписки сохраняет незатронутые замеры на главном экране", async () => {
    const most = mostProby();
    const servers = ["a", "b", "manual"].map(id => ({ id, imya: id, host: `${id}.example`, port: 443, transport: "trojan", iz_podpiski: id !== "manual" }));
    const list = { servery: servers, vybran: "a", podpiska_zadana: true, podpiska_uzel: "panel.example", versii: { a: "a1", b: "b1", manual: "m1" } };
    most.otvechatTelom("listServers", list);
    most.otvechatTelom("listSubscriptions", { podpiski: [{ id: "source", uzel: "panel.example", aktivnaya: true }] });
    most.otvechatTelom("measureDelays", { zamery: [
      { id: "a", versiya: "a1", ping_ms: 111 },
      { id: "b", versiya: "b1", ping_ms: 222 },
      { id: "manual", versiya: "m1", ping_ms: 333 },
    ] });
    render(<App />);
    await screen.findByRole("button", { name: "Подключиться к a" });
    fireEvent.click(screen.getByRole("button", { name: "Проверить серверы" }));
    expect(await screen.findByText("222 мс")).toBeTruthy();
    most.otvechatTelom("listServers", { ...list, versii: { ...list.versii, b: "b2" } });
    fireEvent.click(screen.getByRole("button", { name: "Обновить panel.example" }));
    await waitFor(() => expect(screen.queryByText("222 мс")).toBeNull());
    expect(screen.getByText("111 мс")).toBeTruthy();
    expect(screen.getByText("333 мс")).toBeTruthy();
  });

  it("причина отказа от ядра доезжает до каталога главного экрана", async () => {
    const most = mostProby();
    const servers = ["a", "b"].map(id => ({ id, imya: id, host: `${id}.example`, port: 443, transport: "trojan", iz_podpiski: false }));
    most.otvechatTelom("listServers", { servery: servers, vybran: "a", versii: { a: "a1", b: "b1" } });
    const fraza = "у сервера сертификат на другое имя: адрес в ключе мог устареть, или связь перехватывают по пути";
    most.otvechatTelom("measureDelays", { zamery: [
      { id: "a", versiya: "a1", ping_ms: 48 },
      { id: "b", versiya: "b1", ping_ms: null, ping_otkaz: fraza, ping_prichina: "сертификат на другое имя" },
    ] });
    render(<App />);
    await screen.findByRole("button", { name: "Подключиться к a" });
    fireEvent.click(screen.getByRole("button", { name: "Проверить серверы" }));
    const prichina = await screen.findByText("Сертификат на другое имя");
    expect(prichina).toHaveAttribute("title", fraza);
    expect(screen.getByText("48 мс")).toBeTruthy();
    expect(screen.queryByText("Недоступен")).toBeNull();
  });

  it("ожидание обновления относится к одной подписке и снимается после сетевого отказа", async () => {
    const most = mostProby();
    most.otvechatTelom("listSubscriptions", { podpiski: [
      { id: "a", uzel: "a.example", aktivnaya: true },
      { id: "b", uzel: "b.example", aktivnaya: false },
    ] });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Управлять" }));
    await screen.findByTestId("obnovit-podpisku-b");
    most.otvechatMedlenno(120);
    most.otvechatOtkazom("refreshSubscription", "subscription-unreachable", "TLS handshake timeout");
    fireEvent.click(screen.getByTestId("obnovit-podpisku"));
    expect(screen.getByTestId("obnovit-podpisku")).toBeDisabled();
    expect(screen.getByTestId("obnovit-podpisku-b")).toBeEnabled();
    await waitFor(() => expect(screen.getByTestId("obnovit-podpisku")).toBeEnabled());
    expect(screen.queryByText("Спрашиваю")).toBeNull();
    expect(most.skolkoRaz("refreshSubscription")).toBe(1);
  });

  it("служба без listSubscriptions не даёт баннера", async () => {
    const most = mostProby();
    most.otvechatOtkazom("listSubscriptions", "protocol-mismatch");
    most.otvechatTelom("listServers", {
      servery: [], vybran: "", podpiska_zadana: true, podpiska_uzel: "panel.example.net",
    });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Управлять" }));
    await waitFor(() => expect(most.skolkoRaz("listSubscriptions")).toBeGreaterThan(0));

    expect(await screen.findByTestId("podpiska-odna")).toHaveTextContent(/panel\.example\.net/);
    expect(screen.queryByText(/listSubscriptions/)).toBeNull();
  });

  it("отказ listServers виден на экране и даёт повтор", async () => {
    const most = mostProby();
    most.otvechatOtkazom("listServers", "secrets-unreadable");
    render(<App />);
    await screen.findByText(/файл с серверами не открылся/i);
    expect(screen.getByRole("button", { name: /повтор/i })).toBeTruthy();
  });

  it("отказ списка не запирает вкладку: сервер добавляется и без прочитанного набора", async () => {
    // Adding a server by link needs no list at all. Hiding the whole tab
    // behind the refusal left the person with no reason, no repeat and no
    // form: a dead end reachable in one click (03.09.2026).
    const most = mostProby();
    most.otvechatOtkazom("listServers", "secrets-unreadable");
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Управлять" }));
    expect(await screen.findByTestId("otkaz-spiska")).toBeTruthy();
    expect(screen.getByTestId("dobavit")).toBeTruthy();
  });

  it("отказ правил не запирает вкладку и называет свою причину на ней", async () => {
    const most = mostProby();
    most.otvechatOtkazom("listRules", "secrets-unreadable");
    render(<App />);
    fireEvent.click(await screen.findByRole("tab", { name: /правила/i }));
    expect(await screen.findByTestId("otkaz-pravil")).toBeTruthy();
    // And it must NOT claim the exclusion list is empty.
    expect(screen.queryByText(/весь трафик идёт через туннель/i)).toBeNull();
  });

  it("отказ listRules не выдаёт себя за пустой список", async () => {
    const most = mostProby();
    most.otvechatOtkazom("listRules", "secrets-unreadable");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Правила"));
    await waitFor(() => expect(most.skolkoRaz("listRules")).toBeGreaterThan(0));
    expect(screen.queryByText(/исключениях нет/i)).toBeNull();
  });

  it("битый кадр не пропадает в unhandled rejection, а доезжает до экрана", async () => {
    const most = mostProby();
    render(<App />);
    await screen.findByText(/выключено/i);
    // The bridge answers something that is not a frame: most.ts throws a
    // plain Error, and vypolnit used to rethrow it into nowhere.
    most.lomatKadr("connect");
    fireEvent.click(screen.getByTestId("glavnoe-deystvie"));
    await screen.findByText(/кадр не разбирается/i);
  });

  it("нечитаемый буфер не выдаёт себя за пустой буфер", async () => {
    const most = mostProby();
    most.lomatBufer();
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByRole("button", { name: "Управлять" }));
    fireEvent.click(await screen.findByRole("button", { name: /из буфера/i }));
    await screen.findByText(/буфер обмена не прочитался/i);
  });

  it("несписанные процессы объясняются строкой, а не исчезновением", async () => {
    // Раньше причина уезжала в общий баннер окна: «список не читается» висело
    // поверх всего экрана правил, хотя мешало оно одной форме. C8 перенёс её
    // туда, где человек её и ждёт, вместе с кнопкой повтора.
    const most = mostProby();
    most.lomatProtsessy();
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Правила"));
    fireEvent.click(await screen.findByRole("tab", { name: /Приложения/ }));
    fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
    await screen.findByText(/список процессов не читается/i);
    expect(screen.getByRole("button", { name: "Повторить" })).toBeEnabled();
  });

  it("непонятые строки подписки доезжают до экрана, а не остаются в ответе службы", async () => {
    // 05.09.2026. Служба отдавала поле otkazy обеим командам подписки с
    // первого дня, а App читал тело только для checkLeaks и соседей. Строка,
    // которую клиент не понял, исчезала бесследно: на экране просто меньше
    // серверов, чем прислала панель.
    const most = mostProby();
    most.otvechatTelom("listServers", {
      servery: [], vybran: "", podpiska_zadana: true, podpiska_uzel: "panel.example.net",
    });
    most.otvechatTelom("refreshSubscription", {
      serverov: 2,
      otkazy: [{ stroka: 4, prichina: "транспорт не поддерживается" }],
    });
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByRole("button", { name: "Управлять" }));
    fireEvent.click(await screen.findByTestId("obnovit-podpisku"));
    const k = await screen.findByTestId("otkazy-podpiski");
    expect(k).toHaveTextContent("строка 4");
    expect(k).toHaveTextContent("транспорт не поддерживается");
  });

  it("сорванный диалог архива не выглядит как отмена", async () => {
    const most = mostProby();
    most.lomatArhiv();
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    raskrytRazdelyNastroek();
    fireEvent.click(await screen.findByRole("button", { name: /выбрать архив/i }));
    await screen.findByText(/диалог выбора файла не открылся/i);
  });
});

it("черновик правил переживает переход в настройки и применяется одним запросом", async()=>{
  const most=mostProby();
  const saved={protsessy:[],domeny:[],trafik:{po_umolchaniyu:"vpn",prilozheniya:[],domeny:[],servisy:[]}};
  most.otvechatTelom("listRules",saved);
  render(<App/>);
  fireEvent.click(await screen.findByRole("tab",{name:"Правила"}));
  fireEvent.click(await screen.findByRole("radio",{name:"Только выбранное"}));
  fireEvent.click(screen.getByRole("tab",{name:"Настройки"}));
  fireEvent.click(screen.getByRole("tab",{name:"Правила"}));
  expect(screen.getByRole("radio",{name:"Только выбранное"})).toBeChecked();
  expect(most.skolkoRaz("setRules")).toBe(0);
  const applied={...saved,trafik:{...saved.trafik,po_umolchaniyu:"direct"}};
  most.otvechatTelom("setRules",applied);
  most.otvechatTelom("listRules",applied);
  fireEvent.click(screen.getByRole("button",{name:"Применить изменения"}));
  await waitFor(()=>expect(most.skolkoRaz("setRules")).toBe(1));
  await screen.findByText("Правила сохранены");
  expect(screen.getByRole("radio",{name:"Только выбранное"})).toBeChecked();
});

it("проверка домена соединена с мостом и не включает журнал",async()=>{
  const most=mostProby();
  render(<App/>);
  fireEvent.click(await screen.findByRole("tab",{name:"Правила"}));
  fireEvent.click(await screen.findByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("button",{name:"Проверка правил"}));
  fireEvent.change(screen.getByLabelText("Домен для проверки"),{target:{value:"api.example.org"}});
  fireEvent.click(screen.getByRole("button",{name:"Проверить соединения"}));
  await screen.findByText(/Подключение не запущено/);
  expect(stend.s.sled).toContainEqual({chto:"prochitatSoedineniya",args:[{domen:"api.example.org"}]});
  expect(most.skolkoRaz("setJournal")).toBe(0);
});

it("проверка охвата использует отдельный запрос и не меняет правила",async()=>{
  const most=mostProby();
  most.otvechatTelom("listRules",{protsessy:[],domeny:[],trafik:{po_umolchaniyu:"vpn",servisy:[],domeny:[],prilozheniya:[{put:"C:\\App.exe",imya:"App",potomki:true,marshrut:"vpn"}]}});
  render(<App/>);
  fireEvent.click(await screen.findByRole("tab",{name:"Правила"}));
  fireEvent.click(await screen.findByRole("tab",{name:/Приложения/}));
  fireEvent.click(screen.getByRole("button",{name:"Проверка правил"}));
  fireEvent.click(screen.getByRole("button",{name:"Проверить охват"}));
  await screen.findByText(/Приложение и связанные запуски в этом сеансе не обнаружены/);
  expect(stend.s.sled).toContainEqual({chto:"prochitatOhvatPrilozheniya",args:["C:\\App.exe"]});
  expect(most.skolkoRaz("setRules")).toBe(0);
});

it("кнопка папки журналов вызывает нативный мост без команды службы", async () => {
  const most=mostProby();
  render(<App/>);
  fireEvent.click(await screen.findByRole("tab",{name:"Настройки"}));
  fireEvent.click(await screen.findByTestId("razdel-diagnostika"));
  fireEvent.click(screen.getByRole("button",{name:"Открыть папку с журналами"}));
  await waitFor(()=>expect(stend.s.sled.some(v=>v.chto==="otkrytPapkuZhurnalov")).toBe(true));
  expect(most.skolkoRaz("clearJournal")).toBe(0);
});

it("поздний ответ списка программ не перетирает свежий", async () => {
  // Список запрашивается по открытию формы, по кнопке и по возврату фокуса в
  // окно, поэтому два запроса в полёте это обычное дело. Без номера запроса
  // медленный первый ответ приходил последним и возвращал в форму снимок,
  // который человек уже обновил вручную.
  const most = mostProby();
  most.derzhatProtsessy();
  render(<App />);
  await screen.findByText(/выключено/i);
  fireEvent.click(screen.getByText("Правила"));
  fireEvent.click(await screen.findByRole("tab", { name: /Приложения/ }));
  fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
  await waitFor(() => expect(most.skolkoZaprosovProtsessov()).toBeGreaterThan(0));
  const staryy = most.skolkoZaprosovProtsessov() - 1;
  // Второй запрос приходит от возврата фокуса в окно: кнопка на время чтения
  // занята, а окно за это время могли свернуть и развернуть.
  fireEvent.focus(window);
  await waitFor(() => expect(most.skolkoZaprosovProtsessov()).toBe(staryy + 2));
  await act(async () => { most.otvetitProtsessami(staryy + 1, [{ imya: "Свежий", put: "C:\\Fresh.exe" }]); });
  expect(await screen.findByRole("button", { name: /^Свежий,/ })).toBeInTheDocument();
  await act(async () => { most.otvetitProtsessami(staryy, [{ imya: "Прошлый", put: "C:\\Stale.exe" }]); });
  expect(screen.queryByRole("button", { name: /^Прошлый,/ })).toBeNull();
  expect(screen.getByRole("button", { name: /^Свежий,/ })).toBeInTheDocument();
});

// Прямые маршруты включению защиты больше не мешают (23.09.2026): под ней они
// спят, а набор человека остаётся как есть.
it("настройки видят сохранённые прямые маршруты и включают защиту поверх них",async()=>{
  const most=mostProby();
  most.otvechatTelom("listRules",{protsessy:[],domeny:[],trafik:{po_umolchaniyu:"vpn",servisy:[{id:"youtube",marshrut:"direct"}],domeny:[],prilozheniya:[]}});
  render(<App/>);
  fireEvent.click(await screen.findByRole("tab",{name:"Настройки"}));
  await screen.findByText(/Напрямую настроено: 1 сервис/);
  expect(screen.getByTestId("ves-trafik")).toBeEnabled();
  fireEvent.click(screen.getByTestId("ves-trafik"));
  expect(most.skolkoRaz("setKillSwitch")).toBe(1);
});

describe("баннер отказа", () => {
  it("баннер отказа закрывается крестиком", async () => {
    const most = mostProby();
    most.otvechatOtkazom("clearJournal", "journal-clear-failed");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    fireEvent.click(await screen.findByTestId("razdel-diagnostika"));
    fireEvent.click(screen.getByTestId("ochistit-zhurnal"));
    await screen.findByTestId("otkaz");
    fireEvent.click(screen.getByTestId("otkaz-zakryt"));
    expect(screen.queryByTestId("otkaz")).toBeNull();
  });

  it("баннер не переезжает на другую вкладку", async () => {
    const most = mostProby();
    most.otvechatOtkazom("clearJournal", "journal-clear-failed");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    fireEvent.click(await screen.findByTestId("razdel-diagnostika"));
    fireEvent.click(screen.getByTestId("ochistit-zhurnal"));
    await screen.findByTestId("otkaz");
    // The refusal belongs to the tab that asked for it: a settings failure has
    // no business staring at a person who walked over to Connection.
    fireEvent.click(screen.getByText("Подключение"));
    expect(screen.queryByTestId("otkaz")).toBeNull();
    fireEvent.click(screen.getByText("Настройки"));
    expect(screen.getByTestId("otkaz")).toBeTruthy();
  });

  // 1.10.0: что видел человек, то и лежит в журнале окна, а под плашкой
  // кнопка журнала и версия, которую окно знает без службы.
  it("показанный отказ пишется в журнал окна, под ним журнал и версия", async () => {
    const most = mostProby();
    most.zadatStatus({ sostoyanie: "otkaz", oshibka: { kod: "all-servers-down", tekst: "ни один не ответил" } });
    render(<App periodOprosaMs={BYSTRO} />);
    expect(await screen.findByTestId("otkaz")).toHaveAttribute("data-kod", "all-servers-down");
    expect(await screen.findByTestId("otkaz-versiya")).toHaveTextContent("Affory 1.10.0, Windows 11 (26200)");
    expect(screen.getByRole("button", { name: "Сохранить журнал" })).toBeInTheDocument();
    await waitFor(() => expect(stend.s.zhurnalOkna).toEqual(["отказ на экране: all-servers-down | ни один не ответил"]));
    // Тот же отказ на следующем опросе статуса в журнал второй раз не идёт.
    await new Promise((r) => setTimeout(r, BYSTRO * 4));
    expect(stend.s.zhurnalOkna).toHaveLength(1);
  });

  it("пока служба восстанавливает туннель, причина не встаёт красной плашкой", async () => {
    // Приёмка 1.9.0: между попытками окно показывало отказ, и казалось, что
    // программа сдалась. Причина живёт строкой под статусом, плашки нет.
    const most = mostProby();
    most.zadatStatus({ sostoyanie: "vosstanavlivaetsya", oshibka: { kod: "all-servers-down", tekst: "ни один не ответил" } });
    render(<App periodOprosaMs={BYSTRO} />);
    expect(await screen.findByTestId("hod-vosstanovleniya")).toHaveTextContent("ни один сервер не отвечает");
    await new Promise((r) => setTimeout(r, BYSTRO * 4));
    expect(screen.queryByTestId("otkaz")).toBeNull();
  });

  // Жалоба 15.10.2026: подъём при молчащих серверах шёл дольше срока канала,
  // и окно рисовало «не получилось выполнить действие», пока служба ещё
  // поднимала. Канал при этом жив, и итог служба присылает событием state.
  it("истёкший срок ответа на подключение не объявляет провал, итог приезжает состоянием", async () => {
    const most = mostProby();
    most.otvechatOtkazom("connect", "srok-otveta", "служба не ответила вовремя: connect за 3m0s");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByTestId("glavnoe-deystvie"));
    await waitFor(() => expect(most.skolkoRaz("connect")).toBe(1));
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByTestId("otkaz")).toBeNull();
    act(() => most.sobytie({ tip: "sobytie", imya: "state", telo: { sostoyanie: "otkaz", oshibka: { kod: "all-servers-down", tekst: "ни один не ответил" } } }));
    expect(await screen.findByTestId("otkaz")).toHaveAttribute("data-kod", "all-servers-down");
  });

  it("истёкший срок ответа на смену сервера тоже не рисует плашку", async () => {
    const most = mostProby();
    const servery = ["a", "b"].map((id) => ({ id, imya: id, host: `${id}.example`, port: 443, transport: "trojan" }));
    most.otvechatTelom("listServers", { servery, vybran: "a", podpiska_zadana: false, podpiska_uzel: "" });
    most.otvechatOtkazom("setServer", "srok-otveta", "служба не ответила вовремя: setServer за 3m0s");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByRole("button", { name: "Управлять" }));
    fireEvent.click(await screen.findByTestId("server-b"));
    await waitFor(() => expect(most.skolkoRaz("setServer")).toBe(1));
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByTestId("otkaz")).toBeNull();
  });

  it("истёкший срок у остальных команд остаётся плашкой сбоя", async () => {
    // Очистка журнала не присылает итог событием: молчание тут и есть ответ,
    // и спрятать его значило бы кнопку, которая ничего не делает.
    const most = mostProby();
    most.otvechatOtkazom("clearJournal", "srok-otveta", "служба не ответила вовремя: clearJournal за 5s");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    fireEvent.click(await screen.findByTestId("razdel-diagnostika"));
    fireEvent.click(screen.getByTestId("ochistit-zhurnal"));
    expect(await screen.findByTestId("otkaz-tekst")).toHaveTextContent("не получилось выполнить действие");
    expect(screen.getByTestId("otkaz-prichina")).toHaveTextContent("служба не ответила вовремя");
  });

  it("закрытый баннер не воскресает из status.oshibka", async () => {
    const most = mostProby();
    // The service holds a standing refusal in status: it survived every
    // zadatOtkaz(null), so the banner could not be got rid of at all.
    most.zadatStatus({ sostoyanie: "otkaz", oshibka: { kod: "tun-create-failed", tekst: "адаптер не создан" } });
    render(<App periodOprosaMs={BYSTRO} />);
    await screen.findByTestId("otkaz");
    fireEvent.click(screen.getByTestId("otkaz-zakryt"));
    expect(screen.queryByTestId("otkaz")).toBeNull();
    // Several polls later it must still be closed.
    await new Promise((r) => setTimeout(r, BYSTRO * 6));
    expect(screen.queryByTestId("otkaz")).toBeNull();
  });

  it("кнопка-квитанция гасит и стоячий отказ службы, а не только свежий", async () => {
    const most = mostProby();
    // proverit-set is an acknowledgement: it has a button and no command, so
    // pressing it must actually get rid of the banner.
    most.zadatStatus({ sostoyanie: "otkaz", oshibka: { kod: "all-servers-down", tekst: "ни один не ответил" } });
    render(<App periodOprosaMs={BYSTRO} />);
    await screen.findByTestId("otkaz");
    fireEvent.click(screen.getByTestId("otkaz-deystvie"));
    await new Promise((r) => setTimeout(r, BYSTRO * 6));
    expect(screen.queryByTestId("otkaz")).toBeNull();
  });

  it("баннер закреплён в прокрутке, иначе его не видно снизу страницы", async () => {
    const most = mostProby();
    most.otvechatOtkazom("clearJournal", "journal-clear-failed");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    fireEvent.click(await screen.findByTestId("razdel-diagnostika"));
    fireEvent.click(screen.getByTestId("ochistit-zhurnal"));
    await screen.findByTestId("otkaz");
    // Баннер и содержимое вкладки прокручиваются ОДНИМ окном (af-viewport).
    // Без закрепления человек у нижней кнопки отказ получал за верхней кромкой:
    // замер в госте 13.09.2026 дал баннер на Y от -72 при окне от 12.
    const obertka = screen.getByTestId("otkaz-obertka");
    expect(obertka.className).toContain("sticky");
    expect(obertka.className).toContain("top-0");
  });

  it("запрос прав уносит с собой вкладку, где нажали кнопку", async () => {
    const most = mostProby();
    most.zadatStatus({ sostoyanie: "otkaz", oshibka: { kod: "admin-required", tekst: "только для администратора" } });
    render(<App />);
    fireEvent.click(await screen.findByText("Настройки"));
    fireEvent.click(await screen.findByRole("button", { name: /от администратора/i }));
    // Без этого новое окно открывалось на «Подключении», и человек заново искал
    // кнопку, ради которой права и просил (живой прогон 13.09.2026).
    await waitFor(() => expect(most.vkladkaZaprosaPrav()).toBe("nastroyki"));
  });

  it("окно, поднятое с правами, открывается на той же вкладке", async () => {
    const most = mostProby();
    most.zadatStartovuyuVkladku("nastroyki");
    render(<App />);
    await screen.findByText(/запускать при входе в Windows/i);
  });

  it("отказ от повышения прав не выглядит успехом", async () => {
    const most = mostProby();
    most.zadatStatus({ sostoyanie: "otkaz", oshibka: { kod: "admin-required", tekst: "только для администратора" } });
    most.otkazatVPravah("повышение не состоялось: The operation was canceled by the user.");
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: /от администратора/i }));
    await screen.findByText(/повышение не состоялось/i);
  });
});

describe("профиль выносится и вносится из окна", () => {
  const PAROL = "dlinnyy-parol-profilya";

  it("вынос профиля пишет файл, а пароль уходит только в тело команды", async () => {
    const most = mostProby();
    most.zadatPutSohraneniya("C:\vygruzka\profil.affory");
    most.otvechatTelom("exportProfile", { profil: "QUZGT1JZLVBST0ZJTA==" });
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    raskrytRazdelyNastroek();
    fireEvent.change(await screen.findByTestId("parol-profilya"), { target: { value: PAROL } });
    fireEvent.click(screen.getByTestId("vyvesti-profil"));

    await screen.findByText(/профиль записан/i);
    expect(most.zapisannyyProfil()).toEqual({ put: "C:\vygruzka\profil.affory", profil: "QUZGT1JZLVBST0ZJTA==" });
    expect(most.teloKomandy("exportProfile")).toEqual({ parol: PAROL });
    // The password is a secret of the same class as a key: one place, and
    // never a command line argument, a file name or a log line.
    expect(most.sledSo(PAROL)).toEqual(["zvat exportProfile"]);
  });

  it("отмена диалога не шлёт команду и не трогает пароль", async () => {
    const most = mostProby();
    most.zadatPutSohraneniya("");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    raskrytRazdelyNastroek();
    fireEvent.change(await screen.findByTestId("parol-profilya"), { target: { value: PAROL } });
    fireEvent.click(screen.getByTestId("vyvesti-profil"));
    await waitFor(() => expect(most.sledSo("")).toContain("vybratKudaSohranit"));
    expect(most.skolkoRaz("exportProfile")).toBe(0);
  });

  it("внос профиля читает файл и заново спрашивает список", async () => {
    const most = mostProby();
    most.zadatFayl("C:\vygruzka\profil.affory", "QUZGT1JZLVBST0ZJTA==");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    raskrytRazdelyNastroek();
    fireEvent.change(await screen.findByTestId("parol-profilya"), { target: { value: PAROL } });
    const bylo = most.skolkoRaz("listServers");
    fireEvent.click(screen.getByTestId("vvesti-profil"));

    await screen.findByText(/профиль принят/i);
    expect(most.teloKomandy("importProfile")).toEqual({ parol: PAROL, profil: "QUZGT1JZLVBST0ZJTA==" });
    expect(most.sledSo(PAROL)).toEqual(["zvat importProfile"]);
    await waitFor(() => expect(most.skolkoRaz("listServers")).toBeGreaterThan(bylo));
  });

  it("неверный пароль виден на экране, а не проглатывается", async () => {
    const most = mostProby();
    most.zadatFayl("C:\vygruzka\profil.affory", "QUZGT1JZ");
    most.otvechatOtkazom("importProfile", "secrets-unreadable", "пароль неверен либо файл повреждён");
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    raskrytRazdelyNastroek();
    fireEvent.change(await screen.findByTestId("parol-profilya"), { target: { value: PAROL } });
    fireEvent.click(screen.getByTestId("vvesti-profil"));
    await screen.findByText(/пароль неверен либо файл повреждён/i);
  });
});

describe("замер полосы", () => {
  // Кнопка тратит десятки мегабайт настоящего трафика. Ответ, не доехавший до
  // экрана, читается как сломанная кнопка, и человек жмёт ещё раз: это уже
  // стоило нам разбора 03.09.2026 на проверке адреса выхода.
  it("измеренное доезжает до экрана числами и советом", async () => {
    const most = mostProby();
    most.otvechatTelom("measureBandwidth", {
      mbit_vniz: 87.4, sovet_vniz: 78,
      mbit_vverh: 27.1, sovet_vverh: 24,
      cherez_tunnel: true, potokov: 4,
    });
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    raskrytRazdelyNastroek();
    fireEvent.change(await screen.findByTestId("polosa-mishen"), {
      target: { value: "https://example.org/big.bin" },
    });
    fireEvent.change(screen.getByTestId("polosa-mishen-vverh"), {
      target: { value: "https://example.org/__up" },
    });
    fireEvent.click(screen.getByTestId("izmerit-polosu"));

    const blok = await screen.findByTestId("zamer-polosy");
    expect(blok).toHaveTextContent("87.4");
    expect(blok).toHaveTextContent("27.1");
    expect(screen.getByTestId("obyavit-polosu")).toHaveTextContent("24");
    expect(screen.getByTestId("obyavit-polosu")).toHaveTextContent("78");
  });

  it("отказ замера уносит прошлые числа, а не оставляет их за свежие", async () => {
    // Тот же класс, что и с проверкой утечек: отказ писался мимо результата, и
    // предыдущий отчёт оставался на экране как текущий.
    const most = mostProby();
    most.otvechatTelom("measureBandwidth", {
      mbit_vniz: 87.4, sovet_vniz: 78, mbit_vverh: 27.1, sovet_vverh: 24, cherez_tunnel: true,
    });
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    raskrytRazdelyNastroek();
    fireEvent.change(await screen.findByTestId("polosa-mishen"), {
      target: { value: "https://example.org/big.bin" },
    });
    fireEvent.click(screen.getByTestId("izmerit-polosu"));
    await screen.findByTestId("zamer-polosy");

    most.otvechatOtkazom("measureBandwidth", "bandwidth-unmeasured", "мишень не отдала ни байта");
    fireEvent.click(screen.getByTestId("izmerit-polosu"));

    await waitFor(() => expect(screen.queryByTestId("zamer-polosy")).toBeNull());
    await screen.findByText(/мишень не отдала ни байта/i);
  });

  it("неизмеренная отдача не превращается в ноль", async () => {
    // Ноль в объявлении это Brutal с нулевой оценкой канала. Отсутствие числа
    // обязано оставаться отсутствием на всём пути от службы до кнопки.
    const most = mostProby();
    most.otvechatTelom("measureBandwidth", {
      mbit_vniz: 87.4, sovet_vniz: 78,
      otkaz_vverh: "мишень не принимает заливку, ответив 405",
      cherez_tunnel: false,
    });
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    raskrytRazdelyNastroek();
    fireEvent.change(await screen.findByTestId("polosa-mishen"), {
      target: { value: "https://example.org/big.bin" },
    });
    fireEvent.click(screen.getByTestId("izmerit-polosu"));

    const blok = await screen.findByTestId("zamer-polosy");
    expect(blok).toHaveTextContent("405");
    expect(blok).not.toHaveTextContent("0.0 Мбит/с");
    expect((screen.getByTestId("obyavit-polosu") as HTMLButtonElement).disabled).toBe(true);
  });

  // Решено 10.09.2026: «постоянно обновлять это в фоне нет никакого
  // смысла, учитывая то, что 95 процентов времени приложение находится в трее».
  //
  // Намерение в коде уже было записано: комментарий у subscribeStats дословно
  // говорил про свёрнутое окно. Отписка при этом висела на размонтировании
  // компонента, а оно при уходе в трей не наступает НИКОГДА, потому что окно
  // прячется, а не закрывается. Построено, но не подключено.
  it("отписывается от статистики, когда окно уходит в трей", async () => {
    const most = mostProby();
    render(<App periodOprosaMs={BYSTRO} />);
    await waitFor(() => expect(most.skolkoRaz("subscribeStats")).toBeGreaterThan(0));

    most.okno(false);
    await waitFor(() => expect(most.teloKomandy("subscribeStats")).toEqual({ vkl: false }));
  });

  it("подписывается обратно, когда окно возвращается", async () => {
    const most = mostProby();
    render(<App periodOprosaMs={BYSTRO} />);
    await waitFor(() => expect(most.skolkoRaz("subscribeStats")).toBeGreaterThan(0));

    most.okno(false);
    await waitFor(() => expect(most.teloKomandy("subscribeStats")).toEqual({ vkl: false }));
    most.okno(true);
    await waitFor(() => expect(most.teloKomandy("subscribeStats")).toEqual({ vkl: true }));
  });

  // Жалоба 26.09.2026: «окно с замером скорости иногда умирает само по себе».
  // При возврате окна эффекты слали отписку и две подписки почти разом, а служба
  // выполняет команды одного соединения каждую своим потоком. Отписка,
  // выполненная последней, оставляла окно без цифр до следующего сворачивания:
  // скорость гасла в прочерк, итоги и задержка стояли от последнего снимка.
  it("подписка на статистику переживает ответы службы не по порядку", async () => {
    const most = mostProby();
    render(<App periodOprosaMs={BYSTRO} />);
    await waitFor(() => expect(most.podpiskaSluzhby()).toBe(true));
    most.okno(false);
    await waitFor(() => expect(most.podpiskaSluzhby()).toBe(false));

    // Первый ответ после возврата окна приходит позже остальных.
    most.zaderzhatPodpisku(150, 0, 0);
    most.okno(true);
    await new Promise((r) => setTimeout(r, 400));
    expect(most.podpiskaSluzhby()).toBe(true);
  });

  // Опрос статуса тоже стоит: он ходил раз в пять секунд ровно затем, чтобы
  // экран не отстал, а у свёрнутого окна отставать нечему. Побочно это убирает
  // спусковой крючок пересборки меню в трее.
  it("не опрашивает статус, пока окно свёрнуто", async () => {
    const most = mostProby();
    render(<App periodOprosaMs={BYSTRO} />);
    await waitFor(() => expect(most.skolkoRaz("status")).toBeGreaterThan(0));

    most.okno(false);
    // Дать нескольким периодам пройти: если опрос жив, счётчик уедет.
    await new Promise((r) => setTimeout(r, BYSTRO * 6));
    const zamerlo = most.skolkoRaz("status");
    await new Promise((r) => setTimeout(r, BYSTRO * 6));
    expect(most.skolkoRaz("status")).toBe(zamerlo);
  });

  // Вернувшееся окно обязано спросить статус СРАЗУ, а не через период: человек
  // развернул программу, чтобы посмотреть, и увидел бы снимок пятисекундной
  // давности.
  it("спрашивает статус сразу при возврате окна", async () => {
    const most = mostProby();
    render(<App periodOprosaMs={100000} />);
    await waitFor(() => expect(most.skolkoRaz("status")).toBeGreaterThan(0));

    most.okno(false);
    // Дождаться, пока сворачивание доедет до экрана. Без этого оба события
    // прилетают в один такт React, состояние возвращается к прежнему значению,
    // и перерисовки не будет вовсе: тест меряет собственную спешку.
    await waitFor(() => expect(most.teloKomandy("subscribeStats")).toEqual({ vkl: false }));
    const bylo = most.skolkoRaz("status");
    most.okno(true);
    await waitFor(() => expect(most.skolkoRaz("status")).toBeGreaterThan(bylo));
  });

  // О5 аудита 1.6.1. Видимость окна сидела в зависимостях стартовых запросов,
  // и каждый показ и уход в трей заново спрашивали список серверов, hello и
  // статус, а подписку на статистику дёргали лишний раз.
  it("показ и скрытие окна не повторяют стартовые запросы", async () => {
    const most = mostProby();
    render(<App periodOprosaMs={100000} />);
    await waitFor(() => expect(most.skolkoRaz("hello")).toBeGreaterThan(0));
    await waitFor(() => expect(most.podpiskaSluzhby()).toBe(true));
    await new Promise((r) => setTimeout(r, 50));
    const spisok = most.skolkoRaz("listServers");
    const hello = most.skolkoRaz("hello");
    const status = most.skolkoRaz("status");

    most.okno(false);
    await waitFor(() => expect(most.podpiskaSluzhby()).toBe(false));
    most.okno(true);
    await waitFor(() => expect(most.podpiskaSluzhby()).toBe(true));
    await new Promise((r) => setTimeout(r, 100));
    expect(most.skolkoRaz("listServers")).toBe(spisok);
    expect(most.skolkoRaz("hello")).toBe(hello);
    // Ровно один статус: тот, что спрашивается сразу при возврате окна.
    expect(most.skolkoRaz("status")).toBe(status + 1);
  });

  // О4 аудита 1.6.1: замер скорости опрашивался и из трея, раз в секунду, пока
  // он шёл, хотя смотреть на полосу некому.
  it("не опрашивает замер скорости, пока окно в трее", async () => {
    const most = mostProby();
    most.otvechatTelom("speedTestStatus", { id: 1, phase: "download", path: "vpn", provider: "p", name: "p", attempt: 1 });
    render(<App periodOprosaMs={100000} />);
    await waitFor(() => expect(most.skolkoRaz("speedTestStatus")).toBeGreaterThan(1), { timeout: 3000 });

    most.okno(false);
    await waitFor(() => expect(most.podpiskaSluzhby()).toBe(false));
    const bylo = most.skolkoRaz("speedTestStatus");
    await new Promise((r) => setTimeout(r, 1500));
    expect(most.skolkoRaz("speedTestStatus")).toBe(bylo);
  }, 10000);

});

// 13.09.2026, три живые жалобы об одной минуте: «перебросило на dev версию»,
// «ничего не происходит из трея», «ошибка в окне после ручной установки».
describe("оболочка: обновление", () => {
  it("ведёт ход обновления от события службы до карточки настроек", async () => {
    const most = mostProby();
    most.zadatStatus({ sostoyanie: "podnyat", versiya_programmy: "1.0.3" });
    render(<App />);
    await screen.findByRole("tab", { name: "Настройки" });
    fireEvent.click(screen.getByRole("tab", { name: "Настройки" }));

    most.sobytie({
      tip: "sobytie", imya: "obnovlenie-hod",
      telo: { shag: "skachivanie", versiya: "1.1.0", skachano: 6895077, vsego: 27580311 },
    });

    const ryad = await screen.findByTestId("obnovlenie");
    expect(ryad).toHaveTextContent(/обновление до 1\.1\.0/i);
    expect(await screen.findByRole("progressbar", { name: /обновлени/i })).toHaveAttribute("aria-valuenow", "25");
  });

  it("гасит полосу, когда обновление отказало", async () => {
    const most = mostProby();
    most.zadatStatus({ sostoyanie: "podnyat", versiya_programmy: "1.0.3" });
    render(<App />);
    fireEvent.click(await screen.findByRole("tab", { name: "Настройки" }));
    most.sobytie({ tip: "sobytie", imya: "obnovlenie-hod", telo: { shag: "raspakovka", versiya: "1.1.0" } });
    await screen.findByRole("progressbar", { name: /обновлени/i });

    most.sobytie({ tip: "sobytie", imya: "obnovlenie-hod", telo: { shag: "otkaz", tekst: "архив не скачан" } });

    await waitFor(() => expect(screen.queryByRole("progressbar", { name: /обновлени/i })).toBeNull());
  });

  // 13.09.2026, живой прогон в госте: подмена прошла, служба назвалась 1.1.2,
  // а окно осталось ТЕМ ЖЕ процессом с pid 14572. Windows держит открытый файл,
  // подмена отодвигает его в .ubrat, и человек после обновления смотрит на
  // старый интерфейс, пока не закроет программу руками.
  it("перезапускает себя, когда служба вернулась с другой версией", async () => {
    const most = mostProby();
    most.zadatStatus({ sostoyanie: "vyklyuchen", versiya_programmy: "0.9.9" });
    render(<App periodOprosaMs={BYSTRO} />);
    fireEvent.click(await screen.findByRole("tab", { name: "Настройки" }));
    await screen.findByText(/программа 0\.9\.9/i);

    most.sobytie({ tip: "sobytie", imya: "obnovlenie-hod", telo: { shag: "podmena", versiya: "1.1.2", srok_s: 20 } });
    most.oborvat();
    most.ozhit({ sostoyanie: "vyklyuchen", versiya_programmy: "1.1.2" });

    await waitFor(() => expect(most.perezapuskovOkna()).toBe(1), { timeout: 3000 });
  });

  // Полоса гасла только на отказе. После удачной подмены служба уже мертва и
  // события не пришлёт, поэтому «подмена файлов, служба перезапускается,
  // осталось 0 с» висела в окне до конца его жизни.
  it("гасит полосу, когда служба вернулась после подмены", async () => {
    const most = mostProby();
    most.zadatStatus({ sostoyanie: "vyklyuchen", versiya_programmy: "1.1.2" });
    render(<App periodOprosaMs={BYSTRO} />);
    fireEvent.click(await screen.findByRole("tab", { name: "Настройки" }));

    most.sobytie({ tip: "sobytie", imya: "obnovlenie-hod", telo: { shag: "podmena", versiya: "1.1.2", srok_s: 20 } });
    await screen.findByRole("progressbar", { name: /обновлени/i });
    most.oborvat();
    most.ozhit({ sostoyanie: "vyklyuchen", versiya_programmy: "1.1.2" });

    await waitFor(() => expect(screen.queryByRole("progressbar", { name: /обновлени/i })).toBeNull(), { timeout: 3000 });
  });

  // Вертушка на кнопке это не украшение, а единственный ответ на нажатие,
  // пока команда в полёте. Механизм в ui.tsx был написан давно, а список
  // занятых команд держал только пять, гасящих управление: у замеров и
  // проверок вертушка не загоралась НИ РАЗУ, хотя код для неё стоял на месте.
  it("кнопка проверки обновления занята, пока служба отвечает", async () => {
    const most = mostProby();
    most.otvechatMedlenno(300);
    render(<App />);
    await screen.findByText(/выключено/i);
    fireEvent.click(screen.getByText("Настройки"));
    raskrytRazdelyNastroek();

    const knopka = await screen.findByTestId("proverit-versiyu");
    fireEvent.click(knopka);

    await waitFor(() => expect(screen.getByTestId("proverit-versiyu")).toHaveAttribute("aria-busy", "true"));
    // Занятая кнопка ещё и не нажимается второй раз: два нажатия это две
    // команды службе, а человек жмёт второй раз именно тогда, когда первое
    // ничего не показало.
    expect(screen.getByTestId("proverit-versiyu")).toBeDisabled();
    await waitFor(() => expect(screen.getByTestId("proverit-versiyu")).not.toHaveAttribute("aria-busy"));
  });

  it("кнопка замера задержек занята, пока идёт замер", async () => {
    const most = mostProby();
    // Кнопка живёт рядом со списком, а пустой список это экран первого
    // запуска, где её нет вовсе.
    most.otvechatTelom("listServers", {
      servery: [{ id: "a", imya: "vpn-pc-hy2", host: "1.2.3.4", port: 443, transport: "hysteria2", iz_podpiski: false }],
      vybran: "a", podpiska_zadana: false, podpiska_uzel: "",
    });
    most.otvechatMedlenno(300);
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Управлять" }));

    const knopka = await screen.findByTestId("zamerit-zaderzhki");
    fireEvent.click(knopka);

    await waitFor(() => expect(screen.getByTestId("zamerit-zaderzhki")).toHaveAttribute("aria-busy", "true"));
    await waitFor(() => expect(screen.getByTestId("zamerit-zaderzhki")).not.toHaveAttribute("aria-busy"));
  });

  it("открывает настройки, когда трей зовёт к обновлению", async () => {
    Element.prototype.scrollIntoView = vi.fn();
    const most = mostProby();
    most.zadatStatus({ sostoyanie: "podnyat", versiya_programmy: "1.0.3" });
    render(<App />);
    await screen.findByRole("tab", { name: "Настройки" });
    expect(screen.getByRole("tab", { name: "Подключение" })).toHaveAttribute("aria-selected", "true");

    most.vkladka("nastroyki");

    await waitFor(() => expect(screen.getByRole("tab", { name: "Настройки" })).toHaveAttribute("aria-selected", "true"));
    expect(await screen.findByTestId("obnovlenie")).toBeTruthy();
  });
});
