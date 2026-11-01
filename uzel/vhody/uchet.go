// Пакет vhody держит наши копии четырёх входов sing-box (vless, trojan,
// anytls, hysteria2) и учёт их живых соединений.
//
// Зачем копии: у служб этих протоколов есть замена списка людей на лету, но
// входы sing-box доступа к службам не дают. Без копий любая смена состава это
// перезапуск ядра и обрыв всех. Копии отличаются от исходников sing-box ровно
// тремя вещами: состав меняется методом Smenit, каждое соединение заносится в
// учёт под своим человеком, отозванному учёт закрывает соединения сразу.
// Отличия в коде помечены комментарием «наше:».
package vhody

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/sagernet/sing/common"
)

// Klyuch это то, под каким именем человек живёт внутри службы протокола:
// имя плюс отпечаток его секрета.
//
// Голого имени мало. Службы проверяют пароль один раз на соединение, а дальше
// помнят только имя. Смени человеку пароль, и соединение, открытое старым
// паролем, по одному имени выглядело бы законным. С отпечатком в ключе смена
// пароля для учёта то же самое, что отзыв, и старые соединения закрываются.
func Klyuch(imya, sekret string) string {
	summa := sha256.Sum256([]byte(sekret))
	return imya + "\x00" + hex.EncodeToString(summa[:8])
}

// ImyaIz достаёт имя человека из ключа.
func ImyaIz(klyuch string) string {
	if i := strings.IndexByte(klyuch, 0); i >= 0 {
		return klyuch[:i]
	}
	return klyuch
}

var errOtozvan = errors.New("доступ отозван")

// Виды отметок входа. Это слова журнала, центр хранит их как есть.
const (
	// VidVhod: соединение прошло проверку ключа, имя известно.
	VidVhod = "vhod"
	// VidBezVhoda: соединение дошло до службы протокола и закрылось, так и не
	// предъявив годного ключа. Чужой ключ, сканер, оборванное рукопожатие
	// протокола. У hy2 сюда попадает и QUIC-соединение, прошедшее проверку,
	// но не открывшее ни одного потока; клиенты так не делают.
	VidBezVhoda = "bez-vhoda"
	// VidOtozvan: ключ был годен, но отозван.
	VidOtozvan = "otozvan"
	// VidTLSSrok: TLS не сошёлся по сроку. Так выглядит провайдер, который
	// глушит соединение после приветствия клиента.
	VidTLSSrok = "tls-srok"
	// VidTLSObryv: TLS оборван, соединение закрыли или сбросили посреди
	// рукопожатия.
	VidTLSObryv = "tls-obryv"
	// VidTLS: TLS не сошёлся по другой причине (чужое имя, проба reality).
	VidTLS = "tls"
	// VidPriem: принят сокет транспорта (gRPC). Его TLS идёт внутри
	// транспорта и сюда не виден, поэтому отмечается сам приход.
	VidPriem = "priem"
)

// Otmetchik копит отметки входов. Живёт дольше ядра, как счётчик трафика.
type Otmetchik interface {
	Otmetit(vhod, adres, imya, vid string)
}

// Zapis это одно сырое соединение клиента: TCP после TLS, поток gRPC или
// QUIC-соединение hy2. Его закрытие рвёт всё, что внутри: мультиплекс,
// сессию anytls, потоки hy2.
type Zapis struct {
	zakryt io.Closer
	vhod   string // вход, на котором соединение принято
	adres  string // адрес клиента без порта

	// Поля ниже под Uchet.mu.
	teg      string
	klyuch   string
	ssylok   int
	otozvana bool
}

// Uchet знает, какие сырые соединения сейчас открыты и чьи они. Один на ядро.
type Uchet struct {
	mu      sync.Mutex
	dopusk  map[string]map[string]struct{}            // вход -> разрешённые ключи
	zhivye  map[string]map[string]map[*Zapis]struct{} // вход -> ключ -> записи
	otmetki Otmetchik                                 // nil у проверочного ядра
}

func NovyyUchet(otmetki Otmetchik) *Uchet {
	return &Uchet{
		dopusk:  make(map[string]map[string]struct{}),
		zhivye:  make(map[string]map[string]map[*Zapis]struct{}),
		otmetki: otmetki,
	}
}

// Otmetit добавляет отметку входа, если учёту есть куда их копить.
func (u *Uchet) Otmetit(vhod, adres, imya, vid string) {
	if u.otmetki != nil {
		u.otmetki.Otmetit(vhod, adres, imya, vid)
	}
}

// Novaya заводит запись на сырое соединение до того, как известно, чьё оно.
// Вызывающий держит одну ссылку и обязан отпустить её через Otpustit.
func (u *Uchet) Novaya(zakryt io.Closer, vhod, adres string) *Zapis {
	return &Zapis{zakryt: zakryt, vhod: vhod, adres: adres, ssylok: 1}
}

// Privyazat отмечает, что по записи пошёл трафик человека klyuch, и берёт на
// неё ещё одну ссылку. Отказ значит, что ключ уже не допущен или запись
// закрыта отзывом: соединение надо закрыть, а не обслуживать.
//
// Проверка допуска стоит здесь, под тем же замком, что и отзыв, а не только в
// службе протокола. Иначе соединение, прошедшее проверку пароля за миг до
// отзыва, успело бы занести себя в учёт уже после того, как отзыв этот учёт
// обошёл, и осталось бы жить.
func (u *Uchet) Privyazat(z *Zapis, teg, klyuch string) (otpustit func(), ok bool) {
	if z == nil {
		return nil, false
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if z.otozvana {
		return nil, false
	}
	if _, est := u.dopusk[teg][klyuch]; !est {
		// Вызывающий закроет всё сырое соединение, так что запись больше не
		// впустит никого. Отметка одна на соединение, а не на каждый поток.
		if z.klyuch == "" {
			z.otozvana = true
			u.Otmetit(z.vhod, z.adres, ImyaIz(klyuch), VidOtozvan)
		}
		return nil, false
	}
	if z.klyuch == "" {
		z.teg, z.klyuch = teg, klyuch
		u.Otmetit(z.vhod, z.adres, ImyaIz(klyuch), VidVhod)
		poKlyuchu := u.zhivye[teg]
		if poKlyuchu == nil {
			poKlyuchu = make(map[string]map[*Zapis]struct{})
			u.zhivye[teg] = poKlyuchu
		}
		zapisi := poKlyuchu[klyuch]
		if zapisi == nil {
			zapisi = make(map[*Zapis]struct{})
			poKlyuchu[klyuch] = zapisi
		}
		zapisi[z] = struct{}{}
	} else if z.teg != teg || z.klyuch != klyuch {
		// Одно сырое соединение проходит проверку пароля один раз, второго
		// хозяина у него не бывает. Если бывает, это чужая ошибка, и
		// обслуживать такое соединение нельзя.
		return nil, false
	}
	z.ssylok++
	var once sync.Once
	return func() { once.Do(func() { u.Otpustit(z) }) }, true
}

// Otpustit снимает ссылку. Последняя снятая убирает запись из учёта.
func (u *Uchet) Otpustit(z *Zapis) {
	u.mu.Lock()
	defer u.mu.Unlock()
	z.ssylok--
	if z.ssylok > 0 {
		return
	}
	if z.klyuch == "" {
		// Закрылось, так и не войдя. Отозванная запись уже отмечена отзывом.
		if !z.otozvana {
			u.Otmetit(z.vhod, z.adres, "", VidBezVhoda)
		}
		return
	}
	u.ubrat(z)
}

func (u *Uchet) ubrat(z *Zapis) {
	poKlyuchu := u.zhivye[z.teg]
	zapisi := poKlyuchu[z.klyuch]
	delete(zapisi, z)
	if len(zapisi) == 0 {
		delete(poKlyuchu, z.klyuch)
	}
	if len(poKlyuchu) == 0 {
		delete(u.zhivye, z.teg)
	}
}

// Dopustit ставит входу новый список ключей и отдаёт соединения тех, кого в
// нём больше нет. Закрывать их вызывающий должен сам и вне замка: закрытие
// QUIC ждёт, пока уйдёт пакет закрытия.
func (u *Uchet) Dopustit(teg string, klyuchi []string) []io.Closer {
	novyy := make(map[string]struct{}, len(klyuchi))
	for _, k := range klyuchi {
		novyy[k] = struct{}{}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.dopusk[teg] = novyy
	var zakryt []io.Closer
	for klyuch, zapisi := range u.zhivye[teg] {
		if _, est := novyy[klyuch]; est {
			continue
		}
		for z := range zapisi {
			z.otozvana = true
			u.Otmetit(z.vhod, z.adres, ImyaIz(klyuch), VidOtozvan)
			zakryt = append(zakryt, z.zakryt)
		}
		delete(u.zhivye[teg], klyuch)
	}
	if len(u.zhivye[teg]) == 0 {
		delete(u.zhivye, teg)
	}
	return zakryt
}

// Zhivyh отдаёт число открытых сырых соединений по входам.
func (u *Uchet) Zhivyh() map[string]int {
	u.mu.Lock()
	defer u.mu.Unlock()
	itog := make(map[string]int, len(u.zhivye))
	for teg, poKlyuchu := range u.zhivye {
		for _, zapisi := range poKlyuchu {
			itog[teg] += len(zapisi)
		}
	}
	return itog
}

// Zakryt закрывает соединения параллельно и ждёт всех. Отдаёт, сколько
// закрыто.
func Zakryt(spisok []io.Closer) int {
	var wg sync.WaitGroup
	for _, c := range spisok {
		wg.Add(1)
		go func() {
			defer wg.Done()
			zakrytSoSbrosom(c)
		}()
	}
	wg.Wait()
	return len(spisok)
}

// zakrytSoSbrosom рвёт соединение сразу, без дочитывания.
//
// Обычное закрытие TCP вежливое: ядро сперва досылает всё, что лежит в
// буфере отправки, и только потом FIN. На живой пробе 15.10 отозванный
// после этого качал ещё 28 секунд, несколько мегабайт из буферов. Сброс
// (SO_LINGER 0) выбрасывает буфер и шлёт RST.
//
// Голый сокет закрывается ПЕРВЫМ, обёртка потом. Закрытие TLS и reality
// сперва шлёт close_notify и ждёт до 5 секунд, если канал забит медленным
// читателем: так на 188 применение отзыва шло 5 секунд, а соединение всё это
// время жило. До сокета под TLS и reality sing доходит сам, по цепочке
// Upstream/NetConn. У QUIC и потока gRPC своего сокета нет, у них обычное
// закрытие, и оно у них мгновенное.
func zakrytSoSbrosom(c io.Closer) {
	if tcp, ok := common.Cast[*net.TCPConn](c); ok {
		_ = tcp.SetLinger(0) // не встал сброс - останется вежливое закрытие, это не повод не закрывать
		_ = tcp.Close()      // сокет мог уже закрыться сам, это не ошибка
	}
	_ = c.Close() // соединение могло уже закрыться само, это не ошибка
}
