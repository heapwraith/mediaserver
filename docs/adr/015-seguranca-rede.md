# ADR 015 — Segurança na rede local

Status: aceito (02/11/2026)

## Contexto
O servidor é doméstico e escuta em todas as interfaces, sem login de rede. Uma auditoria ([docs/auditoria-seguranca.md](../auditoria-seguranca.md)) mostrou que, mesmo sem expor nada à internet, um aparelho da rede ou uma página aberta num navegador da casa conseguia: baixar o backup com o segredo das sessões e forjar a sessão de um perfil com PIN; disparar ações por CSRF; ler a API por *DNS rebinding*; trancar o PIN dos outros.

## Decisões
- **Backup sem segredos:** sem `session_secret`, chave do TMDB e `pin_hash` (um PIN de 4 dígitos se quebra offline em minutos). Depois de restaurar, definir os PINs de novo.
- **`guard` em toda rota:** `Host` precisa ser IP, `localhost`, nome sem ponto, `*.local`/`*.lan`/`*.home.arpa` ou estar em `ALLOWED_HOSTS`; gravação (`POST/PUT/DELETE`) com `Origin` diferente do `Host` é recusada; corpo JSON exige `Content-Type: application/json`.
- **Cabeçalhos:** `X-Frame-Options: DENY`, `frame-ancestors 'none'`, `nosniff`, `Referrer-Policy: no-referrer`. A CSP completa fica para depois, testada na TV com Chromium 76 (JASSUB, hls.js e o script embutido do SvelteKit).
- **Sessão:** token `id.exp.mac` com validade de 90 dias, dentro da assinatura.
- **PIN:** bloqueio por perfil **e endereço**, dobrando de 30 s até 15 min, para um convidado não trancar o perfil dos outros.
- **`/api/health`:** sem versão; CORS `*` só para páginas que não são `http(s)` (os apps de TV, que descobrem o servidor a partir de `file://`).

## Alternativas descartadas
- Cookie com `SameSite`: o app usa o cabeçalho `X-Profile`, que um site de fora não consegue enviar sem preflight; o `guard` cobre o resto.
- Exigir HTTPS: fora de escopo na LAN (ADR de acesso externo, Backlog).

## Consequências
- Quem usar um nome com ponto no DNS da casa (ex.: `telao.casa`) precisa pôr `ALLOWED_HOSTS=telao.casa`.
- Ainda falta o **PIN de administrador** (Revisar, scan, mesclar/separar, backup, status) e a CSP completa; estão no Backlog.
