import { domenPopadaet } from "./ohvat";
import { programmyTrafika, type KatalogServisov, type Marshrut, type PravilaTrafika } from "./trafik";

export interface ReshenieProby { marshrut: Marshrut | null; prichina: string }
export interface ProbaMarshruta {
  dannye: ReshenieProby;
  dns: ReshenieProby;
  primechaniya: string[];
}

// Only recognise the simple descriptions emitted by our pinned core. An
// unfamiliar or compound rule remains technical data rather than a guess.
export function opisatPraviloSoedineniya(rule:string):string {
  const condition=rule.split(" => ")[0];
  if(condition==="final") return "Сработал общий маршрут";
  if(condition.startsWith("process_path_tree=")) return "Сработало правило программы, которая запустила приложение";
  if(condition.startsWith("process_path=")) return "Сработало отдельное правило приложения";
  if(condition.startsWith("domain_suffix=") || condition.startsWith("domain=")) return "Сработало правило сайта или сервиса";
  if(condition==="inbound=proksi-in") return "Соединение пришло через локальный прокси Affory";
  if(condition.startsWith("rule_set=")) return "Сработал готовый набор сайтов";
  if(condition==="ip_is_private=true" || condition==="ip_is_private") return "Сработало исключение для локального IP-адреса";
  return "Причина выбора видна в технических сведениях ниже";
}

// This is a settings preview, not a network probe. Unknown launch history and
// rule-set membership must not turn into an invented deterministic route.
export function obyasnitMarshrut({domen,put,proksi,bezRu,killSwitch,reklamaVkl=false,trafik,katalog}:{
  domen:string;put:string;proksi:boolean;bezRu:boolean;killSwitch:boolean;reklamaVkl?:boolean;
  trafik:PravilaTrafika;katalog?:KatalogServisov;
}):ProbaMarshruta {
  const own=[...trafik.domeny].sort((a,b)=>b.domen.split(".").length-a.domen.split(".").length).find(d=>domenPopadaet(domen,d.domen));
  const service=trafik.servisy.find(s=>katalog?.servisy.find(k=>k.id===s.id)?.domeny.some(d=>domenPopadaet(domen,d)));
  const domainRule:ReshenieProby|null=own?{marshrut:own.marshrut,prichina:`Правило сайта ${own.domen}, включая поддомены`}:service?{
    marshrut:service.marshrut,prichina:`Правило сервиса ${katalog?.servisy.find(k=>k.id===service.id)?.imya ?? service.id}`,
  }:null;
  // Программы включённых сервисов считаются наравне со своими правилами: с D2
  // карточка сервиса накрывает и клиента, и проба, не знающая об этом, говорила
  // бы «правила нет» при работающем правиле.
  const programmy=programmyTrafika(trafik);
  const app=programmy.find(a=>a.put.toLowerCase()===put.toLowerCase());
  const hasLaunchRules=programmy.some(a=>a.potomki);
  const missingCatalog=!katalog && trafik.servisy.length>0;
  const ru=!bezRu && !killSwitch;
  const primechaniya=["Расчёт для обычного веб-соединения. Служебные адреса Affory и VPN-серверов обходят пользовательские правила."];
  // Блок рекламы стоит в ядре выше всех маршрутов, а списка на руках у окна
  // нет: проба честно говорит, чего она не проверяет.
  if(reklamaVkl) primechaniya.push("Если сайт есть в списке рекламы, он заблокирован раньше любого маршрута; здесь это не проверяется.");
  let dannye:ReshenieProby;
  if(proksi) dannye={marshrut:"vpn",prichina:"Приложение явно использует локальный прокси Affory. Этот выбор важнее правил приложения и сайта"};
  else if(app) dannye={marshrut:app.marshrut,prichina:`Отдельное правило приложения ${app.imya || app.put}. Оно важнее правила сайта и сервиса`};
  else if(!put && programmy.length>0) dannye={marshrut:null,prichina:"Укажи приложение: его правило может изменить маршрут сайта"};
  else if(put && hasLaunchRules) dannye={marshrut:null,prichina:"Нужно знать, какая программа запустила это приложение. По одному пути нельзя выбрать правило; проверь работающие программы во вкладке «Приложения»"};
  else if(!domen) dannye={marshrut:null,prichina:"Укажи сайт: его правило или набор сервиса может изменить общий маршрут"};
  else if(domainRule) dannye=domainRule;
  else if(missingCatalog) dannye={marshrut:null,prichina:"Каталог сервисов недоступен. Нельзя проверить, подходит ли правило сервиса"};
  else if(ru && trafik.po_umolchaniyu==="vpn") dannye={marshrut:null,prichina:"Для российского списка напрямую, для остальных сайтов через VPN. Вхождение в список здесь не проверяется"};
  else dannye={marshrut:trafik.po_umolchaniyu,prichina:"Общий режим: подходящего правила приложения, сайта или сервиса нет"};
  if(!proksi && !app && !domainRule && !killSwitch) primechaniya.push("Локальные IP-адреса идут напрямую, если их не перекрывает явное правило. IP-адрес сайта здесь не запрашивается.");
  if(domainRule && (app || proksi || dannye.marshrut===null)) primechaniya.push(`По правилам сайтов: ${domainRule.prichina.toLowerCase()} (${domainRule.marshrut==="vpn"?"через VPN":"напрямую"}). Правило приложения или прокси может оказаться важнее.`);

  // DNS requests shared by Windows cannot reliably be attributed to one app.
  // Local names precede user domains, then rule sets, then the DNS default.
  const dnsDefault=programmy.some(a=>a.marshrut==="vpn")?"vpn":trafik.po_umolchaniyu;
  let dns:ReshenieProby;
  if(proksi) dns={marshrut:null,prichina:"При передаче имени через прокси адрес может искать VPN-сервер. По этому расчёту нельзя установить, искало ли приложение адрес само"};
  else if(!domen) dns={marshrut:null,prichina:"Укажи сайт, чтобы проверить, где ищется его адрес"};
  else if(!domen.includes(".") || [".local",".lan",".home.arpa"].some(s=>domen.endsWith(s))) dns={marshrut:"direct",prichina:"Локальное имя: адрес ищется в текущей сети, раньше правил сайта"};
  else if(killSwitch) dns={marshrut:"vpn",prichina:"Включена защита сети: адреса всех сайтов ищутся через VPN"};
  else if(domainRule) dns={...domainRule,prichina:`${domainRule.prichina}. Для поиска адреса правило приложения не используется`};
  else if(missingCatalog) dns={marshrut:null,prichina:"Каталог сервисов недоступен. Нельзя проверить, где ищутся адреса сайтов сервисов"};
  else if(ru && dnsDefault==="vpn") dns={marshrut:null,prichina:"Адреса сайтов из российского списка ищутся в текущей сети, остальных через VPN. Вхождение в список здесь не проверяется"};
  else dns={marshrut:dnsDefault,prichina:programmy.some(a=>a.marshrut==="vpn")?"Адреса ищутся через VPN: хотя бы одно приложение направлено в VPN":"Адрес ищется по общему режиму"};
  primechaniya.push("Расчёт поиска адреса относится к обычным запросам, которые идут через Affory. Безопасный DNS самого браузера и поиск адреса на VPN-сервере он не проверяет.");
  primechaniya.push("Правило сайта сработает, только если Affory видит его имя. Если имя скрыто, правило приложения надёжнее.");
  // Под защитой прямых маршрутов не бывает: служба собирает конфиг без них
  // (`bezPryamyh` в internal/genkonfig/trafik.go), и правило человека просто
  // спит до выключения защиты. До 23.09.2026 здесь стоял отказ «настройки
  // нельзя применить» - он остался от времени, когда защита и прямое правило
  // и вправду не сходились, и после снятия того запрета проба врала.
  if(killSwitch && dannye.marshrut!=="vpn"){
    dannye={marshrut:"vpn",prichina:"Защита сети включена: прямые маршруты сейчас не действуют, весь трафик идёт через VPN. Они заработают снова, когда выключишь защиту"};
  }
  return {dannye,dns,primechaniya};
}
