package upravlenie

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/blok"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// zapasProbySerta это срок пробы с выпуском сверх ожидания сертификата: проба
// подбора, которая её сменит, ставится уже после ожидания.
const zapasProbySerta = 3 * time.Minute

// Podbor это всё, что нужно подбору портов заново на поставленном сервере
// (З9, К4 плана): провайдер начал резать выбранный порт, а сносить сервер
// незачем.
type Podbor struct {
	Puti Puti
	// Vhod: root ключом управления и Izvesten, ключ хоста из записи.
	Vhod ssh.Zapros
	Imya string
	// Ruchnye: порт вида, заданный руками, ставится один, даже не пройдя
	// проверку. 0 - подбирать, и рабочий порт вида идёт последним кандидатом.
	Ruchnye porty.Ruchnye
	// Opcii.Staging берётся у программы сервера, если она говорит о выпуске:
	// иначе смена каталога перевыпустила бы сертификат.
	Opcii    konfig.Opcii
	Proverka proverka.Opcii
	Zapusk   proverka.Zapusk
	Pryamo   blok.Dial
	Hod      func(Hod)
	// Klyuchi см. podborPortov.Klyuchi.
	Klyuchi func(Itog) error
}

// PodobratPorty подбирает порты заново на сервере, где стоит своя
// установка: тот же подбор, что при установке, с секретами принятого пакета.
// Меняются только порты, и ключ вида, чей порт остался, тот же. На сервере
// без hy2 сначала пробуется выпуск сертификата: вышел - встаёт и hy2.
// Прежние наши правила ufw закрываются после того, как сервер принял
// окончательный пакет. Ключи в набор кладёт служба.
func PodobratPorty(ctx context.Context, p Podbor) (itog Itog, err error) {
	defer zashchita("PodobratPorty", &err)
	a, err := netip.ParseAddr(p.Vhod.Adres)
	switch {
	case err != nil:
		return Itog{}, fmt.Errorf("подбор портов: адрес %q не IP", ssh.Obrezat(p.Vhod.Adres, 80))
	case p.Vhod.Vhod.Podpis == nil || p.Zapusk == nil || p.Pryamo == nil:
		return Itog{}, errors.New("подбор портов: не задан ключ управления или запуск проверки")
	case p.Imya == "":
		return Itog{}, errors.New("подбор портов: нет имени сервера")
	}
	adres := a.Unmap()
	hod := func(h Hod) {
		if p.Hod != nil {
			p.Hod(h)
		}
	}

	hod(Hod{Shag: ShagVhod})
	s, err := Voyti(ctx, p.Puti, p.Vhod)
	if err != nil {
		return Itog{}, err
	}
	defer s.Zakryt()
	itog.Vhod = s.Svedeniya()

	hod(Hod{Shag: ShagOsmotr})
	o, err := osmotret(ctx, s)
	if err != nil {
		return Itog{}, err
	}
	for _, z := range o.Zamechaniya(seychas()) {
		if z.Kod == server.KodPodCentrom {
			return Itog{}, &Ostanovka{Zamechaniya: []server.Zamechanie{z}}
		}
	}
	switch {
	case o.Otmetka == nil:
		return Itog{}, server.ErrNetUstanovki
	case !nashaOtmetka(o.Otmetka, p.Vhod.Vhod.Podpis.PublicKey()):
		return Itog{}, ErrChuzhayaUstanovka
	}
	itog.ParolVyklyuchen = slices.Contains(o.SshdFayly, server.FaylParolya)

	sostav, paket, err := prochitatUstanovku(ctx, s)
	if err != nil {
		return Itog{}, err
	}
	if len(paket) == 0 {
		return Itog{}, ErrNeDoshla
	}
	sek, vh, err := konfig.IzPaketa(paket, adres.String(), p.Imya)
	if err != nil {
		return Itog{}, err
	}
	st, err := sostoyanie(ctx, s)
	if err != nil {
		return Itog{}, err
	}
	itog.Sert, itog.Prodlenie = prodlenieStoyashchego(vh, st)
	itog.Versiya = nomerVersii(sostav.VpnUzel)
	opcii := p.Opcii
	if st.Sert != nil {
		opcii.Staging = st.Sert.Staging
	}
	if itog.Prodlenie == porty.BezSerta {
		// hy2 нет: сертификат при установке не вышел, а с тех пор у хостера
		// могли открыть 80. Выпуск пробуется снова пробным пакетом с теми же
		// входами: reality работает, а не дождался окончательного пакета -
		// сервер сам вернётся к принятому, и выпуск выключится.
		hod(Hod{Shag: ShagSertifikat})
		oSerta := opcii
		oSerta.Vypusk, oSerta.Proba = true, int((srokSerta + zapasProbySerta).Seconds())
		paketSerta, err := konfig.Paket(sek, vh, oSerta)
		if err != nil {
			return Itog{}, err
		}
		pr, otkazSerta, err := vypustitSert(ctx, s, adres, "", paketSerta, &itog.Sert)
		if err != nil {
			return Itog{}, err
		}
		if itog.Prodlenie = pr; pr == porty.BezSerta {
			itog.Preduprezhdeniya = append(itog.Preduprezhdeniya, Preduprezhdenie{Vid: NetSerta, Otkaz: otkazSerta})
		}
	}

	pp := podborPortov{Adres: adres, Imya: p.Imya, PortSsh: p.Vhod.Port, Ruchnye: p.Ruchnye,
		Rabochie: porty.Ruchnye{Reality: portVida(vh, "reality-tcp"), Hy2: portVida(vh, "hy2")},
		Opcii:    opcii, Proverka: p.Proverka, Zapusk: p.Zapusk, Pryamo: p.Pryamo, Hod: hod, Klyuchi: p.Klyuchi,
		Sayt: vh.Sayt}
	if err := podobratIPostavit(ctx, s, o, sek, itog.Prodlenie, pp, &itog); err != nil {
		return Itog{}, err
	}
	return itog, nil
}
