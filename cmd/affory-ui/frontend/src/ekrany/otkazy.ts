// Spec §9.1, one entry per refusal code. The action column is a contract:
// internal/kachestvo/otkazy_ui_test.go parses this file (one entry per line,
// `deystvie` first) and compares it with the spec both ways. Text is checked
// by Otkaz.test.tsx: dry, no "we", no trailing dot.

/** Verbatim the vocabulary of internal/kachestvo/deystviya_test.go. */
export type Deystvie =
  | "povtorit"
  | "perepodklyuchitsya"
  | "otkryt-servery"
  | "obnovit-podpisku"
  | "obnovit-programmu"
  | "postavit-sluzhbu"
  | "zaprosit-prava"
  | "proverit-set"
  | "pokazat-vinovnika"
  | "nastroit-vremya"
  | "prinyat-klyuch-hosta"
  | "pereustanovit-os"
  | "vzyat-pod-upravlenie"
  | "otkryt-port-u-hostera"
  | "poprosit-drugoy-adres"
  | "nichego";

export interface ZapisOtkaza {
  deystvie: Deystvie;
  tekst: string;
}

export const tekstOtkaza: Record<string, ZapisOtkaza> = {
  "dns-resolve-failed": { deystvie: "povtorit", tekst: "не удалось найти сервер по имени" },
  "server-auth-failed": { deystvie: "obnovit-podpisku", tekst: "сервер не принял ключ, проверь подписку" },
  "probe-timeout": { deystvie: "povtorit", tekst: "сервер не ответил вовремя, попробуй ещё раз или выбери другой" },
  "server-tls-failed": { deystvie: "otkryt-servery", tekst: "защищённое соединение с сервером не установилось" },
  "subscription-unreachable": { deystvie: "obnovit-podpisku", tekst: "подписка не загрузилась" },
  "subscription-timeout": { deystvie: "povtorit", tekst: "подписка не ответила вовремя" },
  "subscription-auth-failed": { deystvie: "obnovit-podpisku", tekst: "сервер подписки не принял ссылку, проверь её" },
  "subscription-device-limit": { deystvie: "obnovit-podpisku", tekst: "подписка не пускает это устройство" },
  "subscription-malformed": { deystvie: "obnovit-podpisku", tekst: "подписка не подошла" },
  "subscription-expired": { deystvie: "obnovit-podpisku", tekst: "подписка отдала сообщение вместо серверов" },
  "selected-server-gone": { deystvie: "otkryt-servery", tekst: "выбранного сервера больше нет в подписке" },
  "tun-create-failed": { deystvie: "povtorit", tekst: "не удалось создать сетевое подключение VPN" },
  "core-not-responding": { deystvie: "povtorit", tekst: "часть программы, которая ведёт соединение, не отвечает" },
  "clock-skew": { deystvie: "nastroit-vremya", tekst: "часы компьютера сбиты, и серверы не принимают подключение" },
  "core-missing": { deystvie: "nichego", tekst: "файл ядра VPN пропал или изменён, обычно его убирает антивирус: верни его из карантина или переустанови Affory" },
  "wintun-missing": { deystvie: "postavit-sluzhbu", tekst: "драйвер VPN не установился" },
  "no-admin": { deystvie: "postavit-sluzhbu", tekst: "служба не установлена или не отвечает" },
  "admin-required": { deystvie: "zaprosit-prava", tekst: "нужны права администратора" },
  "firewall-disabled": { deystvie: "nichego", tekst: "режим «весь трафик» недоступен: в Windows не работает служба базовой фильтрации" },
  "firewall-failed": { deystvie: "povtorit", tekst: "не удалось поставить защиту" },
  "killswitch-orphan": { deystvie: "nichego", tekst: "защита оставалась включённой без VPN, теперь снята" },
  "health-snapshot-stale": { deystvie: "nichego", tekst: "сведениям о состоянии сервера больше суток" },
  "health-snapshot-missing": { deystvie: "nichego", tekst: "сервер не прислал сведений о своём состоянии" },
  "update-rollback": { deystvie: "nichego", tekst: "обновление откатилось, работает прежняя версия" },
  "pipe-squatted": { deystvie: "pokazat-vinovnika", tekst: "другая программа не даёт окну связаться со службой" },
  "protocol-mismatch": { deystvie: "obnovit-programmu", tekst: "служба и программа разных версий, обнови программу" },
  "foreign-registry-hijack": { deystvie: "pokazat-vinovnika", tekst: "настройки прокси меняет другая программа" },
  "secrets-unreadable": { deystvie: "otkryt-servery", tekst: "файл с серверами не открылся или не записался" },
  "foreign-proxy-hijack": { deystvie: "pokazat-vinovnika", tekst: "мешает другая программа" },
  "all-servers-down": { deystvie: "proverit-set", tekst: "ни один сервер не отвечает" },
  "name-resolve-failed": { deystvie: "otkryt-servery", tekst: "эти адреса не удалось найти по имени, они пропущены" },
  "tunnel-not-carrying": { deystvie: "povtorit", tekst: "VPN перестал нести трафик" },
  "server-rejected-by-core": { deystvie: "otkryt-servery", tekst: "этот сервер не принят: проверь его ссылку" },
  "candidate-in-use": { deystvie: "nichego", tekst: "сервер занят подключением, сначала отключись" },
  "rule-invalid": { deystvie: "nichego", tekst: "правило не принято: путь не ведёт к файлу или домен не похож на имя" },
  "rules-rejected-by-core": { deystvie: "nichego", tekst: "новые правила не приняты, VPN работает по прежним" },
  "switch-needs-reconnect": { deystvie: "perepodklyuchitsya", tekst: "переподключись, чтобы выбрать этот сервер" },
  "switch-target-not-carrying": { deystvie: "otkryt-servery", tekst: "этот сервер не отвечает, выбери другой" },
  "switch-failed": { deystvie: "povtorit", tekst: "переключиться не удалось, попробуй ещё раз" },
  "journal-clear-failed": { deystvie: "povtorit", tekst: "файл журнала не стёрся, закрой программу, которая его читает" },
  "diagnostics-stale": { deystvie: "povtorit", tekst: "выгрузка диагностики прервалась, сохрани её заново" },
  "exit-ip-unmeasured": { deystvie: "povtorit", tekst: "адрес выхода не измерен: проверочный сервер не ответил или прокси не поднят" },
  "bandwidth-unmeasured": { deystvie: "povtorit", tekst: "скорость не измерена: сервис замера не задан, не ответил или замер прерван" },
  "request-invalid": { deystvie: "povtorit", tekst: "команда не разобрана, повтори действие" },
  "update-archive-invalid": { deystvie: "nichego", tekst: "архив сборки не принят: он повреждён или собран из другой версии" },
  "update-rollback-failed": { deystvie: "nichego", tekst: "обновление откатилось, но служба не отвечает: переустанови из установщика" },
  "update-check-failed": { deystvie: "povtorit", tekst: "сервер обновлений не ответил, проверить позже" },
  "update-download-failed": { deystvie: "povtorit", tekst: "обновление не скачалось или пришло повреждённым, работает прежняя версия" },
  "internal-error": { deystvie: "povtorit", tekst: "команда не выполнена: внутренняя ошибка службы" },
  "vps-unreachable": { deystvie: "povtorit", tekst: "сервер не отвечает на вход" },
  "vps-login-failed": { deystvie: "nichego", tekst: "сервер не пустил с этими данными входа" },
  "vps-host-key-changed": { deystvie: "prinyat-klyuch-hosta", tekst: "ключ сервера сменился" },
  "vps-tunnel-via-server": { deystvie: "otkryt-servery", tekst: "VPN сейчас идёт через этот же сервер, переключись на другой" },
  "vps-unsuitable": { deystvie: "nichego", tekst: "этот сервер не подходит" },
  "vps-not-empty": { deystvie: "pereustanovit-os", tekst: "сервер не пустой, ставь на чистую систему" },
  "vps-take-control": { deystvie: "vzyat-pod-upravlenie", tekst: "сервер уже под Affory, возьми его под управление" },
  "vps-install-failed": { deystvie: "povtorit", tekst: "установка на сервер не прошла" },
  "vps-cert-blocked": { deystvie: "otkryt-port-u-hostera", tekst: "сертификат не выпущен, поставлен только ключ reality" },
  "vps-address-blocked": { deystvie: "poprosit-drugoy-adres", tekst: "твоя сеть не пускает к адресу сервера" },
  "vps-hoster-firewall": { deystvie: "otkryt-port-u-hostera", tekst: "порт закрыт у хостера" },
  "vps-ports-failed": { deystvie: "povtorit", tekst: "порты не подобрались" },
  "vps-network-limited": { deystvie: "proverit-set", tekst: "твоя сеть сейчас пускает не всё" },
  "vps-busy": { deystvie: "nichego", tekst: "со своим сервером уже идёт другое дело" },
  "vps-last-login": { deystvie: "nichego", tekst: "ключ не снят, чтобы не остаться без входа на сервер" },
  "vps-password-failed": { deystvie: "povtorit", tekst: "вход по паролю не переключился" },
  "vps-password-other": { deystvie: "nichego", tekst: "пароль включён другой настройкой сервера" },
};

/** Button label per action; null means the screen only informs. */
export const podpisDeystviya: Record<Deystvie, string | null> = {
  povtorit: "Повторить",
  perepodklyuchitsya: "Переподключиться",
  "otkryt-servery": "Открыть серверы",
  "obnovit-podpisku": "Обновить подписку",
  "obnovit-programmu": "Обновить программу",
  "postavit-sluzhbu": "Установить службу",
  "zaprosit-prava": "Повторить от администратора",
  "proverit-set": "Проверить сеть",
  "pokazat-vinovnika": null,
  "nastroit-vremya": "Настроить время",
  "prinyat-klyuch-hosta": "Систему переустановили",
  "pereustanovit-os": null,
  "vzyat-pod-upravlenie": "Взять под управление",
  "otkryt-port-u-hostera": null,
  "poprosit-drugoy-adres": null,
  nichego: null,
};

/** Codes whose server-side text replaces ours verbatim (§9.1: the panel's
 *  message, not our wording on top). */
export const TEKST_SLUZHBY_DOSLOVNO = new Set(["subscription-expired"]);

/** Сбой самой ОБОЛОЧКИ: диалог не открылся, буфер не отдал текст, канал
 *  оборвался на полуслове. В §9.1 такого кода нет и быть не должно: та
 *  таблица про отказы СЛУЖБЫ, и ворота сверяют её со спекой в обе стороны,
 *  поэтому запись здесь живёт отдельно от `tekstOtkaza`.
 *
 *  Текст у него всегда технический: «connect: read pipe: The pipe has been
 *  ended». Крупно такая строка читается как вывод отладчика, поэтому крупно
 *  идёт наша фраза, а техника уходит в причину мелким. */
export const KOD_OBOLOCHKI = "oshibka-obolochki";
export const TEKST_OBOLOCHKI = "не получилось выполнить действие";

/** Срок ответа канала истёк, а канал жив (kodSroka в most.go). Команда при
 *  этом могла и не кончиться: служба после срока работает дальше. Подключение
 *  и смена сервера этот код плашкой не рисуют, а ждут итога состоянием (жалоба
 *  15.10.2026). Остальным командам он показывается как сбой оболочки. */
export const KOD_SROKA = "srok-otveta";

/** Что писать крупно, когда сказать нечем: кода нет в таблице и текста к нему
 *  тоже нет.
 *
 *  Раньше на этом месте печатался САМ КОД, строка вида `switch-failed`.
 *  Человеку её некуда приложить, а выглядит она как сбой программы, а не как
 *  ответ. Код никуда не девается: он стоит в `data-kod` для того, кто читает
 *  DOM или снимок экрана. */
export const TEKST_BEZ_KODA = "что-то пошло не так";
