import { describe, expect, it } from "vitest";
import { razobratSoobshchenie } from "./soobshchenieHostera";

// Формы выдуманы по образцу писем и сообщений хостеров. Сообщения со скриншота
// владельца текстом в репозитории нет: его форма (метки по-русски, эмодзи,
// длинное тире) повторена во втором случае.

describe("razobratSoobshchenie", () => {
  it("письмо по-английски", () => {
    const t = ["Your server is ready", "", "IP address: 203.0.113.10", "Username: root", "Password: primer-parolya", "SSH port: 22"].join("\n");
    expect(razobratSoobshchenie(t)).toEqual({ adres: "203.0.113.10", polzovatel: "root", parol: "primer-parolya", port: 22 });
  });

  it("сообщение по-русски с эмодзи и длинным тире", () => {
    const t = ["✅ Ваш сервер готов!", "🖥 IP-адрес — 203.0.113.10", "👤 Логин — root", "🔑 Пароль — Xy7#kL2p-q:w", "📡 Порт SSH: 2222"].join("\r\n");
    expect(razobratSoobshchenie(t)).toEqual({ adres: "203.0.113.10", polzovatel: "root", parol: "Xy7#kL2p-q:w", port: 2222 });
  });

  it("дефис с пробелами, адрес с портом, кавычки вокруг пароля", () => {
    const t = ["Адрес: 203.0.113.10:2200", "Пользователь - admin", "Пароль - `a b=c`"].join("\n");
    expect(razobratSoobshchenie(t)).toEqual({ adres: "203.0.113.10", port: 2200, polzovatel: "admin", parol: "a b=c" });
  });

  it("метка и значение через пробел", () => {
    expect(razobratSoobshchenie("IP 203.0.113.10\nLogin root\nPassword Qw3rty!")).toEqual({
      adres: "203.0.113.10",
      polzovatel: "root",
      parol: "Qw3rty!",
    });
  });

  it("строка ssh в любом порядке", () => {
    expect(razobratSoobshchenie("Подключение: ssh -p 2222 root@203.0.113.10\nПароль: x1")).toEqual({
      adres: "203.0.113.10",
      port: 2222,
      polzovatel: "root",
      parol: "x1",
    });
    expect(razobratSoobshchenie("ssh admin@vps.example.net -p 22")).toEqual({ adres: "vps.example.net", port: 22, polzovatel: "admin" });
  });

  it("доступ к панели и кабинету мимо", () => {
    const t = [
      "Личный кабинет: https://my.example.net",
      "Логин от панели: ivan@example.net",
      "Пароль от панели: panelpass",
      "Адрес панели: panel.example.net",
      "Password reset link https://my.example.net/reset",
      "",
      "Данные для SSH:",
      "IP: 203.0.113.10",
      "Логин: root",
      "Пароль: sshpass",
    ].join("\n");
    expect(razobratSoobshchenie(t)).toEqual({ adres: "203.0.113.10", polzovatel: "root", parol: "sshpass" });
  });

  it("IPv4 важнее имени узла и IPv6", () => {
    const t = ["Hostname: vps1.example.net", "IPv6: 2001:db8::10", "IPv4: 203.0.113.10", "root password: x"].join("\n");
    expect(razobratSoobshchenie(t)).toEqual({ adres: "203.0.113.10", parol: "x" });
  });

  it("без IPv4 берётся имя узла, без него IPv6", () => {
    expect(razobratSoobshchenie("IPv6: 2001:db8::10\nHost: vps1.example.net")).toEqual({ adres: "vps1.example.net" });
    expect(razobratSoobshchenie("IPv6: [2001:db8::10]:2222")).toEqual({ adres: "2001:db8::10", port: 2222 });
  });

  it("адрес без метки, если он в тексте один", () => {
    expect(razobratSoobshchenie("Your server 203.0.113.10 is ready.\nRoot password: abc")).toEqual({ adres: "203.0.113.10", parol: "abc" });
    expect(razobratSoobshchenie("Сервер 203.0.113.10 или 203.0.113.11")).toEqual({});
  });

  it("негодные значения не попадают в поля", () => {
    const t = ["Сервер: VPS-1", "Порт: 70000", "Логин: Иван Петров", "IP: 300.1.2.3"].join("\n");
    expect(razobratSoobshchenie(t)).toEqual({});
  });

  it("первое значение поля не перебивается следующим", () => {
    expect(razobratSoobshchenie("Пароль: первый\nPassword: второй")).toEqual({ parol: "первый" });
  });

  it("логин и пароль одной строкой", () => {
    expect(razobratSoobshchenie("Логин / пароль: root / Xy7#kL2p")).toEqual({ polzovatel: "root", parol: "Xy7#kL2p" });
    expect(razobratSoobshchenie("Login/Password: admin:Qw3/x")).toEqual({ polzovatel: "admin", parol: "Qw3/x" });
    // Одно значение на две метки: не угадывать, что это.
    expect(razobratSoobshchenie("Логин и пароль: root")).toEqual({});
    // «User password» это пароль, а не пара.
    expect(razobratSoobshchenie("User password: Xy7#kL2p")).toEqual({ parol: "Xy7#kL2p" });
    // Обратный порядок и метка на три поля не угадываются.
    expect(razobratSoobshchenie("Пароль/логин: Xy7/root")).toEqual({});
    expect(razobratSoobshchenie("Логин/пароль/IP: root/Xy7/203.0.113.10")).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("Host/login/password: vps1/root/Xy7")).toEqual({});
    expect(razobratSoobshchenie("Логин и пароль от сервера: root / Xy7")).toEqual({ polzovatel: "root", parol: "Xy7" });
    expect(razobratSoobshchenie("Логин/Login и пароль/Password: root / Xy7")).toEqual({ polzovatel: "root", parol: "Xy7" });
    expect(razobratSoobshchenie("Username & password (SSH, SFTP): root / Xy7")).toEqual({ polzovatel: "root", parol: "Xy7" });
  });

  it("SFTP не чужая метка, в отличие от FTP", () => {
    expect(razobratSoobshchenie("SFTP/SSH: root@203.0.113.10")).toEqual({ adres: "203.0.113.10", polzovatel: "root" });
    expect(razobratSoobshchenie("Логин SSH/SFTP: root")).toEqual({ polzovatel: "root" });
    expect(razobratSoobshchenie("Логин FTP: user1\nПароль FTP: ftppass")).toEqual({});
  });

  it("IPv4 без метки важнее имени узла, но не резолвер и не маска", () => {
    expect(razobratSoobshchenie("Your server 203.0.113.10 is ready\nHostname: srv1.example.net")).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("Hostname: vps1.example.net\nResolver 198.51.100.53\nNTP 198.51.100.123")).toEqual({ adres: "vps1.example.net" });
    expect(razobratSoobshchenie("Hostname: vps1.example.net\nSubnet: 255.255.255.0")).toEqual({ adres: "vps1.example.net" });
  });

  it("границы частных и зарезервированных диапазонов", () => {
    const adres = (ip: string) => razobratSoobshchenie(`IP: ${ip}`).adres;
    for (const ip of ["172.15.0.1", "172.32.0.1", "100.63.0.1", "100.128.0.1", "192.167.0.1", "169.253.0.1", "1.0.0.1", "223.255.255.1"]) {
      expect(adres(ip)).toBe(ip);
    }
    for (const ip of ["172.16.0.1", "172.31.0.1", "100.64.0.1", "100.127.0.1", "192.168.0.1", "169.254.0.1", "10.0.0.1", "127.0.0.1", "0.0.0.1", "224.0.0.1", "255.255.255.0"]) {
      expect(adres(ip)).toBeUndefined();
    }
  });

  it("ssh -l", () => {
    expect(razobratSoobshchenie("ssh -p 22 -l root 203.0.113.10")).toEqual({ adres: "203.0.113.10", port: 22, polzovatel: "root" });
    expect(razobratSoobshchenie("ssh -l root vps1.example.net")).toEqual({ adres: "vps1.example.net", polzovatel: "root" });
  });

  it("значения ключей ssh не адрес и не имя", () => {
    expect(razobratSoobshchenie("ssh -i key.pem -l root 203.0.113.10")).toEqual({ adres: "203.0.113.10", polzovatel: "root" });
    expect(razobratSoobshchenie("ssh -J admin@198.51.100.1 root@203.0.113.10")).toEqual({ adres: "203.0.113.10", polzovatel: "root" });
    expect(razobratSoobshchenie("ssh -o StrictHostKeyChecking=no -p 2222 root@203.0.113.10")).toEqual({
      adres: "203.0.113.10",
      port: 2222,
      polzovatel: "root",
    });
  });

  it("заглушка в паре логин/пароль", () => {
    expect(razobratSoobshchenie("Логин/пароль: root / -")).toEqual({});
  });

  it("слова меток с начала слова", () => {
    expect(razobratSoobshchenie("Passport: AB123\nSupport: 24/7\nSuperuser: x")).toEqual({});
    expect(razobratSoobshchenie("Main IP address: 203.0.113.10\nPass: Xy7")).toEqual({ adres: "203.0.113.10", parol: "Xy7" });
  });

  it("IP-KVM и частные адреса не встают адресом", () => {
    expect(razobratSoobshchenie("IP-KVM: 198.51.100.50\nIP: 203.0.113.10")).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("Private IP address: 10.0.0.5\nIP: 203.0.113.10")).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("IP: 192.168.1.10\nСервер 172.16.0.1 готов")).toEqual({});
  });

  it("заглушка вместо значения", () => {
    expect(razobratSoobshchenie("Пароль: -\nЛогин: none")).toEqual({});
    expect(razobratSoobshchenie("Пароль: нет")).toEqual({});
    expect(razobratSoobshchenie("Password: ***")).toEqual({});
  });

  it("адрес с маской", () => {
    expect(razobratSoobshchenie("IP: 203.0.113.10/24\nBroadcast: 203.0.113.255")).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("IPv6: 2001:db8:1:2::/64")).toEqual({});
  });

  it("ssh -p слитно и ssh://", () => {
    expect(razobratSoobshchenie("ssh -p2222 root@203.0.113.10")).toEqual({ adres: "203.0.113.10", port: 2222, polzovatel: "root" });
    expect(razobratSoobshchenie("ssh://root@203.0.113.10:2222")).toEqual({ adres: "203.0.113.10", port: 2222, polzovatel: "root" });
  });

  it("ssh за чужой меткой мимо, «SSH: root@…» разбирается", () => {
    const t = "Консоль восстановления: ssh rescue@198.51.100.99\nRescue: ssh root@198.51.100.98\nIP: 203.0.113.10";
    expect(razobratSoobshchenie(t)).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("SSH: root@203.0.113.10")).toEqual({ adres: "203.0.113.10", polzovatel: "root" });
    expect(razobratSoobshchenie("SSH-порт: 2222\nssh root@web1.example.net")).toEqual({ adres: "web1.example.net", port: 2222, polzovatel: "root" });
  });

  it("сайт хостера не встаёт адресом", () => {
    expect(razobratSoobshchenie("Hosting: hoster.example.net\nПароль: x")).toEqual({ parol: "x" });
  });

  it("порт с пояснением", () => {
    expect(razobratSoobshchenie("Порт: 2222 (SSH)")).toEqual({ port: 2222 });
    expect(razobratSoobshchenie("Port: 2222/tcp")).toEqual({ port: 2222 });
  });

  it("шлюз, DNS и маска не встают адресом", () => {
    const t = ["DNS-сервер: 192.0.2.53", "Шлюз: 192.0.2.1", "Маска: 255.255.255.0", "IP-адрес: 203.0.113.10"].join("\n");
    expect(razobratSoobshchenie(t)).toEqual({ adres: "203.0.113.10" });
    // Запасной поиск адреса без метки тоже мимо строки DNS.
    expect(razobratSoobshchenie("Host: vps1.example.net\nDNS: 192.0.2.53")).toEqual({ adres: "vps1.example.net" });
  });

  it("метка на одной строке, значение на следующей", () => {
    const t = ["Пароль:", "Xy7#k:L2p", "", "IP-адрес:", "", "203.0.113.10", "Логин:", "root"].join("\n");
    expect(razobratSoobshchenie(t)).toEqual({ parol: "Xy7#k:L2p", adres: "203.0.113.10", polzovatel: "root" });
    // Следующая строка про панель или фраза значением не становится.
    expect(razobratSoobshchenie("Пароль:\nПанель: https://my.example.net")).toEqual({});
    expect(razobratSoobshchenie("Пароль:\nбудет отправлен отдельным письмом")).toEqual({});
    // Своя метка на следующей строке отменяет ожидание.
    expect(razobratSoobshchenie("Пароль:\nЛогин: root")).toEqual({ polzovatel: "root" });
    // Метка того же поля строкой ниже одним словом может быть самим паролем:
    // не гадать. С пробелом это заголовок и метка под ним.
    expect(razobratSoobshchenie("Пароль:\nPass:word")).toEqual({});
    expect(razobratSoobshchenie("Данные сервера:\nIP-адрес: 203.0.113.10:2222\nЛогин: root")).toEqual({
      adres: "203.0.113.10",
      port: 2222,
      polzovatel: "root",
    });
    expect(razobratSoobshchenie("Сервер:\nHostname: vps1.example.net")).toEqual({ adres: "vps1.example.net" });
    expect(razobratSoobshchenie("Пароль:\nPassword: Xy7")).toEqual({ parol: "Xy7" });
    expect(razobratSoobshchenie("Пароль:\nUser:abc")).toEqual({});
    expect(razobratSoobshchenie("Пароль:\nЛогин:\nroot")).toEqual({ polzovatel: "root" });
  });

  it("локальные IPv6 не адрес", () => {
    for (const ip of ["fe80::1", "fd00::5", "fc12:3456::1", "ff02::1", "::1"]) {
      expect(razobratSoobshchenie(`IPv6: ${ip}`)).toEqual({});
    }
    expect(razobratSoobshchenie("IPv6: fd::1")).toEqual({ adres: "fd::1" });
  });

  it("имя пользователя строчными", () => {
    expect(razobratSoobshchenie("Логин: Root")).toEqual({ polzovatel: "root" });
    expect(razobratSoobshchenie("ssh Admin@203.0.113.10")).toEqual({ adres: "203.0.113.10", polzovatel: "admin" });
  });

  it("номер пункта перед меткой", () => {
    const t = ["1. IP: 203.0.113.10:2222", "2) Логин: root", "3. Пароль: Xy7", "IPv4 2.0: нет"].join("\n");
    expect(razobratSoobshchenie(t)).toEqual({ adres: "203.0.113.10", port: 2222, polzovatel: "root", parol: "Xy7" });
  });

  it("невидимые символы из копии письма", () => {
    const zws = String.fromCodePoint(0x200b);
    const shy = String.fromCodePoint(0xad);
    expect(razobratSoobshchenie(`Пароль: Xy7${zws}\nIP: 203.0.113.10${shy}`)).toEqual({ parol: "Xy7", adres: "203.0.113.10" });
  });

  it("точка после адреса в конце фразы", () => {
    expect(razobratSoobshchenie("Your VPS is ready at 203.0.113.10.")).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("IP: 203.0.113.10.\nHost: vps1.example.net.")).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("Host: vps1.example.net.")).toEqual({ adres: "vps1.example.net" });
  });

  it("строка ssh не перебивает IPv4", () => {
    expect(razobratSoobshchenie("ssh root@vps1.example.net\nIPv4: 203.0.113.10")).toEqual({ adres: "203.0.113.10", polzovatel: "root" });
    expect(razobratSoobshchenie("ssh root@[2001:db8::10]\nIP: 203.0.113.10")).toEqual({ adres: "203.0.113.10", polzovatel: "root" });
  });

  it("порт из чужого адреса не берётся, порт ssh берётся", () => {
    expect(razobratSoobshchenie("Host: my.example.net:2083\nIPv4: 203.0.113.10")).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("IPv6: [2001:db8::10]:2222\nIPv4: 203.0.113.10")).toEqual({ adres: "203.0.113.10" });
    expect(razobratSoobshchenie("ssh -p 2222 root@vps1.example.net\nIPv4: 203.0.113.10")).toEqual({
      adres: "203.0.113.10",
      port: 2222,
      polzovatel: "root",
    });
  });

  it("в итоге нет ключей без значения", () => {
    expect(Object.keys(razobratSoobshchenie("Порт: 70000\nIP: 300.1.2.3:99999\nssh -p x Иван@z"))).toEqual([]);
    expect(Object.keys(razobratSoobshchenie("IP: 203.0.113.10"))).toEqual(["adres"]);
  });

  it("прямые кавычки остаются в пароле", () => {
    expect(razobratSoobshchenie('Пароль: "Xy7#kL2p"')).toEqual({ parol: '"Xy7#kL2p"' });
    expect(razobratSoobshchenie("Password: 'ab'")).toEqual({ parol: "'ab'" });
    expect(razobratSoobshchenie("Пароль: «Xy7#kL2p»")).toEqual({ parol: "Xy7#kL2p" });
  });

  it("фраза вместо пароля не встаёт паролем", () => {
    expect(razobratSoobshchenie("Пароль: будет отправлен в SMS")).toEqual({});
  });

  it("пустой и посторонний текст", () => {
    expect(razobratSoobshchenie("")).toEqual({});
    expect(razobratSoobshchenie("Спасибо, что выбрали нас!\n\n🎉")).toEqual({});
  });
});
