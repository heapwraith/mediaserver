# Auditoria de segurança (02/11/2026)

Revisão técnica do código (Go, frontend embutido, Docker, apps de TV), **não é teste de invasão profissional**. Modelo de ameaça, pelo propósito do projeto: servidor **doméstico**, só na rede local, usado por quem mora na casa. O adversário realista não é um hacker externo: é (1) um convidado ou aparelho comprometido na mesma rede (Wi-Fi da visita, TV ou IoT infectado), (2) uma página web maliciosa aberta num navegador da casa que dispara requisições para o IP do servidor, e (3) um operador que expõe a porta à internet por engano.

As provas abaixo foram feitas numa instância local descartável (`MEDIA_DIR` vazio).

## Correções aplicadas (02/11/2026)

| Item | O que mudou |
|---|---|
| 1 | O backup sai sem `session_secret`, chave do TMDB e `pin_hash` (copia, limpa e roda um segundo `VACUUM INTO`, para os valores não ficarem em páginas livres). Depois de restaurar, os PINs precisam ser definidos de novo. **Resta:** restringir backup/status a um administrador (item 2). |
| 3 | Middleware `guard`: recusa `Host` que não seja IP, `localhost`, nome sem ponto, `*.local`/`*.lan`/`*.home.arpa` ou `ALLOWED_HOSTS` (DNS rebinding); recusa `POST/PUT/DELETE` com `Origin` de outro site (CSRF); `decodeJSON` exige `Content-Type: application/json`. |
| 4 | Bloqueio de PIN por perfil **e endereço**, dobrando a cada rodada (30 s até 15 min). |
| 5 | Token de sessão com validade de 90 dias, dentro da assinatura (`id.exp.mac`). Quem já estava logado escolhe o perfil de novo uma vez. |
| 6 | `nosniff`, `Referrer-Policy: no-referrer`. Bloqueio de iframe: `X-Frame-Options: DENY` quebrou a TV (`ERR_BLOCKED_BY_RESPONSE`, os apps mostram o servidor num iframe); agora é `frame-ancestors 'self' file:`, que libera só as páginas `file://` dos apps. **Resta:** uma CSP completa (`script-src`, `worker-src`...): o JASSUB, o hls.js e o script embutido do SvelteKit exigem testar numa TV com Chromium 76 antes. |
| 7 | `/api/health` não devolve mais a versão e só envia CORS `*` a páginas de app (`file://`/`null`), não a sites `http(s)`. |

| 2 | PIN de administrador opcional ([ADR 015](adr/015-seguranca-rede.md)): scan, Revisar, mesclar/separar, TMDB e backup pedem `X-Admin`. Desligado por padrão. |
| exposição | Aviso no log com IPv6 global ou IPv4 público; exemplo de `ports` restrito no `docker-compose.yml`. |

**Ainda aberta:** CSP completa (`script-src`, `worker-src`), a testar na TV com Chromium 76.

## Segunda rodada (02/11/2026)

| Item | O que mudou |
|---|---|
| Mídia aberta a quem está na rede | Tudo exige sessão de perfil; URLs de `<video>`/`<img>`/legenda levam token de mídia (ADR 015). Testado num Chromium real contra o Docker: início, detalhes e player (HLS) com imagens, API e segmentos em 200. |
| FFmpeg com arquivo malicioso | `-protocol_whitelist file,pipe` e `-format_whitelist` em todas as chamadas (`internal/ffsafe`); testado com mp4, m4v, mov, avi, ts, webm e mkv no FFmpeg 9. |
| Criar perfil e `status` abertos | Exigem o PIN de administrador quando ele existe. |
| Sessão sem revogação | `Sair de todos os aparelhos` (Configurações). |
| Primeiro uso | O log avisa, a cada início, que não há PIN de administrador. |
| CI | `govulncheck`, `npm audit`, licenças, CodeQL, dependency-review, Dependabot, Actions por hash, atestado de procedência no release. |
| Instalação | Sandbox no systemd e no contêiner Docker (somente leitura, sem capabilities). |
| `SECURITY.md` | Como reportar e os limites conhecidos. |

## Terceira rodada: verificação na TV real (02/11/2026)

Com o app Tizen em modo de depuração, usei o DevTools remoto da The Frame (Tizen 6.5, Chromium 85) para testar **no aparelho**:

| Teste | Resultado |
|---|---|
| Iframe da interface com `frame-ancestors 'self' file:` | Carrega (a página `file://` do app é aceita). `X-Frame-Options` não pode ser usado. |
| Sessão salva, token de mídia, imagens, listas | Todos 200; imagens carregam. |
| Vídeo em *direct play* com `stream?m=…` | Toca em 1080p, retomando de onde parou, sem erro. |
| CSP: script inline injetado, script de `example.com` | Bloqueados na TV. |
| CSP: `WebAssembly.Module` na thread principal | **Bloqueado** sem `'unsafe-eval'` (Chromium 85): quebraria o JASSUB. Corrigido, e confirmado na TV. |
| Worker de `blob:` compilando WebAssembly | Funciona. |

Também: `mediaserver reset-pin`, PIN de administrador oferecido no primeiro perfil, SBOM e atestado no release, CSP completa.

**Não feito:** HTTPS na rede local (descartado: a TV não aceita CA própria; só com domínio real e certificado público); serviço do Windows com conta limitada (pode tirar o acesso à pasta de mídias do usuário, e não há como testar aqui); PIN de perfil com mais de 4 dígitos (o teclado da TV é de 4); perda de progresso com subpasta ilegível e servidor sem o HD de mídias (disponibilidade, no Backlog).

## O que está bom

- **Arquivos fora de `MEDIA_DIR`:** a mídia é aberta com `os.Root` (`stream.openFile`), que recusa `..` e symlinks que escapam. Os nomes de segmento HLS, de fonte e de chave têm *regex* fixa; as imagens aceitam só variante e largura de uma lista.
- **Sem injeção:** SQL parametrizado; FFmpeg sempre por `exec.CommandContext` com argumentos separados (sem shell) e entrada `file:`; o filtro de legenda roda dentro de uma pasta de trabalho, sem caminho para escapar.
- **PIN:** PBKDF2-SHA256 com 100 mil iterações e sal; comparação em tempo constante; 5 erros bloqueiam o perfil por 30 s.
- **Sessão:** HMAC-SHA256 com segredo de 32 bytes aleatórios; trocar o PIN invalida as sessões antigas.
- **TMDB:** host fixo (sem SSRF), erros sem a URL (a chave vai na query na v3), chave nunca volta pela API.
- **Dependências:** `govulncheck ./...` e `npm audit --omit=dev` sem vulnerabilidades. Imagem Docker sem root, mídia `:ro`, FFmpeg e Chromaprint com SHA-256 conferido.
- **App Android:** `allowBackup=false`, o WebView só navega para a origem do servidor configurado, depuração só em build *debug*. Nenhum segredo, chave ou certificado versionado.
- **Servidor HTTP:** `ReadHeaderTimeout` e `IdleTimeout` definidos (sem Slowloris de cabeçalho); corpo de JSON limitado.

## Achados

Gravidade pensando no uso doméstico. Os itens 1 e 2 já estavam no Backlog; aqui estão **comprovados**.

### 1. Alta — qualquer pessoa da rede assume qualquer perfil, mesmo com PIN (comprovado)
Passos testados, sem nenhuma credencial:
1. `POST /api/profiles` cria um perfil novo sem PIN e já devolve a sessão dele.
2. `GET /api/admin/backup` com essa sessão devolve o banco inteiro (200, 294 KB).
3. O banco tem `app_state.session_secret` e o `pin_hash` do perfil-alvo. Com os dois se calcula o HMAC da sessão do perfil-alvo.
4. `GET /api/home` com a sessão forjada responde 200.

Resultado: o PIN não protege nada contra quem está na rede. Também vazam a chave do TMDB (se houver) e o histórico de todos. Com a sessão forjada, dá para apagar ou editar o perfil-alvo.
**Correção sugerida:** (a) tirar `session_secret` e a chave do TMDB do arquivo de backup (zerar na cópia); (b) restringir backup e status a um perfil administrador; (c) criar perfil só com a sessão de um administrador, depois do primeiro (ver item 2).

### 2. Alta — administração sem autenticação
`POST /api/library/scan`, `PUT /api/review/{id}`, `POST /api/series/merge|split` e `DELETE /api/transcode/{key}` não pedem nada. Os perfis (`GET /api/profiles`, com nomes e cores) também são públicos, por desenho (tela "Quem está assistindo?").
**Correção sugerida:** PIN de administrador (definido na primeira execução) exigido para Revisar, Configurações, scan, mesclar/separar, metadados e backup. Os apps de TV só usam as rotas de reprodução.

### 3. Média — requisições de outro site chegam ao servidor (CSRF e *DNS rebinding*) (comprovado em parte)
- O servidor **não confere `Origin` nem `Host`** e `decodeJSON` **não exige `Content-Type: application/json`**. Um `POST` com `text/plain` e `Origin: http://evil.example` para `/api/library/scan` respondeu **202**. Uma página qualquer, aberta num navegador da casa, pode então disparar scan e, se adivinhar IDs, mesclar séries ou alterar Revisar (o `merge` de teste caiu em 400 só por ID inválido, não por barreira).
- Sem checagem de `Host`, um ataque de *DNS rebinding* (o domínio do atacante passa a apontar para `192.168.x.x`) deixa a página **ler** as respostas: perfis, biblioteca e, somando ao item 1, o backup inteiro.
**Correção sugerida:** exigir `Content-Type: application/json` nas rotas que gravam; rejeitar `Origin` presente e diferente do `Host`; aceitar só `Host` que seja IP ou nome configurado (`ALLOWED_HOSTS`, padrão: IPs e `localhost`, `*.local`).

### 4. Média — bloqueio de PIN usado como negação de serviço
O contador é por perfil, não por origem: quem erra 5 vezes o PIN do perfil-alvo o bloqueia para todos por 30 s, em loop. Comprovado: 3 erros e depois 429. Em compensação, o limite também barra a adivinhação do PIN de 4 dígitos (10 mil combinações, ~17 h no pior caso). **Correção:** limitar também por IP e aumentar o tempo de bloqueio a cada rodada.

### 5. Média — sessão sem prazo
O token é *stateless* e só muda quando o PIN muda; não há expiração nem logout no servidor. Um token copiado vale para sempre. Perfil **sem PIN** não tem proteção alguma, por desenho. **Correção:** expiração no HMAC (data dentro do token) e rotação do segredo ao restaurar um backup.

### 6. Baixa — faltam cabeçalhos de segurança
Nenhum `Content-Security-Policy`, `X-Frame-Options`/`frame-ancestors`, `X-Content-Type-Options` (só nas legendas) ou `Referrer-Policy` na interface. Sem *frame-ancestors*, uma página externa pode embutir o Telão num *iframe* (*clickjacking*). **Correção:** middleware com CSP restrita a `'self'` (o JASSUB precisa de `wasm-unsafe-eval` e *workers* `blob:`), `frame-ancestors 'none'`, `nosniff`, `Referrer-Policy: no-referrer`.

### 7. Baixa — `Access-Control-Allow-Origin: *` em `/api/health`
Devolve nome, estado e versão. A versão ajuda a escolher exploits; considerar tirar a versão do endpoint público.

### 8. Baixa — HTTP puro na rede
Tráfego sem TLS (`usesCleartextTraffic=true` no Android, `*` no Tizen). Aceitável em LAN; o PIN e o token trafegam em claro, então qualquer um que farejar a rede vê a sessão. Se um dia houver acesso externo, exigir HTTPS (Tailscale já cifra).

### 9. Baixa — arquivos de legenda e metadados de mídia são entrada não confiável
Legendas ASS/SRT, nomes de arquivo e *tags* do `ffprobe` vêm de arquivos baixados por terceiros. O servidor os trata como texto (`text/plain`, `nosniff`) e a interface usa Svelte, que escapa HTML por padrão; o único `{@html}` (`Player.svelte`) recebe só CSS montado a partir das opções de legenda, não texto de arquivo. O FFmpeg e o libass processam esses arquivos: manter o FFmpeg atualizado (a imagem Docker fixa a 9.0.2; revisar ao sair correções de segurança).

### 10. Informativo — `DELETE /api/transcode/{key}` sem sessão
Quem conhece a chave (aleatória) encerra a transcodificação de outro. Baixo risco; entra no item 2.

## Exposição para fora da rede (privacidade)

- **Saída:** por padrão o servidor não faz nenhuma chamada à internet: sem telemetria, sem checagem de atualização, sem fonte ou CDN. Só com o TMDB ligado ele consulta `api.themoviedb.org` (título, ano e a chave; o TMDB vê o IP da sua conexão).
- **Entrada:** o servidor escuta em `0.0.0.0`, que no Go vira *dual-stack* (o log mostra `[::]:porta`). Se o provedor entrega **IPv6 público** e o roteador não bloqueia entrada, a porta fica **acessível pela internet**, com todos os achados acima. O mesmo vale para encaminhamento de porta no roteador. O servidor não abre porta sozinho (sem UPnP). No Docker, `ports: "8080:8080"` publica em todas as interfaces (no Linux o Docker ignora o `ufw`).
- **Identificação:** `/api/health` é público, com `Access-Control-Allow-Origin: *`, e devolve `name: mediaserver` e a versão. Qualquer site aberto num navegador da casa pode varrer a rede local e descobrir o Telão e a versão; um *scanner* da internet acha o servidor se a porta estiver exposta.
- **Correção sugerida:** tirar a versão do `/api/health`; restringir o CORS à origem dos apps de TV; avisar na inicialização (log e Configurações) quando houver endereço IPv6 público ou o `HOST` não for privado; documentar no README: confirmar no roteador que não há encaminhamento de porta nem IPv6 liberado, e no Docker usar `127.0.0.1:8080:8080` ou o IP da rede interna.

## Ordem sugerida
1. Tirar segredos do backup e restringir backup/status (item 1) — pequeno e fecha o pior caso.
2. `Content-Type` JSON + checagem de `Origin`/`Host` (item 3) — poucas linhas num middleware.
3. PIN de administrador (item 2) e prazo de sessão (item 5) — sprint própria.
4. Cabeçalhos (item 6) e limite por IP (item 4).

## Não coberto
Teste de carga/DoS do transcodificador, *fuzzing* do parser de nomes e de legendas, revisão do instalador de serviço (systemd/launchd/Windows) e do empacotamento `.ipk`/`.wgt`.
