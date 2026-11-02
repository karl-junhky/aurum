import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Karkas } from "./Karkas";
import { VKLADKI, nazvanieVkladki } from "./vkladki";

afterEach(cleanup);

const nichego = () => undefined;

describe("каркас окна", () => {
  // Четыре вкладки §8.3 и пятая «Свой VPN» по решению владельца 30.09.2026.
  it("вкладок пять, «Свой VPN» между «Серверами» и «Настройками»", () => {
    expect([...VKLADKI]).toEqual(["podklyuchenie", "pravila", "servery", "svoy-vpn", "nastroyki"]);
    expect(VKLADKI.map(nazvanieVkladki)).toEqual(["Подключение", "Правила", "Серверы", "Свой VPN", "Настройки"]);
  });

  it("активная вкладка отмечена чернилами, не заливкой", () => {
    // §8.2: fill paints things that carry a label; state is shown in ink.
    // A tab marked by fill alone has no visible edge on black.
    render(
      <Karkas vkladka="servery" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego}>x</Karkas>,
    );
    const akt = screen.getByRole("tab", { selected: true });
    expect(akt).toHaveTextContent("Подключение");
    expect(akt.className).toMatch(/accent-ink/);
    expect(akt.className).not.toMatch(/bg-accent(?!-)/);
  });

  it("щелчок по вкладке отдаёт её имя наверх", () => {
    const na = vi.fn();
    render(
      <Karkas vkladka="podklyuchenie" naVkladku={na} naSvernut={nichego} naZakryt={nichego}>x</Karkas>,
    );
    fireEvent.click(screen.getByRole("tab", { name: "Правила" }));
    expect(na).toHaveBeenCalledWith("pravila");
  });

  it("полоса перетаскивается, кнопки окна нет", () => {
    // The whole reason this task exists: without a drag region a frameless
    // window cannot be moved, and without no-drag the close button drags.
    render(
      <Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego}>x</Karkas>,
    );
    expect(screen.getByTestId("polosa").style.getPropertyValue("--wails-draggable")).toBe("drag");
    expect(screen.getByTestId("zakryt").style.getPropertyValue("--wails-draggable")).toBe("no-drag");
  });

  it("свернуть и закрыть зовут наверх, а не оболочку напрямую", () => {
    const sv = vi.fn();
    const za = vi.fn();
    render(
      <Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={sv} naZakryt={za}>x</Karkas>,
    );
    fireEvent.click(screen.getByTestId("svernut"));
    fireEvent.click(screen.getByTestId("zakryt"));
    expect(sv).toHaveBeenCalledOnce();
    expect(za).toHaveBeenCalledOnce();
  });
});

// First run (§9.2): until the service exists nothing else is active, tabs
// included. The window controls stay live: the human can still close it.
describe("каркас: вкладки на первом запуске", () => {
  it("с zablokirovany вкладки неактивны, свернуть и закрыть работают", () => {
    const na = vi.fn();
    render(
      <Karkas vkladka="podklyuchenie" naVkladku={na} naSvernut={nichego} naZakryt={nichego} zablokirovany>x</Karkas>,
    );
    const tab = screen.getByRole("tab", { name: "Подключение" }) as HTMLButtonElement;
    expect(tab.disabled).toBe(true);
    fireEvent.click(tab);
    expect(na).not.toHaveBeenCalled();
    expect((screen.getByTestId("zakryt") as HTMLButtonElement).disabled).toBe(false);
  });
});

it("GitHub opens through the shell without making the title bar draggable", () => {
  // A repository link should open a browser, not tow the whole window away.
  const open = vi.fn();
  render(<Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego} naGitHub={open}>x</Karkas>);
  const button = screen.getByRole("button", {name: "Открыть GitHub Affory"});
  fireEvent.click(button);
  expect(open).toHaveBeenCalledOnce();
  expect(button.style.getPropertyValue("--wails-draggable")).toBe("no-drag");
});

// Редизайн 16.09.2026: «Серверы» остались вкладкой в коде (на них уводит
// кнопка «Управлять»), но собственной кнопки в полосе у них больше нет: набор
// серверов живёт на экране подключения. С 1.11.0 в полосе четыре раздела.
describe("каркас: разделы полосы и третья кнопка окна", () => {
  it("в полосе четыре раздела, «Серверы» в неё не выходят", () => {
    render(<Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego}>x</Karkas>);
    expect(screen.getAllByRole("tab").map((t) => t.textContent)).toEqual([
      "Подключение", "Правила", "Свой VPN", "Настройки",
    ]);
    expect(screen.queryByRole("tab", { name: "Серверы" })).toBeNull();
  });

  it("метка своего сервера видна точкой и слышна голосом", () => {
    render(<Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego} metkaSvoegoVpn="danger">x</Karkas>);
    const vkladka = screen.getByRole("tab", { name: /Свой VPN/ });
    expect(vkladka).toHaveAccessibleName("Свой VPN, не работает");
    expect(screen.getByTestId("metka-svoego-vpn").className).toMatch(/bg-danger/);
    cleanup();
    render(<Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego}>x</Karkas>);
    expect(screen.queryByTestId("metka-svoego-vpn")).toBeNull();
    expect(screen.getByRole("tab", { name: "Свой VPN" })).toBeTruthy();
  });

  it("на вкладке серверов подсвечено «Подключение»: полоса не теряет место", () => {
    render(<Karkas vkladka="servery" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego}>x</Karkas>);
    expect(screen.getByRole("tab", { selected: true })).toHaveTextContent("Подключение");
  });

  // C9. Полоса разделов была набором отдельных кнопок: стрелки не делали
  // ничего, а Tab проходил по каждой, поэтому с клавиатуры до содержимого окна
  // надо было продавиться через всю полосу.
  it("стрелки ходят по разделам, Tab доносит только до выбранного", () => {
    const na = vi.fn();
    render(<Karkas vkladka="podklyuchenie" naVkladku={na} naSvernut={nichego} naZakryt={nichego}>x</Karkas>);
    const vkladki = screen.getAllByRole("tab");
    expect(vkladki.map((t) => t.tabIndex)).toEqual([0, -1, -1, -1]);
    fireEvent.keyDown(vkladki[0], { key: "ArrowRight" });
    expect(na).toHaveBeenLastCalledWith("pravila");
    fireEvent.keyDown(vkladki[1], { key: "ArrowRight" });
    expect(na).toHaveBeenLastCalledWith("svoy-vpn");
    fireEvent.keyDown(vkladki[0], { key: "ArrowLeft" });
    expect(na).toHaveBeenLastCalledWith("nastroyki");
    fireEvent.keyDown(vkladki[0], { key: "End" });
    expect(na).toHaveBeenLastCalledWith("nastroyki");
    fireEvent.keyDown(vkladki[3], { key: "Home" });
    expect(na).toHaveBeenLastCalledWith("podklyuchenie");
    na.mockClear();
    fireEvent.keyDown(vkladki[0], { key: "a" });
    expect(na).not.toHaveBeenCalled();
  });

  it("развернуть зовёт наверх, а без обработчика кнопка неактивна", () => {
    const ra = vi.fn();
    render(<Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={nichego} naRazvernut={ra} naZakryt={nichego}>x</Karkas>);
    fireEvent.click(screen.getByTestId("razvernut"));
    expect(ra).toHaveBeenCalledOnce();
    cleanup();
    render(<Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego}>x</Karkas>);
    expect((screen.getByTestId("razvernut") as HTMLButtonElement).disabled).toBe(true);
  });
});
