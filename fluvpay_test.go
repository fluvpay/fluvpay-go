package fluvpay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient monta um Client apontado para o servidor de teste, com sleep e
// jitter neutralizados para tornar as retentativas determinísticas e instantâneas.
func newTestClient(t *testing.T, srv *httptest.Server, apiKey string) *Client {
	t.Helper()
	return New(Config{
		APIKey:        apiKey,
		BaseURL:       srv.URL,
		HTTPClient:    srv.Client(),
		MaxRetries:    2,
		maxRetriesSet: true,
		sleep:         func(time.Duration) {},
		randFloat:     func() float64 { return 0 },
	})
}

func TestChargesCreateSendsCorrectRequest(t *testing.T) {
	var gotAuth, gotIdem, gotContentType, gotUserAgent string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("método esperado POST, recebeu %s", r.Method)
		}
		if r.URL.Path != "/charges/" {
			t.Errorf("path esperado /charges/, recebeu %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotIdem = r.Header.Get("Idempotency-Key")
		gotContentType = r.Header.Get("Content-Type")
		gotUserAgent = r.Header.Get("User-Agent")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id":"chg_123","merchant_id":"mer_1","amount_cents":2500,"currency":"BRL",
			"status":"pending","payment_method":"pix","pix_copy_paste":"000201...",
			"fee_processor_cents":40,"fee_platform_cents":10,"metadata":{},
			"created_at":"2026-06-08T00:00:00Z","updated_at":"2026-06-08T00:00:00Z"
		}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "fluv_test_abc")
	charge, err := client.Charges.Create(context.Background(), ChargeCreateParams{
		AmountCents:    2500,
		Description:    String("Pedido #1042"),
		PassFeeToPayer: Bool(true),
		Metadata:       map[string]any{"pedido_id": "1042"},
	}, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if gotAuth != "Bearer fluv_test_abc" {
		t.Errorf("Authorization incorreto: %q", gotAuth)
	}
	if gotIdem == "" {
		t.Error("Idempotency-Key deveria ter sido gerado automaticamente")
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type incorreto: %q", gotContentType)
	}
	if !strings.HasPrefix(gotUserAgent, "fluvpay-go/") {
		t.Errorf("User-Agent incorreto: %q", gotUserAgent)
	}
	if _, ok := gotBody["currency"]; ok {
		t.Error("o corpo não deve conter currency")
	}
	if _, ok := gotBody["method"]; ok {
		t.Error("o corpo não deve conter method")
	}
	if gotBody["amount_cents"].(float64) != 2500 {
		t.Errorf("amount_cents incorreto no corpo: %v", gotBody["amount_cents"])
	}
	if charge.ID != "chg_123" || charge.Status != "pending" {
		t.Errorf("charge decodificada incorretamente: %+v", charge)
	}
	if charge.PixCopyPaste == nil || *charge.PixCopyPaste != "000201..." {
		t.Errorf("pix_copy_paste decodificado incorretamente: %+v", charge.PixCopyPaste)
	}
}

func TestChargesCreateUsesProvidedIdempotencyKey(t *testing.T) {
	var gotIdem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"chg_1","merchant_id":"m","amount_cents":100,"currency":"BRL","status":"pending","payment_method":"pix","fee_processor_cents":0,"fee_platform_cents":0,"metadata":{},"created_at":"2026-06-08T00:00:00Z","updated_at":"2026-06-08T00:00:00Z"}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "fluv_test_abc")
	_, err := client.Charges.Create(context.Background(), ChargeCreateParams{AmountCents: 100}, &RequestOptions{IdempotencyKey: "minha-chave-123"})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if gotIdem != "minha-chave-123" {
		t.Errorf("Idempotency-Key esperado minha-chave-123, recebeu %q", gotIdem)
	}
}

func TestValidationErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"code":"VALIDATION_ERROR","message":"Dados inválidos","details":[{"field":"amount_cents","message":"Input should be greater than or equal to 100","type":"greater_than_equal"}],"trace_id":"01J..."}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "fluv_test_abc")
	_, err := client.Charges.Create(context.Background(), ChargeCreateParams{AmountCents: 1}, nil)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("esperava *ValidationError, recebeu %T (%v)", err, err)
	}
	if verr.Code() != "VALIDATION_ERROR" {
		t.Errorf("code incorreto: %q", verr.Code())
	}
	if verr.StatusCode() != 422 {
		t.Errorf("status incorreto: %d", verr.StatusCode())
	}
	if verr.TraceID() != "01J..." {
		t.Errorf("trace_id incorreto: %q", verr.TraceID())
	}
	if len(verr.Details()) != 1 || verr.Details()[0].Field == nil || *verr.Details()[0].Field != "amount_cents" {
		t.Errorf("details incorretos: %+v", verr.Details())
	}
}

func TestConflictErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"IDEMPOTENCY_CONFLICT","message":"Chave reutilizada com payload diferente"}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "fluv_test_abc")
	_, err := client.Charges.Create(context.Background(), ChargeCreateParams{AmountCents: 100}, nil)
	var cerr *ConflictError
	if !errors.As(err, &cerr) {
		t.Fatalf("esperava *ConflictError, recebeu %T (%v)", err, err)
	}
	if cerr.Code() != "IDEMPOTENCY_CONFLICT" {
		t.Errorf("code incorreto: %q", cerr.Code())
	}
}

func TestRateLimitErrorReadsRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"RATE_LIMITED","message":"Limite excedido"}}`))
	}))
	defer srv.Close()

	client := New(Config{
		APIKey:        "fluv_test_abc",
		BaseURL:       srv.URL,
		HTTPClient:    srv.Client(),
		MaxRetries:    0,
		maxRetriesSet: true,
	})
	_, err := client.Charges.Retrieve(context.Background(), "chg_1")
	var rerr *RateLimitError
	if !errors.As(err, &rerr) {
		t.Fatalf("esperava *RateLimitError, recebeu %T (%v)", err, err)
	}
	if rerr.RetryAfter != 7 {
		t.Errorf("RetryAfter esperado 7, recebeu %d", rerr.RetryAfter)
	}
}

func TestNotFoundAndPermissionMapping(t *testing.T) {
	cases := []struct {
		status int
		assert func(error) bool
	}{
		{http.StatusNotFound, func(e error) bool { var x *NotFoundError; return errors.As(e, &x) }},
		{http.StatusForbidden, func(e error) bool { var x *PermissionError; return errors.As(e, &x) }},
		{http.StatusUnauthorized, func(e error) bool { var x *AuthenticationError; return errors.As(e, &x) }},
		{http.StatusInternalServerError, func(e error) bool { var x *ServerError; return errors.As(e, &x) }},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"error":{"code":"X","message":"falhou"}}`))
		}))
		client := New(Config{APIKey: "fluv_test_abc", BaseURL: srv.URL, HTTPClient: srv.Client(), MaxRetries: 0, maxRetriesSet: true})
		_, err := client.Charges.Retrieve(context.Background(), "chg_1")
		if !tc.assert(err) {
			t.Errorf("status %d: tipo de erro inesperado %T", tc.status, err)
		}
		srv.Close()
	}
}

func TestChargesListParsesPageEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "paid" {
			t.Errorf("filtro status não enviado: %q", r.URL.Query().Get("status"))
		}
		if r.URL.Query().Get("per_page") != "20" {
			t.Errorf("per_page não enviado: %q", r.URL.Query().Get("per_page"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"chg_1","amount_cents":2500,"currency":"BRL","status":"paid","created_at":"2026-06-08T00:00:00Z"}],"page":1,"per_page":20,"total":1,"has_next":false,"has_prev":false}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "fluv_test_abc")
	page, err := client.Charges.List(context.Background(), ChargeListParams{Status: "paid", Page: 1, PerPage: 20, Sort: "-created_at"})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if page.Page != 1 || page.PerPage != 20 || page.HasNext {
		t.Errorf("envelope page/per_page incorreto: %+v", page)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "chg_1" {
		t.Errorf("data incorreto: %+v", page.Data)
	}
}

func TestWithdrawalsListParsesOffsetEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("limit não enviado: %q", r.URL.Query().Get("limit"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"wd_1","status":"pending","amount_cents":5000,"fee_cents":50,"net_cents":4950,"pix_key":"a@b.com","pix_key_type":"email","created_at":"2026-06-08T00:00:00Z"}],"limit":10,"offset":0,"total":1}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "fluv_live_abc")
	page, err := client.Withdrawals.List(context.Background(), WithdrawalListParams{Limit: 10})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if page.Limit != 10 || page.Offset != 0 || page.Total != 1 {
		t.Errorf("envelope limit/offset incorreto: %+v", page)
	}
	if len(page.Data) != 1 || page.Data[0].NetCents != 4950 {
		t.Errorf("data incorreto: %+v", page.Data)
	}
}

func TestRetryOn429ThenSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":"RATE_LIMITED","message":"devagar"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chg_1","merchant_id":"m","amount_cents":100,"currency":"BRL","status":"pending","payment_method":"pix","fee_processor_cents":0,"fee_platform_cents":0,"metadata":{},"created_at":"2026-06-08T00:00:00Z","updated_at":"2026-06-08T00:00:00Z"}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "fluv_test_abc")
	charge, err := client.Charges.Retrieve(context.Background(), "chg_1")
	if err != nil {
		t.Fatalf("erro inesperado após retry: %v", err)
	}
	if charge.ID != "chg_1" {
		t.Errorf("charge incorreta: %+v", charge)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("esperava 2 chamadas (1 falha + 1 sucesso), houve %d", calls)
	}
}

func TestNoRetryOnPostWithoutIdempotencyKeyIsStillRetriedBecauseGenerated(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"SERVER_ERROR","message":"indisponível"}}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"chg_1","merchant_id":"m","amount_cents":100,"currency":"BRL","status":"pending","payment_method":"pix","fee_processor_cents":0,"fee_platform_cents":0,"metadata":{},"created_at":"2026-06-08T00:00:00Z","updated_at":"2026-06-08T00:00:00Z"}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "fluv_test_abc")
	_, err := client.Charges.Create(context.Background(), ChargeCreateParams{AmountCents: 100}, nil)
	if err != nil {
		t.Fatalf("o POST com Idempotency-Key gerado deveria ser retentado: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("esperava 2 chamadas, houve %d", calls)
	}
}

func TestMaxRetriesZeroDisablesRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"SERVER_ERROR","message":"erro"}}`))
	}))
	defer srv.Close()

	client := New(Config{APIKey: "fluv_test_abc", BaseURL: srv.URL, HTTPClient: srv.Client(), MaxRetries: 0, maxRetriesSet: true})
	_, err := client.Charges.Retrieve(context.Background(), "chg_1")
	if err == nil {
		t.Fatal("esperava erro")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("com retries desligados esperava 1 chamada, houve %d", calls)
	}
}

func TestIsTestKey(t *testing.T) {
	if !New(Config{APIKey: "fluv_test_x"}).IsTestKey() {
		t.Error("fluv_test_ deveria ser chave de teste")
	}
	if New(Config{APIKey: "fluv_live_x"}).IsTestKey() {
		t.Error("fluv_live_ não deveria ser chave de teste")
	}
}

func TestNewIdempotencyKeyIsUUIDv4(t *testing.T) {
	client := New(Config{APIKey: "fluv_test_x"})
	key := client.NewIdempotencyKey()
	if len(key) != 36 {
		t.Fatalf("UUID com tamanho inesperado: %q", key)
	}
	parts := strings.Split(key, "-")
	if len(parts) != 5 {
		t.Fatalf("formato de UUID inesperado: %q", key)
	}
	if !strings.HasPrefix(parts[2], "4") {
		t.Errorf("versão do UUID deveria ser 4: %q", key)
	}
}
