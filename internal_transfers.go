package fluvpay

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// InternalTransfersService agrupa as operações de transferência entre contas
// FluvPay. Transferências são live-only: chaves fluv_test_ recebem 403.
type InternalTransfersService struct {
	client *Client
}

// InternalTransferCreateParams são os campos aceitos ao criar uma transferência
// interna. Informe exatamente um entre RecipientEmail e RecipientMerchantID.
type InternalTransferCreateParams struct {
	// AmountCents é o valor em centavos. Obrigatório, entre 100 e 10000000.
	AmountCents int `json:"amount_cents"`
	// RecipientEmail é o email do destinatário. Use este OU RecipientMerchantID.
	RecipientEmail *string `json:"recipient_email,omitempty"`
	// RecipientMerchantID é o ULID (26 caracteres) do merchant destinatário.
	RecipientMerchantID *string `json:"recipient_merchant_id,omitempty"`
	// Description é uma descrição livre opcional (até 140 caracteres).
	Description *string `json:"description,omitempty"`
}

// InternalTransfer representa os detalhes de uma transferência interna.
type InternalTransfer struct {
	ID             string  `json:"id"`
	FromMerchantID string  `json:"from_merchant_id"`
	ToMerchantID   string  `json:"to_merchant_id"`
	ToMerchantName *string `json:"to_merchant_name,omitempty"`
	AmountCents    int     `json:"amount_cents"`
	Description    *string `json:"description,omitempty"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
}

// InternalTransferListParams são os filtros e a paginação aceitos por List.
type InternalTransferListParams struct {
	// Direction filtra entre "sent" (enviadas pelo merchant) e "received"
	// (recebidas). Vazio usa o padrão do servidor ("sent").
	Direction string
	// Limit é a quantidade por página (máx 100). Zero usa o padrão do servidor.
	Limit int
	// Offset é o deslocamento (0-based). Padrão zero.
	Offset int
}

// InternalTransferList é o envelope de transferências internas (paginação por
// limit/offset).
type InternalTransferList struct {
	Data   []InternalTransfer `json:"data"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
	Total  int                `json:"total"`
}

// Create cria uma transferência interna FluvPay para FluvPay. A Idempotency-Key
// é gerada automaticamente (UUIDv4) quando opts é nil ou não traz uma chave
// própria.
func (s *InternalTransfersService) Create(ctx context.Context, params InternalTransferCreateParams, opts *RequestOptions) (*InternalTransfer, error) {
	key := s.client.resolveIdempotencyKey(opts)
	var out InternalTransfer
	err := s.client.doJSON(ctx, apiRequest{
		method:         http.MethodPost,
		path:           "/internal-transfers/",
		body:           params,
		idempotencyKey: key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// List lista transferências internas com filtro por direção e paginação por
// limit/offset.
func (s *InternalTransfersService) List(ctx context.Context, params InternalTransferListParams) (*InternalTransferList, error) {
	q := url.Values{}
	if params.Direction != "" {
		q.Set("direction", params.Direction)
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}

	var out InternalTransferList
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/internal-transfers/",
		query:  q,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrieve recupera uma transferência interna pelo seu ID.
func (s *InternalTransfersService) Retrieve(ctx context.Context, transferID string) (*InternalTransfer, error) {
	var out InternalTransfer
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/internal-transfers/" + url.PathEscape(transferID),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
