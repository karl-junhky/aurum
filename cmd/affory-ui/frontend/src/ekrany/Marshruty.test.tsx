import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Pravila, type PravilaOtvet } from "./Pravila";
import { Glavnyy } from "./Glavnyy";
afterEach(cleanup);
async function primenit() {
  await act(async()=>{fireEvent.click(screen.getByRole("button",{name:"Применить изменения"}));});
}
const rules: PravilaOtvet = {
  protsessy: [],
  domeny: [],
  trafik: {
    po_umolchaniyu: "direct",
    prilozheniya: [],
    domeny: [{ domen: "work.example", marshrut: "direct" }],
    servisy: [],
  },
  katalog: {
    versiya: "test",
    istochnik: "https://example.org",
    servisy: [
      {
        id: "youtube",
        imya: "YouTube",
        domeny: ["youtube.com", "googlevideo.com"],
        programmy: [],
        istochnik: "https://example.org",
      },
      // Сервис с сайтами И клиентом.
      {
        id: "discord",
        imya: "Discord",
        domeny: ["discord.com"],
        programmy: ["Discord.exe", "DiscordPTB.exe"],
        istochnik: "https://example.org",
      },
      // Сервис, у которого набора сайтов нет вовсе: маршрут держится на клиенте.
      { id: "steam", imya: "Steam", domeny: [], programmy: ["steam.exe"] },
    ],
  },
};

it("повтор домена с регистром и точкой заменяет маршрут одной записи", async () => {
  const send=vi.fn();
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Домен сайта"),{target:{value:" WORK.EXAMPLE. "}});
  fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({trafik:expect.objectContaining({domeny:[{domen:"work.example",marshrut:"vpn"}]})}));
});

it("повтор приложения обновляет правило на прежнем месте",async()=>{
  const first={put:"C:\\First.exe",imya:"First.exe",potomki:true,marshrut:"direct" as const};
  const second={put:"C:\\Second.exe",imya:"Second.exe",potomki:false,marshrut:"vpn" as const};
  const send=vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={{...rules,trafik:{...rules.trafik!,prilozheniya:[first,second]}}} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Путь к приложению"),{target:{value:first.put}});
  fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({trafik:expect.objectContaining({prilozheniya:[{...first,marshrut:"vpn"},second]})}));
});

it("замена файла сохраняет маршрут и охват, существующее правило не перетирается",async()=>{
  const app={put:"C:\\Old.exe",imya:"Old.exe",potomki:true,marshrut:"direct" as const};
  const other={...app,put:"C:\\Other.exe",imya:"Other.exe"};
  const choose=vi.fn().mockResolvedValueOnce(other.put).mockResolvedValueOnce("C:\\New.exe");
  const send=vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={{...rules,trafik:{...rules.trafik!,prilozheniya:[app,other]}}} otlozheno={{}} naKomandu={send} naVyborPrilozheniya={choose}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.click(screen.getByRole("button",{name:"Проверка правил"}));
  await act(async()=>fireEvent.click(screen.getByRole("button",{name:"Заменить файл"})));
  expect(screen.getByRole("alert")).toHaveTextContent("уже есть правило");
  expect(screen.queryByRole("button",{name:"Применить изменения"})).toBeNull();
  await act(async()=>fireEvent.click(screen.getByRole("button",{name:"Заменить файл"})));
  expect(send).not.toHaveBeenCalled();
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({trafik:expect.objectContaining({prilozheniya:[{...app,put:"C:\\New.exe",imya:"New.exe"},other]})}));
});

it("новая форма учитывает смену общего режима, открытая сохраняет выбранный маршрут", () => {
  const initial:PravilaOtvet={...rules,trafik:{...rules.trafik!,po_umolchaniyu:"vpn"}};
  const props={status:{sostoyanie:"vyklyuchen" as const},otlozheno:{},naKomandu:vi.fn()};
  const view=render(<Pravila {...props} pravila={initial}/>);
  view.rerender(<Pravila {...props} pravila={rules}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  expect(screen.getByRole("combobox",{name:"Маршрут нового правила"})).toHaveTextContent("Через VPN");
  fireEvent.click(screen.getByRole("combobox",{name:"Маршрут нового правила"}));
  fireEvent.click(screen.getByRole("option",{name:"Напрямую"}));
  view.rerender(<Pravila {...props} pravila={initial}/>);
  expect(screen.getByRole("combobox",{name:"Маршрут нового правила"})).toHaveTextContent("Напрямую");
});
it("service toggle sends a domain bundle route without inventing process exclusions", async () => {
  const send = vi.fn();
  render(
    <Pravila
      status={{ sostoyanie: "vyklyuchen" }}
      pravila={rules}
      otlozheno={{}}
      naKomandu={send}
    />,
  );
  fireEvent.click(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"}));
  fireEvent.click(screen.getByRole("option",{name:"Через VPN"}));
  // Число и слово стоят в разных строках макета, поэтому пробел между ними
  // рисует раскладка, а не текст.
  expect(screen.getByTestId("svodka-pravil").textContent).toMatch(/2\s*отдельных правила/);
  expect(screen.getByText("2 домена с поддоменами")).toBeInTheDocument();
  await primenit();
  expect(send).toHaveBeenCalledWith(
    "setRules",
    expect.objectContaining({
      trafik: {
        ...rules.trafik,
        servisy: [{ id: "youtube", marshrut: "vpn" }],
      },
    }),
  );
});
it("changing the default preserves explicit direct domain intent", async () => {
  const send = vi.fn();
  render(
    <Pravila
      status={{ sostoyanie: "vyklyuchen" }}
      pravila={rules}
      otlozheno={{}}
      naKomandu={send}
    />,
  );
  fireEvent.click(screen.getByRole("radio", { name: "Всё через VPN" }));
  await primenit();
  expect(send).toHaveBeenCalledWith(
    "setRules",
    expect.objectContaining({
      trafik: { ...rules.trafik, po_umolchaniyu: "vpn" },
    }),
  );
});
it("application picker persists the complete path and descendant scope", async () => {
  const send = vi.fn();
  render(
    <Pravila
      status={{ sostoyanie: "vyklyuchen" }}
      pravila={rules}
      otlozheno={{}}
      zapushchennye={[{ imya: "Steam", put: "C:\\Games\\Steam\\steam.exe" }]}
      naKomandu={send}
    />,
  );
  fireEvent.click(screen.getByRole("tab", { name: /Приложения/ }));
  fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
  fireEvent.click(screen.getByRole("button", { name: /^Steam,/ }));
  fireEvent.click(screen.getByRole("button", { name: "Добавить в черновик" }));
  await primenit();
  expect(send).toHaveBeenCalledWith(
    "setRules",
    expect.objectContaining({
      trafik: expect.objectContaining({
        prilozheniya: [
          {
            put: "C:\\Games\\Steam\\steam.exe",
            imya: "steam.exe",
            potomki: true,
            marshrut: "vpn",
          },
        ],
      }),
    }),
  );
});

// A file dialog supplies a path, not a surprise rule and certainly not a launch party.
function openAppForm(picker: () => Promise<string>) {
  const send = vi.fn();
  render(<Pravila status={{ sostoyanie: "vyklyuchen" }} pravila={rules} otlozheno={{}}
    naVyborPrilozheniya={picker} naKomandu={send} />);
  fireEvent.click(screen.getByRole("tab", { name: /Приложения/ }));
  fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
  return send;
}

it("browsing the PC fills a path and waits for an explicit save", async () => {
  const path = "C:\\Мои приложения\\Steam\\steam.EXE";
  const picker = vi.fn(async () => path);
  const send = openAppForm(picker);
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  await waitFor(() => expect(screen.getByLabelText("Путь к приложению")).toHaveValue(path));
  expect(picker).toHaveBeenCalledOnce();
  expect(send).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Добавить в черновик" }));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules", expect.objectContaining({
    trafik: expect.objectContaining({ prilozheniya: [
      { put: path, imya: "steam.EXE", potomki: true, marshrut: "vpn" },
    ] }),
  }));
});

it("cancelling file selection preserves a manually entered path", async () => {
  const send = openAppForm(async () => "");
  fireEvent.change(screen.getByLabelText("Путь к приложению"), { target: { value: "C:\\Apps\\old.exe" } });
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Выбрать на ПК…" })).toBeEnabled());
  expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Apps\\old.exe");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(send).not.toHaveBeenCalled();
});

it("reports a dialog error by the picker and allows another attempt", async () => {
  const picker = vi.fn().mockRejectedValueOnce(new Error("нет доступа")).mockResolvedValueOnce("C:\\Apps\\new.exe");
  const send = openAppForm(picker);
  fireEvent.change(screen.getByLabelText("Путь к приложению"), { target: { value: "C:\\Apps\\old.exe" } });
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось выбрать приложение: нет доступа");
  expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Apps\\old.exe");
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  await waitFor(() => expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Apps\\new.exe"));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(send).not.toHaveBeenCalled();
});

it("blocks duplicate dialogs and discards a selection after leaving the form", async () => {
  let choose!: (path: string) => void;
  const picker = vi.fn(() => new Promise<string>(resolve => { choose = resolve; }));
  const send = openAppForm(picker);
  fireEvent.change(screen.getByLabelText("Путь к приложению"), { target: { value: "C:\\Apps\\old.exe" } });
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  const pending = screen.getByRole("button", { name: "Выбор…" });
  expect(pending).toBeDisabled();
  expect(screen.getByRole("button", { name: "Добавить в черновик" })).toBeDisabled();
  fireEvent.click(pending);
  expect(picker).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("tab", { name: /Сайты/ }));
  await act(async () => choose("C:\\Apps\\new.exe"));
  fireEvent.click(screen.getByRole("tab", { name: /Приложения/ }));
  fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
  expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Apps\\old.exe");
  expect(send).not.toHaveBeenCalled();
});
it("sphere can cancel an in-progress connection and has no duplicate heading", () => {
  const act = vi.fn();
  render(<Glavnyy status={{ sostoyanie: "podnimaetsya" }} naDeystvie={act} />);
  fireEvent.click(screen.getByRole("button", { name: "Отменить подключение" }));
  expect(act).toHaveBeenCalledOnce();
  expect(screen.queryByText(/VPN включён|VPN выключен/)).toBeNull();
});
it("server row connects directly from the home screen", () => {
  const choose = vi.fn();
  render(
    <Glavnyy
      status={{ sostoyanie: "vyklyuchen" }}
      servery={[
        {
          id: "ams",
          imya: "Amsterdam",
          host: "vpn.example.org",
          port: 443,
          transport: "hy2",
          iz_podpiski: true,
        },
      ]}
      naVyborServera={choose}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Подключиться к Amsterdam" }),
  );
  expect(choose).toHaveBeenCalledWith("ams");
});
it("pending rule application disables controls instead of losing a second edit", () => {
  const send = vi.fn();
  render(
    <Pravila
      status={{ sostoyanie: "podnyat" }}
      pravila={rules}
      otlozheno={{}}
      naKomandu={send}
      zanyato
    />,
  );
  expect(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"})).toBeDisabled();
  fireEvent.click(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"}));
  expect(screen.queryByRole("option",{name:"Напрямую"})).toBeNull();
  expect(send).not.toHaveBeenCalled();
});
it("журнал перенесён из правил в настройки", () => {
  render(<Pravila status={{sostoyanie:"podnyat",diagnostika:true}} pravila={rules} otlozheno={{}} naKomandu={vi.fn()}/>);
  expect(screen.queryByTestId("diagnostika")).toBeNull();
  expect(screen.queryByTestId("razdel-zhurnal")).toBeNull();
});

// C8. Пустой ответ оболочки и неудачное чтение рисовались ОДНОЙ строкой «Не
// найдено»: провал выглядел ровно как машина без запущенных программ, и
// повторить чтение было нечем, кроме закрытия формы.
function otkrytFormuPrilozheniy(props: Partial<Parameters<typeof Pravila>[0]> = {}) {
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={vi.fn()} {...props}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
}

it("чтение списка программ не выдаётся за пустой список", () => {
  otkrytFormuPrilozheniy({zapushchennye:null,protsessyChitayutsya:true,obnovitProtsessy:vi.fn()});
  expect(screen.getByText(/Читаю запущенные программы/)).toBeInTheDocument();
  expect(screen.queryByText(/не видно/)).toBeNull();
  // Путь руками остаётся доступен всё это время: список программ удобство, а
  // не единственный способ завести правило.
  expect(screen.getByLabelText("Путь к приложению")).toBeEnabled();
});

it("неудачное чтение списка программ объясняется и повторяется по кнопке", () => {
  const obnovit = vi.fn();
  otkrytFormuPrilozheniy({zapushchennye:null,protsessyOtkaz:"канал оболочки закрыт",obnovitProtsessy:obnovit});
  const soobshchenie = screen.getByRole("alert");
  expect(soobshchenie).toHaveTextContent(/Не удалось прочитать запущенные программы/);
  expect(soobshchenie).toHaveTextContent("канал оболочки закрыт");
  expect(screen.queryByText(/не видно/)).toBeNull();
  expect(screen.getByLabelText("Путь к приложению")).toBeEnabled();
  obnovit.mockClear();
  fireEvent.click(screen.getByRole("button",{name:"Повторить"}));
  expect(obnovit).toHaveBeenCalledTimes(1);
});

it("измеренный ноль программ отличается от отсутствия совпадений", () => {
  otkrytFormuPrilozheniy({zapushchennye:[],obnovitProtsessy:vi.fn()});
  expect(screen.getByText(/Запущенных программ не видно/)).toBeInTheDocument();
  cleanup();
  otkrytFormuPrilozheniy({zapushchennye:[{imya:"Steam",put:"C:\\Games\\steam.exe"}],obnovitProtsessy:vi.fn()});
  fireEvent.change(screen.getByLabelText("Поиск приложения"),{target:{value:"discord"}});
  expect(screen.getByText(/Совпадений нет/)).toBeInTheDocument();
  expect(screen.queryByText(/Запущенных программ не видно/)).toBeNull();
});

it("отказ при списке на руках оставляет снимок с пометкой, а не пустоту", () => {
  otkrytFormuPrilozheniy({
    zapushchennye:[{imya:"Steam",put:"C:\\Games\\steam.exe"}],
    protsessyOtkaz:"оболочка не ответила",
    obnovitProtsessy:vi.fn(),
  });
  expect(screen.getByRole("button",{name:/^Steam,/})).toBeInTheDocument();
  expect(screen.getByText(/Список мог устареть/)).toHaveTextContent("оболочка не ответила");
});

it("кнопка обновления перечитывает список при открытой форме", () => {
  const obnovit = vi.fn();
  otkrytFormuPrilozheniy({zapushchennye:[],obnovitProtsessy:obnovit});
  obnovit.mockClear();
  fireEvent.click(screen.getByRole("button",{name:"Обновить"}));
  expect(obnovit).toHaveBeenCalledTimes(1);
});

// D2. Карточка сервиса это сайты И приложение сразу. Путь к .exe угадывать
// нельзя: у Discord в нём номер сборки, у лаунчеров - диск установки, поэтому
// имя файла из каталога ищется среди запущенных программ.
it("карточка сервиса берёт фактический путь запущенного клиента", async () => {
  const put = "C:\\Users\\home\\AppData\\Local\\Discord\\app-1.0.9187\\Discord.exe";
  const send = vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}
    zapushchennye={[{imya:"Discord",put}]} obnovitProtsessy={vi.fn()}/>);
  fireEvent.click(screen.getByRole("combobox",{name:"Маршрут сервиса Discord"}));
  fireEvent.click(screen.getByRole("option",{name:"Через VPN"}));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({
    trafik: expect.objectContaining({servisy:[{id:"discord",marshrut:"vpn",programmy:[put]}]}),
  }));
});

it("незапущенный клиент не превращается в выдуманный путь", async () => {
  const send = vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}
    zapushchennye={[]} obnovitProtsessy={vi.fn()}/>);
  // На карточке, которую человек не трогал, строки нет: предупреждать не о чем.
  expect(screen.queryByTestId("klient-discord")).toBeNull();
  fireEvent.click(screen.getByRole("combobox",{name:"Маршрут сервиса Discord"}));
  fireEvent.click(screen.getByRole("option",{name:"Через VPN"}));
  // А на включённой сказано прямо: сайты накрыты, приложение нет.
  expect(screen.getByTestId("klient-discord")).toHaveTextContent("Приложение не запущено");
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({
    trafik: expect.objectContaining({servisy:[{id:"discord",marshrut:"vpn"}]}),
  }));
});

it("карточке без сайтов и без запущенного клиента маршрут выбрать нечему", () => {
  // Правило вышло бы пустым, а карточка выглядела бы работающей.
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={vi.fn()}
    zapushchennye={[]} obnovitProtsessy={vi.fn()}/>);
  expect(screen.getByRole("combobox",{name:"Маршрут сервиса Steam"})).toBeDisabled();
  expect(screen.getByTestId("klient-steam")).toHaveTextContent("Запусти приложение, чтобы выбрать маршрут");
});

it("включённая карточка говорит, что накрывает и приложение", () => {
  const put = "C:\\Games\\Steam\\steam.exe";
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} otlozheno={{}} naKomandu={vi.fn()}
    pravila={{...rules,trafik:{...rules.trafik!,servisy:[{id:"steam",marshrut:"vpn",programmy:[put]}]}}}
    zapushchennye={[]} obnovitProtsessy={vi.fn()}/>);
  expect(screen.getByTestId("klient-steam")).toHaveTextContent("и программы, которые оно запускает");
  // Выбор остаётся: путь уже в правиле, и снять маршрут человек должен уметь.
  expect(screen.getByRole("combobox",{name:"Маршрут сервиса Steam"})).toBeEnabled();
});

// C3. Поиск, фильтр и групповые действия по уже сохранённым правилам: лимиты
// в 256 приложений и 1024 сайта делают скролл негодным способом что-то найти.
const mnogoPravil: PravilaOtvet = {
  ...rules,
  trafik: {
    ...rules.trafik!,
    prilozheniya: [
      {put:"C:\\Games\\steam.exe",imya:"steam.exe",potomki:true,marshrut:"vpn"},
      {put:"C:\\Games\\launcher.exe",imya:"launcher.exe",potomki:true,marshrut:"direct"},
      {put:"C:\\Work\\zoom.exe",imya:"zoom.exe",potomki:false,marshrut:"direct"},
    ],
  },
};

it("поиск и фильтр маршрута сужают список и честно считают показанное", () => {
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={mnogoPravil} otlozheno={{}} naKomandu={vi.fn()}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.change(screen.getByLabelText("Поиск среди правил приложений"),{target:{value:"games"}});
  expect(screen.getByRole("button",{name:"Удалить правило steam.exe"})).toBeInTheDocument();
  expect(screen.queryByRole("button",{name:"Удалить правило zoom.exe"})).toBeNull();
  expect(screen.getByText("Показано 2 из 3")).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Поиск среди правил приложений"),{target:{value:""}});
  fireEvent.click(screen.getByRole("combobox",{name:"Показывать маршруты"}));
  fireEvent.click(screen.getByRole("option",{name:"Только напрямую"}));
  expect(screen.queryByRole("button",{name:"Удалить правило steam.exe"})).toBeNull();
  expect(screen.getByText("Показано 2 из 3")).toBeInTheDocument();
});

it("групповая смена маршрута трогает только отмеченные и уходит одним применением", async () => {
  const send = vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={mnogoPravil} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.click(screen.getByRole("checkbox",{name:"Отметить steam.exe"}));
  fireEvent.click(screen.getByRole("checkbox",{name:"Отметить zoom.exe"}));
  expect(screen.getByTestId("gruppovye-deystviya")).toHaveTextContent("Отмечено 2 из 3");
  fireEvent.click(screen.getByRole("button",{name:"Напрямую"}));
  await primenit();
  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({
    trafik: expect.objectContaining({prilozheniya:[
      {put:"C:\\Games\\steam.exe",imya:"steam.exe",potomki:true,marshrut:"direct"},
      {put:"C:\\Games\\launcher.exe",imya:"launcher.exe",potomki:true,marshrut:"direct"},
      {put:"C:\\Work\\zoom.exe",imya:"zoom.exe",potomki:false,marshrut:"direct"},
    ]}),
  }));
});

it("скрытое фильтром не попадает под групповое удаление", async () => {
  const send = vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={mnogoPravil} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  // Отмечено всё, затем список сужен поиском: групповое действие применяется
  // к тому, что человек видит, а не к прошлой отметке.
  fireEvent.click(screen.getByRole("checkbox",{name:"Отметить все показанные правила"}));
  fireEvent.change(screen.getByLabelText("Поиск среди правил приложений"),{target:{value:"zoom"}});
  expect(screen.getByTestId("gruppovye-deystviya")).toHaveTextContent("Отмечено 1 из 1");
  fireEvent.click(screen.getByRole("button",{name:"Удалить отмеченные"}));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({
    trafik: expect.objectContaining({prilozheniya:[
      {put:"C:\\Games\\steam.exe",imya:"steam.exe",potomki:true,marshrut:"vpn"},
      {put:"C:\\Games\\launcher.exe",imya:"launcher.exe",potomki:true,marshrut:"direct"},
    ]}),
  }));
});

it("отметка всех показанных берёт только показанное, а смена вкладки её снимает", () => {
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={mnogoPravil} otlozheno={{}} naKomandu={vi.fn()}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.change(screen.getByLabelText("Поиск среди правил приложений"),{target:{value:"games"}});
  fireEvent.click(screen.getByRole("checkbox",{name:"Отметить все показанные правила"}));
  expect(screen.getByTestId("gruppovye-deystviya")).toHaveTextContent("Отмечено 2 из 2");
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  expect(screen.queryByTestId("gruppovye-deystviya")).toBeNull();
  expect(screen.getByLabelText("Поиск среди правил приложений")).toHaveValue("");
});

// C4. Поле сайта принимало одно готовое имя: адрес из браузера служба
// отвергала целиком, кириллица не принималась, список добавлялся по одному.
function otkrytFormuSaytov(props: Partial<Parameters<typeof Pravila>[0]> = {}) {
  const send = vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send} {...props}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  return send;
}

it("адрес из браузера и кириллица доходят до правила в том виде, что примет служба", async () => {
  const send = otkrytFormuSaytov();
  fireEvent.change(screen.getByLabelText("Домен сайта"),{target:{value:"https://Пример.РФ/страница?a=1"}});
  const razbor = screen.getByTestId("razbor-domenov");
  expect(razbor).toHaveTextContent("xn--e1afmkfd.xn--p1ai");
  expect(razbor).toHaveTextContent("набрано https://Пример.РФ/страница?a=1");
  fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({
    trafik: expect.objectContaining({domeny:[
      {domen:"work.example",marshrut:"direct"},
      {domen:"xn--e1afmkfd.xn--p1ai",marshrut:"vpn"},
    ]}),
  }));
});

it("список сайтов за один ввод уходит одной правкой черновика", async () => {
  const send = otkrytFormuSaytov();
  fireEvent.change(screen.getByLabelText("Домен сайта"),{
    target:{value:"news.example.org, https://video.example.org/watch\nnews.example.org work.example"},
  });
  expect(screen.getByTestId("razbor-domenov")).toHaveTextContent("Добавится 3 правила");
  // Повтор внутри ввода схлопнут, а совпадение с уже сохранённым правилом
  // названо заменой, а не тихо добавлено второй строкой.
  expect(screen.getByTestId("razbor-domenov")).toHaveTextContent("заменит прежний маршрут");
  fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  await primenit();
  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({
    trafik: expect.objectContaining({domeny:[
      {domen:"news.example.org",marshrut:"vpn"},
      {domen:"video.example.org",marshrut:"vpn"},
      {domen:"work.example",marshrut:"vpn"},
    ]}),
  }));
});

it("адрес и мусор объясняются построчно и не дают добавить только себя", () => {
  otkrytFormuSaytov();
  const pole = screen.getByLabelText("Домен сайта");
  fireEvent.change(pole,{target:{value:"192.168.1.10 example"}});
  const razbor = screen.getByTestId("razbor-domenov");
  expect(razbor).toHaveTextContent("192.168.1.10: это адрес, а не имя сайта");
  expect(razbor).toHaveTextContent("example: нужно полное имя");
  expect(screen.getByRole("button",{name:"Добавить в черновик"})).toBeDisabled();
  // Годная строка рядом с негодной не блокирует добавление годной.
  fireEvent.change(pole,{target:{value:"192.168.1.10 shop.example.org"}});
  expect(screen.getByRole("button",{name:"Добавить в черновик"})).toBeEnabled();
  expect(screen.getByTestId("razbor-domenov")).toHaveTextContent("Добавится 1 правило");
});

// C9. Клавиатура, подписи действий и возврат фокуса.
const prilozheniya: PravilaOtvet = {
  ...rules,
  trafik: {
    ...rules.trafik!,
    prilozheniya: [
      {put:"C:\\Program Files\\Mozilla Firefox\\firefox.exe",imya:"firefox.exe",potomki:true,marshrut:"direct"},
      {put:"C:\\Games\\steam.exe",imya:"steam.exe",potomki:false,marshrut:"vpn"},
    ],
  },
};

it("стрелки ходят по вкладкам правил, Tab доносит только до выбранной", () => {
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={vi.fn()}/>);
  const vkladki = screen.getAllByRole("tab");
  expect(vkladki.map(v=>v.tabIndex)).toEqual([0,-1,-1]);
  fireEvent.keyDown(vkladki[0],{key:"ArrowRight"});
  expect(screen.getByRole("tab",{selected:true}).textContent).toMatch(/Приложения/);
  fireEvent.keyDown(screen.getByRole("tab",{selected:true}),{key:"End"});
  expect(screen.getByRole("tab",{selected:true}).textContent).toMatch(/Сайты/);
  fireEvent.keyDown(screen.getByRole("tab",{selected:true}),{key:"ArrowRight"});
  expect(screen.getByRole("tab",{selected:true}).textContent).toMatch(/Сервисы/);
});

it("служба с рекламой даёт четвёртую вкладку: End ведёт на неё, точка и счёт видны снаружи", () => {
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={{...rules,reklama:{vkl:true,uroven:"light",razresheno:["mc.yandex.ru","ad.example"]}}} otlozheno={{}} naKomandu={vi.fn()}/>);
  const vkladki = screen.getAllByRole("tab");
  expect(vkladki.map(v=>v.tabIndex)).toEqual([0,-1,-1,-1]);
  fireEvent.keyDown(vkladki[0],{key:"End"});
  expect(screen.getByRole("tab",{selected:true}).textContent).toMatch(/^Реклама/);
  expect(screen.getByTestId("vklyucheno-reklama")).toHaveAttribute("aria-label","Реклама и трекеры блокируются");
  expect(screen.getByRole("tab",{name:/^Реклама/}).textContent).toContain("2");
});

it("выключенная реклама не ставит точку на вкладке", () => {
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={{...rules,reklama:{vkl:false,uroven:"light",razresheno:[]}}} otlozheno={{}} naKomandu={vi.fn()}/>);
  expect(screen.getByRole("tab",{name:/^Реклама/})).toBeInTheDocument();
  expect(screen.queryByTestId("vklyucheno-reklama")).toBeNull();
});

it("каждая строка называет своё приложение в подписи флажка и удаления", () => {
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={prilozheniya} otlozheno={{}} naKomandu={vi.fn()}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  expect(screen.getByRole("checkbox",{name:"И программы, запущенные firefox.exe"})).toBeChecked();
  expect(screen.getByRole("checkbox",{name:"И программы, запущенные steam.exe"})).not.toBeChecked();
  const udalit = screen.getByRole("button",{name:"Удалить правило firefox.exe"});
  fireEvent.click(udalit);
  expect(screen.getByRole("button",{name:"Подтвердить удаление firefox.exe"})).toBeInTheDocument();
  // Вторая строка при этом остаётся обычной: подтверждение принадлежит одной.
  expect(screen.getByRole("button",{name:"Удалить правило steam.exe"})).toBeInTheDocument();
});

it("удалённая строка отдаёт фокус кнопке добавления, а не пустоте", () => {
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={prilozheniya} otlozheno={{}} naKomandu={vi.fn()}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.click(screen.getByRole("button",{name:"Удалить правило steam.exe"}));
  fireEvent.click(screen.getByRole("button",{name:"Подтвердить удаление steam.exe"}));
  expect(screen.queryByRole("button",{name:/steam\.exe/})).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button",{name:"Добавить"}));
});

it("открытая форма ставит курсор в первое поле", () => {
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={vi.fn()}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  expect(document.activeElement).toBe(screen.getByLabelText("Домен сайта"));
});

it("Enter в поле домена добавляет правило в черновик", async () => {
  const send = vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  const pole = screen.getByLabelText("Домен сайта");
  fireEvent.change(pole,{target:{value:"news.example"}});
  fireEvent.submit(pole);
  // Форма закрылась, а фокус вернулся на кнопку, с которой её открывали.
  expect(screen.queryByLabelText("Домен сайта")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button",{name:"Добавить"}));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({
    trafik: expect.objectContaining({domeny:[{domen:"work.example",marshrut:"direct"},{domen:"news.example",marshrut:"vpn"}]}),
  }));
});

it("Enter в поиске приложения не заводит правило с пустым путём", () => {
  const send = vi.fn();
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}
    zapushchennye={[{imya:"Steam",put:"C:\\Games\\steam.exe"}]} obnovitProtsessy={vi.fn()}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  const poisk = screen.getByLabelText("Поиск приложения");
  fireEvent.change(poisk,{target:{value:"steam"}});
  fireEvent.submit(poisk);
  // Форма осталась открытой, черновика нет: путь ещё не выбран.
  expect(screen.getByLabelText("Путь к приложению")).toBeInTheDocument();
  expect(screen.queryByRole("button",{name:"Применить изменения"})).toBeNull();
  expect(send).not.toHaveBeenCalled();
});
