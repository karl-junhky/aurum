import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useCallback, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  SvoyVpn, chistyyAdres, etapyUstanovki, itogDela, oshibkaAdresa, oshibkaPorta, otchyotDela, probyPortov,
  punktyOsmotra, sostoyanieKlyucha, strokaPerehvata, teloDostupa, zanyatyeDlyaReality,
} from "./SvoyVpn";
import {
  PUSTAYA_FORMA, type HodVps, type ItogKomandy, type Master, type OsmotrVps, type SvoyServer, type SvoyVps,
} from "../svoyVps";
import { razobratSoobshchenie } from "../soobshchenieHostera";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

const SEYCHAS = () => new Date().toISOString();

function server(dop: Partial<SvoyServer> = {}): SvoyServer {
  return {
    id: "svoy-1", imya: "Мой сервер", adres: "203.0.113.40", port_ssh: 22, polzovatel: "root",
    parol_vyklyuchen: false, maska: "www.example.net", reality: 2087, hy2: 5443, porty_ruchnye: false,
    prodlenie: "80", servery: ["k-r", "k-h"], sert_trevoga: false, klyuch_upravleniya: true, ne_iz_seti: [],
    proveren: SEYCHAS(), ...dop,
  };
}

function hod(dop: Partial<HodVps>): HodVps {
  return { id: 1, vid: "proverka", faza: "gotovo", proverki: [], otseyany: [], preduprezhdeniya: [], povtory: [], ...dop };
}

function osmotr(dop: Partial<OsmotrVps> = {}): OsmotrVps {
  return {
    put: "напрямую через Ethernet", root: true, klyuch_hosta_tip: "ssh-ed25519", klyuch_hosta: "S0g=",
    otpechatok: "SHA256:abc", sistema: "Ubuntu 24.04, x86_64", zamechaniya: [], mozhno_stavit: true, ...dop,
  };
}

type Otvety = Record<string, (telo: Record<string, unknown>) => ItogKomandy>;

/** Раздел с подставным useSvoyVps: мастер и ход живут в состоянии обёртки,
 *  команды пишутся в журнал и отвечают по таблице. */
function stend({ servery = [], hody = [], master = null, otvety = {} }: {
  servery?: SvoyServer[] | null;
  hody?: HodVps[];
  master?: Master | null;
  otvety?: Otvety;
} = {}) {
  const vyzovy: [string, Record<string, unknown>][] = [];
  const naPodklyuchit = vi.fn();
  const ochistitBufer = vi.fn(async () => true);
  const vybratKlyuch = vi.fn(async () => ({ imya: "id_ed25519", base64: "a2V5" }));
  const skopirovat = vi.fn(async () => {});
  const obnovit = vi.fn();
  const skryt = vi.fn();
  const sostoyanie: { master: Master | null; zadatHody: (f: (h: HodVps[]) => HodVps[]) => void } = { master, zadatHody: () => {} };

  function Obertka() {
    const [m, zadatMaster] = useState<Master | null>(master);
    const [h, zadatHody] = useState<HodVps[]>(hody);
    // Как в окне: открытие раздела перечитывает список, и приходит новый снимок.
    const [sp, zadatSp] = useState<SvoyServer[] | null>(servery);
    const perechitat = useCallback(() => {
      obnovit();
      zadatSp((x) => (x === null ? x : [...x]));
    }, []);
    sostoyanie.master = m;
    sostoyanie.zadatHody = zadatHody;
    const svoy: SvoyVps = {
      servery: sp, spisokOtkaz: null, hody: h, master: m, zadatMaster, skryty: [], skryt, obnovit: perechitat,
      komanda: async (imya, telo) => {
        const t = (telo ?? {}) as Record<string, unknown>;
        vyzovy.push([imya, t]);
        const r = otvety[imya]?.(t) ?? { telo: {} };
        if ("hod" in r) zadatHody((pr) => [...pr.filter((x) => x.id !== r.hod.id), r.hod]);
        return r;
      },
      adresServera: () => "203.0.113.40",
    };
    return (
      <SvoyVpn
        svoy={svoy} trevogi={[]} aktiven naPodklyuchit={naPodklyuchit} naServery={vi.fn()} skopirovat={skopirovat}
        kodyQr={async () => []} vybratKlyuch={vybratKlyuch} ochistitBufer={ochistitBufer} versiya="Affory 1.11.0"
      />
    );
  }
  render(<Obertka />);
  return {
    vyzovy, naPodklyuchit, ochistitBufer, vybratKlyuch, skopirovat,
    master: () => sostoyanie.master,
    imena: () => vyzovy.map(([i]) => i),
    prishyol: (x: HodVps) => act(() => sostoyanie.zadatHody((pr) => [...pr.filter((y) => y.id !== x.id), x])),
  };
}

const PISMO = ["Your server is ready", "", "IP address: 203.0.113.10", "Username: root", "Password: primer-parolya", "SSH port: 22"].join("\n");

describe("пустой раздел", () => {
  it("ведёт в мастер на шаг «Доступ»", () => {
    const s = stend();
    fireEvent.click(screen.getByTestId("svoy-postavit"));
    expect(s.master()).toMatchObject({ shag: "dostup", cel: "postavit" });
    expect(screen.getByRole("list", { name: "Шаги установки" })).toBeInTheDocument();
    expect(within(screen.getByRole("list", { name: "Шаги установки" })).getByText("Доступ").closest("li")).toHaveAttribute("aria-current", "step");
  });
});

describe("шаг «Доступ»", () => {
  const nachalo: Master = { shag: "dostup", cel: "postavit", forma: PUSTAYA_FORMA };

  it("письмо хостера разбирается по полям, пароль из буфера предлагается стереть", async () => {
    const s = stend({ master: nachalo });
    fireEvent.change(screen.getByTestId("svoy-soobshchenie"), { target: { value: PISMO } });
    expect(screen.getByTestId("svoy-adres")).toHaveValue("203.0.113.10");
    expect(screen.getByTestId("svoy-parol")).toHaveValue("primer-parolya");
    expect(screen.getByText("Пароль остался в буфере обмена")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("svoy-ochistit-bufer"));
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Буфер очищен"));
    expect(s.ochistitBufer).toHaveBeenCalledWith("primer-parolya");
  });

  it("в пустом поле письма пример, и он сам разбирается по полям", () => {
    stend({ master: nachalo });
    const primer = (screen.getByTestId("svoy-soobshchenie").getAttribute("placeholder") ?? "").split("Например:")[1] ?? "";
    expect(razobratSoobshchenie(primer)).toMatchObject({ adres: "203.0.113.10", polzovatel: "root", parol: "Kf8mQ2xLp" });
  });

  it("правка письма не затирает поле, поправленное руками", () => {
    stend({ master: nachalo });
    fireEvent.change(screen.getByTestId("svoy-soobshchenie"), { target: { value: PISMO } });
    fireEvent.change(screen.getByTestId("svoy-parol"), { target: { value: "ispravlennyy" } });
    fireEvent.change(screen.getByTestId("svoy-soobshchenie"), { target: { value: PISMO.replace("Your server is ready", "") } });
    expect(screen.getByTestId("svoy-parol")).toHaveValue("ispravlennyy");
    // Адрес руками не трогали: исправление в письме до него доходит.
    fireEvent.change(screen.getByTestId("svoy-soobshchenie"), { target: { value: PISMO.replace("203.0.113.10", "203.0.113.11") } });
    expect(screen.getByTestId("svoy-adres")).toHaveValue("203.0.113.11");
  });

  it("письмо, вставленное после очистки поля, берёт всё заново", () => {
    stend({ master: nachalo });
    fireEvent.change(screen.getByTestId("svoy-soobshchenie"), { target: { value: PISMO } });
    fireEvent.change(screen.getByTestId("svoy-soobshchenie"), { target: { value: "" } });
    fireEvent.change(screen.getByTestId("svoy-soobshchenie"), {
      target: { value: PISMO.replace("203.0.113.10", "203.0.113.12").replace("primer-parolya", "drugoy-parol") },
    });
    expect(screen.getByTestId("svoy-adres")).toHaveValue("203.0.113.12");
    expect(screen.getByTestId("svoy-parol")).toHaveValue("drugoy-parol");
  });

  it("набор по знаку не считается вставкой", () => {
    stend({ master: { ...nachalo, forma: { ...PUSTAYA_FORMA, soobshchenie: PISMO.slice(0, -1) } } });
    fireEvent.change(screen.getByTestId("svoy-soobshchenie"), { target: { value: PISMO } });
    expect(screen.queryByText("Пароль остался в буфере обмена")).toBeNull();
  });

  it("переход на ключ стирает пароль и письмо, файл выбирается окном", async () => {
    const s = stend({ master: nachalo });
    fireEvent.change(screen.getByTestId("svoy-soobshchenie"), { target: { value: PISMO } });
    fireEvent.click(screen.getByRole("radio", { name: "Ключ" }));
    expect(screen.getByTestId("svoy-soobshchenie")).toHaveValue("");
    expect(screen.queryByTestId("svoy-parol")).toBeNull();
    expect(s.master()).toMatchObject({ forma: { parol: "", soobshchenie: "", sposob: "klyuch" } });
    fireEvent.click(screen.getByTestId("svoy-vybrat-klyuch"));
    await waitFor(() => expect(screen.getByText("id_ed25519")).toBeInTheDocument());
  });

  it("имя вместо адреса и reality на порту SSH не пускают дальше", () => {
    stend({ master: { ...nachalo, forma: { ...PUSTAYA_FORMA, adres: "vps.example.net", parol: "x" } } });
    expect(screen.getByText("нужен IP-адрес из письма хостера, а не имя")).toBeInTheDocument();
    expect(screen.getByTestId("svoy-dalshe")).toBeDisabled();
    fireEvent.change(screen.getByTestId("svoy-adres"), { target: { value: "203.0.113.10" } });
    expect(screen.getByTestId("svoy-dalshe")).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: /Дополнительно/ }));
    fireEvent.change(screen.getByLabelText("reality, TCP"), { target: { value: "22" } });
    expect(screen.getByText("22 занят входом на сервер")).toBeInTheDocument();
    expect(screen.getByTestId("svoy-dalshe")).toBeDisabled();
  });

  it("в службу уходят только поля входа, письмо остаётся в окне", async () => {
    const s = stend({
      master: { ...nachalo, forma: { ...PUSTAYA_FORMA, soobshchenie: PISMO, adres: "203.0.113.10", parol: "primer-parolya" } },
      otvety: { svoyVpsOsmotret: () => ({ hod: hod({ id: 10, vid: "osmotr", faza: "idyot", shag: "vhod" }) }) },
    });
    fireEvent.click(screen.getByTestId("svoy-dalshe"));
    await waitFor(() => expect(s.master()?.shag).toBe("proverka"));
    expect(s.vyzovy[0]).toEqual(["svoyVpsOsmotret", { adres: "203.0.113.10", port: 22, polzovatel: "root", parol: "primer-parolya" }]);
    expect(s.master()).toMatchObject({ forma: { soobshchenie: "" } });
  });

  it("отказ осмотра возвращает на доступ с причиной", async () => {
    const s = stend({
      master: { ...nachalo, forma: { ...PUSTAYA_FORMA, adres: "203.0.113.10", parol: "x" } },
      otvety: { svoyVpsOsmotret: () => ({ otkaz: { kod: "vps-busy", tekst: "служба занята" } }) },
    });
    fireEvent.click(screen.getByTestId("svoy-dalshe"));
    await waitFor(() => expect(s.master()).toMatchObject({ shag: "dostup", otkaz: { kod: "vps-busy" } }));
    expect(screen.getByRole("alert")).toHaveTextContent("Служба занята");
  });
});

describe("шаг «Проверка»", () => {
  const forma = { ...PUSTAYA_FORMA, adres: "203.0.113.10", parol: "primer-parolya" };

  it("пункты осмотра, согласие на пустой сервер и установка", async () => {
    const s = stend({
      master: { shag: "proverka", forma, id: 10, pustoy: false },
      hody: [hod({ id: 10, vid: "osmotr", osmotr: osmotr({ zamechaniya: [{ kod: "net-bbr", stop: false, tekst: "ядро без BBR" }] }) })],
      otvety: { svoyVpsUstanovit: () => ({ hod: hod({ id: 11, vid: "ustanovka", faza: "idyot", shag: "zagruzka" }) }) },
    });
    const punkty = screen.getByTestId("svoy-punkty");
    expect(punkty).toHaveTextContent("Ubuntu 24.04");
    expect(punkty).toHaveTextContent("Скорость");
    expect(punkty).toHaveTextContent("есть замечание");
    expect(screen.getByTestId("svoy-postavit-vpn")).toBeDisabled();
    fireEvent.click(screen.getByTestId("svoy-pustoy"));
    fireEvent.click(screen.getByTestId("svoy-postavit-vpn"));
    await waitFor(() => expect(s.master()).toMatchObject({ shag: "ustanovka", id: 11 }));
    const [imya, telo] = s.vyzovy[0];
    expect(imya).toBe("svoyVpsUstanovit");
    expect(telo).toMatchObject({ adres: "203.0.113.10", pustoy: true, klyuch_hosta_tip: "ssh-ed25519", klyuch_hosta: "S0g=", imya: "" });
    expect(telo).not.toHaveProperty("soobshchenie");
    // Обычная установка запись не заменяет: служба откажет, если сервер уже в разделе.
    expect(telo.poverh).toBeUndefined();
  });

  it("мастер из «Поставить заново» просит ставить поверх записи этого адреса", async () => {
    const s = stend({
      master: { shag: "proverka", forma: { ...forma, poverhZapisi: "203.0.113.10" }, id: 10, pustoy: true },
      hody: [hod({ id: 10, vid: "osmotr", osmotr: osmotr() })],
      otvety: { svoyVpsUstanovit: () => ({ hod: hod({ id: 11, vid: "ustanovka", faza: "idyot", shag: "zagruzka" }) }) },
    });
    fireEvent.click(screen.getByTestId("svoy-postavit-vpn"));
    await waitFor(() => expect(s.vyzovy[0]?.[1]).toMatchObject({ adres: "203.0.113.10", poverh: "203.0.113.10" }));
  });

  it("свой Affory без ключа этого компьютера: только взять под управление", async () => {
    const s = stend({
      master: { shag: "proverka", forma, id: 10, pustoy: false },
      hody: [hod({ id: 10, vid: "osmotr", osmotr: osmotr({ mozhno_stavit: false, zamechaniya: [{ kod: "uzhe-stoit", stop: true, tekst: "Affory уже стоит" }] }) })],
      otvety: { svoyVpsVzyat: () => ({ hod: hod({ id: 12, vid: "vzyat", faza: "idyot", server: "svoy-1" }) }) },
    });
    expect(screen.queryByTestId("svoy-postavit-vpn")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Взять под управление" }));
    await waitFor(() => expect(s.master()).toBeNull());
    expect(s.vyzovy[0]).toEqual(["svoyVpsVzyat", expect.objectContaining({ adres: "203.0.113.10", klyuch_hosta: "S0g=" })]);
  });

  it("путь через свой VPN называется VPN, а не словом службы", () => {
    const p = punktyOsmotra(hod({ vid: "osmotr", osmotr: osmotr({ put: "через т" + "уннель" }) }), "parol", 22);
    expect(p.find((x) => x.imya === "Путь")).toMatchObject({ itog: "vnimanie", podrobno: expect.stringContaining("через VPN") });
  });
});

describe("шаг «Установка»", () => {
  it("фишки подбора с причинами, отмена идущей установки", async () => {
    const s = stend({
      master: { shag: "ustanovka", forma: null, id: 11 },
      hody: [hod({
        id: 11, vid: "ustanovka", faza: "idyot", shag: "podbor",
        otseyany: [{ proto: "tcp", port: 22, prichina: "ssh" }],
        proverki: [{ transport: "reality-tcp", port: 443, vid: "net-otveta" }, { transport: "reality-tcp", port: 2087, vid: "vezyot" }],
        klyuch: { transport: "hy2", port: 5443 },
      })],
    });
    const reality = screen.getByRole("list", { name: "Порты reality, TCP" });
    expect(within(reality).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "22не прошёл: на нём вход на сервер", "443не прошёл: не ответил", "2087прошёл",
    ]);
    expect(within(screen.getByRole("list", { name: "Порты hysteria2, UDP" })).getByText(/проверяется/)).toBeInTheDocument();
    expect(screen.getByText("Пробую 5443 по UDP для hysteria2")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("svoy-otmenit-ustanovku"));
    await waitFor(() => expect(s.imena()).toContain("svoyVpsOtmenit"));
  });

  it("подбор под Avast называет его на фишке, строкой под портами и в итоге", () => {
    const otkaz = "нет ответа через ключ: соединение не установилось";
    const proverki = [
      { transport: "reality-tcp", port: 465, vid: "net-otveta", otkaz, pomeha: "Avast" },
      { transport: "reality-tcp", port: 993, vid: "net-otveta", otkaz, pomeha: "Avast" },
      { transport: "reality-tcp", port: 2053, vid: "vezyot" },
    ];
    const stroka = "Avast на этом компьютере перехватывает порты 465 и 993, VPN через них не проходит";
    stend({ master: { shag: "ustanovka", forma: null, id: 11 }, hody: [hod({ id: 11, vid: "ustanovka", faza: "idyot", shag: "proverka", proverki })] });
    const reality = screen.getByRole("list", { name: "Порты reality, TCP" });
    expect(within(reality).getAllByRole("listitem")[1]).toHaveTextContent("993не прошёл: не ответил: порт перехватывает Avast на этом компьютере");
    expect(screen.getByText(stroka)).toBeInTheDocument();
    cleanup();

    // Фишек на «Готово» уже нет: строка остаётся под ключами.
    const ustanovlen = { server: "svoy-1", klyuchi: [{ transport: "reality-tcp", port: 2053, vid: "vezyot" }], ids: ["k-r"], ne_iz_seti: [] };
    stend({ servery: [server()], master: { shag: "ustanovka", forma: null, id: 11 }, hody: [hod({ id: 11, vid: "ustanovka", faza: "gotovo", proverki, ustanovlen })] });
    expect(screen.getByText(stroka)).toBeInTheDocument();
    expect(itogDela(hod({ vid: "podbor", faza: "gotovo", proverki, ustanovlen }), undefined)?.tekst).toBe(`reality 2053. ${stroka}`);

    // Ключ сам упёрся в Avast: о нём говорят его строка и предупреждение.
    const svoy = { ...ustanovlen, klyuchi: [{ transport: "reality-tcp", port: 993, vid: "net-otveta", otkaz, pomeha: "Avast" }] };
    expect(itogDela(hod({ vid: "podbor", faza: "gotovo", proverki, ustanovlen: svoy }), undefined)?.tekst).toBe("reality 993");
  });

  it("строка о перехвате судит порт по последней пробе", () => {
    const k = (port: number, vid: string, pomeha?: string) => ({ transport: "reality-tcp", port, vid, pomeha });
    expect(strokaPerehvata(hod({ proverki: [k(993, "net-otveta", "Avast")] }))).toBe("Avast на этом компьютере перехватывает порт 993, VPN через него не проходит");
    expect(strokaPerehvata(hod({ proverki: [k(110, "zamerz", "Avast"), k(465, "net-otveta", "Avast"), k(993, "net-otveta", "Avast")] })))
      .toBe("Avast на этом компьютере перехватывает порты 110, 465 и 993, VPN через них не проходит");
    expect(strokaPerehvata(hod({ proverki: [k(993, "net-otveta", "Avast"), k(993, "vezyot")] }))).toBe("");
    expect(strokaPerehvata(hod({ proverki: [k(443, "net-otveta")] }))).toBe("");
  });

  it("отмена, не дошедшая до службы, видна", async () => {
    stend({
      master: { shag: "ustanovka", forma: null, id: 11 },
      hody: [hod({ id: 11, vid: "ustanovka", faza: "idyot", shag: "zagruzka" })],
      otvety: { svoyVpsOtmenit: () => ({ otkaz: { kod: "oshibka-obolochki", tekst: "служба не отвечает" } }) },
    });
    fireEvent.click(screen.getByTestId("svoy-otmenit-ustanovku"));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("Служба не отвечает"));
  });


  it("на проверке готовых ключей прошедший порт не крутится снова", () => {
    stend({
      master: { shag: "ustanovka", forma: null, id: 11 },
      hody: [hod({
        id: 11, vid: "ustanovka", faza: "idyot", shag: "proverka",
        proverki: [{ transport: "reality-tcp", port: 2087, vid: "vezyot" }],
        klyuch: { transport: "reality-tcp", port: 2087 },
      })],
    });
    expect(within(screen.getByRole("list", { name: "Порты reality, TCP" })).getByRole("listitem")).toHaveTextContent("2087прошёл");
    expect(screen.queryByText(/Пробую/)).toBeNull();
    expect(screen.getByText("reality 2087: качаю 2 МБ")).toBeInTheDocument();
  });

  it("упавшая установка без пароля в памяти ведёт ввести доступ заново", () => {
    const s = stend({
      master: { shag: "ustanovka", forma: null, id: 11 },
      hody: [hod({ id: 11, vid: "ustanovka", faza: "oshibka", shag: "podbor", server: "svoy-1", otkaz: { kod: "vps-ports-failed", tekst: "ни один порт не прошёл" } })],
    });
    expect(screen.getByText(/Для повтора введи доступ к серверу заново/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Ввести доступ" }));
    expect(s.master()).toMatchObject({ shag: "dostup", forma: { adres: "203.0.113.40", poverhZapisi: "" } });
    // Итог увиден: окно, открытое заново, к нему не вернётся.
    expect(localStorage.getItem("affory.svoy-vps.uvidennaya-ustanovka")).toBe("11");
  });

  it("упавшая установка поверх записи повторяется тоже поверх записи, с входом из записи", () => {
    const s = stend({
      servery: [server({ port_ssh: 2222, parol_vyklyuchen: true })],
      master: { shag: "ustanovka", forma: null, id: 11 },
      hody: [hod({ id: 11, vid: "ustanovka", faza: "oshibka", shag: "podbor", server: "svoy-1", otkaz: { kod: "vps-ports-failed", tekst: "ни один порт не прошёл" } })],
    });
    fireEvent.click(screen.getByRole("button", { name: "Ввести доступ" }));
    expect(s.master()).toMatchObject({
      shag: "dostup",
      forma: { adres: "203.0.113.40", portSshPolya: "2222", sposob: "klyuch", poverhZapisi: "203.0.113.40" },
    });
  });

  it("готово: подключение к ключу, который провёз проверку, и ключи для телефона", async () => {
    const s = stend({
      servery: [server()],
      master: { shag: "ustanovka", forma: null, id: 11 },
      hody: [hod({
        id: 11, vid: "ustanovka", faza: "gotovo",
        ustanovlen: { server: "svoy-1", klyuchi: [{ transport: "reality-tcp", port: 2087, vid: "net-otveta" }, { transport: "hy2", port: 5443, vid: "vezyot" }], ids: ["k-r", "k-h"], ne_iz_seti: [] },
      })],
      otvety: {
        exportServers: () => ({ telo: { tekst: "hy2://x@203.0.113.40:5443", base64: "aA==", servery: [{ id: "k-h", imya: "Мой сервер, hysteria2", transport: "hy2", adres: "203.0.113.40:5443", ssylka: "hy2://x@203.0.113.40:5443" }] } }),
      },
    });
    fireEvent.click(screen.getByTestId("svoy-klyuchi-telefona"));
    await waitFor(() => expect(s.vyzovy).toContainEqual(["exportServers", { ids: ["k-r", "k-h"] }]));
    // Выгрузка зовётся так же, как кнопка, что её открыла.
    expect(await screen.findByTestId("vygruzka-itog")).toHaveTextContent("Ключи для телефона");
    fireEvent.click(screen.getByTestId("svoy-podklyuchit"));
    expect(s.naPodklyuchit).toHaveBeenCalledWith("k-h");
    expect(s.master()).toBeNull();
  });

  it("пустой круг перепроверки: сразу причина, время повторов и «Не ждать»", () => {
    const povtor = new Date(Date.now() + 2 * 60_000).toISOString();
    const s = stend({
      servery: [server({ ne_iz_seti: ["reality-tcp", "hy2"] })],
      master: { shag: "ustanovka", forma: null, id: 11 },
      hody: [hod({
        id: 11, vid: "ustanovka", faza: "idyot", shag: "detektor", server: "svoy-1", krug: 1,
        krug_tekst: "из твоей сети адрес сервера сейчас не открывается. Так бывает временно", povtory: [povtor],
        ustanovlen: { server: "svoy-1", klyuchi: [{ transport: "reality-tcp", port: 2087 }, { transport: "hy2", port: 5443 }], ids: ["k-r", "k-h"], ne_iz_seti: ["reality-tcp", "hy2"] },
      })],
    });
    const plashka = screen.getByTestId("svoy-povtor-iz-seti");
    expect(plashka).toHaveTextContent("Из твоей сети адрес сервера сейчас не открывается");
    expect(plashka).toHaveTextContent(`Проверю ещё раз в ${new Date(povtor).toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" })}`);
    fireEvent.click(screen.getByTestId("svoy-ne-zhdat"));
    expect(s.master()).toMatchObject({ shag: "ustanovka", neZhdat: true });
    // Окно, поднятое заново, к этой установке мастер не вернёт.
    expect(localStorage.getItem("affory.svoy-vps.uvidennaya-ustanovka")).toBe("11");
    // Готовые ключи с пометкой, перепроверка идёт дальше и не отменяется.
    expect(screen.getByTestId("svoy-perepoverka")).toHaveTextContent("Проверяю адрес из твоей сети ещё раз");
    expect(within(screen.getByTestId("svoy-gotovo")).getAllByText("из твоей сети не открылся")).toHaveLength(2);
    expect(screen.queryByText(/С мобильного интернета на телефоне какой-то/)).toBeNull();
    expect(s.imena()).not.toContain("svoyVpsOtmenit");
    // Служба занята перепроверкой: смену входа по паролю она не начнёт.
    expect(screen.getByTestId("svoy-vhod-po-parolyu")).toBeDisabled();
  });

  it("после детектора плашка перепроверки говорит о ключах, а не об адресе", () => {
    stend({
      servery: [server()],
      master: { shag: "ustanovka", forma: null, id: 11, neZhdat: true },
      hody: [hod({
        id: 11, vid: "ustanovka", faza: "idyot", shag: "proverka", server: "svoy-1",
        ustanovlen: { server: "svoy-1", klyuchi: [{ transport: "reality-tcp", port: 2087 }], ids: ["k-r"], ne_iz_seti: [] },
      })],
    });
    expect(screen.getByTestId("svoy-perepoverka")).toHaveTextContent("Проверяю ключи из твоей сети");
  });

  it("строка дела в карточке называет перепроверку по шагу, как плашка мастера", () => {
    const ustanovlen = { server: "svoy-1", klyuchi: [{ transport: "reality-tcp" as const, port: 2087 }], ids: ["k-r"], ne_iz_seti: ["reality-tcp"] };
    const s = stend({
      servery: [server({ ne_iz_seti: ["reality-tcp"] })],
      hody: [hod({ id: 11, vid: "ustanovka", faza: "idyot", shag: "detektor", server: "svoy-1", povtory: [new Date(Date.now() + 60_000).toISOString()], ustanovlen })],
    });
    expect(screen.getByTestId("svoy-delo")).toHaveTextContent("Проверяю адрес из твоей сети ещё раз");
    s.prishyol(hod({ id: 11, vid: "ustanovka", faza: "idyot", shag: "proverka", server: "svoy-1", ustanovlen }));
    expect(screen.getByTestId("svoy-delo")).toHaveTextContent("Проверяю ключи из твоей сети");
  });
});

describe("карточка сервера", () => {
  it("ключи не прошли из сети: «нужно внимание», и проверка сервера этого не стирает", () => {
    stend({
      servery: [server({ ne_iz_seti: ["hy2"], set_tekst: "сервер жив, ключи стоят" })],
      hody: [
        hod({ id: 2, server: "svoy-1", proverka: { yadro_zhivo: true, porty: [{ vid: "reality", port: 2087, slushaet: true }, { vid: "hy2", port: 5443, slushaet: true }], maska: "godna", pamyat_mb: 1, disk_mb: 1 } }),
        hod({ id: 3, vid: "ustanovka", faza: "gotovo", server: "svoy-1", preduprezhdeniya: [{ kod: "vps-address-blocked", tekst: "сервер жив, ключи стоят", iz_seti: true }] }),
      ],
    });
    expect(screen.getByTestId("svoy-sostoyanie")).toHaveTextContent("нужно внимание");
    expect(screen.getByText("Из твоей сети не открывается hysteria2")).toBeInTheDocument();
    expect(screen.getByText("работает на сервере")).toBeInTheDocument();
    expect(screen.getByText("из твоей сети не открывается")).toBeInTheDocument();
    // Причина одна: из записи, а не ещё раз строкой дела.
    expect(screen.getAllByText("Сервер жив, ключи стоят")).toHaveLength(1);
  });

  it("свежая проверка видела мёртвый VPN: «не работает» с причиной", () => {
    stend({
      servery: [server()],
      hody: [hod({ server: "svoy-1", proverka: { yadro_zhivo: false, oshibka_yadra: "порт занят", porty: [], maska: "godna", pamyat_mb: 1, disk_mb: 1 } })],
    });
    expect(screen.getByTestId("svoy-sostoyanie")).toHaveTextContent("не работает");
    expect(screen.getByText("VPN на сервере не запущен")).toBeInTheDocument();
  });

  it("сменившийся ключ хоста: принять и сразу проверить", async () => {
    const s = stend({
      servery: [server({ novyy_otpechatok: "SHA256:new" })],
      otvety: { svoyVpsProverit: () => ({ hod: hod({ id: 5, server: "svoy-1", faza: "idyot" }) }) },
    });
    expect(screen.getByTestId("svoy-klyuch-hosta")).toHaveTextContent("SHA256:new");
    fireEvent.click(screen.getByRole("button", { name: "Систему переустановили" }));
    await waitFor(() => expect(s.imena()).toEqual(["svoyVpsPrinyatKlyuchHosta", "svoyVpsProverit"]));
    expect(s.vyzovy[0][1]).toEqual({ id: "svoy-1", otpechatok: "SHA256:new" });
  });

  it("ключ управления на другом компьютере: захват через мастер с адресом", () => {
    const s = stend({ servery: [server({ klyuch_upravleniya: false, port_ssh: 2222 })] });
    expect(screen.getByTestId("svoy-proverit")).toBeDisabled();
    fireEvent.click(within(screen.getByTestId("svoy-bez-klyucha")).getByRole("button", { name: "Взять под управление" }));
    expect(s.master()).toMatchObject({ shag: "dostup", cel: "vzyat", forma: { adres: "203.0.113.40", portSshPolya: "2222" } });
    expect(screen.getByTestId("svoy-dalshe")).toHaveTextContent("Взять под управление");
  });

  it("установки на сервере нет: «Поставить заново» ведёт в мастер с адресом из записи", () => {
    const s = stend({
      servery: [server({ port_ssh: 2222, polzovatel: "admin" })],
      hody: [hod({ id: 3, vid: "podbor", faza: "oshibka", server: "svoy-1", otkaz: { kod: "vps-reinstall", tekst: "на сервере нет установки Affory: поставь его заново" } })],
    });
    expect(screen.queryByRole("button", { name: "Повторить" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Поставить заново" }));
    expect(s.master()).toMatchObject({
      shag: "dostup", cel: "postavit",
      forma: { adres: "203.0.113.40", portSshPolya: "2222", polzovatel: "admin", parol: "", sposob: "parol", poverhZapisi: "203.0.113.40" },
    });
    expect(screen.getByTestId("svoy-dalshe")).toHaveTextContent("Проверить сервер");
    expect(s.vyzovy).toEqual([]);
  });

  it("«Поставить заново» на сервере с выключенным паролем сразу предлагает ключ", () => {
    const s = stend({
      servery: [server({ parol_vyklyuchen: true })],
      hody: [hod({ id: 3, vid: "proverka", faza: "oshibka", server: "svoy-1", otkaz: { kod: "vps-reinstall", tekst: "установка на сервере не дошла до ключей: поставь сервер заново" } })],
    });
    fireEvent.click(screen.getByRole("button", { name: "Поставить заново" }));
    expect(s.master()).toMatchObject({ shag: "dostup", cel: "postavit", forma: { sposob: "klyuch", poverhZapisi: "203.0.113.40" } });
  });

  it("снос только после подтверждения", async () => {
    const s = stend({ servery: [server()] });
    fireEvent.click(screen.getByRole("button", { name: "Действия с сервером" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Снести с сервера" }));
    expect(s.vyzovy).toEqual([]);
    expect(screen.getByRole("dialog")).toHaveTextContent("Снести VPN с сервера 203.0.113.40?");
    fireEvent.click(screen.getByTestId("svoy-snesti-da"));
    await waitFor(() => expect(s.vyzovy).toEqual([["svoyVpsSnesti", { id: "svoy-1" }]]));
  });

  it("«забыть» спрашивает, снимать ли ключ с сервера", async () => {
    const s = stend({ servery: [server()] });
    fireEvent.click(screen.getByRole("button", { name: "Действия с сервером" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Забыть сервер" }));
    fireEvent.click(screen.getByTestId("svoy-zabyt-snyat"));
    await waitFor(() => expect(s.vyzovy).toEqual([["svoyVpsOtpustit", { id: "svoy-1" }]]));
  });

  it("порты руками: reality на порту SSH сервера не пускается", () => {
    stend({ servery: [server({ port_ssh: 2222 })] });
    fireEvent.click(screen.getByRole("button", { name: "Действия с сервером" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Задать порты" }));
    fireEvent.change(within(screen.getByTestId("svoy-zadat-porty")).getByLabelText("reality, TCP"), { target: { value: "2222" } });
    expect(screen.getByText("2222 занят входом на сервер")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Поставить порты" })).toBeDisabled();
  });

  it("при открытии проверяет давно не проверенный сервер, и только его", async () => {
    const s = stend({
      servery: [server({ id: "svoy-1" }), server({ id: "svoy-2", adres: "203.0.113.41", proveren: new Date(Date.now() - 3600_000).toISOString() })],
    });
    await waitFor(() => expect(s.vyzovy).toEqual([["svoyVpsProverit", { id: "svoy-2" }]]));
  });

  it("при открытом мастере проверки при открытии нет", async () => {
    const s = stend({
      servery: [server({ proveren: new Date(Date.now() - 3600_000).toISOString() })],
      master: { shag: "dostup", cel: "postavit", forma: PUSTAYA_FORMA },
    });
    await act(async () => {});
    expect(s.vyzovy).toEqual([]);
  });

  it("«Повторить» повторяет упавшее дело, а не проверку", async () => {
    const s = stend({
      servery: [server()],
      hody: [hod({ id: 5, vid: "podbor", faza: "oshibka", server: "svoy-1", otkaz: { kod: "vps-ports-failed", tekst: "не прошёл" } })],
    });
    const delo = screen.getByTestId("svoy-delo");
    expect(delo).toHaveTextContent("Подбор портов: не вышло");
    fireEvent.click(within(delo).getByRole("button", { name: "Повторить" }));
    await waitFor(() => expect(s.vyzovy).toEqual([["svoyVpsPodobratPorty", { id: "svoy-1" }]]));
  });

  it("отказ проверки сказан один раз: строкой дела с кнопкой, а не ещё и плашкой", () => {
    const tekst = "на сервере нет установки Affory: поставь его заново, ключи VPN при этом сменятся";
    stend({
      servery: [server({ proverka_otkaz: tekst })],
      hody: [hod({ id: 5, vid: "proverka", faza: "oshibka", server: "svoy-1", otkaz: { kod: "vps-reinstall", tekst } })],
    });
    expect(screen.queryByText("Последняя проверка не прошла")).toBeNull();
    const delo = screen.getByTestId("svoy-delo");
    // Заголовок говорит, что не вышло, тело почему; строка словаря её не пересказывает.
    expect(within(delo).getByText("Проверка сервера: не вышло")).toBeInTheDocument();
    expect(delo).not.toHaveTextContent("нет рабочей установки");
    expect(within(delo).getByRole("button", { name: "Поставить заново" })).toBeInTheDocument();
    expect(screen.getAllByText(/на сервере нет установки Affory/i)).toHaveLength(1);
  });

  it("отказ в записи новее упавшей строки: плашка остаётся", () => {
    stend({
      servery: [server({ proverka_otkaz: "ключ управления не прочитан" })],
      hody: [hod({ id: 5, vid: "proverka", faza: "oshibka", server: "svoy-1", otkaz: { kod: "vps-unreachable", tekst: "сервер не отвечает на вход" } })],
    });
    expect(screen.getByText("Последняя проверка не прошла")).toBeInTheDocument();
    expect(screen.getByText("Ключ управления не прочитан")).toBeInTheDocument();
  });

  it("строка упавшего дела без причины службы берёт строку словаря", () => {
    stend({
      servery: [server()],
      hody: [hod({ id: 5, vid: "podbor", faza: "oshibka", server: "svoy-1", otkaz: { kod: "vps-ports-failed", tekst: "" } })],
    });
    const delo = screen.getByTestId("svoy-delo");
    expect(within(delo).getByText("Подбор портов: не вышло")).toBeInTheDocument();
    expect(within(delo).getByText("Порты не подобрались")).toBeInTheDocument();
  });

  it("отказ проверки без строки упавшей проверки остаётся плашкой", () => {
    stend({
      servery: [server({ proverka_otkaz: "сервер не отвечает на вход" })],
      hody: [hod({ id: 5, vid: "podbor", faza: "gotovo", server: "svoy-1" })],
    });
    expect(screen.getByText("Последняя проверка не прошла")).toBeInTheDocument();
    expect(screen.getByText("Сервер не отвечает на вход")).toBeInTheDocument();
  });

  it("текст ядра сервера уходит в отчёт, а карточка ведёт к нему", async () => {
    const s = stend({
      servery: [server()],
      hody: [hod({ id: 5, vid: "proverka", faza: "gotovo", server: "svoy-1", konec: new Date().toISOString(),
        proverka: { yadro_zhivo: false, oshibka_yadra: "listen tcp :2087: bind: address already in use", porty: [], maska: "godna", pamyat_mb: 1, disk_mb: 1 } })],
    });
    expect(screen.getByText("VPN на сервере не запущен")).toBeInTheDocument();
    expect(screen.queryByText(/address already in use/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Действия с сервером" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Скопировать отчёт" }));
    await waitFor(() => expect(s.skopirovat).toHaveBeenCalledWith(expect.stringContaining("ошибка VPN на сервере: listen tcp :2087: bind: address already in use")));
  });

  it("пока идёт другое дело, «Повторить» погашен", () => {
    stend({
      servery: [server(), server({ id: "svoy-2", adres: "203.0.113.41" })],
      hody: [
        hod({ id: 5, vid: "proverka", faza: "oshibka", server: "svoy-1", otkaz: { kod: "vps-unreachable", tekst: "нет ответа" } }),
        hod({ id: 6, vid: "proverka", faza: "idyot", server: "svoy-2", planovaya: true }),
      ],
    });
    expect(within(screen.getByTestId("svoy-server-svoy-1")).getByRole("button", { name: "Повторить" })).toBeDisabled();
  });

  it("последний вход root: рядом «Забыть, не снимая ключа»", async () => {
    const s = stend({
      servery: [server()],
      hody: [hod({ id: 7, vid: "otpusk", faza: "oshibka", server: "svoy-1", otkaz: { kod: "vps-last-login", tekst: "других входов нет" } })],
    });
    fireEvent.click(screen.getByTestId("svoy-zabyt-bez-snyatiya"));
    await waitFor(() => expect(s.vyzovy).toEqual([["svoyVpsZabyt", { id: "svoy-1" }]]));
  });

  it("итог сноса виден и после ухода сервера из списка", () => {
    stend({ servery: [], hody: [hod({ id: 4, vid: "snos", server: "svoy-1", snyat: { parol_vernulsya: true } })] });
    expect(screen.getByText("VPN снесён с сервера 203.0.113.40")).toBeInTheDocument();
    expect(screen.getByText(/Вход по паролю снова такой/)).toBeInTheDocument();
    expect(screen.getByTestId("svoy-postavit")).toBeInTheDocument();
  });
});

describe("проверки полей и тексты", () => {
  it("адрес: IPv4 и IPv6 в скобках годятся, имя нет", () => {
    expect(oshibkaAdresa("203.0.113.10")).toBeNull();
    expect(oshibkaAdresa(" [2001:db8::1] ")).toBeNull();
    expect(chistyyAdres(" [2001:db8::1] ")).toBe("2001:db8::1");
    expect(oshibkaAdresa("vps.example.net")).not.toBeNull();
    expect(oshibkaAdresa("203.0.113.256")).not.toBeNull();
  });

  it("порт: число 1-65535 без нулей впереди, занятые с причиной", () => {
    expect(oshibkaPorta("")).toBeNull();
    expect(oshibkaPorta("0443")).not.toBeNull();
    expect(oshibkaPorta("65536")).not.toBeNull();
    expect(oshibkaPorta("80", zanyatyeDlyaReality(22))).toBe("80 нужен выпуску сертификата");
    expect(oshibkaPorta("2222", zanyatyeDlyaReality(22))).toBeNull();
  });

  it("тело входа по ключу без пароля, по паролю без ключа", () => {
    const k = teloDostupa({ ...PUSTAYA_FORMA, adres: "203.0.113.10", sposob: "klyuch", parol: "ostalsya", klyuch: { imya: "id", base64: "a2V5" }, parolKlyucha: "pk" });
    expect(k).toEqual({ adres: "203.0.113.10", port: 22, polzovatel: "root", klyuch: "a2V5", parol_klyucha: "pk" });
    const p = teloDostupa({ ...PUSTAYA_FORMA, adres: "203.0.113.10", portSshPolya: "2222", parol: "x", klyuch: { imya: "id", base64: "a2V5" } });
    expect(p).toEqual({ adres: "203.0.113.10", port: 2222, polzovatel: "root", parol: "x" });
  });

  it("отчёт без логина и секретов", () => {
    const t = otchyotDela(hod({ vid: "ustanovka", faza: "oshibka", shag: "podbor", otkaz: { kod: "vps-ports-failed", tekst: "не прошёл" } }), "203.0.113.10", "Affory 1.11.0");
    expect(t).toContain("сервер: 203.0.113.10");
    expect(t).toContain("vps-ports-failed");
    expect(t).not.toMatch(/root|парол/i);
  });

  it("этапы установки: идущий, пройденные и впереди", () => {
    const e = etapyUstanovki(hod({ vid: "ustanovka", faza: "idyot", shag: "sertifikat" }));
    expect(e.map((x) => x.itog)).toEqual(["da", "idyot", "zhdyot", "zhdyot"]);
    const g = etapyUstanovki(hod({ vid: "ustanovka", faza: "gotovo", preduprezhdeniya: [{ kod: "vps-cert-blocked", tekst: "80 и 443 закрыты" }] }));
    expect(g.find((x) => x.id === "sertifikat")).toMatchObject({ itog: "vnimanie", podrobno: "80 и 443 закрыты" });
  });

  it("строка ключа в карточке: мёртвый на сервере главнее, чем «не из твоей сети»", () => {
    const p = (slushaet: boolean) => ({ yadro_zhivo: true, porty: [{ vid: "hy2", port: 5443, slushaet }], maska: "godna", pamyat_mb: 1, disk_mb: 1 });
    const s = server({ ne_iz_seti: ["hy2"] });
    expect(sostoyanieKlyucha(s, p(false), "hy2")).toBe("на сервере не работает");
    expect(sostoyanieKlyucha(s, p(true), "hy2")).toBe("из твоей сети не открывается");
    expect(sostoyanieKlyucha(s, undefined, "hy2")).toBe("из твоей сети не открывается");
    expect(sostoyanieKlyucha(server(), p(true), "hy2")).toBe("работает на сервере");
    expect(sostoyanieKlyucha(server(), undefined, "hy2")).toBe("порт подобран из твоей сети");
    // Порт подобран заново после проверки: её итог про прежний порт.
    expect(sostoyanieKlyucha(server({ hy2: 40443 }), p(false), "hy2")).toBe("порт подобран из твоей сети");
  });

  it("причина на фишке без повторов одного смысла", () => {
    const p = probyPortov(hod({
      proverki: [
        { transport: "reality-tcp", port: 443, vid: "net-otveta", otkaz: "нет ответа через ключ: соединение не установилось" },
        { transport: "hy2", port: 8443, vid: "chuzhoy-vyhod", otkaz: "выход с 198.51.100.1, а не с адреса сервера" },
      ],
    }));
    expect(p.reality[0].prichina).toBe("не ответил: соединение не установилось");
    expect(p.hy2[0].prichina).toBe("выходит в сеть не с адреса сервера");
  });

  it("порт, пробованный дважды, одной фишкой с последним итогом", () => {
    const p = probyPortov(hod({ proverki: [{ transport: "hy2", port: 443, vid: "net-otveta" }, { transport: "hy2", port: 443, vid: "vezyot" }] }));
    expect(p.hy2).toEqual([{ port: 443, itog: "proshyol", prichina: undefined }]);
  });

  it("итоги дел словами", () => {
    expect(itogDela(hod({ vid: "otpusk", otpushchen: { klyucha_ne_bylo: true } }), "203.0.113.10")?.tekst).toContain("уже не было");
    expect(itogDela(hod({ vid: "parol", parol: { vyklyuchen: false, snaruzhi: "vyklyuchen_chuzhim", fayly: ["/etc/ssh/sshd_config.d/50-cloud-init.conf"] } }), undefined))
      .toMatchObject({ ton: "warn", tekst: expect.stringContaining("50-cloud-init.conf") });
    expect(itogDela(hod({ vid: "proverka", faza: "idyot" }), undefined)).toBeNull();
  });
});
