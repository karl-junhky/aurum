package upravlenie

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// MaskaDomen: маска reality это свой домен человека (режим домена).
const MaskaDomen = "domen"

// versiyaSDomenom это первая программа сервера с режимом своего домена.
var versiyaSDomenom = [3]int{0, 7, 0}

// ErrStarayaProgrammaDomena: программа сервера старше 0.7.0 и сайта режима
// домена не знает.
var ErrStarayaProgrammaDomena = errors.New("программа сервера не умеет работать со своим доменом: сначала обнови её")

// ErrRezhimDomena: дело простого режима на сервере в режиме домена (смена
// маски: маска там сам домен).
var ErrRezhimDomena = errors.New("сервер работает на своём домене: маска это сам домен")

// ErrProstoyRezhim: дело режима домена на сервере в простом режиме.
var ErrProstoyRezhim = errors.New("сервер работает без своего домена")

// ErrDrugoyDomen: сервер уже на другом домене. Сменить домен можно через
// простые ключи.
type ErrDrugoyDomen struct{ Domen string }

func (e *ErrDrugoyDomen) Error() string {
	return "сервер уже работает на домене " + e.Domen
}

// SertDomenaNeVyshel: Let's Encrypt не выдал сертификат на домен, чаще всего
// потому, что A-запись ведёт не на сервер или 80/tcp закрыт у хостера.
// Predel не 0: причина не в этом, а попытки выпуска этого часа у программы
// сервера кончились; следующая не раньше этого времени, Unix-секунды.
type SertDomenaNeVyshel struct {
	Domen, Otkaz string
	Predel       int64
}

func (e *SertDomenaNeVyshel) Error() string {
	return "сертификат на " + e.Domen + " не выпущен: " + e.Otkaz
}

// SmenaRezhima это переход поставленного сервера в режим своего домена и
// обратно к простым ключам.
type SmenaRezhima struct {
	Puti Puti
	// Vhod: root ключом управления и Izvesten, ключ хоста из записи.
	Vhod ssh.Zapros
	Imya string
	// Domen это домен в ASCII; пусто - вернуться к простым ключам.
	Domen string
	// Routing это ссылка профиля Happ для заголовка подписки.
	Routing string
	// Put: путь подписки, который надо сохранить (домен переезжает с другого
	// своего сервера); пусто - новый.
	Put   string
	Opcii konfig.Opcii
	Hod   func(Hod)
}

// stoyashchiy это поставленный сервер, прочитанный для дела над ним.
type stoyashchiy struct {
	s     Svyaz
	adres netip.Addr
	sek   konfig.Sekrety
	vh    konfig.Vhody
	st    server.SostoyanieUzla
	opcii konfig.Opcii
}

// otkrytStoyashchiy входит на сервер, проверяет, что установка наша, и
// читает её пакет: сервер остаётся источником правды о ключах.
func otkrytStoyashchiy(ctx context.Context, puti Puti, vhod ssh.Zapros, imya string, opcii konfig.Opcii, itog *Itog, hod func(Hod)) (*stoyashchiy, error) {
	a, err := netip.ParseAddr(vhod.Adres)
	switch {
	case err != nil:
		return nil, fmt.Errorf("адрес %q не IP", ssh.Obrezat(vhod.Adres, 80))
	case vhod.Vhod.Podpis == nil:
		return nil, errors.New("не задан ключ управления")
	case imya == "":
		return nil, errors.New("нет имени сервера")
	}
	p := &stoyashchiy{adres: a.Unmap()}
	hod(Hod{Shag: ShagVhod})
	if p.s, err = Voyti(ctx, puti, vhod); err != nil {
		return nil, err
	}
	gotovo := false
	defer func() {
		if !gotovo {
			p.s.Zakryt()
		}
	}()
	itog.Vhod = p.s.Svedeniya()

	hod(Hod{Shag: ShagOsmotr})
	o, err := osmotret(ctx, p.s)
	if err != nil {
		return nil, err
	}
	for _, z := range o.Zamechaniya(seychas()) {
		if z.Kod == server.KodPodCentrom {
			return nil, &Ostanovka{Zamechaniya: []server.Zamechanie{z}}
		}
	}
	switch {
	case o.Otmetka == nil:
		return nil, server.ErrNetUstanovki
	case !nashaOtmetka(o.Otmetka, vhod.Vhod.Podpis.PublicKey()):
		return nil, ErrChuzhayaUstanovka
	}
	itog.ParolVyklyuchen = slices.Contains(o.SshdFayly, server.FaylParolya)

	sostav, paket, err := prochitatUstanovku(ctx, p.s)
	if err != nil {
		return nil, err
	}
	if len(paket) == 0 {
		return nil, ErrNeDoshla
	}
	if p.sek, p.vh, err = konfig.IzPaketa(paket, p.adres.String(), imya); err != nil {
		return nil, err
	}
	if p.st, err = sostoyanie(ctx, p.s); err != nil {
		return nil, err
	}
	itog.Sert, itog.Prodlenie = prodlenieStoyashchego(p.vh, p.st)
	itog.Versiya = nomerVersii(sostav.VpnUzel)
	p.opcii = opcii
	if p.st.Sert != nil {
		p.opcii.Staging = p.st.Sert.Staging
	}
	gotovo = true
	return p, nil
}

// SmenitRezhim переводит сервер в режим своего домена или возвращает к
// простым ключам. Порты и пароли прежние, меняются маска reality и имя в
// ключах, и ключи на телефоне надо добавить заново (в режиме домена одной
// подпиской). Ключи в набор кладёт служба. Повтор после обрыва, когда
// сервер уже в нужном режиме, пакет не трогает, а только отдаёт ключи.
//
// В режим домена переход идёт в два пакета: сначала прежние входы с
// выпуском сертификата на домен, и только когда он вышел, входы на домене и
// сайт. Reality всё это время работает, а hysteria2 нет: выпуск уже ждёт
// сертификата на домен, и на IP входу отдавать нечего. Любой отказ после
// первого пакета (сертификат не вышел, второй пакет не принят, сайт не
// поднялся) возвращает сервер к прежнему пакету.
func SmenitRezhim(ctx context.Context, p SmenaRezhima) (itog Itog, err error) {
	defer zashchita("SmenitRezhim", &err)
	hod := func(h Hod) {
		if p.Hod != nil {
			p.Hod(h)
		}
	}
	if p.Domen != "" && konfig.ProveritMasku(p.Domen) != nil {
		return Itog{}, fmt.Errorf("домен %q не годится", ssh.Obrezat(p.Domen, 80))
	}
	ps, err := otkrytStoyashchiy(ctx, p.Puti, p.Vhod, p.Imya, p.Opcii, &itog, hod)
	if err != nil {
		return Itog{}, err
	}
	defer ps.s.Zakryt()
	if !versiyaNeNizhe(ps.st.Versiya, versiyaSDomenom) {
		return Itog{}, ErrStarayaProgrammaDomena
	}
	sek, vh := ps.sek, ps.vh

	switch {
	case p.Domen != "" && vh.Sayt != nil && vh.Sayt.Domen == p.Domen:
		// Пакет на домене уже стоит (обрыв после него или откат не прошёл):
		// запись возьмёт его ключи, а молчащий сайт человек увидит.
		itog.MaskaIstochnik = MaskaDomen
		if err := saytVSostoyanii(ps.st.Sayt, p.Domen); err != nil {
			itog.Preduprezhdeniya = append(itog.Preduprezhdeniya, *preduprezhdenieIz(p.Domen, err))
		}
	case p.Domen != "" && vh.Sayt != nil:
		return Itog{}, &ErrDrugoyDomen{Domen: vh.Sayt.Domen}
	case p.Domen != "":
		if err := naDomen(ctx, ps, p.Domen, p.Routing, p.Put, &sek, &vh, &itog, hod); err != nil {
			return Itog{}, err
		}
	case vh.Sayt == nil:
		// Уже простые ключи: откуда маска, сервер не хранит.
	default:
		if err := naProstye(ctx, ps, &sek, &vh, &itog, hod); err != nil {
			return Itog{}, err
		}
	}
	klyuchi, err := konfig.Klyuchi(sek, vh)
	if err != nil {
		return Itog{}, err
	}
	itog.Adres, itog.Sekrety, itog.Vhody, itog.Klyuchi = ps.adres.String(), sek, vh, klyuchi
	return itog, nil
}

func naDomen(ctx context.Context, ps *stoyashchiy, domen, routing, put string, sek *konfig.Sekrety, vh *konfig.Vhody, itog *Itog, hod func(Hod)) error {
	// Пакет на домене собирается до первого шага: негодный (порт reality
	// занят сайтом) не трогает сервер вовсе.
	sayt, err := konfig.NovyySayt(domen, routing)
	if err != nil {
		return err
	}
	if put != "" {
		sayt.Put = put
	}
	novyeSek, novyeVh := *sek, *vh
	novyeSek.Maska, novyeVh.Sayt = domen, sayt
	paket, err := konfig.Paket(novyeSek, novyeVh, ps.opcii)
	if err != nil {
		return err
	}

	hod(Hod{Shag: ShagSertifikat})
	pered, err := konfig.PaketPeredSmenoy(*sek, *vh, domen, ps.opcii)
	if err != nil {
		return err
	}
	// otkatit возвращает прежний пакет: выпуск снова на IP, hy2 снова со
	// своим сертификатом. Отказ отката не важнее исходного, но без него
	// человек не узнал бы, что сервер ждёт не того сертификата.
	otkatit := func(err error) error {
		prezhniy, errP := konfig.Paket(*sek, *vh, ps.opcii)
		if errP == nil {
			_, errP = primenit(context.WithoutCancel(ctx), ps.s, prezhniy)
		}
		if errP != nil {
			return &OtkatNeVyshel{Err: err, Otkat: errP}
		}
		return err
	}
	pr, otkaz, predel, err := vypustitSert(ctx, ps.s, ps.adres, domen, pered, &itog.Sert)
	if err == nil && pr == porty.BezSerta {
		err = &SertDomenaNeVyshel{Domen: domen, Otkaz: otkaz, Predel: predel}
	}
	if err != nil {
		return otkatit(err)
	}

	hod(Hod{Shag: ShagPaket})
	if _, err := primenit(ctx, ps.s, paket); err != nil {
		return otkatit(err)
	}
	if err := saytPodnyalsya(ctx, ps.s, domen); err != nil {
		return otkatit(err)
	}
	*sek, *vh = novyeSek, novyeVh
	itog.Prodlenie, itog.MaskaIstochnik = pr, MaskaDomen
	return nil
}

// SaytNeVstal: пакет режима домена принят, а сайт на сервере не поднялся,
// и reality, чья цель он, не пускает никого.
type SaytNeVstal struct{ Prichina string }

func (e *SaytNeVstal) Error() string {
	return "сайт своего домена на сервере не поднялся: " + e.Prichina
}

// OtkatNeVyshel: переход на домен не удался (Err), и прежний пакет на
// сервер не вернулся (Otkat). Ключи в записи прежние, а сервер ждёт не их.
type OtkatNeVyshel struct{ Err, Otkat error }

func (e *OtkatNeVyshel) Error() string {
	return e.Err.Error() + " (прежний пакет не вернулся: " + e.Otkat.Error() + ")"
}

func (e *OtkatNeVyshel) Unwrap() error { return e.Err }

// saytPodnyalsya спрашивает программу сервера, поднят ли сайт на домене и
// есть ли ему чем ответить. Слушатели сайт поднимает при приёме пакета,
// поэтому ответ сразу после него уже правда.
func saytPodnyalsya(ctx context.Context, m server.Mashina, domen string) error {
	st, err := sostoyanie(ctx, m)
	if err != nil {
		return err
	}
	return saytVSostoyanii(st.Sayt, domen)
}

// saytVSostoyanii судит по уже полученному состоянию программы сервера.
func saytVSostoyanii(c *server.SaytUzla, domen string) error {
	switch {
	case c == nil || c.Domen != domen:
		return &SaytNeVstal{Prichina: "программа сервера о нём не знает"}
	case !c.Https:
		prichina := c.Oshibka
		if prichina == "" {
			prichina = "HTTPS не слушает"
		}
		return &SaytNeVstal{Prichina: prichina}
	case !c.Sert:
		return &SaytNeVstal{Prichina: "нет сертификата на домен"}
	}
	return nil
}

// preduprezhdenieSayta: пакет с сайтом уже на сервере, поэтому молчащий
// сайт не отказ, а предупреждение. Не вышло спросить программу сервера -
// это другое предупреждение: о сайте тогда ничего не известно.
func preduprezhdenieSayta(ctx context.Context, m server.Mashina, domen string) *Preduprezhdenie {
	if err := saytPodnyalsya(ctx, m, domen); err != nil {
		return preduprezhdenieIz(domen, err)
	}
	return nil
}

// preduprezhdenieIz переводит отказ saytPodnyalsya в предупреждение.
func preduprezhdenieIz(domen string, err error) *Preduprezhdenie {
	var ne *SaytNeVstal
	if errors.As(err, &ne) {
		return &Preduprezhdenie{Vid: SaytNeOtvetil, Domen: domen, Otkaz: ne.Prichina}
	}
	return &Preduprezhdenie{Vid: SaytNeProveren, Domen: domen, Otkaz: err.Error()}
}

// naProstye возвращает сервер к простым ключам: маска подбирается у соседей,
// сайт гаснет, сертификат снова выпускается на IP. Пока он не вышел, hy2 не
// пускает: имя в его новых ключах это IP.
func naProstye(ctx context.Context, ps *stoyashchiy, sek *konfig.Sekrety, vh *konfig.Vhody, itog *Itog, hod func(Hod)) error {
	hod(Hod{Shag: ShagMaska})
	mk, err := podobratMasku(ctx, ps.s, ps.adres, sek.Maska)
	if err != nil {
		return err
	}
	sek.Maska, vh.Sayt = mk.Imya, nil
	itog.MaskaIstochnik = mk.Istochnik

	hod(Hod{Shag: ShagPaket})
	paket, err := konfig.Paket(*sek, *vh, ps.opcii)
	if err != nil {
		return err
	}
	if len(vh.Hy2) == 0 {
		if _, err := primenit(ctx, ps.s, paket); err != nil {
			return err
		}
		itog.Sert, itog.Prodlenie = nil, porty.BezSerta
		return nil
	}
	hod(Hod{Shag: ShagSertifikat})
	pr, otkaz, predel, err := vypustitSert(ctx, ps.s, ps.adres, "", paket, &itog.Sert)
	if err != nil {
		return err
	}
	if itog.Prodlenie = pr; pr == porty.BezSerta {
		// hy2 в пакете с выпуском: сертификат программа получит сама.
		itog.Preduprezhdeniya = append(itog.Preduprezhdeniya, Preduprezhdenie{Vid: NetSerta, Otkaz: otkaz, Predel: predel, Hy2Zhdyot: true})
	}
	return nil
}

// SmenaPodpiski это новое содержимое подписки на сервере режима домена:
// правила Happ, имя сервера или новый путь.
type SmenaPodpiski struct {
	Puti Puti
	Vhod ssh.Zapros
	// Imya это имя сервера: оно идёт в название профиля и в ключи подписки.
	Imya string
	// Routing: новая ссылка профиля Happ; nil - прежняя.
	Routing *string
	// NovyyPut: выдать новую ссылку подписки, прежняя перестаёт отвечать.
	NovyyPut bool
	Opcii    konfig.Opcii
	Hod      func(Hod)
}

// ObnovitPodpisku пересобирает подписку на сервере режима домена. Ядро
// прежнее: программа меняет только ответ сайта, соединения не рвутся.
func ObnovitPodpisku(ctx context.Context, p SmenaPodpiski) (itog Itog, err error) {
	defer zashchita("ObnovitPodpisku", &err)
	hod := func(h Hod) {
		if p.Hod != nil {
			p.Hod(h)
		}
	}
	ps, err := otkrytStoyashchiy(ctx, p.Puti, p.Vhod, p.Imya, p.Opcii, &itog, hod)
	if err != nil {
		return Itog{}, err
	}
	defer ps.s.Zakryt()
	sek, vh := ps.sek, ps.vh
	if vh.Sayt == nil {
		return Itog{}, ErrProstoyRezhim
	}
	sayt := *vh.Sayt
	if p.Routing != nil {
		sayt.Routing = *p.Routing
	}
	if p.NovyyPut {
		n, err := sayt.NovyyPut()
		if err != nil {
			return Itog{}, err
		}
		sayt = *n
	}
	vh.Sayt = &sayt

	hod(Hod{Shag: ShagPaket})
	paket, err := konfig.Paket(sek, vh, ps.opcii)
	if err != nil {
		return Itog{}, err
	}
	if _, err := primenit(ctx, ps.s, paket); err != nil {
		return Itog{}, err
	}
	// Пакет принят: на сервере уже новая подписка, и запись обязана её
	// знать.
	if pr := preduprezhdenieSayta(ctx, ps.s, sayt.Domen); pr != nil {
		itog.Preduprezhdeniya = append(itog.Preduprezhdeniya, *pr)
	}
	klyuchi, err := konfig.Klyuchi(sek, vh)
	if err != nil {
		return Itog{}, err
	}
	itog.Adres, itog.Sekrety, itog.Vhody, itog.Klyuchi = ps.adres.String(), sek, vh, klyuchi
	itog.MaskaIstochnik = MaskaDomen
	return itog, nil
}

// versiyaNeNizhe: версия программы вида «0.7.0» не старше nuzhna.
// Нечитаемая версия считается старой.
func versiyaNeNizhe(v string, nuzhna [3]int) bool {
	chasti := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".", 3)
	if len(chasti) != 3 {
		return false
	}
	var n [3]int
	for i, c := range chasti {
		// «0.7.0-rc1»: хвост после цифр не меняет номера.
		c, _, _ = strings.Cut(c, "-")
		x, err := strconv.Atoi(c)
		if err != nil {
			return false
		}
		n[i] = x
	}
	for i := range n {
		if n[i] != nuzhna[i] {
			return n[i] > nuzhna[i]
		}
	}
	return true
}
