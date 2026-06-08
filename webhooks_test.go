package fluvpay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"
)

const (
	vectorSecret    = "whsec_test_secret_123"
	vectorTimestamp = "1718000000"
	vectorBody      = `{"type":"charge.paid","data":{"id":"chg_123","status":"paid"}}`
	// vectorSignature foi pré-computado:
	// HMAC_SHA256("whsec_test_secret_123", "1718000000." + vectorBody) em hex.
	vectorSignature = "7205312c44f9a81ec49de54fc7237e59e9ac90465c2f3ec86678858596f89d4e"
)

func TestVerifySignatureKnownVector(t *testing.T) {
	event, err := Webhooks.VerifySignature(VerifyParams{
		Payload:         []byte(vectorBody),
		SignatureHeader: "v1=" + vectorSignature,
		Timestamp:       vectorTimestamp,
		Secret:          vectorSecret,
	})
	if err != nil {
		t.Fatalf("assinatura válida foi rejeitada: %v", err)
	}
	if event.Type != "charge.paid" {
		t.Errorf("tipo de evento incorreto: %q", event.Type)
	}
	if event.Data["id"] != "chg_123" {
		t.Errorf("data do evento incorreto: %+v", event.Data)
	}
	if string(event.Raw) != vectorBody {
		t.Error("o corpo cru deveria ser preservado em Raw")
	}
}

func TestVerifySignatureTamperedBody(t *testing.T) {
	_, err := Webhooks.VerifySignature(VerifyParams{
		Payload:         []byte(vectorBody + " "),
		SignatureHeader: "v1=" + vectorSignature,
		Timestamp:       vectorTimestamp,
		Secret:          vectorSecret,
	})
	var serr *SignatureVerificationError
	if !errors.As(err, &serr) {
		t.Fatalf("corpo adulterado deveria falhar com *SignatureVerificationError, recebeu %T (%v)", err, err)
	}
}

func TestVerifySignatureWrongSecret(t *testing.T) {
	_, err := Webhooks.VerifySignature(VerifyParams{
		Payload:         []byte(vectorBody),
		SignatureHeader: "v1=" + vectorSignature,
		Timestamp:       vectorTimestamp,
		Secret:          "whsec_outro_segredo",
	})
	var serr *SignatureVerificationError
	if !errors.As(err, &serr) {
		t.Fatalf("segredo errado deveria falhar, recebeu %T (%v)", err, err)
	}
}

func TestVerifySignatureMissingScheme(t *testing.T) {
	_, err := Webhooks.VerifySignature(VerifyParams{
		Payload:         []byte(vectorBody),
		SignatureHeader: vectorSignature,
		Timestamp:       vectorTimestamp,
		Secret:          vectorSecret,
	})
	var serr *SignatureVerificationError
	if !errors.As(err, &serr) {
		t.Fatalf("header sem esquema v1 deveria falhar, recebeu %T (%v)", err, err)
	}
}

func TestVerifySignatureToleranceRejectsOld(t *testing.T) {
	_, err := Webhooks.VerifySignature(VerifyParams{
		Payload:          []byte(vectorBody),
		SignatureHeader:  "v1=" + vectorSignature,
		Timestamp:        vectorTimestamp,
		Secret:           vectorSecret,
		ToleranceSeconds: 300,
	})
	var serr *SignatureVerificationError
	if !errors.As(err, &serr) {
		t.Fatalf("timestamp antigo dentro de tolerância curta deveria falhar, recebeu %T (%v)", err, err)
	}
}

func TestVerifySignatureToleranceAcceptsFresh(t *testing.T) {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	body := `{"type":"charge.created","data":{}}`
	sig := computeSignatureForTest(t, vectorSecret, now, body)
	event, err := Webhooks.VerifySignature(VerifyParams{
		Payload:          []byte(body),
		SignatureHeader:  "v1=" + sig,
		Timestamp:        now,
		Secret:           vectorSecret,
		ToleranceSeconds: 300,
	})
	if err != nil {
		t.Fatalf("timestamp recente deveria passar: %v", err)
	}
	if event.Type != "charge.created" {
		t.Errorf("tipo incorreto: %q", event.Type)
	}
}

// computeSignatureForTest reaproveita o mesmo algoritmo do helper para gerar uma
// assinatura válida em testes que precisam de timestamp dinâmico.
func computeSignatureForTest(t *testing.T, secret, timestamp, body string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}
