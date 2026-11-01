package petlya

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Сертификаты на лету.
//
// Транспорты с TLS (hysteria2, trojan, anytls, tuic, ws+tls) без сертификата не
// поднимаются вовсе, а класть в репозиторий файл с ключом нельзя: он протухает
// по сроку и однажды роняет всю приёмку в дату, к продукту отношения не имеющую.
// Поэтому корень и лист рождаются в каталоге теста и умирают вместе с ним, как
// mkcert.go у самого sing-box.
type Sertifikat struct {
	// Imya это SNI, с которым клиент придёт на сервер.
	Imya string
	// PutSert и PutKlyuch отдаются СЕРВЕРНОМУ ядру.
	PutSert   string
	PutKlyuch string
	// CaPEM отдаётся КЛИЕНТСКОМУ ядру полем tls.certificate. Своего корня в
	// хранилище машины мы не заводим: испортить доверие рабочей машины ради
	// теста недопустимо, а поле в конфиге живёт ровно один прогон.
	CaPEM string
	// Pin это base64 от sha256 открытого ключа ЛИСТА, в написании поля
	// certificate_public_key_sha256 и параметра ссылки pinPubKeySHA256.
	Pin string
}

// NovyySertifikat выдаёт свежую пару корень плюс лист на каждый вызов.
func NovyySertifikat(t *testing.T, imya string) Sertifikat {
	t.Helper()

	kornevoyKlyuch := novyyKlyuch(t)
	koren := &x509.Certificate{
		SerialNumber:          nomer(t),
		Subject:               pkix.Name{CommonName: "affory petlya CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	kornevoyDER := podpisat(t, koren, koren, &kornevoyKlyuch.PublicKey, kornevoyKlyuch)
	kornevoyRazobran, err := x509.ParseCertificate(kornevoyDER)
	if err != nil {
		t.Fatalf("корень не разобрался: %v", err)
	}

	listovoyKlyuch := novyyKlyuch(t)
	list := &x509.Certificate{
		SerialNumber: nomer(t),
		Subject:      pkix.Name{CommonName: imya},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{imya},
		// Петля в SAN обязательна: сервер стоит на 127.0.0.1, и часть проверок
		// идёт по адресу, а не по имени.
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		BasicConstraintsValid: true,
	}
	// Сертификат на адрес, как у Let's Encrypt на IP: адрес идёт в SAN адресом,
	// а не именем, иначе проверка его не узнает.
	if ip := net.ParseIP(imya); ip != nil {
		list.DNSNames = nil
		list.IPAddresses = append(list.IPAddresses, ip)
	}
	listovoyDER := podpisat(t, list, kornevoyRazobran, &listovoyKlyuch.PublicKey, kornevoyKlyuch)
	listRazobran, err := x509.ParseCertificate(listovoyDER)
	if err != nil {
		t.Fatalf("лист не разобрался: %v", err)
	}

	rab := t.TempDir()
	putSert := filepath.Join(rab, "list.pem")
	putKlyuch := filepath.Join(rab, "list.key")
	zapisat(t, putSert, "CERTIFICATE", listovoyDER)
	zapisatKlyuch(t, putKlyuch, listovoyKlyuch)

	summa := sha256.Sum256(listRazobran.RawSubjectPublicKeyInfo)
	return Sertifikat{
		Imya:      imya,
		PutSert:   putSert,
		PutKlyuch: putKlyuch,
		CaPEM:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: kornevoyDER})),
		Pin:       base64.StdEncoding.EncodeToString(summa[:]),
	}
}

func novyyKlyuch(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ключ не сгенерился: %v", err)
	}
	return k
}

func nomer(t *testing.T) *big.Int {
	t.Helper()
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("серийный номер не сгенерился: %v", err)
	}
	return n
}

func podpisat(t *testing.T, chto, kem *x509.Certificate, otkrytyy any, klyuch *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, chto, kem, otkrytyy, klyuch)
	if err != nil {
		t.Fatalf("сертификат не подписался: %v", err)
	}
	return der
}

func zapisat(t *testing.T, put, tip string, der []byte) {
	t.Helper()
	if err := os.WriteFile(put, pem.EncodeToMemory(&pem.Block{Type: tip, Bytes: der}), 0o600); err != nil {
		t.Fatalf("%s не записался: %v", put, err)
	}
}

func zapisatKlyuch(t *testing.T, put string, k *ecdsa.PrivateKey) {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		t.Fatalf("ключ не сериализовался: %v", err)
	}
	zapisat(t, put, "EC PRIVATE KEY", der)
}
