package fluvpay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WebhookEvent é o evento já parseado a partir do corpo de uma entrega de
// webhook verificada. Type é um dos eventos do catálogo (charge.created,
// charge.paid, charge.expired, charge.cancelled, charge.refunded,
// payout.created, payout.completed, payout.failed). Data é o objeto bruto do
// evento, deixado como map para você decodificar conforme a sua necessidade.
type WebhookEvent struct {
	// Type é o tipo do evento (ex: charge.paid).
	Type string `json:"type"`
	// Data é o conteúdo do evento, mantido como objeto livre.
	Data map[string]any `json:"data"`
	// Raw é o corpo cru recebido, preservado para inspeção ou reprocessamento.
	Raw []byte `json:"-"`
}

// VerifyParams reúne os dados necessários para verificar a assinatura de um
// webhook. Use o corpo CRU da requisição: nunca re-serialize o JSON, pois isso
// altera os bytes e invalida a assinatura.
type VerifyParams struct {
	// Payload é o corpo cru da requisição, exatamente como recebido.
	Payload []byte
	// SignatureHeader é o valor do header X-FluvPay-Signature, no formato v1=<hex>.
	SignatureHeader string
	// Timestamp é o valor do header X-FluvPay-Timestamp.
	Timestamp string
	// Secret é o segredo do webhook (whsec_...) exibido na sua criação.
	Secret string
	// ToleranceSeconds, se maior que zero e com Timestamp numérico, rejeita
	// entregas mais antigas que essa janela. Zero desliga a checagem de tempo.
	ToleranceSeconds int
}

// webhooks é o ponto de acesso ao helper de verificação de assinatura. Como a
// verificação não depende de credenciais nem de rede, é exposto como singleton
// de pacote: use fluvpay.Webhooks.VerifySignature(...).
type webhooksHelper struct{}

// Webhooks expõe o helper de verificação de assinatura de webhooks.
var Webhooks = webhooksHelper{}

// VerifySignature confere a assinatura de uma entrega de webhook e devolve o
// evento parseado. A assinatura esperada é
// HMAC_SHA256(secret, timestamp + "." + payload) em hexadecimal, transportada
// no header X-FluvPay-Signature no formato v1=<hex>. A comparação é feita em
// tempo constante. Se ToleranceSeconds for informado e o timestamp for
// numérico, entregas velhas demais são recusadas. Em qualquer falha retorna um
// *SignatureVerificationError.
func (webhooksHelper) VerifySignature(params VerifyParams) (*WebhookEvent, error) {
	if params.Secret == "" {
		return nil, signatureError("Segredo do webhook não informado.")
	}
	if params.Timestamp == "" {
		return nil, signatureError("Header de timestamp ausente.")
	}

	provided, err := extractSignature(params.SignatureHeader)
	if err != nil {
		return nil, err
	}

	mac := hmac.New(sha256.New, []byte(params.Secret))
	mac.Write([]byte(params.Timestamp))
	mac.Write([]byte("."))
	mac.Write(params.Payload)
	expected := mac.Sum(nil)

	providedBytes, decErr := hex.DecodeString(provided)
	if decErr != nil || !hmac.Equal(providedBytes, expected) {
		return nil, signatureError("Assinatura do webhook inválida.")
	}

	if params.ToleranceSeconds > 0 {
		if ts, convErr := strconv.ParseInt(strings.TrimSpace(params.Timestamp), 10, 64); convErr == nil {
			age := time.Now().Unix() - ts
			if age < 0 {
				age = -age
			}
			if age > int64(params.ToleranceSeconds) {
				return nil, signatureError(fmt.Sprintf("Timestamp do webhook fora da tolerância de %d segundos.", params.ToleranceSeconds))
			}
		}
	}

	event := &WebhookEvent{Raw: params.Payload}
	if len(params.Payload) > 0 {
		if jsonErr := json.Unmarshal(params.Payload, event); jsonErr != nil {
			return nil, signatureError(fmt.Sprintf("Falha ao decodificar o corpo do webhook: %v", jsonErr))
		}
	}
	return event, nil
}

// extractSignature lê o hexadecimal após o prefixo "v1=" do header de
// assinatura, suportando múltiplos esquemas separados por vírgula.
func extractSignature(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", signatureError("Header de assinatura ausente.")
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "v1=") {
			value := strings.TrimSpace(strings.TrimPrefix(part, "v1="))
			if value == "" {
				return "", signatureError("Header de assinatura sem valor v1.")
			}
			return value, nil
		}
	}
	return "", signatureError("Header de assinatura sem esquema v1.")
}

// signatureError monta um *SignatureVerificationError com a mensagem dada.
func signatureError(msg string) *SignatureVerificationError {
	return &SignatureVerificationError{BaseError{
		Msg:     msg,
		ErrCode: "SIGNATURE_VERIFICATION_FAILED",
	}}
}
