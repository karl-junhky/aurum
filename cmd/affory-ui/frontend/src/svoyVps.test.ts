import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Kadr } from "./most";
import {
  PUSTAYA_FORMA, hodIz, metkaRazdela, ocenitServer, posledneeDelo, sobytieHodaIz, spisokIz, svezhayaProverka, useSvoyVps,
  type HodVps, type SvoyServer,
} from "./svoyVps";

// Ответы службы с провода: формы повторяют cmd/affory-svc/svoy_vps*.go.

function server(dop: Partial<SvoyServer> = {}): SvoyServer {
  return {
    id: "svoy-1", imya: "Мой сервер", adres: "203.0.113.40", port_ssh: 22, polzovatel: "root",
    parol_vyklyuchen: false, maska: "www.example.net", reality: 2087, hy2: 5443, porty_ruchnye: false,
    prodlenie: "80", servery: ["a", "b"], sert_trevoga: false, klyuch_upravleniya: true, ne_iz_seti: [], ...dop,
  };
}

function hod(dop: Partial<HodVps> = {}): HodVps {
  return { id: 1, vid: "proverka", faza: "gotovo", server: "svoy-1", proverki: [], otseyany: [], preduprezhdeniya: [], povtory: [], ...dop };
}

describe("разбор ответов службы", () => {
  it("полный снимок установки", () => {
    const h = hodIz({
      id: 1762000000000, vid: "ustanovka", faza: "idyot", shag: "podbor", server: "svoy-1", adres: "203.0.113.10",
      klyuch: { transport: "hy2", port: 5443 },
      proverki: [{ transport: "reality-tcp", port: 443, vid: "net-otveta", otkaz: "молчит" }, { port: 1 }],
      otseyany: [{ proto: "tcp", port: 22, prichina: "ssh" }],
      preduprezhdeniya: [{ kod: "vps-cert-blocked", tekst: "порты 80 и 443 закрыты" }],
      nachalo: "2026-11-02T05:00:00Z", konec: "0001-01-01T00:00:00Z",
    });
    expect(h).not.toBeNull();
    expect(h?.adres).toBe("203.0.113.10");
    expect(h?.klyuch).toEqual({ transport: "hy2", port: 5443, vid: undefined, otkaz: undefined });
    // Запись без транспорта выпадает, а не становится нулём.
    expect(h?.proverki).toHaveLength(1);
    expect(h?.otseyany[0].prichina).toBe("ssh");
    expect(h?.preduprezhdeniya[0].kod).toBe("vps-cert-blocked");
    expect(h?.nachalo).toBe("2026-11-02T05:00:00Z");
    // Нулевое время Go значит «не было».
    expect(h?.konec).toBeUndefined();
  });

  it("перепроверка из сети: время повторов, ключи до её конца, предупреждение о сети", () => {
    const h = hodIz({
      id: 1, vid: "ustanovka", faza: "idyot", shag: "detektor", krug: 1,
      krug_tekst: "из твоей сети адрес сервера сейчас не открывается", povtory: ["2026-11-02T20:16:00Z", "негодно", "2026-11-02T20:24:00Z"],
      ustanovlen: { server: "svoy-1", klyuchi: [], ids: ["a"], ne_iz_seti: ["hy2", 5] },
      preduprezhdeniya: [{ kod: "vps-address-blocked", tekst: "x", iz_seti: true }],
    });
    expect(h?.povtory).toEqual(["2026-11-02T20:16:00Z", "2026-11-02T20:24:00Z"]);
    expect(h?.krug_tekst).toContain("не открывается");
    expect(h?.ustanovlen?.ne_iz_seti).toEqual(["hy2"]);
    expect(h?.preduprezhdeniya[0].iz_seti).toBe(true);
    // Служба старее окна полей не шлёт: пусто, а не сбой.
    expect(hodIz({ id: 2, vid: "proverka", faza: "gotovo" })?.povtory).toEqual([]);
    const s = spisokIz({ servery: [{ id: "svoy-1", adres: "203.0.113.40", ne_iz_seti: ["reality-tcp"], set_tekst: "сервер жив" }] });
    expect(s?.[0]).toMatchObject({ ne_iz_seti: ["reality-tcp"], set_tekst: "сервер жив" });
    expect(spisokIz({ servery: [{ id: "svoy-1", adres: "203.0.113.40" }] })?.[0].ne_iz_seti).toEqual([]);
  });

  it("обновление программы: номер новой в записи и итог в ходе, без номера итога нет", () => {
    expect(spisokIz({ servery: [{ id: "svoy-1", adres: "203.0.113.40", novaya_versiya: "0.7.0" }] })?.[0].novaya_versiya).toBe("0.7.0");
    expect(hodIz({ id: 1, vid: "obnovlenie", faza: "gotovo", obnovlenie: { bylo: "0.6.0", versiya: "0.7.0", ne_proshli: ["hy2", 5] } })?.obnovlenie)
      .toEqual({ bylo: "0.6.0", versiya: "0.7.0", uzhe: false, ne_proshli: ["hy2"] });
    expect(hodIz({ id: 1, vid: "obnovlenie", faza: "gotovo", obnovlenie: { versiya: "0.7.0", uzhe: true } })?.obnovlenie)
      .toEqual({ bylo: undefined, versiya: "0.7.0", uzhe: true, ne_proshli: [] });
    expect(hodIz({ id: 1, vid: "obnovlenie", faza: "gotovo", obnovlenie: { bylo: "0.6.0" } })?.obnovlenie).toBeUndefined();
  });

  it("маска: источник в записи, своя и вставшая в ходе", () => {
    expect(spisokIz({ servery: [{ id: "svoy-1", adres: "203.0.113.40", maska: "shop.example.org", maska_istochnik: "sosedi" }] })?.[0])
      .toMatchObject({ maska: "shop.example.org", maska_istochnik: "sosedi" });
    expect(spisokIz({ servery: [{ id: "svoy-1", adres: "203.0.113.40", maska: "dl.google.com" }] })?.[0].maska_istochnik).toBeUndefined();
    expect(hodIz({ id: 1, vid: "maska", faza: "gotovo", maska: { svoya: "www.adobe.com", brend_soglasen: true, imya: "www.adobe.com", istochnik: "brend" } })?.maska)
      .toEqual({ svoya: "www.adobe.com", brend_soglasen: true, imya: "www.adobe.com", istochnik: "brend" });
    expect(hodIz({ id: 1, vid: "maska", faza: "idyot", maska: { svoya: 5, brend_soglasen: "da" } })?.maska)
      .toEqual({ svoya: undefined, brend_soglasen: undefined, imya: undefined, istochnik: undefined });
    expect(hodIz({ id: 1, vid: "maska", faza: "idyot" })?.maska).toBeUndefined();
  });

  it("«заданий не было» и чужая фаза дают null", () => {
    expect(hodIz({ faza: "net" })).toBeNull();
    expect(hodIz({ id: 1, vid: "osmotr", faza: "zhdyot" })).toBeNull();
    expect(hodIz(null)).toBeNull();
    expect(hodIz([])).toBeNull();
  });

  it("осмотр без ключа хоста не принимается: установка по нему не вошла бы", () => {
    const h = hodIz({ id: 1, vid: "osmotr", faza: "gotovo", osmotr: { put: "напрямую через Ethernet", root: true, zamechaniya: [] } });
    expect(h?.osmotr).toBeUndefined();
  });

  it("событие хода несёт только номер, вид, фазу и шаг", () => {
    expect(sobytieHodaIz({ id: 5, vid: "snos", faza: "gotovo", shag: "vhod", server: "svoy-1", otkaz: { kod: "x" } }))
      .toEqual({ id: 5, vid: "snos", faza: "gotovo", shag: "vhod", server: "svoy-1", planovaya: undefined });
    expect(sobytieHodaIz({ id: "5", vid: "snos", faza: "gotovo" })).toBeNull();
  });

  it("список: без массива это служба старее окна, запись без адреса выпадает", () => {
    expect(spisokIz({})).toBeNull();
    expect(spisokIz({ servery: [] })).toEqual([]);
    const s = spisokIz({ servery: [{ id: "svoy-1", adres: "203.0.113.40", proveren: "0001-01-01T00:00:00Z" }, { id: "svoy-2" }] });
    expect(s).toHaveLength(1);
    expect(s?.[0].port_ssh).toBe(22);
    expect(s?.[0].proveren).toBeUndefined();
    expect(s?.[0].klyuch_upravleniya).toBe(false);
  });
});

describe("оценка сервера", () => {
  it("последнее законченное дело, а не идущее", () => {
    const hody = [hod({ id: 1 }), hod({ id: 3, faza: "idyot" }), hod({ id: 2, vid: "snos" })];
    expect(posledneeDelo(hody, "svoy-1", "proverka")?.id).toBe(1);
  });

  it("свежая проверка видела мёртвый VPN: не работает", () => {
    const h = hod({ proverka: { yadro_zhivo: false, oshibka_yadra: "порт занят", porty: [], maska: "godna", pamyat_mb: 1, disk_mb: 1 } });
    const o = ocenitServer(server(), [], [h]);
    expect(o.sostoyanie).toBe("slomano");
    // Текст ядра сервера уходит в отчёт, карточка советует подбор и ведёт к отчёту.
    expect(o.sovet).toContain("Подбери порты заново");
    expect(o.sovet).toContain("Скопировать отчёт");
    expect(o.sovet).not.toContain("порт занят");
    const bezTeksta = hod({ proverka: { yadro_zhivo: false, porty: [], maska: "godna", pamyat_mb: 1, disk_mb: 1 } });
    expect(ocenitServer(server(), [], [bezTeksta]).sovet).not.toContain("отчёт");
  });

  it("ни один порт не слушает: не работает, совет подобрать заново", () => {
    const h = hod({
      proverka: { yadro_zhivo: true, porty: [{ vid: "reality", port: 2087, slushaet: false }, { vid: "hy2", port: 5443, slushaet: false }], maska: "godna", pamyat_mb: 1, disk_mb: 1 },
    });
    expect(ocenitServer(server(), [], [h])).toMatchObject({ sostoyanie: "slomano", sovet: expect.stringContaining("Подбери порты") });
  });

  it("не работает один порт: нужно внимание, главнее того, что не открывается из сети", () => {
    const h = hod({
      proverka: { yadro_zhivo: true, porty: [{ vid: "reality", port: 2087, slushaet: false }, { vid: "hy2", port: 5443, slushaet: true }], maska: "godna", pamyat_mb: 1, disk_mb: 1 },
    });
    expect(ocenitServer(server({ ne_iz_seti: ["reality-tcp"] }), [], [h])).toEqual({
      sostoyanie: "vnimanie", prichina: "На сервере не работает reality", sovet: expect.stringContaining("Подбери порты"),
    });
  });

  it("маска перестала отвечать: главнее сети человека, но не мёртвого порта", () => {
    const s = server({ maska_ne_otvechaet: "сайт не отвечает h2", ne_iz_seti: ["reality-tcp"] });
    expect(ocenitServer(s, [], [])).toEqual({
      sostoyanie: "vnimanie", prichina: "Маска перестала отвечать",
      sovet: "Сайт не отвечает h2. С ней ключ reality не подключится, смени маску", maska: true,
    });
    const h = hod({
      proverka: { yadro_zhivo: true, porty: [{ vid: "reality", port: 2087, slushaet: false }, { vid: "hy2", port: 5443, slushaet: true }], maska: "godna", pamyat_mb: 1, disk_mb: 1 },
    });
    expect(ocenitServer(s, [], [h]).prichina).toBe("На сервере не работает reality");
    // reality нет - маска ни на что не влияет.
    expect(ocenitServer(server({ maska_ne_otvechaet: "нет", reality: undefined }), [], []).sostoyanie).toBe("horosho");
    expect(spisokIz({ servery: [{ ...server(), maska_ne_otvechaet: "нет" }] })?.[0].maska_ne_otvechaet).toBe("нет");
  });

  it("после проверки сервер подбирали на тех же портах: старая проверка не судит", () => {
    const h = hod({
      konec: "2026-11-02T10:00:00Z",
      proverka: { yadro_zhivo: true, porty: [{ vid: "reality", port: 2087, slushaet: false }, { vid: "hy2", port: 5443, slushaet: true }], maska: "godna", pamyat_mb: 1, disk_mb: 1 },
    });
    expect(ocenitServer(server({ proveren: "2026-11-02T10:05:00Z" }), [], [h]).sostoyanie).toBe("horosho");
    expect(svezhayaProverka(server({ proveren: "2026-11-02T09:59:59Z" }), [h])).toBe(h.proverka);
    // Принят новый ключ сервера: отметки визита нет, прежняя проверка не в счёт.
    expect(svezhayaProverka(server(), [h])).toBeUndefined();
  });

  it("проверка видела прежние порты: о нынешних не судит", () => {
    const h = hod({
      proverka: { yadro_zhivo: true, porty: [{ vid: "reality", port: 443, slushaet: false }, { vid: "hy2", port: 8443, slushaet: false }], maska: "godna", pamyat_mb: 1, disk_mb: 1 },
    });
    expect(ocenitServer(server(), [], [h]).sostoyanie).toBe("horosho");
  });

  it("программа сервера не ответила: о VPN проверка не судит", () => {
    const h = hod({ proverka: { yadro_zhivo: false, programma_otkaz: "нет ответа", porty: [], maska: "ne-provereno", pamyat_mb: -1, disk_mb: -1 } });
    expect(ocenitServer(server(), [], [h]).sostoyanie).toBe("horosho");
  });

  it("тревога статуса без хвоста про раздел", () => {
    const o = ocenitServer(server(), [{ id: "svoy-1", imya: "Мой сервер", tekst: "сертификат истекает через 5 дней. Проверь сервер в разделе «Свой VPN»" }], []);
    expect(o).toEqual({ sostoyanie: "vnimanie", prichina: "Сертификат истекает через 5 дней" });
    const st = ocenitServer(server(), [{ id: "svoy-1", imya: "Мой сервер",
      tekst: "на сервере программа 0.5.0 с уязвимостью, исправленной в новой. Обнови её в разделе «Свой VPN»" }], []);
    expect(st).toEqual({ sostoyanie: "vnimanie", prichina: "На сервере программа 0.5.0 с уязвимостью, исправленной в новой" });
  });

  it("отказ последней проверки и новый ключ хоста: нужно внимание", () => {
    expect(ocenitServer(server({ proverka_otkaz: "сервер молчит" }), [], []).sostoyanie).toBe("vnimanie");
    expect(ocenitServer(server({ novyy_otpechatok: "SHA256:x" }), [], []).sostoyanie).toBe("vnimanie");
    expect(ocenitServer(server(), [], []).sostoyanie).toBe("horosho");
  });

  it("ключи не прошли из сети: нужно внимание, и здоровый сервер его не снимает, снимает только запись", () => {
    const zdorovyy = hod({ proverka: { yadro_zhivo: true, porty: [{ vid: "reality", port: 2087, slushaet: true }], maska: "godna", pamyat_mb: 1, disk_mb: 1 } });
    const o = ocenitServer(server({ ne_iz_seti: ["reality-tcp", "hy2"], set_tekst: "сервер жив, ключи стоят" }), [], [zdorovyy]);
    expect(o).toEqual({ sostoyanie: "vnimanie", prichina: "Из твоей сети не открываются reality и hysteria2", sovet: "Сервер жив, ключи стоят", izSeti: true });
    // Тревога главнее: причина о сети не из записи, и строка дела её не прячет.
    expect(ocenitServer(server({ ne_iz_seti: ["hy2"] }), [{ id: "svoy-1", imya: "Мой сервер", tekst: "сертификат не продлевается" }], []).izSeti).toBeUndefined();
    expect(ocenitServer(server({ ne_iz_seti: ["hy2"] }), [], []).prichina).toBe("Из твоей сети не открывается hysteria2");
    expect(metkaRazdela([server({ ne_iz_seti: ["hy2"] })], [], [])).toBe("warn");
  });

  it("метка раздела: худшее из состояний, тревога видна и без списка", () => {
    const mertvyy = hod({ server: "svoy-2", proverka: { yadro_zhivo: false, porty: [], maska: "godna", pamyat_mb: 1, disk_mb: 1 } });
    expect(metkaRazdela([server({ proverka_otkaz: "x" }), server({ id: "svoy-2" })], [], [mertvyy])).toBe("danger");
    expect(metkaRazdela(null, [{ id: "svoy-1", imya: "", tekst: "x" }], [])).toBe("warn");
    expect(metkaRazdela([server()], [], [])).toBeUndefined();
  });
});

// Подставная служба для хука: ответы по имени команды и рассылка событий.
function sluzhba(otvety: Record<string, (telo: unknown) => Kadr | Promise<Kadr>>) {
  const podpischiki: ((k: Kadr) => void)[] = [];
  const zvat = vi.fn(async (imya: string, telo: unknown = {}): Promise<Kadr> => {
    const o = otvety[imya];
    return o ? o(telo) : { tip: "otvet", id: 1, imya, telo: {} };
  });
  const naSobytie = (o: (k: Kadr) => void) => {
    podpischiki.push(o);
    return () => { podpischiki.splice(podpischiki.indexOf(o), 1); };
  };
  const sobytie = (telo: unknown) => { for (const p of [...podpischiki]) p({ tip: "sobytie", id: 0, imya: "svoy-vps-hod", telo }); };
  return { zvat, naSobytie, sobytie };
}

const otvet = (imya: string, telo: unknown): Kadr => ({ tip: "otvet", id: 1, imya, telo });

describe("связь раздела со службой", () => {
  afterEach(() => localStorage.clear());

  it("при связи спрашивает список и ход", async () => {
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [{ id: "svoy-1", adres: "203.0.113.40" }] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { faza: "net" }),
    });
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey: vi.fn(), aktiven: true }));
    await waitFor(() => expect(result.current.servery).toHaveLength(1));
    expect(result.current.adresServera("svoy-1")).toBe("203.0.113.40");
    expect(s.zvat).toHaveBeenCalledWith("svoyVpsHod", {});
  });

  it("без связи не спрашивает ничего", async () => {
    const s = sluzhba({});
    renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey: vi.fn(), aktiven: false }));
    await act(async () => { await Promise.resolve(); });
    expect(s.zvat).not.toHaveBeenCalled();
  });

  it("установка, начатая до окна, возвращает мастер на свой шаг", async () => {
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { id: 7, vid: "ustanovka", faza: "idyot", shag: "podbor" }),
    });
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey: vi.fn(), aktiven: true }));
    await waitFor(() => expect(result.current.master).toEqual({ shag: "ustanovka", forma: null, id: 7 }));
  });

  it("увиденная упавшая установка мастер не возвращает", async () => {
    localStorage.setItem("affory.svoy-vps.uvidennaya-ustanovka", "7");
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { id: 7, vid: "ustanovka", faza: "oshibka", konec: new Date().toISOString(), otkaz: { kod: "vps-ports-failed" } }),
    });
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey: vi.fn(), aktiven: true }));
    await waitFor(() => expect(result.current.hody).toHaveLength(1));
    expect(result.current.master).toBeNull();
  });

  it("установка, перепроверки которой не стали ждать, мастер не возвращает", async () => {
    localStorage.setItem("affory.svoy-vps.uvidennaya-ustanovka", "7");
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { id: 7, vid: "ustanovka", faza: "idyot", shag: "detektor", ustanovlen: { server: "svoy-1", klyuchi: [], ids: ["a"] } }),
    });
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey: vi.fn(), aktiven: true }));
    await waitFor(() => expect(result.current.hody).toHaveLength(1));
    expect(result.current.master).toBeNull();
  });

  it("ключи записаны до конца установки: общие ключи перечитываются сразу и раз", async () => {
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { id: 7, vid: "ustanovka", faza: "idyot", shag: "detektor", ustanovlen: { server: "svoy-1", klyuchi: [], ids: ["a"] } }),
    });
    const naSmenuKlyuchey = vi.fn();
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey, aktiven: true }));
    await waitFor(() => expect(result.current.hody[0]?.ustanovlen).toBeDefined());
    act(() => s.sobytie({ id: 7, vid: "ustanovka", faza: "idyot", shag: "detektor" }));
    await waitFor(() => expect(s.zvat.mock.calls.filter(([i]) => i === "svoyVpsHod").length).toBeGreaterThan(1));
    expect(naSmenuKlyuchey).toHaveBeenCalledTimes(1);
  });

  it("конец установки событием: список и общие ключи перечитываются", async () => {
    let faza = "idyot";
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { id: 7, vid: "ustanovka", faza }),
    });
    const naSmenuKlyuchey = vi.fn();
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey, aktiven: true }));
    await waitFor(() => expect(result.current.hody[0]?.faza).toBe("idyot"));
    const spiskov = s.zvat.mock.calls.filter(([i]) => i === "svoyVpsSpisok").length;
    faza = "gotovo";
    act(() => s.sobytie({ id: 7, vid: "ustanovka", faza: "gotovo" }));
    await waitFor(() => expect(result.current.hody[0]?.faza).toBe("gotovo"));
    expect(naSmenuKlyuchey).toHaveBeenCalledTimes(1);
    expect(s.zvat.mock.calls.filter(([i]) => i === "svoyVpsSpisok").length).toBeGreaterThan(spiskov);
  });

  it("конец смены маски: ключ reality сменился, общие ключи перечитываются", async () => {
    let faza = "idyot";
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { id: 8, vid: "maska", faza }),
    });
    const naSmenuKlyuchey = vi.fn();
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey, aktiven: true }));
    await waitFor(() => expect(result.current.hody[0]?.faza).toBe("idyot"));
    faza = "gotovo";
    act(() => s.sobytie({ id: 8, vid: "maska", faza: "gotovo" }));
    await waitFor(() => expect(result.current.hody[0]?.faza).toBe("gotovo"));
    expect(naSmenuKlyuchey).toHaveBeenCalledTimes(1);
  });

  it("опоздавший снимок «идёт» не возвращает законченное дело назад", async () => {
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { faza: "net" }),
      svoyVpsProverit: () => otvet("svoyVpsProverit", { id: 3, vid: "proverka", faza: "gotovo", server: "svoy-1" }),
      svoyVpsSnesti: () => otvet("svoyVpsSnesti", { id: 3, vid: "proverka", faza: "idyot", server: "svoy-1" }),
    });
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey: vi.fn(), aktiven: true }));
    await act(async () => { await result.current.komanda("svoyVpsProverit", { id: "svoy-1" }); });
    await act(async () => { await result.current.komanda("svoyVpsSnesti", { id: "svoy-1" }); });
    expect(result.current.hody.find((h) => h.id === 3)?.faza).toBe("gotovo");
  });

  it("начальный снимок из ответа команды не откатывает шаг, пришедший событием", async () => {
    let zapusk: (k: Kadr) => void = () => {};
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { id: 8, vid: "ustanovka", faza: "idyot", shag: "programma" }),
      svoyVpsUstanovit: () => new Promise<Kadr>((r) => { zapusk = r; }),
    });
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey: vi.fn(), aktiven: true }));
    let otvetKomandy: Promise<unknown> = Promise.resolve();
    act(() => { otvetKomandy = result.current.komanda("svoyVpsUstanovit", {}); });
    act(() => s.sobytie({ id: 8, vid: "ustanovka", faza: "idyot", shag: "programma" }));
    await waitFor(() => expect(result.current.hody.find((h) => h.id === 8)?.shag).toBe("programma"));
    await act(async () => {
      zapusk(otvet("svoyVpsUstanovit", { id: 8, vid: "ustanovka", faza: "idyot" }));
      await otvetKomandy;
    });
    expect(result.current.hody.find((h) => h.id === 8)?.shag).toBe("programma");
  });

  it("после удачной установки пароль и ключ уходят из мастера и без экрана", async () => {
    let faza = "idyot";
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { id: 9, vid: "ustanovka", faza }),
    });
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey: vi.fn(), aktiven: true }));
    await waitFor(() => expect(result.current.hody[0]?.faza).toBe("idyot"));
    const forma = { ...PUSTAYA_FORMA, adres: "203.0.113.10", parol: "primer-parolya", parolKlyucha: "pk" };
    act(() => result.current.zadatMaster({ shag: "ustanovka", forma, id: 9 }));
    expect(result.current.master).toMatchObject({ forma: { parol: "primer-parolya" } });
    faza = "gotovo";
    act(() => s.sobytie({ id: 9, vid: "ustanovka", faza: "gotovo" }));
    await waitFor(() => expect(result.current.master).toEqual({ shag: "ustanovka", id: 9, forma: { ...PUSTAYA_FORMA, adres: "203.0.113.10" } }));
  });

  it("обрыв канала уходит наверх и возвращается отказом на месте", async () => {
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { faza: "net" }),
      svoyVpsProverit: () => { throw new Error("канал оборвался"); },
    });
    const naSboy = vi.fn();
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy, naSmenuKlyuchey: vi.fn(), aktiven: true }));
    let itog: unknown;
    await act(async () => { itog = await result.current.komanda("svoyVpsProverit", { id: "svoy-1" }); });
    expect(naSboy).toHaveBeenCalledTimes(1);
    expect(itog).toMatchObject({ otkaz: { tekst: expect.stringContaining("канал оборвался") } });
  });

  it("«забыть» идёт без задания: список и общие ключи перечитываются сразу", async () => {
    const s = sluzhba({
      svoyVpsSpisok: () => otvet("svoyVpsSpisok", { servery: [] }),
      svoyVpsHod: () => otvet("svoyVpsHod", { faza: "net" }),
      svoyVpsZabyt: () => otvet("svoyVpsZabyt", { id: "svoy-1" }),
    });
    const naSmenuKlyuchey = vi.fn();
    const { result } = renderHook(() => useSvoyVps({ ...s, naSboy: vi.fn(), naSmenuKlyuchey, aktiven: true }));
    await waitFor(() => expect(result.current.servery).toEqual([]));
    const spiskov = s.zvat.mock.calls.filter(([i]) => i === "svoyVpsSpisok").length;
    await act(async () => { await result.current.komanda("svoyVpsZabyt", { id: "svoy-1" }); });
    expect(naSmenuKlyuchey).toHaveBeenCalledTimes(1);
    expect(s.zvat.mock.calls.filter(([i]) => i === "svoyVpsSpisok").length).toBe(spiskov + 1);
    expect(result.current.hody).toEqual([]);
  });
});
