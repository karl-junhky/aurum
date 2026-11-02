import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Otkaz } from "./Otkaz";
import {
  KOD_OBOLOCHKI, TEKST_BEZ_KODA, TEKST_OBOLOCHKI, tekstOtkaza, type Deystvie,
} from "./otkazy";

afterEach(cleanup);

// The list of codes comes from kody.go on disk, not from a copy here. A copy
// goes stale the day a code is added, and a stale copy is a green test about
// a screen that does not exist. The Go-side gate (internal/kachestvo) checks
// the actions against spec §9.1; this side checks that every code has text.
function kodyIzGo(): string[] {
  // process.cwd(), not import.meta.url: this file needs jsdom for render(),
  // and under jsdom import.meta.url is not a file: URL. vitest runs from the
  // frontend directory (vitest.config.ts lives there), so the path is fixed.
  const put = join(process.cwd(), "..", "..", "..", "internal", "protokol", "kody.go");
  const tekst = readFileSync(put, "utf8");
  const itog: string[] = [];
  for (const m of tekst.matchAll(/Kod[A-Za-z]+\s*=\s*"([a-z-]+)"/g)) itog.push(m[1]);
  return itog;
}

// not-implemented is the fallback contract's business (dash, not a refusal
// screen), and the Go gate excludes it by the same name.
const BEZ_EKRANA = new Set(["not-implemented"]);

const VSE_DEYSTVIYA: Deystvie[] = [
  "povtorit", "perepodklyuchitsya", "otkryt-servery", "obnovit-podpisku",
  "obnovit-programmu", "postavit-sluzhbu", "zaprosit-prava", "proverit-set",
  "pokazat-vinovnika", "nastroit-vremya", "prinyat-klyuch-hosta", "pereustanovit-os",
  "vzyat-pod-upravlenie", "otkryt-port-u-hostera", "poprosit-drugoy-adres", "nichego",
];

describe("экраны отказов", () => {
  const kody = kodyIzGo().filter((k) => !BEZ_EKRANA.has(k));

  it("коды прочитаны из kody.go, и их шестьдесят шесть", () => {
    // Pinning the count is deliberate: a new code must fail here until the
    // screen for it exists, and an empty read must not pass as "all covered".
    // Тридцать восьмой это internal-error, заведён 04.09.2026 полосой З:
    // паника службы отвечала кодом «разные версии», и человек шёл
    // переустанавливать исправную программу.
    // Тридцать девятый и сороковой заведены 05.09.2026 задачей 8:
    // bandwidth-unmeasured про число, которое пойдёт в объявление hysteria2, и
    // request-invalid про форму запроса. Оба отдельные потому, что чинятся
    // по-разному, а прежний общий код отправлял человека переустанавливать
    // исправную программу.
    // Сорок первый, сорок второй и сорок третий заведены 22.09.2026 (A7):
    // probe-timeout, subscription-timeout и subscription-auth-failed. Все три
    // отделяют «не успели» от «отказали»: прежде таймаут пробы назывался
    // отказом ключа и отправлял чинить исправную подписку.
    // Сорок четвёртый заведён 22.09.2026 (A6): rules-rejected-by-core. Набор
    // правил, который ядро не приняло, отличается от негодной строки: чинится
    // он не правкой строки, а отказом от неё, и VPN при этом цел.
    // Сорок пятый заведён 28.09.2026 (П9 аудита 1.6.1):
    // subscription-device-limit. Лимит устройств чинится в панели, а не в
    // ссылке и не в сети. Сорок шестой в тот же день (О6): diagnostics-stale,
    // выгрузка диагностики пережила перезапуск службы.
    // Сорок седьмой заведён 02.10.2026: server-tls-failed. Истёкший
    // сертификат сервера назывался отказом ключа и вёл обновлять подписку.
    // Сорок восьмой заведён 21.10.2026 (1.10.0): core-missing. Ядро, унесённое
    // антивирусом, называлось несозданным адаптером и звало повторять.
    // Сорок девятый в тот же день: clock-skew. Со сбитыми часами серверы
    // отказывают при любом ключе, а человек шёл чинить сеть или подписку.
    // С пятидесятого по шестьдесят шестой заведены планом «Свой VPN»
    // (2026-11-01): семнадцать кодов vps-* про свой сервер. Чинятся они
    // по-разному: вход, ключ хоста, порты у хостера, занятость службы.
    expect(kody.length).toBe(66);
  });

  it.each(kody)("у кода %s есть свой текст и действие из словаря", (kod) => {
    const z = tekstOtkaza[kod];
    expect(z, `нет записи для ${kod}`).toBeTruthy();
    expect(z.tekst.trim().length).toBeGreaterThan(0);
    expect(VSE_DEYSTVIYA).toContain(z.deystvie);
  });

  it("тексты сухие: без «мы» и без точки в конце", () => {
    for (const kod of kody) {
      const t = tekstOtkaza[kod].tekst;
      expect(t, kod).not.toMatch(/\bмы\b/i);
      expect(t, kod).not.toMatch(/\.$/);
    }
  });

  it("pipe-squatted показывает имя виновника", () => {
    render(<Otkaz kod="pipe-squatted" vinovnik="chuzhoy.exe" naDeystvie={() => undefined} />);
    expect(screen.getByTestId("vinovnik")).toHaveTextContent("chuzhoy.exe");
    // And it is NOT the first-run screen: no install button on a squatted pipe.
    expect(screen.queryByText(/установить службу/i)).toBeNull();
  });

  it("subscription-expired показывает текст панели дословно, без нашего поверх", () => {
    render(<Otkaz kod="subscription-expired" tekst="Оплатите тариф до 05.09" naDeystvie={() => undefined} />);
    const el = screen.getByTestId("otkaz-tekst");
    expect(el).toHaveTextContent("Оплатите тариф до 05.09");
    expect(el.textContent).not.toContain(tekstOtkaza["subscription-expired"].tekst);
  });

  it("у nichego нет кнопки, у povtorit есть, и она отдаёт действие наверх", () => {
    const na = vi.fn();
    render(<Otkaz kod="firewall-disabled" naDeystvie={na} />);
    expect(screen.queryByTestId("otkaz-deystvie")).toBeNull();
    cleanup();
    render(<Otkaz kod="dns-resolve-failed" naDeystvie={na} />);
    fireEvent.click(screen.getByTestId("otkaz-deystvie"));
    expect(na).toHaveBeenCalledWith("povtorit");
  });
  it("не печатает причину, которая слово в слово повторяет заголовок", () => {
    // Служба и окно называют одно и то же своими словами; на экране это
    // читалось как две строки об одном (живой прогон 13.09.2026).
    render(<Otkaz kod="admin-required" tekst="нужны права администратора" naDeystvie={() => undefined} />);
    expect(screen.queryByTestId("otkaz-prichina")).toBeNull();
  });

  it("печатает причину, когда она добавляет новое", () => {
    render(<Otkaz kod="admin-required" tekst="права не выданы: запрос отклонён" naDeystvie={() => undefined} />);
    expect(screen.getByTestId("otkaz-prichina").textContent).toContain("запрос отклонён");
  });

  it("незнакомый код без текста не печатается кодом", () => {
    // Код это имя для нас. Человеку строка `kod-iz-budushchego` не говорит
    // ничего и выглядит как сбой программы, а не как ответ.
    render(<Otkaz kod="kod-iz-budushchego" naDeystvie={() => undefined} />);
    const el = screen.getByTestId("otkaz-tekst");
    expect(el).toHaveTextContent(TEKST_BEZ_KODA);
    expect(el.textContent).not.toContain("kod-iz-budushchego");
    // Но из DOM код не пропадает: по нему разбирают снимок экрана.
    expect(screen.getByTestId("otkaz")).toHaveAttribute("data-kod", "kod-iz-budushchego");
  });

  it("сбой оболочки: крупно наша фраза, английская техника только в подсказке", () => {
    // Текст приходит из Windows и по-английски. На экране он читается как
    // вывод отладчика (01.10.2026), поэтому остаётся только подсказкой.
    render(
      <Otkaz kod={KOD_OBOLOCHKI} tekst="connect: read pipe: The pipe has been ended."
             naDeystvie={() => undefined} />,
    );
    const el = screen.getByTestId("otkaz-tekst");
    expect(el).toHaveTextContent(TEKST_OBOLOCHKI);
    expect(screen.queryByTestId("otkaz-prichina")).toBeNull();
    expect(el).toHaveAttribute("title", "connect: read pipe: The pipe has been ended.");
  });

  it("сбой оболочки: наша часть причины остаётся, хвост уходит в подсказку", () => {
    render(
      <Otkaz kod={KOD_OBOLOCHKI} tekst="Архив не выбран: dialog: The operation was canceled by the user."
             naDeystvie={() => undefined} />,
    );
    const p = screen.getByTestId("otkaz-prichina");
    expect(p).toHaveTextContent("Архив не выбран");
    expect(p.textContent).not.toContain("canceled");
    expect(p).toHaveAttribute("title", "Архив не выбран: dialog: The operation was canceled by the user.");
  });
});

// Просьба владельца 21.10.2026: под любой ошибкой кнопка, которая сохраняет
// журнал файлом, чтобы его переслать.
describe("журнал под отказом", () => {
  it("без обработчиков полосы журнала нет", () => {
    render(<Otkaz kod="all-servers-down" naDeystvie={() => undefined} />);
    expect(screen.queryByTestId("otkaz-zhurnal")).toBeNull();
  });

  it("сохраняет журнал и печатает итог под кнопками", async () => {
    const sohranit = vi.fn(() => Promise.resolve("сохранено: affory-zhurnal-1.txt, 2 МБ"));
    render(<Otkaz kod="all-servers-down" naDeystvie={() => undefined} naSohranitZhurnal={sohranit}
                  versiya="Affory 1.10.0, Windows 11 (26200)" />);
    expect(screen.getByTestId("otkaz-versiya")).toHaveTextContent("Affory 1.10.0, Windows 11 (26200)");
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Сохранить журнал" })); });
    expect(sohranit).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("otkaz-itog-zhurnala")).toHaveTextContent("сохранено: affory-zhurnal-1.txt, 2 МБ");
  });

  it("отмена диалога не печатает ничего", async () => {
    render(<Otkaz kod="all-servers-down" naDeystvie={() => undefined} naSohranitZhurnal={() => Promise.resolve("")} />);
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Сохранить журнал" })); });
    expect(screen.queryByTestId("otkaz-itog-zhurnala")).toBeNull();
  });

  it("отказ сохранения говорится нашими словами, без английского хвоста", async () => {
    const sohranit = () => Promise.reject(new Error("файл не записан: open C:\\x: Access is denied."));
    render(<Otkaz kod="all-servers-down" naDeystvie={() => undefined} naSohranitZhurnal={sohranit} />);
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Сохранить журнал" })); });
    const itog = screen.getByTestId("otkaz-itog-zhurnala");
    expect(itog).toHaveTextContent("журнал не сохранён: файл не записан");
    expect(itog.textContent).not.toContain("Access");
    expect(screen.getByRole("button", { name: "Сохранить журнал" })).toBeEnabled();
  });

  it("копирует текст целиком с кодом и версией", async () => {
    const skopirovat = vi.fn((_: string) => Promise.resolve());
    render(<Otkaz kod={KOD_OBOLOCHKI} tekst="connect: read pipe: The pipe has been ended."
                  naDeystvie={() => undefined} naSkopirovat={skopirovat} versiya="Affory 1.10.0, Windows 10 (19045)" />);
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Скопировать текст" })); });
    expect(skopirovat).toHaveBeenCalledWith(
      `${TEKST_OBOLOCHKI}\nconnect: read pipe: The pipe has been ended.\nкод: ${KOD_OBOLOCHKI}\nAffory 1.10.0, Windows 10 (19045)`,
    );
    expect(screen.getByTestId("otkaz-itog-zhurnala")).toHaveTextContent("текст ошибки скопирован");
  });
});
