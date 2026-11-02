import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ProverkaPrilozheniya } from "./ProverkaPrilozheniya";
import { razobratOhvat, type OhvatPrilozheniya } from "../ohvat";
import type { PraviloPrilozheniya } from "../trafik";

afterEach(cleanup);
const root:PraviloPrilozheniya={put:"C:\\Launcher.exe",imya:"Launcher",potomki:true,marshrut:"direct"};
const result:OhvatPrilozheniya={pravilo:root,reviziya_pravil:"v1",trebuet_podyoma:false,vremya:"2026-09-22T12:00:00Z",fayl:"net",samo:0,vsego:1,
  zapushchennye:[{pid:123,created:"134000000000000001",put:"C:\\Game.exe",imya:"Game",cherez:[root.put,"C:\\Game.exe"],marshrut:"vpn",pravilo_put:"C:\\Game.exe",pravilo_imya:"Game",pereopredelen:true}],
  neizvestno:1,neizvestnye:["C:\\Unknown.exe"],ogranichen:false};
const base={rules:[root],draft:false,disabled:false,soedineniya:vi.fn()};
const check=()=>act(async()=>fireEvent.click(screen.getByRole("button",{name:"Проверить охват"})));

it("показывает работающую игру после закрытия лаунчера и более точное правило",async()=>{
  const connections=vi.fn(),inspect=vi.fn().mockResolvedValue(result);
  render(<ProverkaPrilozheniya {...base} proverit={inspect} soedineniya={connections}/>);
  await check();
  expect(inspect).toHaveBeenCalledWith(root.put);
  expect(screen.getByRole("status")).toHaveTextContent("Файл не найден");
  expect(screen.getByRole("status")).toHaveTextContent("Приложение здесь не запущено, но запущенные им программы работают");
  expect(screen.getByText("Launcher.exe → Game.exe")).toBeInTheDocument();
  expect(screen.getByText(/Действует другое, более точное правило: Game/)).toBeInTheDocument();
  expect(screen.getByText(/не удалось полностью установить/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button",{name:"Проверить соединения Game"}));
  expect(connections).toHaveBeenCalledWith("C:\\Game.exe");
});

it("файл существует, но пустой список не обозначается как работающая программа",async()=>{
  render(<ProverkaPrilozheniya {...base} proverit={vi.fn().mockResolvedValue({...result,fayl:"est",vsego:0,zapushchennye:[],neizvestno:0})}/>);
  await check();
  expect(screen.getByRole("status")).toHaveTextContent("Файл найден");
  expect(screen.getByRole("status")).toHaveTextContent("В этом сеансе Windows приложение не запущено, и запущенных им программ нет");
});

it("ошибка не превращается в пустой список, черновик нельзя проверить как сохранённый",async()=>{
  const inspect=vi.fn().mockRejectedValue(new Error("Наблюдение недоступно"));
  const view=render(<ProverkaPrilozheniya {...base} proverit={inspect}/>);
  await check();
  expect(screen.getByRole("alert")).toHaveTextContent("Наблюдение недоступно");
  expect(screen.queryByText(/В этом сеансе Windows приложение не запущено, и запущенных им программ нет/)).toBeNull();
  view.rerender(<ProverkaPrilozheniya {...base} draft proverit={inspect}/>);
  expect(screen.getByRole("button",{name:"Проверить охват"})).toBeDisabled();
  expect(screen.queryByRole("alert")).toBeNull();
});

it("поздний ответ не относится к другому выбранному приложению",async()=>{
  let finish:(r:OhvatPrilozheniya)=>void=()=>{throw new Error("not requested");};
  const inspect=vi.fn(()=>new Promise<OhvatPrilozheniya>(resolve=>{finish=resolve;}));
  render(<ProverkaPrilozheniya {...base} rules={[root,{...root,put:"C:\\Other.exe",imya:"Other"}]} proverit={inspect}/>);
  fireEvent.click(screen.getByRole("button",{name:"Проверить охват"}));
  expect(screen.getByRole("status")).toHaveTextContent("Проверяю");
  fireEvent.click(screen.getByRole("combobox",{name:"Приложение для проверки охвата"}));
  fireEvent.click(screen.getByRole("option",{name:"Other"}));
  await act(async()=>finish(result));
  expect(screen.queryByText(/Файл не найден/)).toBeNull();
});

it("отмена выбора файла не меняет правило, отключение экрана отменяет поздний выбор",async()=>{
  const replace=vi.fn().mockReturnValue(true);
  const view=render(<ProverkaPrilozheniya {...base} vybratFayl={vi.fn().mockResolvedValue("")} zamenit={replace}/>);
  await act(async()=>fireEvent.click(screen.getByRole("button",{name:"Заменить файл"})));
  expect(replace).not.toHaveBeenCalled();
  let finish:(path:string)=>void=()=>{throw new Error("not requested");};
  const choose=()=>new Promise<string>(resolve=>{finish=resolve;});
  view.rerender(<ProverkaPrilozheniya {...base} vybratFayl={choose} zamenit={replace}/>);
  fireEvent.click(screen.getByRole("button",{name:"Заменить файл"}));
  view.rerender(<ProverkaPrilozheniya {...base} disabled vybratFayl={choose} zamenit={replace}/>);
  await act(async()=>finish("C:\\New.exe"));
  expect(replace).not.toHaveBeenCalled();
});

it("большой список и ожидающие применения правила помечены явно",async()=>{
  const rows=Array.from({length:200},(_,i)=>({...result.zapushchennye[0],pid:i+1}));
  render(<ProverkaPrilozheniya {...base} proverit={vi.fn().mockResolvedValue({...result,ogranichen:true,trebuet_podyoma:true,vsego:250,zapushchennye:rows})}/>);
  await check();
  expect(screen.getByText(/Показана часть списка/)).toHaveTextContent("250");
  expect(screen.getByText(/ещё не применены к подключению/)).toBeInTheDocument();
  expect(screen.getAllByRole("button",{name:"Проверить соединения Game"})).toHaveLength(200);
});

it("разбор ответа сохраняет точное время создания и отбрасывает неверный контракт",()=>{
  expect(razobratOhvat(result).zapushchennye[0].created).toBe("134000000000000001");
  expect(()=>razobratOhvat({...result,samo:"0"})).toThrow();
  expect(()=>razobratOhvat({...result,fayl:"green"})).toThrow();
});
