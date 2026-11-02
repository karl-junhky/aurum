import { useEffect, useRef, useState } from "react";
import { type OhvatPrilozheniya, type ProveritPrilozhenie } from "../ohvat";
import { imyaMarshruta, type PraviloPrilozheniya } from "../trafik";
import { Knopka } from "./ui";
import { Vybor } from "./Vybor";
import { tekstOshibki } from "../ponyatno";

const fileText={est:"Файл найден",net:"Файл не найден",nedostupen:"Не удалось прочитать сведения о файле",ne_proveren:"Файл не проверен",papka:"Указана папка, а не файл приложения"};
const nameOf=(path:string)=>path.split(/[/\\]/).pop() || path;

export function ProverkaPrilozheniya({rules,draft,disabled,connectionBusy=false,proverit,vybratFayl,zamenit,soedineniya}:{
  rules:PraviloPrilozheniya[];draft:boolean;disabled:boolean;proverit?:ProveritPrilozhenie;
  connectionBusy?:boolean;vybratFayl?:()=>Promise<string>;zamenit?:(oldPath:string,newPath:string)=>boolean;soedineniya:(path:string)=>void;
}){
  const [path,setPath]=useState(rules[0]?.put ?? "");
  const [result,setResult]=useState<OhvatPrilozheniya|null>(null),[error,setError]=useState(""),[notice,setNotice]=useState("");
  const [busy,setBusy]=useState(false),[picking,setPicking]=useState(false);
  const epoch=useRef(0);
  const selected=rules.find(r=>r.put===path);
  const ruleSnapshot=JSON.stringify(rules);
  useEffect(()=>{epoch.current++;setResult(null);setError("");setBusy(false);setPicking(false);if(!draft)setNotice("");},[draft,ruleSnapshot]);
  useEffect(()=>{if(!selected)setPath(rules[0]?.put ?? "");},[selected,rules]);
  useEffect(()=>()=>{epoch.current++;},[]);
  useEffect(()=>{if(disabled){epoch.current++;setBusy(false);setPicking(false);}},[disabled]);
  const choose=(next:string)=>{epoch.current++;setPath(next);setResult(null);setError("");setNotice("");setBusy(false);};
  const check=async()=>{
    if(!proverit || !selected || draft || disabled || busy || picking)return;
    const ticket=++epoch.current;setBusy(true);setError("");setResult(null);setNotice("");
    try{const response=await proverit(path);if(ticket===epoch.current)setResult(response);}
    catch(e:unknown){if(ticket===epoch.current)setError(tekstOshibki(e));}
    finally{if(ticket===epoch.current)setBusy(false);}
  };
  const replace=async()=>{
    if(!vybratFayl || !zamenit || !selected || disabled || busy || picking)return;
    const ticket=++epoch.current;setPicking(true);setError("");
    try{const next=await vybratFayl();if(ticket!==epoch.current || !next)return;if(zamenit(path,next)){setPath(next);setResult(null);setNotice("Новый файл выбран. Примени изменения, чтобы сохранить правило.");}else{setNotice("Этот файл уже выбран.");}}
    catch(e:unknown){if(ticket===epoch.current)setError(tekstOshibki(e));}
    finally{if(ticket===epoch.current)setPicking(false);}
  };
  return <section aria-label="Работающие программы под правилом" className="border-border flex flex-col gap-3 border-t pt-4">
    <h4 className="text-foreground font-medium">Какие программы входят в правило</h4>
    <p className="text-fg-muted">Проверяются работающие программы текущего сеанса Windows.</p>
    {!rules.length?<p className="text-fg-muted">Добавь приложение, чтобы проверить его запуски.</p>:<>
      <div className="flex flex-wrap items-center gap-2">
        <div className="min-w-0 max-w-full flex-1"><Vybor label="Приложение для проверки охвата" value={path} options={rules.map(r=>({value:r.put,label:r.imya || nameOf(r.put)}))} disabled={disabled || picking} onChange={choose}/></div>
        <Knopka rang="vtoraya" aktiven={!disabled && !draft && !busy && !picking && !!proverit} zhdyot={busy} onClick={()=>void check()}>Проверить охват</Knopka>
        <Knopka rang="tekst" aktiven={!disabled && !busy && !picking && !!vybratFayl && !!zamenit} zhdyot={picking} onClick={()=>void replace()}>Заменить файл</Knopka>
      </div>
      <p className="text-fg-muted break-all">{path}</p>
      {draft && <p className="text-warn">Сначала примени изменения: проверка использует сохранённые правила.</p>}
    </>}
    {busy && <p role="status">Проверяю запущенные программы</p>}
    {error && <p role="alert" className="text-danger">{error}</p>}
    {notice && <p role="status">{notice}</p>}
    {result && <div className="flex flex-col gap-3" role="status">
      <div className="flex flex-wrap justify-between gap-2">
        <p className={result.fayl==="est"?"text-fg-secondary":"text-warn"}>{fileText[result.fayl]}</p>
        <p className="text-fg-muted">Проверено в {new Date(result.vremya).toLocaleTimeString("ru-RU")}</p>
      </div>
      <p>{result.samo>0?"Приложение запущено":result.vsego>0?"Приложение здесь не запущено, но запущенные им программы работают":"В этом сеансе Windows приложение не запущено, и запущенных им программ нет"}</p>
      <p className="text-fg-muted">Маршруты ниже рассчитаны по сохранённым правилам. Реальные соединения можно проверить отдельно.</p>
      {result.trebuet_podyoma && <p className="text-warn">Сохранённые правила ещё не применены к подключению.</p>}
      {result.zapushchennye.length>0 && <ul className="border-border max-h-96 divide-y overflow-y-auto rounded-lg border">
        {result.zapushchennye.map(p=><li key={`${p.pid}:${p.created}`} className="flex flex-col gap-1 p-3">
          <div className="flex flex-wrap items-center justify-between gap-2"><span className="text-foreground font-medium">{p.imya}</span><span>{imyaMarshruta(p.marshrut)}</span></div>
          <span className="text-fg-muted break-all">{p.put}</span>
          {p.cherez.length>1 && <span className="text-fg-secondary break-words">{p.cherez.map(nameOf).join(" → ")}</span>}
          <span className={p.pereopredelen?"text-accent-ink":"text-fg-muted"}>{p.pereopredelen?"Действует другое, более точное правило: ":"Правило: "}{p.pravilo_imya || nameOf(p.pravilo_put)}</span>
          <div className="flex items-center justify-between gap-2"><details className="text-fg-faint"><summary>Сведения о запуске</summary><span>PID: {p.pid}</span></details><Knopka rang="tekst" aktiven={!disabled && !busy && !picking && !connectionBusy} onClick={()=>soedineniya(p.put)} aria-label={`Проверить соединения ${p.imya}`}>Соединения</Knopka></div>
        </li>)}
      </ul>}
      {result.ogranichen && <p className="text-fg-muted">Показана часть списка. Найдено запусков под этим правилом: {result.vsego}.</p>}
      {result.neizvestno>0 && <details className="text-fg-muted"><summary>Не все сведения о других запусках доступны ({result.neizvestno})</summary>
        <p className="mt-2">Для этих приложений не удалось полностью установить, какие программы их запускали. Они не включены в подтверждённый список выше. При необходимости добавь само приложение отдельным правилом.</p>
        <ul className="mt-2 flex flex-col gap-1">{result.neizvestnye.map(p=><li key={p} className="break-all">{p}</li>)}</ul>
      </details>}
    </div>}
  </section>;
}
