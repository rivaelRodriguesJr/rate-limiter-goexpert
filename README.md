# Rate Limiter

Um middleware HTTP (Go, `net/http` + chi) que limita requisições por segundo por **IP do cliente** ou por **token de acesso** (header `API_KEY`), guardando o estado no **Redis**. O limite do token tem precedência sobre o limite do IP.

## Arquitetura

```
requisição ─▶ middleware.RateLimit ─▶ limiter.Limiter ─▶ storage.Store (interface)
              (IP + API_KEY, 429)     (regra de negócio)    ├─ Redis   (produção)
                                                            └─ Memory  (testes unitários)
```

- `internal/middleware` – extrai IP/token, chama o limiter e escreve o 429.
- `internal/limiter` – lógica independente de HTTP: verifica bloqueio → contador (janela de 1s) → bloqueia ao estourar o limite.
- `internal/storage` – interface `Store` (strategy) com implementação em Redis (`INCR` + `PEXPIRE NX` em uma transação; os bloqueios são chaves com TTL) e em memória (com `Clock` injetável).
- `internal/config` – variáveis de ambiente + `.env` opcional; falha logo na inicialização se algum valor for inválido.

Quando o limite é excedido, o cliente recebe `429` com o corpo
`you have reached the maximum number of requests or actions allowed within a certain time frame` e fica bloqueado por `BLOCK_DURATION_SECONDS`; toda requisição nesse período recebe o mesmo 429. O header `Retry-After` também é enviado.

**Falhas do Redis:** os erros são logados; com `FAIL_OPEN=true` (padrão) as requisições são permitidas, com `false` são rejeitadas com 429.

**IP do cliente:** primeiro valor de `X-Forwarded-For`, senão `X-Real-IP`, senão `RemoteAddr` sem a porta. Esses headers são controlados pelo cliente; só confie neles atrás de um proxy que os sobrescreva.

## Configuração

| Variável | Padrão | Descrição |
|---|---|---|
| `RATE_LIMIT_IP` | `10` | Máximo de requisições/segundo por IP |
| `RATE_LIMIT_TOKEN` | `100` | Máximo de requisições/segundo por token (padrão) |
| `TOKEN_LIMITS` | _(vazio)_ | Limites específicos por token, `token:limite,token:limite` |
| `BLOCK_DURATION_SECONDS` | `300` | Tempo de bloqueio após exceder o limite |
| `REDIS_ADDR` | _(não definido)_ | `host:porta`; se não definido, usa `REDIS_HOST`:`REDIS_PORT` |
| `REDIS_HOST` / `REDIS_PORT` | `localhost` / `6379` | Usados quando `REDIS_ADDR` não está definido |
| `REDIS_PASSWORD` | _(vazio)_ | Senha do Redis |
| `REDIS_DB` | `0` | Banco do Redis |
| `SERVER_PORT` | `8080` | Porta HTTP |
| `FAIL_OPEN` | `true` | Permite requisições quando o Redis falha |

Copie `.env.example` para `.env` para personalizar (variáveis de ambiente reais têm prioridade sobre o `.env`).

## Como rodar

```sh
cp .env.example .env   # opcional
docker compose up --build
```

## Testes (somente Docker)

```sh
docker compose run --rm test     # ou: make test
docker compose --profile test down -v
```

Isso executa `go test -race ./...` contra o container do Redis. Sem Redis (rodando localmente com `go test -race ./...`), o teste de integração do Redis é pulado.

## Exemplos

Com `RATE_LIMIT_IP=2` e `TOKEN_LIMITS=abc123:5`:

```sh
for i in 1 2 3; do curl -s -o /dev/null -w "%{http_code}\n" localhost:8080/; done
# 200 200 429
curl -i localhost:8080/            # 429, corpo: you have reached the maximum ...
# Precedência do token: mesmo IP, mas o token abc123 permite 5 req/s
for i in 1 2 3 4 5 6; do curl -s -o /dev/null -w "%{http_code}\n" -H "API_KEY: abc123" localhost:8080/; done
# 200 200 200 200 200 429
```

## Trocando a estratégia de persistência

1. Crie `internal/storage/<backend>.go` com um tipo que implemente `storage.Store`:
   `Incr(ctx, key, window) (int64, error)` (atômico; o primeiro incremento inicia a janela e o contador expira depois dela), `BlockTTL(ctx, key) (time.Duration, error)` (0 quando não está bloqueado), `Block(ctx, key, d) error` (expira sozinho), `Ping` e `Close`.
2. Adicione `var _ storage.Store = (*SeuStore)(nil)` e escreva testes parecidos com os de `internal/storage/storage_test.go` (contador, janela, bloqueio e expiração).
3. Em `cmd/server/main.go`, troque o bloco `storage.NewRedis(...)` pelo construtor do seu backend. Mais nada muda: o limiter e o middleware só conhecem a interface.