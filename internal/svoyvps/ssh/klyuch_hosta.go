package ssh

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"slices"

	gossh "golang.org/x/crypto/ssh"
)

// KlyuchHosta это открытый ключ сервера в том виде, в каком он хранится в
// записи о сервере: тип отдельно, тело в формате протокола.
//
// Своя проверка вместо knownhosts: knownhosts не говорит клиенту, какой тип
// ключа ждать, и сервер вправе предъявить другой тип, чем записан
// (golang/go#29286). Тогда исправная машина выглядела бы подменённой.
type KlyuchHosta struct {
	Tip  string
	Telo []byte
}

func klyuchHostaIz(k gossh.PublicKey) KlyuchHosta {
	return KlyuchHosta{Tip: k.Type(), Telo: k.Marshal()}
}

// Otpechatok это SHA256:... как у ssh-keygen -lf: его человек сверяет на
// самой машине.
func (k KlyuchHosta) Otpechatok() string {
	pk, err := gossh.ParsePublicKey(k.Telo)
	if err != nil {
		return ""
	}
	return gossh.FingerprintSHA256(pk)
}

// TeloBase64 для записи о сервере.
func (k KlyuchHosta) TeloBase64() string { return base64.StdEncoding.EncodeToString(k.Telo) }

// KlyuchHostaIzZapisi собирает ключ из записи о сервере и сверяет, что тело
// разбирается и тип в нём тот же, что записан рядом.
func KlyuchHostaIzZapisi(tip, teloBase64 string) (KlyuchHosta, error) {
	telo, err := base64.StdEncoding.DecodeString(teloBase64)
	if err != nil {
		return KlyuchHosta{}, fmt.Errorf("тело ключа хоста не base64: %w", err)
	}
	pk, err := gossh.ParsePublicKey(telo)
	if err != nil {
		return KlyuchHosta{}, fmt.Errorf("ключ хоста не разбирается: %w", err)
	}
	if pk.Type() != tip {
		return KlyuchHosta{}, fmt.Errorf("в записи тип %q, а тело ключа %q", tip, pk.Type())
	}
	return KlyuchHosta{Tip: tip, Telo: telo}, nil
}

// Порядок для первой встречи: ed25519 есть во всех образах Ubuntu, RSA
// только с SHA-2, SHA-1 и DSA не берутся.
var algoritmyPoUmolchaniyu = []string{
	gossh.KeyAlgoED25519,
	gossh.KeyAlgoECDSA256, gossh.KeyAlgoECDSA384, gossh.KeyAlgoECDSA521,
	gossh.KeyAlgoRSASHA512, gossh.KeyAlgoRSASHA256,
}

// algoritmyKlyuchaHosta ставит тип записанного ключа первым. Сервер выбирает
// первый общий в порядке клиента, поэтому исправная машина предъявит тот же
// тип, что записан. Остальные оставлены после него: если записанного типа на
// машине больше нет, она предъявит другой ключ, и это честно поймается как
// смена ключа, а не как непонятный отказ согласования.
func algoritmyKlyuchaHosta(izvesten *KlyuchHosta) []string {
	if izvesten == nil {
		return algoritmyPoUmolchaniyu
	}
	var pervye []string
	switch izvesten.Tip {
	case gossh.KeyAlgoRSA:
		pervye = []string{gossh.KeyAlgoRSASHA512, gossh.KeyAlgoRSASHA256}
	default:
		pervye = []string{izvesten.Tip}
	}
	itog := append([]string(nil), pervye...)
	for _, a := range algoritmyPoUmolchaniyu {
		if !slices.Contains(itog, a) {
			itog = append(itog, a)
		}
	}
	return itog
}

// sverkaKlyuchaHosta запоминает предъявленный ключ и решает, верить ли ему.
//
// Первая встреча это доверие при первой встрече: отпечаток показывается на
// шаге проверки. За защиту это не выдаётся: пароль уходит серверу раньше,
// чем человек видит отпечаток. Сменившийся ключ проходит, только если
// человек одобрил именно его отпечаток («Я переустановил систему»).
type sverkaKlyuchaHosta struct {
	izvesten *KlyuchHosta
	odobren  string

	predyavlen *KlyuchHosta
	smenilsya  bool
}

func (s *sverkaKlyuchaHosta) proverit(_ string, _ net.Addr, k gossh.PublicKey) (err error) {
	// x/crypto зовёт сверку из своей горутины обмена ключами (и при
	// перекладке ключей посреди работы), а не из Podklyuchit: его перехват
	// сюда не дотягивается.
	defer zashchita("sverkaKlyuchaHosta", &err)
	p := klyuchHostaIz(k)
	s.predyavlen = &p
	if s.izvesten == nil || bytes.Equal(s.izvesten.Telo, p.Telo) {
		return nil
	}
	s.smenilsya = true
	if s.odobren != "" && s.odobren == gossh.FingerprintSHA256(k) {
		return nil
	}
	o := otkaz(PrichinaKlyuchHostaSmenilsya,
		fmt.Sprintf("записан %s, предъявлен %s; ничего не отправлено", s.izvesten.Otpechatok(), p.Otpechatok()), nil)
	o.Predyavlen = &p
	return o
}
