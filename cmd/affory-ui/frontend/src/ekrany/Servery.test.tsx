import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Servery, itogVstavkiIz, pohozheNaAdres, razlozhitVstavku, strokaItoga, svodkaQrIz, type PodpiskaNaEkrane, type QrSEkrana, type SpisokServerov, type SvodkaQr, type ZamerZaderzhki } from "./Servery";
import type { Server, StatusOtvet } from "../protokol";

afterEach(cleanup);

// Task 4.9. The only tab whose commands all exist in the service, so it is
// accepted whole: list, subscription, add, remove, select. The "many data"
// state is measured, not imagined: five foreign subscriptions gave 59
// servers after folding (02.09.2026), and that is the floor, not the ceiling.

function server(n: number, pere: Partial<Server> = {}): Server {
  return {
    id: `id${n}`, imya: `Сервер ${n}`, transport: "reality-tcp",
    host: `s${n}.example.net`, port: 443, iz_podpiski: true, ...pere,
  };
}

function spisok(servery: Server[], pere: Partial<SpisokServerov> = {}): SpisokServerov {
  return { servery, vybran: "", podpiska_zadana: true, podpiska_uzel: "panel.example.net", ...pere };
}

const VYKL: StatusOtvet = { sostoyanie: "vyklyuchen", rezhim_marshruta: "ruchnoy" };

function risovat(s: SpisokServerov | null, status: StatusOtvet = VYKL, naKomandu = vi.fn(), podpiski: PodpiskaNaEkrane[] = []) {
  render(<Servery status={status} spisok={s} naKomandu={naKomandu} podpiski={podpiski} />);
  return naKomandu;
}

describe("серверы: список", () => {
  it("рисует 59 строк, каждая со своим именем, в прокручиваемом списке", () => {
    const mnogo = Array.from({ length: 59 }, (_, i) => server(i + 1));
    risovat(spisok(mnogo));
    const rows = screen.getAllByRole("option");
    expect(rows).toHaveLength(59);
    expect(rows[58]).toHaveTextContent("Сервер 59");
    // The list scrolls inside itself; the tab header stays put.
    expect(screen.getByRole("listbox").className).toMatch(/overflow-y-auto/);
  });

  it("выбранный, которого нет в свежем списке, помечен «нет в подписке» и не молчит", () => {
    risovat(spisok([server(1), server(2)], { vybran: "ushol" }), { ...VYKL, vybran_id: "ushol" });
    const pometka = screen.getByTestId("vybran-propal");
    expect(pometka).toHaveTextContent(/нет в подписке/);
    // A silent replacement by the first server is what this marker exists
    // against: the human must see that what runs is not what they chose.
    expect(pometka).toHaveTextContent(/первый по списку/);
  });

  it("в авто пустой выбор это не «сервер не выбран»", () => {
    risovat(spisok([server(1)]), { sostoyanie: "vyklyuchen", rezhim_marshruta: "avto", vybran_id: "" });
    expect(screen.queryByText(/не выбран/)).toBeNull();
    expect(screen.getByTestId("svodka")).toHaveTextContent(/автоматически/);
  });

  it("щелчок по строке выбирает сервер одной командой setServer", () => {
    const na = risovat(spisok([server(1), server(2)]));
    fireEvent.click(screen.getAllByRole("option")[1]);
    expect(na).toHaveBeenCalledTimes(1);
    expect(na).toHaveBeenCalledWith("setServer", { id: "id2" });
  });

  it("выбранная строка отмечена aria-selected, несущий помечен «активен»", () => {
    risovat(spisok([server(1), server(2)], { vybran: "id1" }), { ...VYKL, vybran_id: "id1", nesushchiy_id: "id2", sostoyanie: "podnyat" });
    const rows = screen.getAllByRole("option");
    expect(rows[0].getAttribute("aria-selected")).toBe("true");
    expect(rows[1].getAttribute("aria-selected")).toBe("false");
    expect(within(rows[1]).getByText("активен")).toBeTruthy();
  });

  // Происхождение записи теперь называет ЗАГОЛОВОК ПОЛОСЫ, а не хвост каждой
  // строки: две подписки подряд давали полтора десятка одинаковых строк, в
  // которых одно и то же пояснение повторялось построчно.
  it("ручная запись лежит в своей полосе, пометка сертификата остаётся в строке", () => {
    risovat(spisok([server(1, { iz_podpiski: false }), server(2, { nebezopasnyy_ignorirovan: true })]));
    const ruchnye = screen.getByRole("group", { name: "Добавлены вручную" });
    expect(within(ruchnye).getAllByRole("option")).toHaveLength(1);
    expect(within(ruchnye).getByTestId("server-id1")).toBeTruthy();
    expect(screen.getByTestId("server-id2")).toHaveTextContent(/проверка сертификата/);
  });

  it("непроверяемый отпечаток сертификата виден в строке", () => {
    // П4 аудита 1.6.1: ядро сверяет только ключ, и ссылка обещала больше.
    risovat(spisok([server(1, { otpechatok_ne_proveryaetsya: true }), server(2)]));
    expect(screen.getByTestId("server-id1")).toHaveTextContent(/отпечаток сертификата не проверяется/);
    expect(screen.getByTestId("server-id2")).not.toHaveTextContent(/отпечаток/);
  });

  it("удержанная запись лежит в полосе про переподключение, а не среди ручных", () => {
    risovat(spisok([server(1, { iz_podpiski: false, uderzhan: true }), server(2, { iz_podpiski: false })]));
    const uderzhannye = screen.getByRole("group", { name: /Пропали из подписки/ });
    const ruchnye = screen.getByRole("group", { name: "Добавлены вручную" });
    expect(within(uderzhannye).getByTestId("server-id1")).toBeTruthy();
    expect(within(ruchnye).getByTestId("server-id2")).toBeTruthy();
    expect(within(uderzhannye).queryByTestId("server-id2")).toBeNull();
    // Удержанная запись НЕ должна попасть ещё и в ручные: она не ручная, и
    // строка, показанная дважды, это тот же сплошной столбец, только длиннее.
    expect(within(ruchnye).queryByTestId("server-id1")).toBeNull();
    expect(screen.getAllByTestId("server-id1")).toHaveLength(1);
  });

  it("полоса подписки названа её узлом: две подписки перестают быть сплошным столбцом", () => {
    risovat(spisok([server(1), server(2, { iz_podpiski: false })]), VYKL, vi.fn(), [
      { id: "p1", uzel: "vpn.example.net", aktivnaya: true, serverov: 6 },
      { id: "p2", uzel: "zapas.example.net", aktivnaya: false, serverov: 8 },
    ]);
    expect(screen.getByRole("group", { name: "Из подписки vpn.example.net" })).toBeTruthy();
    expect(screen.getByRole("group", { name: "Добавлены вручную" })).toBeTruthy();
  });

  it("пометка строки: сертификат закреплён у hy2 с pinSHA256", () => {
    risovat(spisok([server(1, { transport: "hy2", s_pinom: true }), server(2, { transport: "hy2" })]));
    const rows = screen.getAllByRole("option");
    expect(rows[0]).toHaveTextContent(/сертификат закреплён/);
    expect(rows[1]).not.toHaveTextContent(/сертификат закреплён/);
  });

  it("поиск фильтрует по имени и по адресу", () => {
    risovat(spisok([server(1, { imya: "Amsterdam" }), server(2, { imya: "Paris", host: "fr.example.net" })]));
    fireEvent.change(screen.getByTestId("poisk"), { target: { value: "fr.exa" } });
    const rows = screen.getAllByRole("option");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("Paris");
  });

  it("удаление в два щелчка: первый спрашивает, второй шлёт removeServer", () => {
    const na = risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("udalit-id1"));
    expect(na).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("udalit-id1"));
    expect(na).toHaveBeenCalledWith("removeServer", { id: "id1" });
  });
});

describe("серверы: подписка и добавление", () => {
  it("адрес подписки не рисуется никогда, только узел", () => {
    risovat(spisok([server(1)]));
    expect(screen.getByTestId("podpiska")).toHaveTextContent("panel.example.net");
    expect(document.body.textContent).not.toMatch(/https?:\/\//);
  });

  it("«Обновить» шлёт refreshSubscription; подписка задаётся через ту же «Добавить», тем же полем", () => {
    // Owner's remark 02.09.2026: a server via "Добавить" and a subscription
    // via "изменить" were two buttons with two meanings for one act. Since
    // 26.09.2026 there is not even a choice: the field tells them apart.
    const na = risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("obnovit-podpisku"));
    expect(na).toHaveBeenCalledWith("refreshSubscription", {});
    expect(screen.queryByTestId("izmenit-podpisku")).toBeNull();
    fireEvent.click(screen.getByTestId("dobavit"));
    fireEvent.change(screen.getByTestId("ssylka"), { target: { value: "  https://p.example/sub  " } });
    fireEvent.click(screen.getByTestId("dobavit-ssylku"));
    // addSubscription с 12.09.2026: подписок стало несколько, и «добавить» не
    // имеет права подменить ту, по которой человек сейчас работает.
    expect(na).toHaveBeenCalledWith("addSubscription", { adres: "https://p.example/sub" });
  });

  it("непонятые строки подписки видны на экране, с номером и причиной", () => {
    // 05.09.2026. Служба причину знала с первого дня и отдавала полем otkazy,
    // а экран его не читал вовсе. Человек видел просто МЕНЬШЕ серверов, чем
    // прислала панель, без единого слова о том, почему. Ровно так и терялись
    // ссылки anytls, пока разбора для них не было.
    render(
      <Servery
        status={VYKL}
        spisok={spisok([server(1)])}
        naKomandu={vi.fn()}
        otkazyPodpiski={[
          { stroka: 3, prichina: "транспорт не поддерживается" },
          { stroka: 7, prichina: "ссылка не разобрана" },
        ]}
      />,
    );
    const k = screen.getByTestId("otkazy-podpiski");
    expect(k).toHaveTextContent("2");
    expect(k).toHaveTextContent("строка 3");
    expect(k).toHaveTextContent("транспорт не поддерживается");
    expect(k).toHaveTextContent("строка 7");
  });

  it("без непонятых строк карточки отказов нет вовсе", () => {
    // Пустой список это не «ноль отказов» на экране, а отсутствие разговора.
    risovat(spisok([server(1)]));
    expect(screen.queryByTestId("otkazy-podpiski")).toBeNull();
  });

  it("причина отказа рисуется, а сама ссылка никогда", () => {
    // В отказе служба присылает уже очищенный текст, но экран обязан не
    // подставить ссылку сам: в ней uuid и ключи.
    render(
      <Servery
        status={VYKL}
        spisok={spisok([server(1)])}
        naKomandu={vi.fn()}
        otkazyPodpiski={[{ stroka: 1, prichina: "транспорт не поддерживается: схема mieru" }]}
      />,
    );
    expect(screen.getByTestId("otkazy-podpiski")).toHaveTextContent("схема mieru");
    expect(document.body.textContent).not.toMatch(/https?:\/\/|vless:|anytls:/);
  });

  // Решение владельца 26.09.2026: форма, которой нужна подпись о том, как она
  // работает, негодна. Строка «ляжет про запас… на экране не показывается»
  // снята, и вернуться ей не дают.
  it("под формой нет пояснений о том, как она работает", () => {
    risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("dobavit"));
    expect(screen.getByTestId("forma")).not.toHaveTextContent(/про запас|на экране не показывается|раз в 12 часов/);
  });

  it("пустой список это первый запуск §9.2: поле для ссылки и одно действие, без «0 серверов»", () => {
    risovat(spisok([], { podpiska_zadana: false, podpiska_uzel: "" }));
    expect(screen.getByTestId("pusto")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/\b0 серверов/);
    expect(screen.queryByRole("option")).toBeNull();
    // The form is open by default: the first thing the human does here is
    // paste something, not look for a button. One field takes both kinds.
    const pole = screen.getByTestId("ssylka");
    expect(pole.tagName).toBe("TEXTAREA");
    expect(pole.getAttribute("placeholder")).toMatch(/https:\/\//);
  });

  it("«Добавить» раскрывает поле, текст уходит в addServers целиком", () => {
    const na = risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("dobavit"));
    fireEvent.change(screen.getByTestId("ssylka"), { target: { value: " vless://x@h:443?security=reality " } });
    fireEvent.click(screen.getByTestId("dobavit-ssylku"));
    expect(na).toHaveBeenCalledWith("addServers", { tekst: " vless://x@h:443?security=reality " }, expect.any(Function), expect.any(Function));
  });

  it("пока списка нет, экран говорит «загрузка», а не рисует пустоту с нулями", () => {
    risovat(null);
    expect(screen.getByTestId("zagruzka")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/\b0\b/);
  });

  it("пока служба молчит, всё неактивно", () => {
    risovat(spisok([server(1)]), { sostoyanie: "sluzhba-molchit" });
    expect((screen.getByTestId("dobavit") as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId("obnovit-podpisku") as HTMLButtonElement).disabled).toBe(true);
  });
});

// План «шесть удобств» §3: ссылка из буфера обмена и QR с экрана. Текст
// буфера на экран не попадает: либо ушёл в службу, либо отвергнут словами.
describe("серверы: из буфера и с экрана", () => {
  // До 26.09.2026 окно держало свой список схем, и он отстал от разбора в
  // службе: anytls:// и tuic:// кнопка буфера отвергала, хотя служба их
  // принимала. Теперь окно отсекает только явный мусор.
  it("вставка: любые схемы и base64 это ключи, https это подписка, прочее мусор", () => {
    for (const s of ["vless://a", " HY2://b ", "anytls://c", "tuic://d", "mieru://e", "vmess://f"]) {
      expect(razlozhitVstavku(s)).toEqual({ adresa: [], estKlyuchi: true });
    }
    expect(razlozhitVstavku("aHkyOi8vcEAxLjIuMy40OjQ0Mw==\ndm1lc3M6Ly94")).toEqual({ adresa: [], estKlyuchi: true });
    expect(razlozhitVstavku("anytls://a\r\n\r\n https://p.example/sub \ntuic://b")).toEqual({ adresa: ["https://p.example/sub"], estKlyuchi: true });
    expect(razlozhitVstavku("HTTPS://P.EXAMPLE/sub")).toEqual({ adresa: ["HTTPS://P.EXAMPLE/sub"], estKlyuchi: false });
    for (const s of ["", "  \n ", "просто текст", "example.org"]) expect(razlozhitVstavku(s)).toEqual({ adresa: [], estKlyuchi: false });
  });

  it("строка итога называет каждый исход, пустые пропускает", () => {
    expect(strokaItoga(itogVstavkiIz({ dobavleno: 6 }))).toBe("добавлено 6");
    expect(strokaItoga(itogVstavkiIz({ dobavleno: 0, obnovleno: 2, uzhe_bylo: 4, otkazy: [{ stroka: 3, prichina: "x" }] })))
      .toBe("добавлено 0, переименовано 2, уже были 4, пропущено 1");
    // Мусор в ответе не роняет экран: чужие поля отброшены, числа нулём.
    expect(itogVstavkiIz({ dobavleno: "5", otkazy: [{ stroka: "1" }, null] })).toEqual({ dobavleno: 0, obnovleno: 0, uzhe_bylo: 0, otkazy: [] });
  });

  it("кнопок нет, пока оболочка не дала способа читать буфер и экран", () => {
    risovat(spisok([]));
    expect(screen.queryByTestId("iz-bufera")).toBeNull();
    expect(screen.queryByTestId("qr-s-ekrana")).toBeNull();
  });

  it("ключи из буфера уходят в addServers, поле остаётся пустым", async () => {
    const naKomandu = vi.fn();
    const bufer = "anytls://p@h:995#anytls\ntuic://u:p@h:443#tuic";
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={naKomandu} chitatBufer={async () => bufer} />);
    fireEvent.click(screen.getByTestId("iz-bufera"));
    await vi.waitFor(() => expect(naKomandu).toHaveBeenCalledWith("addServers", { tekst: bufer }, expect.any(Function), expect.any(Function)));
    expect((screen.getByTestId("ssylka") as HTMLTextAreaElement).value).toBe("");
  });

  it("мусор в буфере: отказ словами, команды нет, текст буфера не показан", async () => {
    const naKomandu = vi.fn();
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={naKomandu} chitatBufer={async () => "секретный текст"} />);
    fireEvent.click(screen.getByTestId("iz-bufera"));
    const ishod = await screen.findByTestId("ishod-vvoda");
    expect(ishod).toHaveTextContent(/в буфере не ключ и не адрес подписки/);
    expect(ishod).not.toHaveTextContent(/секретный/);
    expect(naKomandu).not.toHaveBeenCalled();
  });

  it("пустой буфер так и называется", async () => {
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} chitatBufer={async () => "  "} />);
    fireEvent.click(screen.getByTestId("iz-bufera"));
    expect(await screen.findByTestId("ishod-vvoda")).toHaveTextContent("буфер обмена пуст");
  });

  // Ключи и адрес вперемешку: ключи уходят одним текстом ВМЕСТЕ со строкой
  // адреса (служба её пропускает, номера строк в отказах не съезжают), адрес
  // отдельной командой подписки.
  it("ключи и адрес в одной вставке: две команды, текст целиком", () => {
    const na = risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("dobavit"));
    const tekst = "hy2://p@h:443#a\nhttps://p.example/sub\nanytls://p@h:995#b";
    fireEvent.change(screen.getByTestId("ssylka"), { target: { value: tekst } });
    fireEvent.click(screen.getByTestId("dobavit-ssylku"));
    expect(na).toHaveBeenCalledWith("addServers", { tekst }, expect.any(Function), expect.any(Function));
    expect(na).toHaveBeenCalledWith("addSubscription", { adres: "https://p.example/sub" });
    expect(na).toHaveBeenCalledTimes(2);
  });

  it("итог вставки виден в форме вместе с пропущенными строками, и форма не закрывается", () => {
    const na = vi.fn((komanda: string, _telo: unknown, naOtvet?: (telo: unknown) => void) => {
      if (komanda === "addServers") naOtvet?.({ dobavleno: 5, obnovleno: 0, uzhe_bylo: 0, otkazy: [{ stroka: 3, prichina: "ссылка не разобрана: hy2 без пароля" }] });
    });
    risovat(spisok([], { podpiska_zadana: false }), VYKL, na);
    fireEvent.change(screen.getByTestId("ssylka"), { target: { value: "hy2://a\nhy2://b\nhy2://c" } });
    fireEvent.click(screen.getByTestId("dobavit-ssylku"));
    const itog = screen.getByTestId("itog-vstavki");
    expect(itog).toHaveTextContent("добавлено 5, пропущено 1");
    expect(itog).toHaveTextContent("строка 3: ссылка не разобрана: hy2 без пароля");
    expect((screen.getByTestId("ssylka") as HTMLTextAreaElement).value).toBe("");
  });

  // Отказ всей вставки показывается в форме словами службы. Баннер для кода
  // subscription-malformed сказал бы «подписка не подошла» про текст,
  // который никакая подписка не присылала.
  it("отказ всей вставки остаётся в форме, а не уходит баннером", () => {
    const na = vi.fn((_k: string, _t: unknown, _o?: (telo: unknown) => void, naOtkaz?: (o: { kod: string; tekst: string }) => void) => {
      naOtkaz?.({ kod: "subscription-malformed", tekst: "это файл настроек, а не ссылки" });
    });
    risovat(spisok([server(1)]), VYKL, na);
    fireEvent.click(screen.getByTestId("dobavit"));
    fireEvent.change(screen.getByTestId("ssylka"), { target: { value: "vless://x" } });
    fireEvent.click(screen.getByTestId("dobavit-ssylku"));
    expect(screen.getByTestId("ishod-vvoda")).toHaveTextContent("это файл настроек, а не ссылки");
  });

  // О8 аудита 1.6.1. QR с экрана добавлялся без вопроса, и любая страница с
  // кодом на экране подсовывала свой сервер или свою подписку. Теперь окно
  // сначала показывает, что нашлось, и добавляет только по нажатию.
  const SVODKA: SvodkaQr = {
    klyuchi: [{ imya: "Германия", transport: "reality-tcp", adres: "203.0.113.9:8443" }],
    podpiski: ["panel.example"],
    negodnyh: 1,
  };
  function qrIz(pere: Partial<QrSEkrana> = {}): QrSEkrana {
    return { nayti: vi.fn(async () => SVODKA), dobavit: vi.fn(async () => "добавлено 1"), zabyt: vi.fn(), ...pere };
  }

  it("QR с экрана: сначала показывает найденное и ничего не добавляет", async () => {
    const qr = qrIz();
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} qr={qr} />);
    fireEvent.click(screen.getByTestId("qr-s-ekrana"));
    const naydeno = await screen.findByTestId("naydeno-qr");
    expect(naydeno).toHaveTextContent("Германия");
    expect(naydeno).toHaveTextContent("reality");
    expect(naydeno).toHaveTextContent("203.0.113.9:8443");
    expect(naydeno).toHaveTextContent("panel.example");
    expect(naydeno).toHaveTextContent(/не разобралась 1 строка/);
    expect(qr.dobavit).not.toHaveBeenCalled();
  });

  it("«Добавить» отправляет найденное и показывает итог оболочки", async () => {
    const qr = qrIz({ dobavit: vi.fn(async () => "добавлено 1; подписка добавлена, серверов: 6") });
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} qr={qr} />);
    fireEvent.click(screen.getByTestId("qr-s-ekrana"));
    fireEvent.click(await screen.findByTestId("qr-dobavit"));
    expect(await screen.findByTestId("ishod-vvoda")).toHaveTextContent(/подписка добавлена, серверов: 6/);
    expect(qr.dobavit).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId("naydeno-qr")).toBeNull();
  });

  it("«Отмена» забывает найденное и ничего не добавляет", async () => {
    const qr = qrIz();
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} qr={qr} />);
    fireEvent.click(screen.getByTestId("qr-s-ekrana"));
    fireEvent.click(await screen.findByTestId("qr-otmena"));
    expect(qr.zabyt).toHaveBeenCalledTimes(1);
    expect(qr.dobavit).not.toHaveBeenCalled();
    expect(screen.queryByTestId("naydeno-qr")).toBeNull();
  });

  it("отказ добавления показан экраном отказа своими словами", async () => {
    const qr = qrIz({ dobavit: vi.fn(async () => { throw new Error("служба не ответила"); }) });
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} qr={qr} />);
    fireEvent.click(screen.getByTestId("qr-s-ekrana"));
    fireEvent.click(await screen.findByTestId("qr-dobavit"));
    const otkaz = await screen.findByTestId("otkaz-qr");
    expect(otkaz).toHaveTextContent(/QR не добавился/);
    expect(otkaz).toHaveTextContent(/служба не ответила/);
    expect(screen.queryByTestId("naydeno-qr")).toBeNull();
  });

  it("мусор в сводке не роняет экран", () => {
    expect(svodkaQrIz(null)).toEqual({ klyuchi: [], podpiski: [], negodnyh: 0 });
    expect(svodkaQrIz({ klyuchi: [{ imya: 5 }, null, SVODKA.klyuchi[0]], podpiski: ["a", 7], negodnyh: "2" }))
      .toEqual({ klyuchi: [SVODKA.klyuchi[0]], podpiski: ["a"], negodnyh: 0 });
  });

  // Разбор 03.09.2026: отказ службы на «QR с экрана» показывался серым мелким
  // шрифтом под полем, мимо §9.1 и без кнопки.
  it("отказ QR с экрана показывается экраном отказа, а не мелким серым текстом", async () => {
    const nayti = vi.fn(async (): Promise<SvodkaQr> => { throw new Error("QR на экране не найден"); });
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} qr={qrIz({ nayti })} />);
    fireEvent.click(screen.getByTestId("qr-s-ekrana"));
    const otkaz = await screen.findByTestId("otkaz-qr");
    expect(otkaz.getAttribute("role")).toBe("alert");
    expect(otkaz).toHaveTextContent(/не найден/);
    expect(screen.queryByTestId("ishod-vvoda")).toBeNull();
    fireEvent.click(screen.getByTestId("otkaz-qr-deystvie"));
    expect(nayti).toHaveBeenCalledTimes(2);
  });

  // Экран показывал этому отказу код `qr-s-ekrana`, которого нет ни в словаре
  // протокола, ни в §9.1, ни в otkazy.ts. Компонент неизвестный код переживает
  // молча, поэтому экран выглядел правильным. Чтение QR это работа окна, а не
  // службы: своя фраза и своё действие у отказа есть, а кода на проводе быть
  // не должно.
  it("отказ QR не показывает кода, которого нет в словаре", async () => {
    const nayti = vi.fn(async (): Promise<SvodkaQr> => { throw new Error("QR на экране не найден"); });
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} qr={qrIz({ nayti })} />);
    fireEvent.click(screen.getByTestId("qr-s-ekrana"));
    const otkaz = await screen.findByTestId("otkaz-qr");
    expect(otkaz.getAttribute("data-kod")).toBeNull();
    expect(otkaz).toHaveTextContent(/QR с экрана не прочитался/);
  });
});

// Правило полосы Е: у каждого экрана тесты на четыре состояния. Обход
// интерфейса 03.09.2026 нашёл здесь тупик: при отказе listServers на вкладке
// не было ни списка, ни кнопки, ни формы, и уйти с неё было некуда.
describe("серверы: четыре состояния", () => {
  it("пусто: списка нет, а добавить можно сразу", () => {
    risovat(spisok([]));
    expect(screen.getByTestId("pusto")).toBeTruthy();
    expect(screen.getByTestId("ssylka")).toBeTruthy();
    expect(screen.queryByTestId("otkaz-spiska")).toBeNull();
  });

  it("отказ: причина названа словами и есть чем повторить", () => {
    const obnovit = vi.fn();
    render(
      <Servery
        status={VYKL}
        spisok={null}
        spisokOtkaz={{ kod: "secrets-unreadable" }}
        obnovitSpisok={obnovit}
        naKomandu={vi.fn()}
      />,
    );
    expect(screen.getByText(/файл с серверами не открылся/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /повтор/i }));
    expect(obnovit).toHaveBeenCalledTimes(1);
    // Отказ это ответ, а не ожидание: строка «загружается» уходит.
    expect(screen.queryByTestId("zagruzka")).toBeNull();
  });

  it("отказ не запирает вкладку: сервер можно добавить и без прочитанного списка", () => {
    render(
      <Servery status={VYKL} spisok={null} spisokOtkaz={{ kod: "secrets-unreadable" }} naKomandu={vi.fn()} />,
    );
    fireEvent.click(screen.getByTestId("dobavit"));
    expect(screen.getByTestId("ssylka")).toBeTruthy();
  });

  it("много: пятьсот серверов остаются внутри прокручиваемого списка", () => {
    risovat(spisok(Array.from({ length: 500 }, (_, i) => server(i + 1))));
    expect(screen.getAllByRole("option")).toHaveLength(500);
    const spisokEl = screen.getByRole("listbox");
    expect(spisokEl.className).toMatch(/max-h-\[60vh\]/);
    expect(spisokEl.className).toMatch(/overflow-y-auto/);
  });

  it("длинное: имя без пробелов не выезжает за колонку", () => {
    const dlinnoe = "сервер-" + "ы".repeat(200);
    risovat(spisok([server(1, { imya: dlinnoe })]));
    const imya = screen.getByTestId("imya-servera");
    expect(imya.className).toMatch(/truncate/);
    // shrink-0 на имени и был причиной наезда на теги и корзину.
    expect(imya.className).not.toMatch(/shrink-0/);
  });
});

/** Свойства для замеров: один сервер с id s1, всё остальное по умолчанию. */
function svoystva(pere: { naKomandu?: () => void; zaderzhki?: ZamerZaderzhki[] }) {
  return {
    status: VYKL,
    spisok: spisok([server(1, { id: "s1" })]),
    naKomandu: pere.naKomandu ?? vi.fn(),
    ...(pere.zaderzhki ? { zaderzhki: pere.zaderzhki } : {}),
  };
}

describe("серверы: замер задержки", () => {
  // Задача 8, части А и Б. До неё клиент знал одно число, задержку последней
  // пробы urltest, одну на всё подключение. Выбирать сервер по нему нельзя:
  // оно про тот сервер, который УЖЕ выбран.
  //
  // С 01.10.2026 число одно: пинг через сервер по прогретому соединению, то
  // же, что строка «Задержка» и пинг в Discord. Меряется и при выключенном
  // VPN: служба поднимает для замера своё ядро без туннеля.
  it("кнопка шлёт measureDelays", () => {
    const na = vi.fn();
    render(<Servery {...svoystva({ naKomandu: na })} />);
    fireEvent.click(screen.getByTestId("zamerit-zaderzhki"));
    expect(na).toHaveBeenCalledWith("measureDelays", {});
  });

  it("пинг виден в строке одним числом", () => {
    render(<Servery {...svoystva({ zaderzhki: [{ id: "s1", ping_ms: 47 }] })} />);
    expect(screen.getByTestId("zaderzhka-s1")).toHaveTextContent(/^47 мс$/);
  });

  it("мёртвый сервер это слово, а не ноль, и причина в подсказке", () => {
    // Ноль на экране читается как «мгновенно» и ставит сервер первым по
    // пингу, то есть наверх списка. Ровно наоборот тому, что есть.
    render(<Servery {...svoystva({ zaderzhki: [{ id: "s1", ping_otkaz: "сервер не ответил вовремя" }] })} />);
    const r = screen.getByTestId("zaderzhka-s1");
    expect(r).toHaveTextContent("недоступен");
    expect(r).not.toHaveTextContent(/\b0\b/);
    expect(r).toHaveAttribute("title", "сервер не ответил вовремя");
  });

  it("причину, названную ядром, видно в строке, а фразу целиком в подсказке", () => {
    // 02.10.2026 пять ключей из шести стояли «недоступен», а истёкший
    // сертификат сервера нашёлся только в журнале ядра.
    const fraza = "сертификат сервера просрочен или ещё не начал действовать: если часы на компьютере верные, его должен продлить владелец сервера";
    render(<Servery {...svoystva({ zaderzhki: [{ id: "s1", ping_otkaz: fraza, ping_prichina: "сертификат не действует" }] })} />);
    const r = screen.getByTestId("zaderzhka-s1");
    expect(r).toHaveTextContent(/^сертификат не действует$/);
    expect(r).toHaveAttribute("title", fraza);
  });

  it("без замера строка задержки не появляется вовсе", () => {
    render(<Servery {...svoystva({})} />);
    expect(screen.queryByTestId("zaderzhka-s1")).toBeNull();
  });
});

// Несколько подписок: активная одна, остальные про запас. Экран обязан
// показывать обе и говорить, какая сейчас работает, иначе «переключить» это
// действие вслепую.
describe("серверы: несколько подписок", () => {
  const dve: PodpiskaNaEkrane[] = [
    { id: "aaa", uzel: "panel.example.net", aktivnaya: true, obnovlena: new Date().toISOString() },
    { id: "bbb", uzel: "zapasnaya.example.net", aktivnaya: false },
  ];

  it("у запасной видно, есть ли готовые ключи", () => {
    risovat(spisok([server(1)]), VYKL, vi.fn(), [
      { id: "aaa", uzel: "panel.example.net", aktivnaya: true, serverov: 7 },
      { id: "bbb", uzel: "zapasnaya.example.net", aktivnaya: false, serverov: 4, obnovlena: new Date().toISOString() },
    ]);
    expect(screen.getByTestId("podpiska-bbb")).toHaveTextContent(/4 сервера/);
  });

  it("отказ запасной подписки виден её строкой, а не молчанием", () => {
    risovat(spisok([server(1)]), VYKL, vi.fn(), [
      { id: "aaa", uzel: "panel.example.net", aktivnaya: true, serverov: 7 },
      { id: "bbb", uzel: "zapasnaya.example.net", aktivnaya: false, serverov: 0, otkaz: "подписка истекла" },
    ]);
    expect(screen.getByTestId("podpiska-bbb")).toHaveTextContent(/подписка истекла/);
  });

  it("показывает обе подписки без отдельного режима активности", () => {
    risovat(spisok([server(1)]), VYKL, vi.fn(), dve);
    const stroki = screen.getAllByTestId(/^podpiska-/);
    expect(stroki).toHaveLength(2);
    expect(stroki[0]).toHaveTextContent(/panel\.example\.net/);
    expect(stroki[0]).not.toHaveTextContent(/активна/);
    expect(stroki[1]).toHaveTextContent(/zapasnaya\.example\.net/);
    expect(stroki[1]).not.toHaveTextContent(/активна/);
  });

  it("обновление конкретной подписки не переключает источник", () => {
    const na = risovat(spisok([server(1)]), VYKL, vi.fn(), dve);
    fireEvent.click(screen.getByTestId("obnovit-podpisku-bbb"));
    expect(na).toHaveBeenCalledWith("refreshSubscription", { id: "bbb" });
    expect(screen.queryByText("Сделать активной")).toBeNull();
  });

  it("у активной подписки переключателя нет, а обновление есть", () => {
    risovat(spisok([server(1)]), VYKL, vi.fn(), dve);
    expect(screen.queryByTestId("vklyuchit-aaa")).toBeNull();
    expect(screen.getByTestId("obnovit-podpisku")).toBeTruthy();
  });

  it("удаление подписки спрашивает подтверждение и шлёт removeSubscription", () => {
    const na = risovat(spisok([server(1)]), VYKL, vi.fn(), dve);
    fireEvent.click(screen.getByTestId("udalit-podpisku-bbb"));
    expect(na).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("udalit-podpisku-bbb"));
    expect(na).toHaveBeenCalledWith("removeSubscription", { id: "bbb" });
  });

  // Форма шлёт addSubscription, а не setSubscription: вторая подписка ложится
  // про запас и не подменяет ту, по которой человек сейчас работает.
  it("форма добавления кладёт подписку про запас", () => {
    const na = risovat(spisok([server(1)]), VYKL, vi.fn(), dve);
    fireEvent.click(screen.getByTestId("dobavit"));
    fireEvent.change(screen.getByTestId("ssylka"), { target: { value: "https://tretya.example.net/sub" } });
    fireEvent.click(screen.getByTestId("dobavit-ssylku"));
    expect(na).toHaveBeenCalledWith("addSubscription", { adres: "https://tretya.example.net/sub" });
  });
});

// Кнопка удаления подписки рисуется одной иконкой. Без подписи у неё нет имени
// в дереве доступности: 12.09.2026 обход гостя нашёл шесть корзин серверов и
// ни одной подписки, то есть с клавиатуры её было не назвать.
it("у корзины подписки есть имя", () => {
  render(
    <Servery
      status={VYKL}
      spisok={spisok([server(1)])}
      naKomandu={vi.fn()}
      podpiski={[
        { id: "aaa", uzel: "panel.example.net", aktivnaya: true },
        { id: "bbb", uzel: "zapasnaya.example.net", aktivnaya: false },
      ]}
    />,
  );
  expect(screen.getByRole("button", { name: "удалить подписку zapasnaya.example.net" })).toBeTruthy();
});

// Жалоба 13.09.2026: «нет кнопки из буфера для вставки ссылки подписки», и
// вокруг кнопки «из буфера» оставалась рамка выделения. Адрес подписки человек
// получает тем же способом, что и ссылку на сервер: копирует.
describe("серверы: адрес подписки из буфера", () => {
  it("похоже на адрес подписки: только http и https, регистр и пробелы не мешают", () => {
    for (const a of ["https://a.example/x", " HTTP://b.example ", "https://c.example"]) expect(pohozheNaAdres(a)).toBe(true);
    for (const a of ["", "vless://x", "example.org", "ftp://d.example", "просто текст"]) expect(pohozheNaAdres(a)).toBe(false);
  });

  it("кнопки нет, пока оболочка не дала способа читать буфер", () => {
    risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("dobavit"));
    expect(screen.queryByTestId("iz-bufera")).toBeNull();
  });

  it("адрес из буфера уходит в addSubscription, и ничего больше", async () => {
    const naKomandu = vi.fn();
    render(<Servery status={VYKL} spisok={spisok([server(1)])} naKomandu={naKomandu} chitatBufer={async () => " https://zxc.example/dbae9043 "} />);
    fireEvent.click(screen.getByTestId("dobavit"));
    fireEvent.click(screen.getByTestId("iz-bufera"));
    await vi.waitFor(() => expect(naKomandu).toHaveBeenCalledWith("addSubscription", { adres: "https://zxc.example/dbae9043" }));
    expect(naKomandu).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).not.toMatch(/zxc\.example/);
  });

  // Курсор в поле снимает и рамку на кнопке позади формы, и лишний щелчок
  // перед вводом.
  it("открытая форма ставит курсор в поле", () => {
    risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("dobavit"));
    expect(document.activeElement).toBe(screen.getByTestId("ssylka"));
  });
});

// Выгрузка (26.09.2026). Текст и QR закрыты, пока их не попросили: панель
// открывают и при демонстрации экрана.
describe("серверы: выгрузка", () => {
  const OTVET = {
    tekst: "hy2://p@h:443#a\nanytls://p@h:995#b",
    base64: "aHkyOi8vcEBoOjQ0MyNhCmFueXRsczovL3BAaDo5OTUjYg==",
    vsego: 2,
    servery: [
      { id: "id1", imya: "Сервер 1", transport: "hy2", adres: "h:443", ssylka: "hy2://p@h:443#a" },
      { id: "id2", imya: "Сервер 2", transport: "anytls", adres: "h:995", ssylka: "anytls://p@h:995#b" },
    ],
    propushcheny: [{ imya: "старый", prichina: "этот вид сервера в ссылку не переводится" }],
  };

  function otkryt(pere: { skopirovat?: (t: string) => Promise<void>; kodyQr?: (t: string) => Promise<string[]> } = {}, otvet: unknown = OTVET) {
    const na = vi.fn((komanda: string, _telo: unknown, naOtvet?: (telo: unknown) => void) => {
      if (komanda === "exportServers") naOtvet?.(otvet);
    });
    render(<Servery status={VYKL} spisok={spisok([server(1), server(2)])} naKomandu={na} {...pere} />);
    fireEvent.click(screen.getByTestId("vygruzit"));
    return na;
  }

  it("кнопка шлёт exportServers, панель называет число и пропущенных, ключей не видно", () => {
    const na = otkryt();
    expect(na).toHaveBeenCalledWith("exportServers", {}, expect.any(Function));
    expect(screen.getByTestId("vygruzka-itog")).toHaveTextContent("Выгрузка: 2 сервера");
    expect(screen.getByTestId("vygruzka-propushcheny")).toHaveTextContent("не выгружен старый");
    expect(document.body.textContent).not.toMatch(/hy2:\/\//);
  });

  // Жалоба 02.10.2026 со скриншотом: сплошной текст из дюжины ссылок не давал
  // понять, какая из них какой сервер.
  it("каждый ключ это строка со своим сервером: имя, адрес, протокол словами", () => {
    otkryt();
    const stroki = within(screen.getByTestId("vygruzka-spisok")).getAllByRole("listitem");
    expect(stroki).toHaveLength(2);
    expect(stroki[0]).toHaveTextContent("Сервер 1");
    expect(stroki[0]).toHaveTextContent("h:443 · hysteria2");
    expect(stroki[1]).toHaveTextContent("h:995 · anytls");
  });

  it("«Скопировать всё» кладёт список или base64, не показывая их", async () => {
    const skopirovat = vi.fn(async () => {});
    otkryt({ skopirovat });
    fireEvent.click(screen.getByTestId("vygruzka-kopirovat"));
    await vi.waitFor(() => expect(skopirovat).toHaveBeenCalledWith(OTVET.tekst));
    expect(await screen.findByTestId("vygruzka-soobshchenie")).toHaveTextContent("скопировано");
    fireEvent.click(screen.getByRole("radio", { name: "одной закодированной строкой" }));
    fireEvent.click(screen.getByTestId("vygruzka-kopirovat"));
    await vi.waitFor(() => expect(skopirovat).toHaveBeenLastCalledWith(OTVET.base64));
    expect(document.body.textContent).not.toMatch(/hy2:\/\//);
  });

  it("ключ строки копируется отдельно, и строка говорит, что скопирована", async () => {
    const skopirovat = vi.fn(async () => {});
    otkryt({ skopirovat });
    fireEvent.click(screen.getByRole("button", { name: "скопировать ключ Сервер 2" }));
    await vi.waitFor(() => expect(skopirovat).toHaveBeenCalledWith("anytls://p@h:995#b"));
    expect(await screen.findByRole("button", { name: "скопировано: ключ Сервер 2" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "скопировать ключ Сервер 1" })).toHaveTextContent("Скопировать");
    expect(document.body.textContent).not.toMatch(/anytls:\/\//);
  });

  it("«Показать ключи» открывает ссылку под каждой строкой", () => {
    otkryt();
    fireEvent.click(screen.getByTestId("vygruzka-tekst"));
    expect(screen.getByTestId("vygruzka-klyuch-id1")).toHaveTextContent("hy2://p@h:443#a");
    expect(screen.getByTestId("vygruzka-klyuch-id2")).toHaveTextContent("anytls://p@h:995#b");
    fireEvent.click(screen.getByTestId("vygruzka-tekst"));
    expect(screen.queryByTestId("vygruzka-klyuch-id1")).toBeNull();
  });

  it("QR строки рисуется из её ссылки и открывается под ней", async () => {
    const kodyQr = vi.fn(async () => ["data:image/png;base64,AAA"]);
    otkryt({ kodyQr });
    fireEvent.click(screen.getByRole("button", { name: "QR ключа Сервер 1" }));
    await vi.waitFor(() => expect(kodyQr).toHaveBeenCalledWith("hy2://p@h:443#a"));
    const stroka = screen.getByTestId("vygruzka-stroka-id1");
    expect(await within(stroka).findByRole("img", { name: "QR Сервер 1" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "скрыть QR ключа Сервер 1" }));
    expect(within(stroka).queryByRole("img")).toBeNull();
  });

  // Служба старее окна поля servery не знает. Строки тогда собираются из
  // текста: имя из хвоста ссылки, протокол по схеме.
  it("без строк от службы список собирается из текста", () => {
    const { servery: _, ...bezStrok } = OTVET;
    otkryt({}, bezStrok);
    const stroki = within(screen.getByTestId("vygruzka-spisok")).getAllByRole("listitem");
    expect(stroki).toHaveLength(2);
    expect(stroki[0]).toHaveTextContent("a");
    expect(stroki[1]).toHaveTextContent("anytls");
  });

  it("QR рисуется из списка, а не из base64, и несколько кодов подписаны", async () => {
    const kodyQr = vi.fn(async () => ["data:image/png;base64,AAA", "data:image/png;base64,BBB"]);
    otkryt({ kodyQr });
    fireEvent.click(screen.getByRole("radio", { name: "одной закодированной строкой" }));
    fireEvent.click(screen.getByTestId("vygruzka-qr"));
    await vi.waitFor(() => expect(kodyQr).toHaveBeenCalledWith(OTVET.tekst));
    const kody = await screen.findByTestId("vygruzka-kody");
    expect(within(kody).getAllByRole("img")).toHaveLength(2);
    expect(kody).toHaveTextContent("код 1 из 2");
  });

  it("отказ оболочки нарисовать код назван словами", async () => {
    otkryt({ kodyQr: async () => { throw new Error("одна ссылка не помещается в QR, скопируй текстом"); } });
    fireEvent.click(screen.getByTestId("vygruzka-qr"));
    expect(await screen.findByTestId("vygruzka-soobshchenie")).toHaveTextContent("не помещается в QR");
  });

  it("на пустом списке выгружать нечего, кнопки нет", () => {
    risovat(spisok([]));
    expect(screen.queryByTestId("vygruzit")).toBeNull();
  });
});

// Ключ одного сервера из меню правой кнопки (02.10.2026, просьба владельца).
describe("серверы: ключ из меню", () => {
  const KLYUCH = { id: "id2", imya: "Сервер 2", transport: "reality-tcp", adres: "s2.example.net:443", ssylka: "vless://u@s2.example.net:443#s2" };
  const OTVET = { tekst: KLYUCH.ssylka, base64: "", vsego: 1, servery: [KLYUCH], propushcheny: [] };

  function otkryt(pere: { skopirovat?: (t: string) => Promise<void>; kodyQr?: (t: string) => Promise<string[]> } = {},
    otvet: (naOtvet?: (telo: unknown) => void, naOtkaz?: (o: { kod: string; tekst: string }) => void) => void = (naOtvet) => naOtvet?.(OTVET)) {
    const na = vi.fn((komanda: string, _telo: unknown, naOtvet?: (telo: unknown) => void, naOtkaz?: (o: { kod: string; tekst: string }) => void) => {
      if (komanda === "exportServers") otvet(naOtvet, naOtkaz);
    });
    render(<Servery status={VYKL} spisok={spisok([server(1), server(2)])} naKomandu={na} {...pere} />);
    fireEvent.contextMenu(screen.getAllByRole("option")[1]);
    return na;
  }

  it("«Показать ключ и QR» берёт ключ одного сервера и показывает его с кодом", async () => {
    const kodyQr = vi.fn(async () => ["data:image/png;base64,AAA"]);
    const na = otkryt({ kodyQr });
    fireEvent.click(screen.getByRole("menuitem", { name: "Показать ключ и QR" }));
    expect(na).toHaveBeenCalledWith("exportServers", { ids: ["id2"] }, expect.any(Function), expect.any(Function));
    const okno = screen.getByRole("dialog", { name: "Ключ сервера Сервер 2" });
    expect(within(okno).getByTestId("klyuch-servera-ssylka")).toHaveTextContent(KLYUCH.ssylka);
    expect(okno).toHaveTextContent("s2.example.net:443 · reality");
    await vi.waitFor(() => expect(kodyQr).toHaveBeenCalledWith(KLYUCH.ssylka));
    expect(await within(okno).findByRole("img", { name: "QR Сервер 2" })).toBeTruthy();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("«Скопировать ключ» кладёт ключ в буфер, не показывая его, и строка это отмечает", async () => {
    const skopirovat = vi.fn(async () => {});
    otkryt({ skopirovat });
    fireEvent.click(screen.getByRole("menuitem", { name: "Скопировать ключ" }));
    await vi.waitFor(() => expect(skopirovat).toHaveBeenCalledWith(KLYUCH.ssylka));
    expect(await screen.findByTestId("klyuch-skopirovan-id2")).toHaveTextContent("ключ скопирован");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.body.textContent).not.toMatch(/vless:\/\//);
  });

  it("буфер отказал: окно ключа объясняет и даёт скопировать руками", async () => {
    otkryt({ skopirovat: async () => { throw new Error("буфер занят другой программой"); } });
    fireEvent.click(screen.getByRole("menuitem", { name: "Скопировать ключ" }));
    const okno = await screen.findByRole("dialog");
    expect(within(okno).getByTestId("klyuch-servera-otkaz")).toHaveTextContent("не скопировалось");
    expect(within(okno).getByTestId("klyuch-servera-ssylka")).toHaveTextContent(KLYUCH.ssylka);
  });

  it("сервер, который в ссылку не переводится, назван причиной", () => {
    otkryt({}, (naOtvet) => naOtvet?.({ tekst: "", base64: "", vsego: 0, servery: [], propushcheny: [{ imya: "Сервер 2", prichina: "этот вид сервера в ссылку не переводится" }] }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Показать ключ и QR" }));
    expect(screen.getByTestId("klyuch-servera-otkaz")).toHaveTextContent("этот вид сервера в ссылку не переводится");
  });

  it("отказ службы показан в окне ключа, а не общим баннером", () => {
    otkryt({}, (_naOtvet, naOtkaz) => naOtkaz?.({ kod: "secrets-unreadable", tekst: "ключи на этой машине не читаются" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Показать ключ и QR" }));
    expect(screen.getByTestId("klyuch-servera-otkaz")).toHaveTextContent("ключи на этой машине не читаются");
  });

  it("без буфера у оболочки пункта копирования нет", () => {
    otkryt();
    expect(screen.queryByRole("menuitem", { name: "Скопировать ключ" })).toBeNull();
    expect(screen.getByRole("menuitem", { name: "Показать ключ и QR" })).toBeTruthy();
  });
});

