package fluvpay

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// WithdrawalsService agrupa as operações de saque PIX. Saques são live-only:
// chaves fluv_test_ recebem 403.
type WithdrawalsService struct {
	client *Client
}

// WithdrawalCreateParams são os campos aceitos ao criar um saque PIX.
type WithdrawalCreateParams struct {
	// AmountCents é o valor bruto em centavos. Obrigatório, entre 100 e 10000000.
	AmountCents int `json:"amount_cents"`
	// PixKey é a chave PIX de destino (1 a 140 caracteres). Obrigatória.
	PixKey string `json:"pix_key"`
	// PixKeyType é o tipo da chave: cpf, cnpj, email, phone ou evp. Obrigatório.
	PixKeyType string `json:"pix_key_type"`
	// Description é uma descrição livre opcional (até 140 caracteres).
	Description *string `json:"description,omitempty"`
}

// Withdrawal representa os detalhes de uma solicitação de saque.
type Withdrawal struct {
	ID            string         `json:"id"`
	Status        string         `json:"status"`
	AmountCents   int            `json:"amount_cents"`
	FeeCents      int            `json:"fee_cents"`
	NetCents      int            `json:"net_cents"`
	PixKey        string         `json:"pix_key"`
	PixKeyType    string         `json:"pix_key_type"`
	Description   *string        `json:"description,omitempty"`
	CreatedAt     string         `json:"created_at"`
	CompletedAt   *string        `json:"completed_at,omitempty"`
	FailureReason *string        `json:"failure_reason,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// WithdrawalListParams são os filtros e a paginação aceitos por List.
type WithdrawalListParams struct {
	// Limit é a quantidade por página (máx 100). Zero usa o padrão do servidor.
	Limit int
	// Offset é o deslocamento (0-based). Padrão zero.
	Offset int
	// Status filtra por estado (pending, processing, completed, failed).
	Status string
}

// WithdrawalList é o envelope de saques (paginação por limit/offset).
type WithdrawalList struct {
	Data   []Withdrawal `json:"data"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
	Total  int          `json:"total"`
}

// Create cria um saque PIX. A Idempotency-Key é gerada automaticamente
// (UUIDv4) quando opts é nil ou não traz uma chave própria.
func (s *WithdrawalsService) Create(ctx context.Context, params WithdrawalCreateParams, opts *RequestOptions) (*Withdrawal, error) {
	key := s.client.resolveIdempotencyKey(opts)
	var out Withdrawal
	err := s.client.doJSON(ctx, apiRequest{
		method:         http.MethodPost,
		path:           "/withdrawals/",
		body:           params,
		idempotencyKey: key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// List lista saques com filtro opcional por status e paginação por
// limit/offset.
func (s *WithdrawalsService) List(ctx context.Context, params WithdrawalListParams) (*WithdrawalList, error) {
	q := url.Values{}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}
	if params.Status != "" {
		q.Set("status", params.Status)
	}

	var out WithdrawalList
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/withdrawals/",
		query:  q,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrieve recupera um saque pelo seu ID.
func (s *WithdrawalsService) Retrieve(ctx context.Context, withdrawalID string) (*Withdrawal, error) {
	var out Withdrawal
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/withdrawals/" + url.PathEscape(withdrawalID),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
