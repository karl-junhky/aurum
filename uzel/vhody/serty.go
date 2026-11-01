package vhody

import (
	"crypto/tls"
	"fmt"
	"strings"
	"sync/atomic"

	stls "github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/option"
)

// Сертификаты TLS-входов (trojan, anytls, hy2) приходят в пакете центра.
// Свой выдан на IP сервера: ссылки ведут на адрес, и сервер не зависит ни
// от какого имени. Сертификат имени подписки едет только серверам подписки
// и отдаётся тем, кто сам спросил это имя, то есть ссылкам, выданным, пока
// входы жили на имени.
//
// Выбор идёт на каждом рукопожатии, а не один раз при подъёме: QUIC-слушатель
// hy2 держит свою копию настроек TLS с подъёма, и продлённый сертификат из
// файла он увидел бы только после перезапуска ядра. Свой сертификат живёт
// неделю, перезапуск ради него каждые три дня рвал бы всех.

// Serty это сертификаты из последнего принятого пакета.
type Serty struct {
	// Imya это имя в сертификате имени подписки.
	Imya    string
	PoImeni *tls.Certificate
	// Svoy это сертификат на IP сервера.
	Svoy *tls.Certificate
}

var tekushchie atomic.Pointer[Serty]

// PostavitSerty меняет сертификаты всех TLS-входов разом: новые рукопожатия
// идут уже с ними, открытые соединения не трогаются.
func PostavitSerty(s *Serty) { tekushchie.Store(s) }

// podklyuchitSerty вешает выбор сертификата на настройку TLS входа. Файл из
// certificate_path остаётся запасным: им вход отвечает, пока центр не
// прислал ни одного сертификата.
func podklyuchitSerty(c stls.ServerConfig, opcii *option.InboundTLSOptions) error {
	if c == nil || opcii == nil || !opcii.Enabled || (opcii.Reality != nil && opcii.Reality.Enabled) {
		return nil
	}
	std, est := c.(*stls.STDServerConfig)
	if !est {
		return fmt.Errorf("настройка TLS входа неожиданного вида %T", c)
	}
	konfig, err := std.STDConfig()
	if err != nil {
		return err
	}
	if len(konfig.Certificates) == 0 {
		return fmt.Errorf("у TLS входа нет сертификата")
	}
	zapas := konfig.Certificates[0]
	// Пустой список, чтобы выбор звался и без имени в приветствии: иначе
	// crypto/tls отдаёт первый сертификат списка мимо выбора, а ссылки на
	// IP имени не шлют вовсе.
	konfig.Certificates = nil
	konfig.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return vybratSert(hello.ServerName, &zapas), nil
	}
	return nil
}

// vybratSert: спросили имя подписки - сертификат имени; иначе свой на IP;
// без своего сертификат имени (так входы жили до сертификатов на IP); без
// обоих файл.
func vybratSert(imya string, zapas *tls.Certificate) *tls.Certificate {
	s := tekushchie.Load()
	switch {
	case s == nil:
		return zapas
	case s.PoImeni != nil && imya != "" && strings.EqualFold(imya, s.Imya):
		return s.PoImeni
	case s.Svoy != nil:
		return s.Svoy
	case s.PoImeni != nil:
		return s.PoImeni
	}
	return zapas
}
