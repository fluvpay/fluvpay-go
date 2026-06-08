package fluvpay

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// TransactionsService agrupa as operações de leitura do extrato financeiro.
type TransactionsService struct {
	client *Client
}

// Transaction é uma linha do extrato consolidado (entradas e saídas).
type Transaction struct {
	ID                         string         `json:"id"`
	MerchantID                 string         `json:"merchant_id"`
	ChargeID                   *string        `json:"charge_id,omitempty"`
	Type                       string         `json:"type"`
	Direction                  string         `json:"direction"`
	AmountCents                int            `json:"amount_cents"`
	FeeCents                   int            `json:"fee_cents"`
	NetAmountCents             int            `json:"net_amount_cents"`
	Status                     string         `json:"status"`
	Description                *string        `json:"description,omitempty"`
	Metadata                   map[string]any `json:"metadata"`
	CreatedAt                  string         `json:"created_at"`
	CounterpartyName           *string        `json:"counterparty_name,omitempty"`
	CounterpartyDocumentMasked *string        `json:"counterparty_document_masked,omitempty"`
	CounterpartyPixKey         *string        `json:"counterparty_pix_key,omitempty"`
}

// TransactionListParams são a paginação e a ordenação aceitas por List.
type TransactionListParams struct {
	// Page é a página (1-based). Zero usa o padrão do servidor.
	Page int
	// PerPage é a quantidade por página (máx 100). Zero usa o padrão.
	PerPage int
	// Sort é o campo de ordenação. Ex: "-created_at".
	Sort string
}

// TransactionList é o envelope paginado de transações (paginação por
// page/per_page).
type TransactionList struct {
	Data    []Transaction `json:"data"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Total   int           `json:"total"`
	HasNext bool          `json:"has_next"`
	HasPrev bool          `json:"has_prev"`
}

// List lista lançamentos do extrato com paginação por page/per_page.
func (s *TransactionsService) List(ctx context.Context, params TransactionListParams) (*TransactionList, error) {
	q := url.Values{}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(params.PerPage))
	}
	if params.Sort != "" {
		q.Set("sort", params.Sort)
	}

	var out TransactionList
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/transactions/",
		query:  q,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrieve recupera um lançamento do extrato pelo seu ID.
func (s *TransactionsService) Retrieve(ctx context.Context, txID string) (*Transaction, error) {
	var out Transaction
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/transactions/" + url.PathEscape(txID),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
