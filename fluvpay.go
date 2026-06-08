// Package fluvpay é o SDK oficial da FluvPay para Go.
//
// Ele cobre cobranças PIX, saques, transferências internas, o extrato de
// transações, utilitários de sandbox e a verificação de assinatura de
// webhooks. Usa apenas a biblioteca padrão (net/http, encoding/json) e não
// tem dependências externas.
//
// A API Key define o modo de operação pelo prefixo: fluv_live_ para produção e
// fluv_test_ para o sandbox. Você só precisa passar a chave; o SDK cuida do
// resto.
//
//	client := fluvpay.New(fluvpay.Config{APIKey: os.Getenv("FLUVPAY_API_KEY")})
//	charge, err := client.Charges.Create(ctx, fluvpay.ChargeCreateParams{
//	    AmountCents: 2500,
//	}, nil)
package fluvpay

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version é a versão do SDK, embutida no header User-Agent.
const Version = "1.0.0"

const defaultBaseURL = "https://api.fluvpay.com/api/v1"
const defaultTimeout = 30 * time.Second
const defaultMaxRetries = 2

// Config reúne as opções de construção do cliente FluvPay.
type Config struct {
	// APIKey é obrigatória. Prefixo fluv_live_ (produção) ou fluv_test_ (sandbox).
	APIKey string
	// BaseURL sobrescreve a URL base da API. Padrão https://api.fluvpay.com/api/v1.
	BaseURL string
	// HTTPClient permite injetar um *http.Client customizado (timeouts, proxy,
	// transporte de teste). Se nulo, o SDK cria um com Timeout padrão.
	HTTPClient *http.Client
	// MaxRetries é o número máximo de retentativas. Padrão 2 quando não
	// informado. Passe um valor negativo (ex: -1) para desligar as retentativas.
	MaxRetries int
	// maxRetriesSet diferencia "0 explícito" de "não informado".
	maxRetriesSet bool
	// sleep é injetável para tornar os testes de retentativa determinísticos.
	sleep func(time.Duration)
	// randFloat é injetável para tornar o jitter determinístico nos testes.
	randFloat func() float64
}

// Client é o cliente HTTP da FluvPay. Cuida de autenticação, serialização,
// idempotência, mapeamento de erros e retentativas. Os recursos (Charges,
// Withdrawals, ...) delegam aqui. Um Client é seguro para uso concorrente.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int
	sleep      func(time.Duration)
	randFloat  func() float64

	// Charges expõe as operações de cobrança PIX.
	Charges *ChargesService
	// Transactions expõe as operações de extrato.
	Transactions *TransactionsService
	// Withdrawals expõe as operações de saque PIX.
	Withdrawals *WithdrawalsService
	// InternalTransfers expõe as operações de transferência interna.
	InternalTransfers *InternalTransfersService
	// Sandbox expõe os utilitários de teste (somente chave fluv_test_).
	Sandbox *SandboxService
}

// New constrói um Client a partir da Config informada. Causa pânico se a APIKey
// estiver vazia, já que toda chamada à API depende dela.
func New(cfg Config) *Client {
	if cfg.APIKey == "" {
		panic("fluvpay: APIKey é obrigatória")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	maxRetries := defaultMaxRetries
	if cfg.maxRetriesSet {
		maxRetries = cfg.MaxRetries
	} else if cfg.MaxRetries != 0 {
		maxRetries = cfg.MaxRetries
	}
	if maxRetries < 0 {
		maxRetries = 0
	}

	sleep := cfg.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	randFloat := cfg.randFloat
	if randFloat == nil {
		randFloat = secureRandFloat
	}

	c := &Client{
		apiKey:     cfg.APIKey,
		baseURL:    baseURL,
		httpClient: httpClient,
		maxRetries: maxRetries,
		sleep:      sleep,
		randFloat:  randFloat,
	}

	c.Charges = &ChargesService{client: c}
	c.Transactions = &TransactionsService{client: c}
	c.Withdrawals = &WithdrawalsService{client: c}
	c.InternalTransfers = &InternalTransfersService{client: c}
	c.Sandbox = &SandboxService{client: c}

	return c
}

// IsTestKey indica se a chave em uso é de sandbox (prefixo fluv_test_).
func (c *Client) IsTestKey() bool {
	return strings.HasPrefix(c.apiKey, "fluv_test_")
}

// NewIdempotencyKey gera uma chave de idempotência (UUIDv4). É o valor usado
// automaticamente nos POSTs de escrita quando o chamador não informa uma.
func (c *Client) NewIdempotencyKey() string {
	return newUUIDv4()
}

// RequestOptions carrega opções por chamada compartilhadas pelos recursos de
// escrita. Hoje expõe apenas a chave de idempotência.
type RequestOptions struct {
	// IdempotencyKey, se informada, é enviada no header Idempotency-Key. Quando
	// vazia em um POST de escrita, o SDK gera uma UUIDv4 automaticamente.
	IdempotencyKey string
}

// apiRequest descreve uma requisição interna ao cliente HTTP.
type apiRequest struct {
	method         string
	path           string
	query          url.Values
	body           any
	idempotencyKey string
}

// doJSON executa uma requisição, aplica retentativas quando seguro e decodifica
// o corpo de sucesso em out (que pode ser nil para descartar a resposta).
//
// Retenta com backoff exponencial e jitter para falhas transientes (429 e
// 5xx/conexão), respeitando o header Retry-After. Retentativa só ocorre em GETs
// e em POSTs que carregam Idempotency-Key.
func (c *Client) doJSON(ctx context.Context, req apiRequest, out any) error {
	fullURL := c.baseURL + req.path
	if len(req.query) > 0 {
		fullURL += "?" + req.query.Encode()
	}

	var bodyBytes []byte
	if req.body != nil && req.method != http.MethodGet {
		b, err := json.Marshal(req.body)
		if err != nil {
			return &ConnectionError{BaseError{
				Msg:   fmt.Sprintf("Falha ao serializar o corpo da requisição: %v", err),
				Cause: err,
			}}
		}
		bodyBytes = b
	}

	retryable := c.isRetryable(req)

	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}

		httpReq, err := http.NewRequestWithContext(ctx, req.method, fullURL, reader)
		if err != nil {
			return &ConnectionError{BaseError{
				Msg:   fmt.Sprintf("Falha ao montar a requisição: %v", err),
				Cause: err,
			}}
		}
		c.applyHeaders(httpReq, req, bodyBytes != nil)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			if retryable && attempt < c.maxRetries {
				c.backoff(attempt, 0)
				continue
			}
			return &ConnectionError{BaseError{
				Msg:   fmt.Sprintf("Falha de conexão com a FluvPay: %v", err),
				Cause: err,
			}}
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if retryable && attempt < c.maxRetries {
				c.backoff(attempt, 0)
				continue
			}
			return &ConnectionError{BaseError{
				Msg:   fmt.Sprintf("Falha ao ler a resposta da FluvPay: %v", readErr),
				Cause: readErr,
			}}
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out == nil || len(respBody) == 0 {
				return nil
			}
			if err := json.Unmarshal(respBody, out); err != nil {
				return &ConnectionError{BaseError{
					Msg:    fmt.Sprintf("Falha ao decodificar a resposta da FluvPay: %v", err),
					Status: resp.StatusCode,
					Cause:  err,
				}}
			}
			return nil
		}

		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		apiErr := errorFromResponse(resp.StatusCode, respBody, retryAfter)

		isTransient := resp.StatusCode == 429 || resp.StatusCode >= 500
		if retryable && isTransient && attempt < c.maxRetries {
			c.backoff(attempt, retryAfter)
			continue
		}

		return apiErr
	}
}

// isRetryable diz se uma requisição pode ser repetida com segurança: GETs
// sempre, POSTs apenas quando carregam Idempotency-Key.
func (c *Client) isRetryable(req apiRequest) bool {
	if c.maxRetries <= 0 {
		return false
	}
	if req.method == http.MethodGet {
		return true
	}
	return req.method == http.MethodPost && req.idempotencyKey != ""
}

// applyHeaders preenche os headers padrão da requisição (auth, content-type,
// user-agent e idempotência).
func (c *Client) applyHeaders(httpReq *http.Request, req apiRequest, hasBody bool) {
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "fluvpay-go/"+Version)
	if hasBody {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if req.idempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", req.idempotencyKey)
	}
}

// backoff aguarda antes da próxima tentativa. Se houver Retry-After, respeita-o;
// caso contrário usa backoff exponencial com jitter.
func (c *Client) backoff(attempt, retryAfter int) {
	if retryAfter > 0 {
		c.sleep(time.Duration(retryAfter) * time.Second)
		return
	}
	base := 250 * (1 << attempt)
	jitter := c.randFloat() * float64(base)
	c.sleep(time.Duration(float64(base)+jitter) * time.Millisecond)
}

// resolveIdempotencyKey devolve a chave informada pelo chamador ou gera uma
// UUIDv4 nova quando ela estiver vazia.
func (c *Client) resolveIdempotencyKey(opts *RequestOptions) string {
	if opts != nil && opts.IdempotencyKey != "" {
		return opts.IdempotencyKey
	}
	return c.NewIdempotencyKey()
}

// parseRetryAfter lê o header Retry-After (segundos ou data HTTP) e devolve o
// número de segundos a aguardar. Zero quando ausente ou ilegível.
func parseRetryAfter(value string) int {
	if value == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		if secs < 0 {
			return 0
		}
		return secs
	}
	if t, err := http.ParseTime(value); err == nil {
		secs := int(time.Until(t).Seconds())
		if secs < 0 {
			return 0
		}
		return secs
	}
	return 0
}

// secureRandFloat devolve uma fração em [0, 1) usando crypto/rand, com fallback
// para zero caso a fonte de entropia falhe.
func secureRandFloat() float64 {
	const precision = 1 << 53
	n, err := rand.Int(rand.Reader, big.NewInt(precision))
	if err != nil {
		return 0
	}
	return float64(n.Int64()) / float64(precision)
}

// newUUIDv4 gera um UUID versão 4 a partir de crypto/rand, sem dependências
// externas.
func newUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("urn-fallback-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
