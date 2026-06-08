package fluvpay

import (
	"context"
	"net/http"
)

// SandboxService agrupa os utilitários de teste do sandbox. Disponíveis apenas
// com chave de teste (fluv_test_).
type SandboxService struct {
	client *Client
}

// SandboxResetResult é o retorno do reset do sandbox.
type SandboxResetResult struct {
	Reset          bool   `json:"reset"`
	DeletedCharges int    `json:"deleted_charges"`
	MerchantID     string `json:"merchant_id"`
}

// SandboxScenarios lista os valores mágicos do sandbox e o comportamento que
// cada um simula.
type SandboxScenarios struct {
	Info      string           `json:"info"`
	Scenarios []map[string]any `json:"scenarios"`
}

// Reset apaga todos os dados do sandbox da conta. Só funciona com chave de teste.
func (s *SandboxService) Reset(ctx context.Context) (*SandboxResetResult, error) {
	var out SandboxResetResult
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodPost,
		path:   "/test/reset",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Scenarios lista os valores mágicos do sandbox para facilitar a simulação de
// cenários durante o desenvolvimento.
func (s *SandboxService) Scenarios(ctx context.Context) (*SandboxScenarios, error) {
	var out SandboxScenarios
	err := s.client.doJSON(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/test/scenarios",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
