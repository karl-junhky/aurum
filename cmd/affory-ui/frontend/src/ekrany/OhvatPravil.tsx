import { useEffect, useRef, useState } from "react";
import { domenPopadaet, normalizovatProbuDomena, type ProveritSoedineniya, type SnimokSoedineniy, type ProveritPrilozhenie } from "../ohvat";
import { obyasnitMarshrut, opisatPraviloSoedineniya, type ReshenieProby } from "../proba-marshruta";
import { imyaMarshruta, type KatalogServisov, type PravilaTrafika } from "../trafik";
import { Flazhok, Knopka, Pole, Svorachivaemyy } from "./ui";
import { ProverkaPrilozheniya } from "./ProverkaPrilozheniya";
import { tekstOshibki } from "../ponyatno";

export function OhvatPravil({vid,trafik,katalog,chernovik,ozhidayut=false,bezRu=false,killSwitch=false,reklamaVkl=false,disabled,naSbros,proverit,proveritPrilozhenie,vybratFayl,zamenit}:{
  vid:"apps"|"sites"; trafik:PravilaTrafika; katalog?:KatalogServisov; chernovik:boolean;
  ozhidayut?:boolean; bezRu?:boolean; killSwitch?:boolean; reklamaVkl?:boolean; disabled:boolean; naSbros:()=>void; proverit?:ProveritSoedineniya;
  proveritPrilozhenie?:ProveritPrilozhenie;vybratFayl?:()=>Promise<string>;zamenit?:(oldPath:string,newPath:string)=>boolean;
}) {
  const [open,setOpen]=useState(false), [confirm,setConfirm]=useState(false);
  const [input,setInput]=useState(""), [busy,setBusy]=useState(false), [error,setError]=useState("");
  const [extra,setExtra]=useState(""),[proksi,setProksi]=useState(false);
  const [result,setResult]=useState<SnimokSoedineniy|null>(null);
  const epoch=useRef(0);
  useEffect(()=>()=>{epoch.current++;},[]);
  const sites=vid==="sites";
  const domainInput=sites?input:extra;
  const domen=normalizovatProbuDomena(domainInput);
  const put=(sites?extra:input).trim();
  const preview=obyasnitMarshrut({domen,put,proksi,bezRu,killSwitch,reklamaVkl,trafik,katalog});
  const settingsSnapshot=JSON.stringify({trafik,katalog,bezRu,killSwitch,reklamaVkl,chernovik,ozhidayut});
  useEffect(()=>{epoch.current++;setResult(null);setError("");setBusy(false);},[settingsSnapshot,disabled]);
  const count=sites?trafik.domeny.length:trafik.prilozheniya.length;
  const changeInput=(value:string)=>{epoch.current++;setInput(value);setResult(null);setError("");setBusy(false);};
  const changeExtra=(value:string)=>{epoch.current++;setExtra(value);setResult(null);setError("");setBusy(false);};
  const inspect=async(value=input)=>{
    if (!proverit || busy || disabled) return;
    const rawDomain=sites?value:extra;
    const host=normalizovatProbuDomena(rawDomain), path=(sites?extra:value).trim();
    if (rawDomain.trim() && !host) {setError("Укажи домен или ссылку HTTP/HTTPS");return;}
    const ticket=++epoch.current;
    setBusy(true);setError("");setResult(null);
    try {const snapshot=await proverit({...host?{domen:host}:{},...path?{put:path}:{}});if(ticket===epoch.current)setResult(snapshot);}
    catch(e:unknown){if(ticket===epoch.current)setError(tekstOshibki(e));}
    finally{if(ticket===epoch.current)setBusy(false);}
  };
  return <section aria-label="Охват и проверка правил" className="border-border flex flex-col gap-3 border-t pt-4 text-[13px]">
    <div className="flex flex-wrap gap-2">
      <Knopka rang="vtoraya" aria-expanded={open} onClick={()=>setOpen(!open)}>{open?"Скрыть охват правил":"Показать охват правил"}</Knopka>
      <Knopka rang={confirm?"opasnaya":"vtoraya"} aktiven={!disabled && count>0} onClick={()=>{if(confirm){naSbros();setConfirm(false);}else setConfirm(true);}}>{confirm?"Подтвердить сброс в черновике":sites?"Сбросить правила сайтов":"Сбросить правила приложений"}</Knopka>
      {confirm && <Knopka rang="tekst" onClick={()=>setConfirm(false)}>Не сбрасывать</Knopka>}
    </div>
    {open && <div className="bg-surface border-border flex flex-col gap-3 rounded-xl border p-4">
      <p className="text-fg-secondary">{chernovik?"Охват черновика, ещё не применён":"Охват сохранённых правил"}</p>
      {!count && <p className="text-fg-muted">Отдельных правил {sites?"сайтов":"приложений"} нет</p>}
      <ul className="flex max-h-72 flex-col gap-3 overflow-y-auto">
        {sites?trafik.domeny.map(d=><li key={d.domen} className="flex flex-col gap-1">
          <div className="flex flex-wrap justify-between gap-2"><span className="text-foreground break-all">{d.domen}</span><span>{imyaMarshruta(d.marshrut)}</span></div>
          <span className="text-fg-muted">Сам домен и все поддомены</span>
          {trafik.domeny.filter(child=>child.domen!==d.domen && domenPopadaet(child.domen,d.domen)).map(child=><span key={child.domen} className="text-fg-secondary break-all">Более точное правило: {child.domen}, {imyaMarshruta(child.marshrut).toLowerCase()}</span>)}
        </li>):trafik.prilozheniya.map(a=><li key={a.put} className="flex flex-col gap-1">
          <div className="flex flex-wrap justify-between gap-2"><span className="text-foreground">{a.imya}</span><span>{imyaMarshruta(a.marshrut)}</span></div>
          <span className="text-fg-muted break-all">{a.put}</span>
          <span>{a.potomki?"Приложение и программы, которые оно запускает, если Affory успел заметить их запуск":"Только это приложение"}</span>
        </li>)}
      </ul>
      <p className="text-fg-muted">{sites?"Сначала действует правило приложения, затем сайта, затем сервиса. Например, отдельное правило для api.example.com важнее общего правила для example.com.":"Отдельное правило приложения действует первым. Например, если Steam открыл лаунчер, а тот запустил игру, сначала учитывается правило игры, затем лаунчера, затем Steam. Если запуск не удалось заметить, Affory не угадывает, какие программы связаны."}</p>
      <p className="text-fg-muted">Это настройки маршрута, а не результат сетевой проверки.</p>
    </div>}
    <Svorachivaemyy zagolovok="Проверка правил" deti={<div className="flex flex-col gap-4">
    {!sites && <ProverkaPrilozheniya rules={trafik.prilozheniya} draft={chernovik} disabled={disabled} connectionBusy={busy} proverit={proveritPrilozhenie} vybratFayl={vybratFayl} zamenit={zamenit} soedineniya={path=>{changeInput(path);void inspect(path);}}/>}
    <form className="flex flex-col gap-3" onSubmit={e=>{e.preventDefault();void inspect();}}>
      <p className="text-foreground font-medium">Проверка по соединениям</p>
      <p className="text-fg-muted">Укажи приложение и сайт вместе или заполни только одно поле. Открой сайт в этом приложении и нажми «Проверить соединения». Оба поля пусты: все текущие соединения.</p>
      <div className="flex flex-wrap gap-2">
        <Pole aria-label={sites?"Домен для проверки":"Путь приложения для проверки"} placeholder={sites?"api.example.org или ссылка":"Полный путь к файлу приложения (.exe)"} znachenie={input} naVvod={changeInput} className="min-w-[180px] flex-1"/>
        <Pole aria-label={sites?"Путь приложения для проверки":"Домен для проверки"} placeholder={sites?"Уточнить приложение: полный путь .exe":"Уточнить сайт: домен или ссылка"} znachenie={extra} naVvod={changeExtra} className="min-w-[180px] flex-1"/>
        <Knopka rang="vtoraya" type="submit" aktiven={!disabled && !busy && !!proverit} zhdyot={busy}>Проверить соединения</Knopka>
      </div>
      {(put || domen) && <div className="bg-surface border-border flex flex-col gap-3 rounded-xl border p-3" aria-label="Расчёт маршрута по настройкам">
        <p className="text-foreground font-medium">По настройкам{chernovik?" черновика":""}</p>
        {domainInput.trim() && !domen?<p className="text-warn">Домен не распознан. Исправь его для совместного расчёта.</p>:<>
          <Reshenie label="Соединение" result={preview.dannye}/>
          <Reshenie label="Поиск адреса сайта" result={preview.dns}/>
          {!proksi && <p className="text-fg-muted">Для обычного поиска адреса, который идёт через Affory.</p>}
        </>}
        <details className="text-fg-muted"><summary>Условия расчёта</summary>
          <div className="mt-2 flex flex-col gap-2">
            <Flazhok podpis="Показать расчёт для локального прокси Affory" vkl={proksi} naSmenu={setProksi}/>
            <p>Обычно приложения используют VPN без настройки прокси. Выбери прокси только если указал его адрес вручную в приложении. Этот выбор меняет расчёт, а снимок ниже показывает соединения обоих способов.</p>
            {preview.primechaniya.map(note=><p key={note}>{note}</p>)}
          </div>
        </details>
        <p className="text-fg-muted">Это расчёт по правилам, а не проверка, что сайт открывается.</p>
      </div>}
      {chernovik && <p className="text-warn">Подсказка учитывает черновик. Проверка показывает текущие соединения, ещё без этих изменений.</p>}
      {!chernovik && ozhidayut && <p className="text-warn">Сохранённые правила ещё не применены. Проверка показывает соединения с прежними правилами.</p>}
      {error && <p role="alert" className="text-danger">{error}</p>}
      {busy && <p role="status">Проверяю текущие соединения</p>}
      {result && <div className="flex flex-col gap-2" role="status">
        <p>Соединения на {new Date(result.vremya).toLocaleTimeString("ru-RU")}</p>
        {!result.yadro?<p>Подключение не запущено. Подключи VPN и повтори проверку.</p>:!result.soedineniya.length?<p>Подходящих открытых соединений не обнаружено. Это не означает, что правило не работает: короткие соединения могли уже закрыться, а адрес сайта может быть не виден Affory.</p>:<ul className="border-border max-h-80 divide-y overflow-y-auto rounded-lg border">
          {result.soedineniya.map((c,index)=><li key={`${c.Id}-${index}`} className="flex flex-col gap-1 p-3">
            <span className="text-foreground break-all">{c.Host || c.Adres}:{c.Port}</span>
            <span className="break-all">{c.Protsess || "Не удалось определить приложение"}</span>
            <span>Куда идёт соединение: {c.Vyhod==="direct"?"напрямую":c.Vyhod.startsWith("srv-")?"через VPN":"не удалось определить"}</span>
            <span className="text-fg-secondary">{opisatPraviloSoedineniya(c.Pravilo)}</span>
            <details className="text-fg-muted break-all"><summary>Технические сведения</summary><p>Выход: {c.Vyhod || "неизвестен"}</p><p>Правило: {c.Pravilo || "не сообщено"}</p></details>
          </li>)}
        </ul>}
        {result.ogranichen && <p>Показаны первые 200 совпадений. Уточни фильтр.</p>}
      </div>}
      <p className="text-fg-muted">{sites?"Проверка example.com включает соединения с api.example.com и другими его поддоменами. Она не перебирает все возможные адреса.":"По указанному пути видны соединения всех запусков этого файла, в том числе открытых отдельно. Для другой программы нажми «Соединения» в проверке охвата выше."} Здесь видно, куда направлено соединение, но это ещё не означает, что сайт ответил. Запись журнала не включается.</p>
    </form>
    </div>}/>
  </section>;
}

function Reshenie({label,result}:{label:string;result:ReshenieProby}) {
  return <div><p className="text-foreground">{label}: {result.marshrut?imyaMarshruta(result.marshrut):"нужны дополнительные сведения"}</p><p className="text-fg-secondary">{result.prichina}</p></div>;
}
