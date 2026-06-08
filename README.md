# FluvPay SDK para Go

SDK oficial da FluvPay para Go. Cobranças PIX, saques, transferências internas e
verificação de webhooks, com tipagem forte e zero dependência externa (usa apenas
a biblioteca padrão: `net/http` e `encoding/json`).

## Instalação

Diferente de outras linguagens, Go não usa um registry central de pacotes. O
`go get` resolve o módulo direto do GitHub, então o comando abaixo já funciona
hoje, sem nenhum cadastro intermediário:

```bash
go get github.com/fluvpay/fluvpay-go@latest
```

O `@latest` pega o commit mais recente do branch principal. Depois é só importar
o pacote no seu código:

```go
import "github.com/fluvpay/fluvpay-go"
```

O caminho do módulo é `github.com/fluvpay/fluvpay-go` (igual ao declarado no
`go.mod`), por isso o import e o `go get` usam exatamente essa string.

Quando a versão estável `v1.0.0` for tagueada no GitHub, você poderá fixar um
release específico (recomendado para produção):

```bash
# disponível quando a tag v1.0.0 for publicada no GitHub
go get github.com/fluvpay/fluvpay-go@v1.0.0
```

Requisitos: Go 1.21 ou superior.

## Configuração

A API Key define o modo de operação pelo prefixo: `fluv_live_` para produção e
`fluv_test_` para o sandbox. Você só precisa passar a chave; o SDK cuida do resto.

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fluvpay/fluvpay-go"
)

func main() {
	client := fluvpay.New(fluvpay.Config{
		APIKey: os.Getenv("FLUVPAY_API_KEY"),
		// BaseURL:    "https://api.fluvpay.com/api/v1", // padrão
		// MaxRetries: 2,                                // padrão (valor negativo desliga)
	})

	fmt.Println(client.IsTestKey()) // true se a chave for fluv_test_

	ctx := context.Background()
	charge, err := client.Charges.Create(ctx, fluvpay.ChargeCreateParams{
		AmountCents: 2500, // R$ 25,00 (mín 100, máx 100000)
		Description: fluvpay.String("Pedido #1042"),
		Customer: &fluvpay.ChargeCustomer{
			Name:  fluvpay.String("Cliente Exemplo"),
			Email: fluvpay.String("cliente@exemplo.com"),
		},
		PassFeeToPayer: fluvpay.Bool(true),
		Metadata:       map[string]any{"pedido_id": "1042"},
	}, nil)
	if err != nil {
		panic(err)
	}

	fmt.Println(charge.ID)
	fmt.Println(charge.Status) // pending | paid | expired | cancelled | refunded
	if charge.PixCopyPaste != nil {
		fmt.Println(*charge.PixCopyPaste) // código copia-e-cola
	}
}
```

## Criar uma cobrança PIX

A criação de cobrança aceita apenas os campos do contrato. Não envie `currency`
nem `method`: a moeda e o método (PIX) são implícitos, e a API rejeita campos
extras com erro de validação.

A `Idempotency-Key` é gerada automaticamente (UUIDv4) se você não informar uma.
Para controlar a chave (por exemplo, reusar entre tentativas do seu lado), passe
pelo terceiro argumento:

```go
charge, err := client.Charges.Create(ctx,
	fluvpay.ChargeCreateParams{AmountCents: 2500},
	&fluvpay.RequestOptions{IdempotencyKey: "pedido-1042-tentativa-1"},
)
```

Os campos opcionais são ponteiros. Use os helpers `fluvpay.String`,
`fluvpay.Int` e `fluvpay.Bool` para preenchê-los de forma enxuta.

## Recuperar e listar

```go
charge, err := client.Charges.Retrieve(ctx, "chg_...")

page, err := client.Charges.List(ctx, fluvpay.ChargeListParams{
	Status:  "paid",
	Page:    1,
	PerPage: 20,
	Sort:    "-created_at",
})

fmt.Println(page.Data)    // []ChargeListItem
fmt.Println(page.HasNext) // paginação por page/per_page
```

## Saques e transferências internas

Estas operações são live-only: chaves `fluv_test_` recebem 403.

```go
withdrawal, err := client.Withdrawals.Create(ctx, fluvpay.WithdrawalCreateParams{
	AmountCents: 5000,
	PixKey:      "chave@exemplo.com",
	PixKeyType:  "email", // cpf | cnpj | email | phone | evp
}, nil)

wPage, err := client.Withdrawals.List(ctx, fluvpay.WithdrawalListParams{Limit: 20, Offset: 0})
fmt.Println(wPage.Total) // paginação por limit/offset

transfer, err := client.InternalTransfers.Create(ctx, fluvpay.InternalTransferCreateParams{
	AmountCents:    1000,
	RecipientEmail: fluvpay.String("destino@exemplo.com"), // ou RecipientMerchantID
}, nil)
```

## Extrato (transactions)

```go
txPage, err := client.Transactions.List(ctx, fluvpay.TransactionListParams{Page: 1, PerPage: 50})
tx, err := client.Transactions.Retrieve(ctx, "tx_...")
```

## Sandbox

Disponível apenas com chave `fluv_test_`.

```go
scenarios, err := client.Sandbox.Scenarios(ctx)
reset, err := client.Sandbox.Reset(ctx)
```

## Verificação de webhooks

A FluvPay assina cada entrega. Verifique a assinatura usando o corpo CRU da
requisição (nunca re-serialize o JSON, pois isso muda os bytes e invalida a
assinatura). O cálculo é `HMAC_SHA256(secret, timestamp + "." + rawBody)` em hex,
e o header `X-FluvPay-Signature` vem no formato `v1=<hex>`.

```go
package main

import (
	"io"
	"net/http"
	"os"

	"github.com/fluvpay/fluvpay-go"
)

func handler(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err := fluvpay.Webhooks.VerifySignature(fluvpay.VerifyParams{
		Payload:          rawBody, // bytes crus, sem re-serializar
		SignatureHeader:  r.Header.Get("X-FluvPay-Signature"),
		Timestamp:        r.Header.Get("X-FluvPay-Timestamp"),
		Secret:           os.Getenv("FLUVPAY_WEBHOOK_SECRET"), // whsec_...
		ToleranceSeconds: 300,
	})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "charge.paid":
		// processar pagamento confirmado
	case "payout.completed":
		// processar saque concluído
	}

	w.WriteHeader(http.StatusOK)
}
```

Eventos disponíveis: `charge.created`, `charge.paid`, `charge.expired`,
`charge.cancelled`, `charge.refunded`, `payout.created`, `payout.completed` e
`payout.failed`.

## Tratamento de erros

Cada falha vira um erro tipado. Todos carregam `Code()`, `Details()`,
`TraceID()` e `StatusCode()` (interface `fluvpay.Error`). Use `errors.As` para
destrinchar o tipo concreto.

```go
import (
	"errors"

	"github.com/fluvpay/fluvpay-go"
)

_, err := client.Charges.Create(ctx, fluvpay.ChargeCreateParams{AmountCents: 1}, nil)

var verr *fluvpay.ValidationError
var rerr *fluvpay.RateLimitError
switch {
case errors.As(err, &verr):
	// dados inválidos: verr.Code(), verr.Details()
case errors.As(err, &rerr):
	// aguardar rerr.RetryAfter segundos
}
```

Mapeamento: 400/422 para `ValidationError`, 401 para `AuthenticationError`, 403
para `PermissionError`, 404 para `NotFoundError`, 409 para `ConflictError` (inclui
`IDEMPOTENCY_CONFLICT`), 429 para `RateLimitError` (lê `Retry-After`), 5xx para
`ServerError`, e falha de rede ou timeout para `ConnectionError`. A verificação de
webhook usa `SignatureVerificationError`.

## Retentativas

O SDK retenta automaticamente (padrão 2 tentativas, backoff exponencial com
jitter) apenas em situações seguras: requisições GET e POSTs que carregam
`Idempotency-Key`, nos casos de 429 e 5xx ou falha de conexão. O header
`Retry-After` é respeitado. Para desligar, passe um `MaxRetries` negativo (ex:
`MaxRetries: -1`) na `Config`.

## Desenvolvimento

```bash
go build ./...
go test ./...
```

O smoke no sandbox roda somente se a variável `FLUVPAY_TEST_KEY` (prefixo
`fluv_test_`) estiver presente; caso contrário, é ignorado.

## Licença

MIT.
