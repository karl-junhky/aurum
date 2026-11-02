import { imyaMarshruta, type KatalogServisov, type PravilaTrafika, type PraviloPrilozheniya, type PraviloDomena, type Marshrut } from "./trafik";

export interface ZapuskPrilozheniya {
  pid:number; created:string; put:string; imya:string; cherez:string[];
  marshrut:Marshrut; pravilo_put:string; pravilo_imya:string; pereopredelen:boolean;
}
export interface OhvatPrilozheniya {
  pravilo:PraviloPrilozheniya; reviziya_pravil:string; trebuet_podyoma:boolean; vremya:string;
  fayl:"est"|"net"|"nedostupen"|"ne_proveren"|"papka";
  samo:number; vsego:number; zapushchennye:ZapuskPrilozheniya[];
  neizvestno:number; neizvestnye:string[]; ogranichen:boolean;
}
export type ProveritPrilozhenie = (put:string)=>Promise<OhvatPrilozheniya>;

function objectValue(value:unknown):Record<string,unknown>{
  if (!value || typeof value!=="object" || Array.isArray(value)) throw new Error("Некорректные сведения о приложении");
  return value as Record<string,unknown>;
}
function stringValue(value:unknown):string{if(typeof value!=="string")throw new Error("Некорректное текстовое поле проверки");return value;}
function countValue(value:unknown):number{if(typeof value!=="number" || !Number.isSafeInteger(value) || value<0)throw new Error("Некорректный счётчик проверки");return value;}
function boolValue(value:unknown):boolean{if(typeof value!=="boolean")throw new Error("Некорректное состояние проверки");return value;}
function stringsValue(value:unknown):string[]{if(!Array.isArray(value))throw new Error("Некорректный список проверки");return value.map(stringValue);}
function routeValue(value:unknown):Marshrut{if(value!=="vpn" && value!=="direct")throw new Error("Неизвестный маршрут проверки");return value;}
export function razobratOhvat(value:unknown):OhvatPrilozheniya{
  const v=objectValue(value),r=objectValue(v.pravilo);
  const fayl=v.fayl;
  if(fayl!=="est" && fayl!=="net" && fayl!=="nedostupen" && fayl!=="ne_proveren" && fayl!=="papka")throw new Error("Неизвестное состояние файла");
  const vremya=stringValue(v.vremya);
  if(!Number.isFinite(Date.parse(vremya)) || !Array.isArray(v.zapushchennye))throw new Error("Некорректный результат проверки приложения");
  return {pravilo:{put:stringValue(r.put),imya:stringValue(r.imya),potomki:boolValue(r.potomki),marshrut:routeValue(r.marshrut)},
    reviziya_pravil:stringValue(v.reviziya_pravil),trebuet_podyoma:boolValue(v.trebuet_podyoma),vremya,fayl,
    samo:countValue(v.samo),vsego:countValue(v.vsego),neizvestno:countValue(v.neizvestno),neizvestnye:stringsValue(v.neizvestnye),ogranichen:boolValue(v.ogranichen),
    zapushchennye:v.zapushchennye.map((row:unknown)=>{const c=objectValue(row);return {pid:countValue(c.pid),created:stringValue(c.created),put:stringValue(c.put),imya:stringValue(c.imya),cherez:stringsValue(c.cherez),marshrut:routeValue(c.marshrut),pravilo_put:stringValue(c.pravilo_put),pravilo_imya:stringValue(c.pravilo_imya),pereopredelen:boolValue(c.pereopredelen)};})};
}

export interface FiltrSoedineniy { domen?: string; put?: string }
export interface Soedinenie {
  Id: string; Host: string; Adres: string; Port: number;
  Protsess: string; Vyhod: string; Pravilo: string; Nachalo: string;
}
export interface SnimokSoedineniy {
  yadro: boolean; vremya: string; ogranichen: boolean; soedineniya: Soedinenie[];
}
export type ProveritSoedineniya = (filter:FiltrSoedineniy)=>Promise<SnimokSoedineniy>;

export function normalizovatProbuDomena(input:string):string {
  let value=input.trim();
  if (!value) return "";
  try {
    const url=new URL(value.includes("://") ? value : `https://${value}`);
    if (!["https:","http:"].includes(url.protocol) || url.username || url.password) return "";
    value=url.hostname.toLowerCase().replace(/\.$/,"");
    return value.length<=253 && !/^[\d.]+$/.test(value) && value.split(".").every(part=>part.length<=63 && /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(part)) ? value : "";
  } catch { return ""; }
}
export function domenPopadaet(host:string,rule:string):boolean {
  return host===rule || host.endsWith(`.${rule}`);
}

// Probe each service suffix and each explicit subdomain boundary. This avoids
// reporting a broad domain rule which a more specific same-route rule eclipses.
export function perekrytiyaServisa(hosts:string[],domains:PraviloDomena[],route:Marshrut):PraviloDomena[] {
  const sorted=[...domains].sort((a,b)=>b.domen.split(".").length-a.domen.split(".").length);
  const probes=[...hosts,...domains.filter(d=>hosts.some(host=>domenPopadaet(d.domen,host))).map(d=>d.domen)];
  const winners=new Map<string,PraviloDomena>();
  for(const host of probes){const winner=sorted.find(d=>domenPopadaet(host,d.domen));if(winner && winner.marshrut!==route)winners.set(winner.domen,winner);}
  return [...winners.values()];
}
// Только доменная часть намерения. Правила приложения, DNS и системные
// исключения объясняются отдельно: здесь нет сведений о конкретном сокете.
export function opisatDomen(host:string,trafik:PravilaTrafika,katalog?:KatalogServisov):string {
  const own=[...trafik.domeny].sort((a,b)=>b.domen.split(".").length-a.domen.split(".").length).find(r=>domenPopadaet(host,r.domen));
  if (own) return `${imyaMarshruta(own.marshrut)}: правило ${own.domen}, включая поддомены`;
  for (const rule of trafik.servisy) {
    const service=katalog?.servisy.find(s=>s.id===rule.id);
    if (service?.domeny.some(d=>domenPopadaet(host,d))) return `${imyaMarshruta(rule.marshrut)}: сервис ${service.imya}`;
  }
  return `Отдельного правила сайта нет. Общий режим: ${imyaMarshruta(trafik.po_umolchaniyu).toLowerCase()}; российский список и системные исключения могут изменить маршрут`;
}

export function razobratSnimok(value:unknown):SnimokSoedineniy {
  if (!value || typeof value!=="object") throw new Error("Служба не вернула снимок соединений");
  const v=value as Record<string,unknown>;
  if (typeof v.yadro!=="boolean" || typeof v.ogranichen!=="boolean" || typeof v.vremya!=="string" || !Number.isFinite(Date.parse(v.vremya)) || !Array.isArray(v.soedineniya)) throw new Error("Некорректный снимок соединений");
  const soedineniya=v.soedineniya.map((row:unknown)=>{
    if (!row || typeof row!=="object") throw new Error("Некорректная запись соединения");
    const c=row as Record<string,unknown>;
    if (typeof c.Id!=="string" || typeof c.Host!=="string" || typeof c.Adres!=="string" || typeof c.Port!=="number" || typeof c.Protsess!=="string" || typeof c.Vyhod!=="string" || typeof c.Pravilo!=="string" || typeof c.Nachalo!=="string") throw new Error("Некорректная запись соединения");
    return {Id:c.Id,Host:c.Host,Adres:c.Adres,Port:c.Port,Protsess:c.Protsess,Vyhod:c.Vyhod,Pravilo:c.Pravilo,Nachalo:c.Nachalo};
  });
  return {yadro:v.yadro,vremya:v.vremya,ogranichen:v.ogranichen,soedineniya};
}
