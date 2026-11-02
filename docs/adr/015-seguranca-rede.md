# ADR 015 — Segurança na rede local

Status: aceito (02/11/2026)

## Contexto
O servidor é doméstico e escuta em todas as interfaces, sem login de rede. Uma auditoria ([docs/auditoria-seguranca.md](../auditoria-seguranca.md)) mostrou que, mesmo sem expor nada à internet, um aparelho da rede ou uma página aberta num navegador da casa conseguia: baixar o backup com o segredo das sessões e forjar a sessão de um perfil com PIN; disparar ações por CSRF; ler a API por *DNS rebinding*; trancar o PIN dos outros.

## Decisões
- **Backup sem segredos:** sem `session_secret`, chave do TMDB e `pin_hash` (um PIN de 4 dígitos se quebra offline em minutos). Depois de restaurar, definir os PINs de novo.
- **`guard` em toda rota:** `Host` precisa ser IP, `localhost`, nome sem ponto, `*.local`/`*.lan`/`*.home.arpa` ou estar em `ALLOWED_HOSTS`; gravação (`POST/PUT/DELETE`) com `Origin` diferente do `Host` é recusada; corpo JSON exige `Content-Type: application/json`.
- **Cabeçalhos:** `nosniff` e `Referrer-Policy: no-referrer`. `Content-Security-Policy: frame-ancestors 'self' file:`: os apps Tizen e webOS mostram a interface num `<iframe>` de uma página `file://` (`web/src/shell`), então só elas (e o próprio servidor) podem embutir; sites `http(s)` não. **Nunca** `X-Frame-Options`, que não aceita `file:` (`DENY` já causou `ERR_BLOCKED_BY_RESPONSE` na TV). Testado num Chromium atual (pai `file://` carrega, pai `http://` é bloqueado). A CSP completa fica para depois, testada na TV com Chromium 76 (JASSUB, hls.js e o script embutido do SvelteKit).
- **Sessão:** token `id.exp.mac` com validade de 90 dias, dentro da assinatura.
- **PIN:** bloqueio por perfil **e endereço**, dobrando de 30 s até 15 min, para um convidado não trancar o perfil dos outros.
- **PIN de administrador (opcional, desligado por padrão):** hash em `app_state.admin_pin`; com ele, scan, Revisar, mesclar/separar, TMDB e backup exigem o cabeçalho `X-Admin` (token de 30 min assinado com o hash do PIN; trocar o PIN encerra as sessões). `POST /api/admin/unlock`, `PUT /api/admin/pin`, `GET /api/admin/access`. A interface pede o PIN na hora da ação (`withAdmin`). Desligado por padrão para não mudar o uso de quem já tem a casa funcionando.
- **Aviso de exposição:** na inicialização, o log avisa se há endereço IPv6 global ou IPv4 público.
- **Sessão para tudo (segunda rodada):** listas, detalhes, vídeo, imagens, legendas e fontes exigem sessão de perfil. O que o navegador carrega sozinho (`<video>`, `<img>`, legendas, JASSUB) não manda `X-Profile`, então leva um **token de mídia** (`?m=`, só `GET/HEAD`, derivado do perfil e do hash do PIN, 36 h, estável por 12 h para o cache de imagens) devolvido no login e renovado por `GET /api/media-token`. Cookie `SameSite` foi descartado: o iframe da TV é de outro site (`file://`) e o cookie não seria enviado. Os segmentos HLS usam a chave aleatória de 128 bits da sessão de conversão como credencial (o hls.js não manda cabeçalhos).
- **CSP da interface** (`internal/api/spa.go`): `default-src 'self'`, `script-src 'self' 'unsafe-eval'` + o hash do único script inline do SvelteKit, `worker-src 'self' blob:`, `object-src 'none'`, `frame-ancestors 'self' file:`. **Medido na The Frame (Tizen 6.5, Chromium 85):** `'wasm-unsafe-eval'` não existe antes do Chromium 97, e o JASSUB faz `new WebAssembly.Module` na thread principal, que sem `'unsafe-eval'` falha ("Wasm code generation disallowed") e quebraria as legendas ASS. Inline injetado e script externo foram bloqueados na TV.
- **PIN esquecido:** `mediaserver reset-pin admin|<perfil>`. **Primeiro uso:** ao criar o primeiro perfil, a interface oferece o PIN de administrador (pode pular).
- **HTTPS na LAN: descartado por ora.** As TVs não têm como instalar uma CA própria, então um certificado autoassinado seria recusado; só valeria com um domínio real e certificado público (Let's Encrypt, DNS-01). Fica no Backlog.
- **Sair de todos os aparelhos:** `POST /api/admin/revoke` troca o segredo das sessões.
- **Administrador:** também criar perfil e ver o status exigem o PIN de administrador quando ele existe.
- **FFmpeg/ffprobe:** `-protocol_whitelist file,pipe` e `-format_whitelist` com os demuxers dos contêineres aceitos (`internal/ffsafe`).
- **Instalação:** unidade systemd com sandbox (`NoNewPrivileges`, `ProtectSystem`, famílias de endereço restritas); contêiner sem capabilities, com sistema de arquivos somente leitura.
- **CI:** `govulncheck`, `npm audit`, CodeQL, revisão de dependências, Actions por hash, Dependabot, atestado de procedência no release; branch `main` protegida (só entra por PR com CI verde).
- **`/api/health`:** sem versão; CORS `*` só para páginas que não são `http(s)` (os apps de TV, que descobrem o servidor a partir de `file://`).

## Alternativas descartadas
- Cookie com `SameSite`: o app usa o cabeçalho `X-Profile`, que um site de fora não consegue enviar sem preflight; o `guard` cobre o resto.
- Exigir HTTPS: fora de escopo na LAN (ADR de acesso externo, Backlog).

## Consequências
- Quem usar um nome com ponto no DNS da casa (ex.: `telao.casa`) precisa pôr `ALLOWED_HOSTS=telao.casa`.
- Quem definir o PIN de administrador primeiro é o dono (não há conta); o reset é pelo banco, com o servidor parado.
- `'unsafe-eval'` na CSP é o preço do JASSUB nas TVs antigas; sem ele as legendas ASS não funcionam na Tizen 6.5.
