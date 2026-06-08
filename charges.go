package fluvpay

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// ChargesService agrupa as operações de cobrança PIX.
type ChargesService struct {
	client *Client
}

// ChargeCustomer descreve os dados opcionais do pagador de uma cobrança. Todos
// os campos são opcionais; envie apenas o que tiver.
type ChargeCustomer struct {
	Name     *string `json:"name,omitempty"`
	Email    *string `json:"email,omitempty"`
	Document *string `json:"document,omitempty"`
	Phone    *string `json:"phone,omitempty"`
}

// ChargeCreateParams são os campos aceitos ao criar uma cobrança. O contrato é
// estrito: não envie currency nem method, pois a moeda e o método (PIX) são
// implícitos e a API rejeita campos extras com erro de validação.
type ChargeCreateParams struct {
	// AmountCents é o valor em centavos. Obrigatório, entre 100 e 100000.
	AmountCents int `json:"amount_cents"`
	// Description é uma descrição livre de até 500 caracteres.
	Description *string `json:"description,omitempty"`
	// Customer são os dados opcionais do pagador.
	Customer *ChargeCustomer `json:"customer,omitempty"`
	// ExpiresInSeconds define o tempo de expiração (entre 60 e 604800). Usa o
	// padrão do processador se omitido.
	ExpiresInSeconds *int `json:"expires_in_seconds,omitempty"`
	// AffiliateCode é um código de afiliado opcional (4 a 24 caracteres).
	AffiliateCode *string `json:"affiliate_code,omitempty"`
	// SplitRuleID referencia uma regra de split do merchant (20 a 32 caracteres).
	SplitRuleID *string `json:"split_rule_id,omitempty"`
	// PassFeeToPayer repassa a taxa ao pagador (soma no QR/PIX). Padrão ligado;
	// deixe nil para usar o default do servidor ou aponte explicitamente.
	PassFeeToPayer *bool `json:"pass_fee_to_payer,omitempty"`
	// Metadata é um objeto livre de metadados.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Charge representa os detalhes completos de uma cobrança PIX.
type Charge struct {
	ID                string          `json:"id"`
	MerchantID        string          `json:"merchant_id"`
	AmountCents       int             `json:"amount_cents"`
	Currency          string          `json:"currency"`
	Description       *string         `json:"description,omitempty"`
	Customer          *ChargeCustomer `json:"customer,omitempty"`
	Status            string          `json:"status"`
	PaymentMethod     string          `json:"payment_method"`
	ExpiresAt         *string         `json:"expires_at,omitempty"`
	PaidAt            *string         `json:"paid_at,omitempty"`
	PixQRCode         *string         `json:"pix_qr_code,omitempty"`
	PixCopyPaste      *string         `json:"pix_copy_paste,omitempty"`
	FeeProcessorCents int             `json:"fee_processor_cents"`
	FeePlatformCents  int             `json:"fee_platform_cents"`
	NetAmountCents    *int            `json:"net_amount_cents,omitempty"`
	Metadata          map[string]any  `json:"metadata"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

// ChargeListItem é a versão enxuta de cobrança devolvida nas listagens.
type ChargeListItem struct {
	ID          string  `json:"id"`
	AmountCents int     `json:"amount_cents"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
	Description *string `json:"description,omitempty"`
	PaidAt      *string `json:"paid_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

// ChargeListParams são os filtros e a paginação aceitos por List.
type ChargeListParams struct {
	// Status filtra por estado (pending, paid, expired, cancelled, refunded).
	Status string
	// Page é a página (1-based). Zero usa o padrão do servidor.
	Page int
	// PerPage é a quantidade por página (máx 100). Zero usa o padrão.
	PerPage int
	// Sort é o campo de ordenação. Ex: "-created_at".
	Sort string
}

// ChargeList é o envelope paginado de cobranças (paginação por page/per_page).
type ChargeList struct {
	Data    []ChargeListItem `json:"data"`
	Page    int              `json:"page"`
	PerPage int              `json:"per_page"`
	Total   int              `json:"total"`
	HasNext bool             `json:"has_next"`
	HasPrev bool             `json:"has_prev"`
}

// Create cria uma cobrança PIX. A Idempotency-Key é gerada automaticamente
// (UUIDv4) quando opts é nil ou não traz uma chave própria.
func (s *ChargesService) Create(ctx context.Context, params ChargeCreateParams, opts *RequestOptions) (*Charge, error) {
	key := s.client.resolveIdempotencyKey(opts)
	var out Charge
	err := s.client.doJSON(ctx, apiRequest{
		method:         http.MethodPost,
		path:           "/charges/",
		body:           params,
		idempotencyKey: key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrieve recupera uma cobrança pelo seu ID.
func (s *ChargesService) Retrieve(ctx context.Context, chargeID string) (*Charge, error) {
	var out Charge
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/charges/" + url.PathEscape(chargeID),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// List lista cobranças com filtro opcional por status e paginação por
// page/per_page.
func (s *ChargesService) List(ctx context.Context, params ChargeListParams) (*ChargeList, error) {
	q := url.Values{}
	if params.Status != "" {
		q.Set("status", params.Status)
	}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(params.PerPage))
	}
	if params.Sort != "" {
		q.Set("sort", params.Sort)
	}

	var out ChargeList
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/charges/",
		query:  q,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
