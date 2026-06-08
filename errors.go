package fluvpay

import (
	"encoding/json"
	"fmt"
)

// ErrorDetail descreve um problema pontual de validação retornado pela API,
// normalmente apontando o campo que falhou.
type ErrorDetail struct {
	// Field é o nome do campo que causou o erro, quando aplicável.
	Field *string `json:"field,omitempty"`
	// Message é a descrição legível do problema, em PT-BR.
	Message string `json:"message"`
	// Type é o tipo canônico do erro de validação, quando informado.
	Type *string `json:"type,omitempty"`
}

// Error é a interface implementada por todas as falhas levantadas pelo SDK.
// Use errors.As para destrinchar o tipo concreto (por exemplo, *ValidationError)
// e ler Code, Details, TraceID e StatusCode.
type Error interface {
	error
	// Code devolve o código canônico do erro (ex: VALIDATION_ERROR, NOT_FOUND).
	Code() string
	// Details devolve a lista de detalhes de validação, quando houver.
	Details() []ErrorDetail
	// TraceID devolve o ID de correlação da requisição nos logs do servidor.
	TraceID() string
	// StatusCode devolve o status HTTP da resposta. Zero em erros de conexão.
	StatusCode() int
}

// BaseError é a estrutura comum embutida em todas as exceções tipadas do SDK.
// Carrega os campos extraídos do envelope { error: { code, message, details,
// trace_id } } e implementa a interface Error.
type BaseError struct {
	// Msg é a mensagem legível, em PT-BR quando vinda da API.
	Msg string
	// ErrCode é o código canônico do erro retornado pela API.
	ErrCode string
	// ErrDetails são os detalhes de validação, quando a API os fornece.
	ErrDetails []ErrorDetail
	// Trace é o ID de correlação para suporte.
	Trace string
	// Status é o status HTTP, quando a falha veio de uma resposta.
	Status int
	// Cause é o erro original que originou esta exceção (ex: falha de rede).
	Cause error
}

// Error implementa a interface error padrão da linguagem.
func (e *BaseError) Error() string { return e.Msg }

// Code devolve o código canônico do erro.
func (e *BaseError) Code() string { return e.ErrCode }

// Details devolve os detalhes de validação.
func (e *BaseError) Details() []ErrorDetail { return e.ErrDetails }

// TraceID devolve o ID de correlação da requisição.
func (e *BaseError) TraceID() string { return e.Trace }

// StatusCode devolve o status HTTP da resposta.
func (e *BaseError) StatusCode() int { return e.Status }

// Unwrap expõe o erro subjacente para uso com errors.Is e errors.As.
func (e *BaseError) Unwrap() error { return e.Cause }

// ValidationError corresponde a 400/422: dados inválidos ou estado impeditivo
// (ex: INSUFFICIENT_BALANCE).
type ValidationError struct{ BaseError }

// AuthenticationError corresponde a 401: autenticação obrigatória ou chave
// inválida.
type AuthenticationError struct{ BaseError }

// PermissionError corresponde a 403: escopo insuficiente ou operação não
// permitida (inclui operações não suportadas em sandbox).
type PermissionError struct{ BaseError }

// NotFoundError corresponde a 404: recurso não encontrado.
type NotFoundError struct{ BaseError }

// ConflictError corresponde a 409: conflito (inclui IDEMPOTENCY_CONFLICT).
type ConflictError struct{ BaseError }

// RateLimitError corresponde a 429: limite de requisições excedido.
type RateLimitError struct {
	BaseError
	// RetryAfter são os segundos a aguardar antes de tentar de novo, lidos do
	// header Retry-After. Zero quando o header não foi enviado.
	RetryAfter int
}

// ServerError corresponde a 5xx: erro interno do servidor.
type ServerError struct{ BaseError }

// ConnectionError representa falha de rede, timeout ou interrupção antes de
// obter uma resposta HTTP.
type ConnectionError struct{ BaseError }

// SignatureVerificationError indica que a verificação da assinatura de um
// webhook falhou.
type SignatureVerificationError struct{ BaseError }

// errorEnvelope é a forma do corpo de erro retornado pela API.
type errorEnvelope struct {
	Error struct {
		Code    string        `json:"code"`
		Message string        `json:"message"`
		Details []ErrorDetail `json:"details"`
		TraceID string        `json:"trace_id"`
	} `json:"error"`
}

// errorFromResponse mapeia um status HTTP e o corpo do erro para a exceção
// tipada correta. Lê o envelope { error: { code, message, details, trace_id } }
// quando presente; caso contrário usa um fallback legível pelo status.
func errorFromResponse(statusCode int, rawBody []byte, retryAfter int) error {
	var env errorEnvelope
	if len(rawBody) > 0 {
		_ = json.Unmarshal(rawBody, &env)
	}

	msg := env.Error.Message
	if msg == "" {
		msg = fmt.Sprintf("Requisição falhou com status HTTP %d", statusCode)
	}

	base := BaseError{
		Msg:        msg,
		ErrCode:    env.Error.Code,
		ErrDetails: env.Error.Details,
		Trace:      env.Error.TraceID,
		Status:     statusCode,
	}

	switch {
	case statusCode == 400 || statusCode == 422:
		return &ValidationError{base}
	case statusCode == 401:
		return &AuthenticationError{base}
	case statusCode == 403:
		return &PermissionError{base}
	case statusCode == 404:
		return &NotFoundError{base}
	case statusCode == 409:
		return &ConflictError{base}
	case statusCode == 429:
		return &RateLimitError{BaseError: base, RetryAfter: retryAfter}
	case statusCode >= 500:
		return &ServerError{base}
	default:
		return &BaseError{
			Msg:        msg,
			ErrCode:    env.Error.Code,
			ErrDetails: env.Error.Details,
			Trace:      env.Error.TraceID,
			Status:     statusCode,
		}
	}
}
