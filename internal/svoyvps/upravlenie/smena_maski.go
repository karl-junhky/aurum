package upravlenie

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// MaskaSvoya: маску ввёл человек, и её нет среди известных.
const MaskaSvoya = "svoya"

// SmenaMaski это всё, что нужно смене маски на поставленном сервере (МА2,
// З7): «Подобрать другую» или «Ввести свою».
type SmenaMaski struct {
	Puti Puti
	// Vhod: root ключом управления и Izvesten, ключ хоста из записи.
	Vhod ssh.Zapros
	Imya string
	// Svoya это имя, которое ввёл человек; пусто - подобрать другую у
	// соседей сервера.
	Svoya string
	// BrendSoglasen: человек видел, что известное имя глушат на мобильном, и
	// всё равно ставит его.
	BrendSoglasen bool
	// Bylo это маска в записи службы. Маска сервера разошлась с ней, когда
	// прошлая смена встала, а ответ оборвался, или маску сменил другой
	// компьютер с ключом управления. Подбор тогда не идёт: набор доводится до
	// маски сервера, а другую человек подберёт следующим нажатием.
	Bylo string
	// Opcii.Staging берётся у программы сервера, как при подборе портов.
	Opcii konfig.Opcii
	Hod   func(Hod)
}

// MaskaNegodna: программа сервера проверила введённое имя, и в маску оно не
// годится. Podrobno это исходная ошибка с сервера, для журнала службы.
type MaskaNegodna struct{ Imya, Prichina, Podrobno string }

func (m *MaskaNegodna) Error() string {
	t := "маска " + m.Imya + " не годится: " + m.Prichina
	if m.Podrobno != "" {
		t += " (" + m.Podrobno + ")"
	}
	return t
}

// ErrNetDrugoyMaski: подбор не нашёл годной маски, кроме той, что стоит, или
// ответили только известные имена (прежние маски 1.11.0).
var ErrNetDrugoyMaski = errors.New("другой годной маски не нашлось")

// MaskaIzvestnaya: введённое имя из известных, а согласия ставить его нет.
type MaskaIzvestnaya struct{ Imya string }

func (m *MaskaIzvestnaya) Error() string {
	return "маска " + m.Imya + " из известных имён: на мобильном её глушат"
}

// odnaMaska: имена совпадают без учёта регистра и точки в конце.
func odnaMaska(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(a, "."), strings.TrimSuffix(b, "."))
}

// SmenitMasku ставит на сервер другую маску reality: подобранную у соседей
// (кроме нынешней) или введённую человеком и проверенную с сервера. Порты,
// пароли и ключи reality те же, меняется только имя в ссылках reality, и
// ключи на телефоне с прежним именем перестают работать. Ключи в набор кладёт
// служба. Своя маска, что уже стоит на сервере, и подбор при маске сервера,
// разошедшейся с Bylo, пакет не трогают, а только отдают ключи: так повтор
// после обрыва посреди смены доводит набор до сервера.
func SmenitMasku(ctx context.Context, p SmenaMaski) (itog Itog, err error) {
	defer zashchita("SmenitMasku", &err)
	a, err := netip.ParseAddr(p.Vhod.Adres)
	switch {
	case err != nil:
		return Itog{}, fmt.Errorf("смена маски: адрес %q не IP", ssh.Obrezat(p.Vhod.Adres, 80))
	case p.Vhod.Vhod.Podpis == nil:
		return Itog{}, errors.New("смена маски: не задан ключ управления")
	case p.Imya == "":
		return Itog{}, errors.New("смена маски: нет имени сервера")
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
	if vh.Sayt != nil {
		return Itog{}, ErrRezhimDomena
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

	hod(Hod{Shag: ShagMaska})
	var mk Maska
	if svoya := strings.TrimSpace(p.Svoya); svoya != "" {
		pr, err := proveritSvoyuMasku(ctx, s, svoya)
		if err != nil {
			return Itog{}, err
		}
		mk = Maska{Imya: pr.Imya, Istochnik: MaskaSvoya}
		if pr.Brend {
			mk.Istochnik = MaskaBrend
		}
		switch {
		case odnaMaska(pr.Imya, sek.Maska):
			// Уже стоит: сервер работает с ней, годна она сейчас или нет, и
			// набору нужны только её ключи.
			mk.Imya = sek.Maska
		case !pr.Godna:
			return Itog{}, &MaskaNegodna{Imya: pr.Imya, Prichina: pr.Prichina, Podrobno: pr.Podrobno}
		case pr.Brend && !p.BrendSoglasen:
			return Itog{}, &MaskaIzvestnaya{Imya: pr.Imya}
		}
	} else if p.Bylo != "" && !odnaMaska(sek.Maska, p.Bylo) {
		// Откуда эта маска, сервер не хранит.
		mk = Maska{Imya: sek.Maska}
	} else if mk, err = podobratMasku(ctx, s, adres, sek.Maska); errors.Is(err, ErrNetMaski) {
		return Itog{}, fmt.Errorf("%w: %w", ErrNetDrugoyMaski, err)
	} else if err != nil {
		return Itog{}, err
	} else if mk.Istochnik == MaskaBrend {
		// Подбор упал на прежние маски 1.11.0: менять известное имя на
		// известное незачем, их глушат одинаково.
		return Itog{}, fmt.Errorf("%w: ответила только известная маска %s", ErrNetDrugoyMaski, mk.Imya)
	}
	uzhe := odnaMaska(mk.Imya, sek.Maska)
	sek.Maska = mk.Imya
	itog.MaskaIstochnik = mk.Istochnik

	if !uzhe {
		hod(Hod{Shag: ShagPaket})
		novyy, err := konfig.Paket(sek, vh, opcii)
		if err != nil {
			return Itog{}, err
		}
		if _, err := primenit(ctx, s, novyy); err != nil {
			return Itog{}, err
		}
	}
	klyuchi, err := konfig.Klyuchi(sek, vh)
	if err != nil {
		return Itog{}, err
	}
	itog.Adres, itog.Sekrety, itog.Vhody, itog.Klyuchi = adres.String(), sek, vh, klyuchi
	return itog, nil
}
