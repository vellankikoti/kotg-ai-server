# kotg-ai-server

Minimal Go gRPC sidecar that fronts LLM providers (Ollama, OpenAI, Anthropic) for the Kubilitics AI integration. Designed to be exec'd by the kubilitics-backend AI supervisor over a per-spawn ephemeral mTLS handshake.

## Wire contract

`github.com/vellankikoti/kotg-schema@v1.0.1` — see that repo for the full proto definitions.

This server implements:

- `kotg.v1.AIControl/Capabilities` — returns `{schema_version:"1.0.1", ai_version, providers, models, supports_undo:false, supports_plans:false}`
- `kotg.v1.Chat/CreateSession`, `Send` (bidi stream), `CancelTurn`, `ListSessions`
- `grpc.health.v1.Health/Check`

## Lifecycle

The supervisor:
1. Mints an ephemeral CA + server cert + client cert in memory.
2. Exec's `kotg-ai-server` with provider flags.
3. Writes 3 length-prefixed PEM payloads (CA, server cert, server key) to stdin.

The server:
1. Reads the cert blob from stdin.
2. Validates flags (`--provider`, `--endpoint`, `--model`, optional `--api-key-env`).
3. Binds `127.0.0.1:0`.
4. Prints `READY <port>\n` to stdout (only after bind succeeds).
5. Serves gRPC over mTLS until SIGTERM.

## CLI flags

```
--provider                 ollama|openai|anthropic   (required)
--endpoint                 base URL                  (required)
--model                    provider-specific model   (required)
--api-key-env              env-var name holding API key (optional; required for openai/anthropic)
--session-ttl              idle session TTL          (default 15m)
--max-sessions             cap                       (default 1000)
--max-messages-per-session cap                       (default 100)
--max-budget-tokens        per-call token budget     (default 16000)
```

## Smoke test

Run a local Ollama, then exercise the full handshake via the e2e test:

```sh
# 1. Build
go build -o /tmp/kotg-ai-server ./cmd/kotg-ai-server/

# 2. Pull a model
ollama pull qwen2.5-coder:7b

# 3. Run the e2e harness (uses test cert+blob plumbing; no real LLM call)
go test ./cmd/kotg-ai-server/ -run TestE2E -v
```

For a real chat smoke test, drive the binary from kubilitics-backend with `KUBILITICS_AI_ENABLED=true KUBILITICS_AI_BINARY_PATH=/tmp/kotg-ai-server` and curl the AI WS endpoint per the supervisor spec.

## Security model

- mTLS is **mandatory** even on localhost — supervisor mints fresh CA + certs per spawn, server requires `tls.RequireAndVerifyClientCert`.
- API keys are **never** passed via argv. The server reads them from the env var named in `--api-key-env`.
- API keys, full prompts, and full completions are **never** logged. Only `provider`, `model`, `endpoint`, `port`, and per-request metadata (cluster_id, session_id, latency, status code) are logged.
- Sessions are in-memory only — no persistence. All state is lost on process restart by design.

## Out of scope (later versions)

- Tool calling / function calling — v1.5
- MCP server registration — v1.5
- RAG / vector search — v2
- Multi-agent — v2
- Server-side session summarization or persistence — v2

See `docs/superpowers/specs/2026-04-19-kotg-ai-server-v1-design.md` in the kubilitics repo for the full design + roadmap.

## License

Apache 2.0 (planned). See `LICENSE` if/when added.
