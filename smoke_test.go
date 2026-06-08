package fluvpay

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestSandboxSmoke faz um smoke real contra o sandbox da FluvPay. Só roda quando
// a variável de ambiente FLUVPAY_TEST_KEY (prefixo fluv_test_) está presente;
// caso contrário é ignorado. Cria uma cobrança, recupera, lista e reseta o
// sandbox. Saques e transferências não são exercitados aqui porque são
// live-only no produto.
func TestSandboxSmoke(t *testing.T) {
	key := os.Getenv("FLUVPAY_TEST_KEY")
	if key == "" {
		t.Skip("FLUVPAY_TEST_KEY ausente: smoke do sandbox ignorado")
	}
	if !strings.HasPrefix(key, "fluv_test_") {
		t.Fatalf("FLUVPAY_TEST_KEY deve ter o prefixo fluv_test_")
	}

	cfg := Config{APIKey: key}
	if base := os.Getenv("FLUVPAY_BASE_URL"); base != "" {
		cfg.BaseURL = base
	}
	client := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	created, err := client.Charges.Create(ctx, ChargeCreateParams{
		AmountCents: 2500,
		Description: String("Smoke do SDK Go"),
	}, nil)
	if err != nil {
		t.Fatalf("falha ao criar cobrança no sandbox: %v", err)
	}
	if created.ID == "" {
		t.Fatal("cobrança criada sem ID")
	}

	fetched, err := client.Charges.Retrieve(ctx, created.ID)
	if err != nil {
		t.Fatalf("falha ao recuperar cobrança: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("ID divergente: criado %q, recuperado %q", created.ID, fetched.ID)
	}

	page, err := client.Charges.List(ctx, ChargeListParams{PerPage: 5})
	if err != nil {
		t.Fatalf("falha ao listar cobranças: %v", err)
	}
	if page.PerPage == 0 {
		t.Error("listagem retornou per_page zerado")
	}

	if _, err := client.Sandbox.Scenarios(ctx); err != nil {
		t.Fatalf("falha ao listar cenários do sandbox: %v", err)
	}

	reset, err := client.Sandbox.Reset(ctx)
	if err != nil {
		t.Fatalf("falha ao resetar o sandbox: %v", err)
	}
	if !reset.Reset {
		t.Error("reset do sandbox não confirmou reset=true")
	}
}
