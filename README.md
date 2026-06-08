# FluvPay SDK para Go

SDK oficial da FluvPay para Go. Cobre cobranças PIX, saques, transferências internas, o extrato de transações e a verificação de assinatura de webhooks. A tipagem é forte e não há dependências externas: o SDK usa apenas a biblioteca padrão (`net/http` e `encoding/json`). A interface é estável e previsível, adequada tanto a integrações construídas por desenvolvedores quanto a agentes de IA que consomem esta documentação para integrar.

Requisitos: Go 1.21 ou superior.

## Instalação

Go resolve módulos diretamente do GitHub. Não há registry central de pacotes, portanto não é necessário nenhum cadastro intermediário.

```bash
go get github.com/fluvpay/fluvpay-go@v1.0.0
```

Importe o pacote no código:

```go
import "github.com/fluvpay/fluvpay-go"
```

O caminho do módulo é `github.com/fluvpay/fluvpay-go`, idêntico ao declarado no `go.mod`. O import e o `go get` usam exatamente essa string. Fixar uma tag de release (como `@v1.0.0`) é recomendado em produção.

## Início rápido

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

`fluvpay.New` exige uma `APIKey` e causa pânico se ela estiver vazia. O `Client` resultante é seguro para uso concorrente e centraliza autenticação, serialização, idempotência, mapeamento de erros e retentativas.

## Autenticação

A autenticação usa a API Key no header `Authorization`, enviada como `Bearer <api_key>`. O ambiente é determinado pelo prefixo da chave: `fluv_live_` opera em produção e `fluv_test_` opera no sandbox. O método `client.IsTestKey()` retorna `true` quando a chave em uso é de sandbox.

A configuração do cliente é definida via `fluvpay.Config`.

| Campo | Tipo | Padrão | Descrição |
| --- | --- | --- | --- |
| `APIKey` | `string` | obrigatório | Chave de API. Prefixo `fluv_live_` (produção) ou `fluv_test_` (sandbox). |
| `BaseURL` | `string` | `https://api.fluvpay.com/api/v1` | URL base da API. |
| `HTTPClient` | `*http.Client` | cliente com timeout de 30s | Cliente HTTP customizado (timeouts, proxy, transporte de teste). |
| `MaxRetries` | `int` | `2` | Número máximo de retentativas. Um valor negativo desliga as retentativas. |

## Cobranças PIX

O recurso `Charges` cria, recupera e lista cobranças PIX.

### Criar

A criação aceita apenas os campos do contrato. Os campos `currency` e `method` não devem ser enviados: a moeda e o método (PIX) são implícitos, e a API rejeita campos extras com erro de validação.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `AmountCents` | `int` | sim | Valor em centavos, entre 100 e 100000. |
| `Description` | `*string` | não | Descrição livre de até 500 caracteres. |
| `Customer` | `*ChargeCustomer` | não | Dados do pagador (`Name`, `Email`, `Document`, `Phone`), todos opcionais. |
| `ExpiresInSeconds` | `*int` | não | Tempo de expiração entre 60 e 604800. Usa o padrão do processador se omitido. |
| `AffiliateCode` | `*string` | não | Código de afiliado, de 4 a 24 caracteres. |
| `SplitRuleID` | `*string` | não | Referência a uma regra de split do merchant, de 20 a 32 caracteres. |
| `PassFeeToPayer` | `*bool` | não | Repassa a taxa ao pagador (soma no QR/PIX). Padrão do servidor quando `nil`. |
| `Metadata` | `map[string]any` | não | Objeto livre de metadados. |

Os campos opcionais são ponteiros. Os helpers `fluvpay.String`, `fluvpay.Int` e `fluvpay.Bool` constroem esses ponteiros de forma concisa.

```go
charge, err := client.Charges.Create(ctx, fluvpay.ChargeCreateParams{
	AmountCents: 2500,
	Description: fluvpay.String("Pedido #1042"),
}, nil)
```

A `Idempotency-Key` é gerada automaticamente como UUIDv4 quando o terceiro argumento é `nil` ou não traz uma chave. Para controlar o valor, informe-o em `RequestOptions`:

```go
charge, err := client.Charges.Create(ctx,
	fluvpay.ChargeCreateParams{AmountCents: 2500},
	&fluvpay.RequestOptions{IdempotencyKey: "pedido-1042-tentativa-1"},
)
```

### Recuperar e listar

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

A listagem de cobranças usa paginação por `page`/`per_page`. O envelope `ChargeList` expõe `Data`, `Page`, `PerPage`, `Total`, `HasNext` e `HasPrev`. O campo `Status` aceita `pending`, `paid`, `expired`, `cancelled` e `refunded`.

## Saques e transferências internas

As operações de saque e transferência interna são exclusivas de produção. Chaves `fluv_test_` recebem 403.

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

A listagem de saques usa paginação por `limit`/`offset`. A transferência interna identifica o destinatário por `RecipientEmail` ou `RecipientMerchantID`.

## Extrato de transações

O recurso `Transactions` lista e recupera lançamentos do extrato. A listagem usa paginação por `page`/`per_page`.

```go
txPage, err := client.Transactions.List(ctx, fluvpay.TransactionListParams{Page: 1, PerPage: 50})
tx, err := client.Transactions.Retrieve(ctx, "tx_...")
```

## Sandbox

O recurso `Sandbox` está disponível apenas com chave `fluv_test_`.

```go
scenarios, err := client.Sandbox.Scenarios(ctx)
reset, err := client.Sandbox.Reset(ctx)
```

## Webhooks

A FluvPay assina cada entrega de webhook. A verificação usa o corpo cru da requisição. O JSON não deve ser re-serializado, pois isso altera os bytes e invalida a assinatura.

A assinatura é calculada como `HMAC_SHA256(secret, timestamp + "." + rawBody)` em hexadecimal. O header `X-FluvPay-Signature` chega no formato `v1=<hex>` e o timestamp chega em `X-FluvPay-Timestamp`. O parâmetro `ToleranceSeconds` define a janela máxima de tolerância entre o timestamp e o instante da verificação.

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

Uma assinatura inválida ou um timestamp fora da tolerância retorna `SignatureVerificationError`.

Eventos emitidos: `charge.created`, `charge.paid`, `charge.expired`, `charge.cancelled`, `charge.refunded`, `payout.created`, `payout.completed` e `payout.failed`.

## Erros

Toda falha é retornada como um erro tipado que implementa a interface `fluvpay.Error`, com os métodos `Code()`, `Details()`, `TraceID()` e `StatusCode()`. Use `errors.As` para acessar o tipo concreto.

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

O mapeamento de status HTTP para tipo de erro é o seguinte.

| Status HTTP | Tipo | Observação |
| --- | --- | --- |
| 400, 422 | `ValidationError` | Dados inválidos ou estado impeditivo (inclui `INSUFFICIENT_BALANCE`). |
| 401 | `AuthenticationError` | Autenticação obrigatória ou chave inválida. |
| 403 | `PermissionError` | Escopo insuficiente ou operação não permitida (inclui operações não suportadas em sandbox). |
| 404 | `NotFoundError` | Recurso não encontrado. |
| 409 | `ConflictError` | Conflito, inclui `IDEMPOTENCY_CONFLICT`. |
| 429 | `RateLimitError` | Limite de requisições excedido. `RetryAfter` é lido do header `Retry-After`. |
| 5xx | `ServerError` | Erro interno do servidor. |
| sem resposta | `ConnectionError` | Falha de rede, timeout ou interrupção antes da resposta HTTP. |

A verificação de webhook retorna `SignatureVerificationError`.

## Retentativas

O SDK retenta automaticamente apenas operações seguras: requisições GET e POSTs que carregam `Idempotency-Key`. A retentativa ocorre em respostas 429, respostas 5xx e falhas de conexão. O padrão é de 2 tentativas adicionais, com backoff exponencial e jitter. O header `Retry-After` é respeitado quando presente.

Para desligar as retentativas, defina um `MaxRetries` negativo na `Config`:

```go
client := fluvpay.New(fluvpay.Config{
	APIKey:     os.Getenv("FLUVPAY_API_KEY"),
	MaxRetries: -1,
})
```

## Desenvolvimento

```bash
go build ./...
go test ./...
```

O smoke test contra o sandbox roda somente quando a variável `FLUVPAY_TEST_KEY` (prefixo `fluv_test_`) está presente. Sem ela, o teste é ignorado.

## Licença

MIT.
