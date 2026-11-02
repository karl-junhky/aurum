// Вкладки окна по порядку. Спека §8.3 закрепила четыре, пятая «Свой VPN»
// добавлена решением владельца 30.09.2026 (план «Свой VPN», §4). «Серверы»
// остались вкладкой в коде, но в полосу не выходят (Karkas.tsx).
export const VKLADKI = ["podklyuchenie", "pravila", "servery", "svoy-vpn", "nastroyki"] as const;

export type Vkladka = (typeof VKLADKI)[number];

const NAZVANIYA: Record<Vkladka, string> = {
  podklyuchenie: "Подключение",
  pravila: "Правила",
  servery: "Серверы",
  "svoy-vpn": "Свой VPN",
  nastroyki: "Настройки",
};

export function nazvanieVkladki(v: Vkladka): string {
  return NAZVANIYA[v];
}
