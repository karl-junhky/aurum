import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Karta, Knopka, MenyuUKursora, Ryad, Segment, Shapka, Teg, Tumbler } from "./ui";

afterEach(cleanup);

// Layout grammar, accepted 02.09.2026 on the mockup: one column, section
// cards with rows (label left, control right), three ranks of buttons, one
// switch component. Screens compose these and never restyle them, which is
// how four tabs stop drifting apart the way the first three already had.

describe("тумблер", () => {
  it("это нативный чекбокс с ролью switch, чтобы клавиатура и disabled были бесплатны", () => {
    const na = vi.fn();
    render(<Tumbler testId="t" vkl={false} aktiven naSmenu={na} podpis="x" />);
    const el = screen.getByTestId("t") as HTMLInputElement;
    expect(el.type).toBe("checkbox");
    expect(el.getAttribute("role")).toBe("switch");
    expect(el.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(el);
    expect(na).toHaveBeenCalledWith(true);
  });

  it("неактивный тумблер это disabled на самом input", () => {
    // Only the attribute is asserted: jsdom still toggles a disabled
    // checkbox on a synthetic click, a browser does not, and testing jsdom
    // here would test the wrong thing.
    render(<Tumbler testId="t" vkl aktiven={false} naSmenu={vi.fn()} podpis="x" />);
    expect((screen.getByTestId("t") as HTMLInputElement).disabled).toBe(true);
  });
});

describe("кнопка", () => {
  it("главная красится заливкой, второстепенная обводкой, опасная красным текстом", () => {
    render(
      <>
        <Knopka rang="glavnaya" testId="g">a</Knopka>
        <Knopka rang="vtoraya" testId="v">b</Knopka>
        <Knopka rang="opasnaya" testId="o">c</Knopka>
        <Knopka rang="tekst" testId="t">d</Knopka>
      </>,
    );
    expect(screen.getByTestId("g").className).toMatch(/bg-accent(?!-)/);
    expect(screen.getByTestId("v").className).toMatch(/border-border/);
    expect(screen.getByTestId("v").className).not.toMatch(/bg-accent(?!-)/);
    expect(screen.getByTestId("o").className).toMatch(/text-danger/);
    expect(screen.getByTestId("t").className).not.toMatch(/border-border/);
  });

  it("текст в кнопке центрирован по обеим осям: flex, не блок с отступами", () => {
    // The mockup's crooked labels were exactly this: inline-block plus
    // padding, and the baseline drifted with the font. Flex centring is the
    // one property the owner asked to be fixed at implementation time.
    render(<Knopka rang="glavnaya" testId="g">a</Knopka>);
    const c = screen.getByTestId("g").className;
    expect(c).toMatch(/inline-flex/);
    expect(c).toMatch(/items-center/);
    expect(c).toMatch(/justify-center/);
  });

  it("неактивная кнопка это disabled, а не класс", () => {
    const na = vi.fn();
    render(<Knopka rang="vtoraya" testId="v" aktiven={false} onClick={na}>b</Knopka>);
    const el = screen.getByTestId("v") as HTMLButtonElement;
    expect(el.disabled).toBe(true);
    fireEvent.click(el);
    expect(na).not.toHaveBeenCalled();
  });
});

describe("строка, карточка, шапка, сегмент", () => {
  it("строка рисует название и пояснение, элемент управления справа", () => {
    render(
      <Karta>
        <Ryad nazvanie="имя" poyasnenie="почему">
          <button type="button" data-testid="ctl">x</button>
        </Ryad>
      </Karta>,
    );
    expect(screen.getByText("имя")).toBeTruthy();
    expect(screen.getByText("почему")).toBeTruthy();
    expect(screen.getByTestId("ctl")).toBeTruthy();
  });

  it("шапка это заголовок и место под главное действие", () => {
    render(
      <Shapka zagolovok="Серверы" svodka="12 штук">
        <button type="button">Добавить</button>
      </Shapka>,
    );
    expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Серверы");
    expect(screen.getByText("12 штук")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Добавить" })).toBeTruthy();
  });

  it("сегмент это группа radio: один выбран, щелчок отдаёт значение", () => {
    const na = vi.fn();
    render(
      <Segment
        znacheniya={[{ z: "avto", podpis: "автоматически" }, { z: "ruchnoy", podpis: "вручную" }]}
        vybrano="avto"
        naVybor={na}
        aria-label="режим"
      />,
    );
    const radios = screen.getAllByRole("radio");
    expect(radios).toHaveLength(2);
    expect(radios[0].getAttribute("aria-checked")).toBe("true");
    fireEvent.click(radios[1]);
    expect(na).toHaveBeenCalledWith("ruchnoy");
  });

  it("пометка красится по тону, а тон один из четырёх", () => {
    render(<Teg ton="preduprezhdenie" testId="tg">нет в подписке</Teg>);
    expect(screen.getByTestId("tg").className).toMatch(/text-warn/);
  });
});

// Стрелка в группе переключателей до 23.09.2026 не делала ничего: с
// клавиатуры режим менялся только Tab и пробелом, а стрелку человек пробует
// первой. По практике ARIA стрелка и двигает фокус, и меняет выбор.
describe("стрелки в группе переключателей", () => {
  const znacheniya = [
    { z: "vpn" as const, podpis: "Всё через VPN" },
    { z: "direct" as const, podpis: "Только выбранное" },
  ];

  it("двигают выбор по кругу и ведут за собой фокус", () => {
    const na = vi.fn();
    render(<Segment znacheniya={znacheniya} vybrano="vpn" naVybor={na} aria-label="Куда идёт трафик" />);
    const gruppa = screen.getByRole("radiogroup");
    fireEvent.keyDown(gruppa, { key: "ArrowRight" });
    expect(na).toHaveBeenCalledExactlyOnceWith("direct");
    expect(document.activeElement).toBe(screen.getByRole("radio", { name: "Только выбранное" }));
    // Круг замкнут: со второго назад возвращает к первому.
    na.mockClear();
    fireEvent.keyDown(gruppa, { key: "ArrowLeft" });
    expect(na).toHaveBeenCalledExactlyOnceWith("direct");
  });

  it("молчат у выключенной группы и на чужих клавишах", () => {
    const na = vi.fn();
    const { rerender } = render(<Segment znacheniya={znacheniya} vybrano="vpn" naVybor={na} aktiven={false} aria-label="Куда идёт трафик" />);
    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "ArrowRight" });
    expect(na).not.toHaveBeenCalled();
    rerender(<Segment znacheniya={znacheniya} vybrano="vpn" naVybor={na} aria-label="Куда идёт трафик" />);
    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "Home" });
    expect(na).not.toHaveBeenCalled();
  });
});

describe("меню у курсора", () => {
  // jsdom не раскладывает страницу: размер меню задаётся руками.
  const razmer = (width: number, height: number) =>
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ width, height } as DOMRect);
  afterEach(() => vi.restoreAllMocks());
  const punkty = [{ podpis: "Скопировать ключ", naZhmyh: () => {} }];

  it("открывается в точке клика, если помещается", () => {
    razmer(224, 120);
    render(<MenyuUKursora x={100} y={100} punkty={punkty} podpis="Меню" naZakrytie={() => {}} />);
    expect(screen.getByRole("menu").style).toMatchObject({ left: "100px", top: "100px" });
  });

  it("у нижнего края открывается вверх, у правого влево", () => {
    razmer(224, 120);
    render(<MenyuUKursora x={window.innerWidth - 20} y={window.innerHeight - 30} punkty={punkty} podpis="Меню" naZakrytie={() => {}} />);
    expect(screen.getByRole("menu").style).toMatchObject({
      left: `${window.innerWidth - 20 - 224}px`,
      top: `${window.innerHeight - 30 - 120}px`,
    });
  });

  it("с клавиатуры у нижнего края встаёт над строкой, а не на неё", () => {
    razmer(224, 120);
    const niz = window.innerHeight - 10;
    render(<MenyuUKursora x={100} y={niz} yVverh={niz - 44} punkty={punkty} podpis="Меню" naZakrytie={() => {}} />);
    expect(screen.getByRole("menu").style.top).toBe(`${niz - 44 - 120}px`);
  });

  it("выше окна не уходит: прижимается к краю с зазором", () => {
    razmer(224, window.innerHeight);
    render(<MenyuUKursora x={10} y={50} punkty={punkty} podpis="Меню" naZakrytie={() => {}} />);
    expect(screen.getByRole("menu").style.top).toBe("8px");
  });
});
