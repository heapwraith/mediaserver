# SPRINTS.md

Planejamento do projeto. **Sprint ativa: 5.**

Regras:
- Marcar `[x]` só quando o critério de aceite da tarefa for cumprido.
- Uma sprint só termina quando **todos** os critérios de aceite dela passam.
- Tarefa nova que surgir no meio do caminho vai para o **Backlog**, não pra sprint ativa.

Duração sugerida: 1 semana por sprint (ajustável).

---

## Sprint 0 — Fundação

**Objetivo:** repositório pronto, servidor sobe, banco migra, frontend vazio é servido pelo binário.

- [x] Inicializar módulo Go 1.27 (`mediaserver`), estrutura de pastas do `AGENTS.md` §3
- [x] `internal/config`: flags > env > `config.toml`, com validação (`MEDIA_DIR` existe e é legível)
- [x] Logs com `slog` (texto em dev, JSON em prod)
- [x] `internal/db`: SQLite em WAL, `busy_timeout`, migrações `.sql` embutidas e versionadas
- [x] Migração 001: tabelas `libraries`, `media_files`, `movies`, `series`, `seasons`, `episodes`, `subtitles`, `profiles`, `progress`
- [x] Servidor HTTP com `chi`, graceful shutdown, `GET /api/health`
- [x] SvelteKit 3 (`npx sv create`) com `adapter-static` em `web/`, build embutido via `embed.FS`, fallback para SPA
- [x] `Makefile` com `dev`, `build`, `test`, `fmt`, `check`, `bench`
- [x] `.golangci.yml` (v2) com os linters e formatters do `AGENTS.md` §9
- [x] `.oxlintrc.json` + config do Oxfmt + svelte-check com tsgo + Vitest no `web/`
- [x] `web/.browserslistrc`, Vite com `build.target` = `chrome76` e Lightning CSS como transformer
- [x] `uno.config.ts` com `preset-wind3`, `preset-legacy-compat`, tema base e `blocklist` do `AGENTS.md` §6
- [x] Build cruzado: `windows`, `linux` e `darwin` (amd64 e arm64)
- [x] `.gitignore`, `README.md` mínimo, ADR 001 (escolha da stack)

**Aceite**
- `make build` gera os 6 binários (Windows, Linux e macOS; amd64 e arm64) sem CGO
- O binário sobe em < 500 ms, cria o banco e responde `/api/health`
- Abrir `http://IP:8080` no navegador mostra a página do frontend

---

## Sprint 1 — Scanner e normalização virtual

**Objetivo:** qualquer pasta de mídias, organizada ou não, vira uma biblioteca limpa no banco, sem tocar nos arquivos.

- [x] Parser de nomes (`internal/scanner/parse.go`) cobrindo todos os padrões do `AGENTS.md` §4
- [x] Testes table-driven do parser com **no mínimo 150 casos** reais (nomes de release bagunçados de filmes, séries e animes)
- [x] Classificação automática (filme / série / anime / revisar) pela tabela do `AGENTS.md` §4, usando pasta só como pista
- [x] Agrupamento por título normalizado (episódios espalhados em pastas diferentes viram uma série só)
- [x] Classificação de arquivos secundários: sample (ignorado), **trailer** e **extras** (vinculados ao filme/série/temporada da mesma pasta, tabela `extras` com tipo)
- [x] Migração `overrides` + hash parcial (primeiros e últimos 64 KB) para identificar arquivo movido/renomeado
- [x] Varredura com `filepath.WalkDir` e workers concorrentes (limite configurável)
- [x] Scan incremental: caminho + tamanho + `mtime`; remove do banco o que sumiu do disco
- [x] `internal/probe`: `ffprobe` em JSON → duração, contêiner, codecs, resolução, HDR, faixas de áudio e legenda
- [x] Detecção de legendas laterais com idioma
- [x] Detecção de `poster.jpg` / `folder.jpg` / `fanart.jpg` locais
- [x] Fila **Revisar** com arquivos ambíguos ou não reconhecidos
- [x] API de correção: `PUT /api/review/{fileId}` (tipo, título, ano, temporada, episódio) e `POST /api/series/merge` / `split`
- [x] Watcher com `fsnotify` + debounce (arquivo copiado só é processado quando termina de copiar)
- [x] `POST /api/library/scan` (scan manual) e status do scan em `GET /api/library/scan`

**Aceite**
- Scan inicial de uma biblioteca de teste com 1.000 arquivos indexa tudo corretamente
- Segundo scan sem mudanças: < 3 s para 10 mil arquivos (benchmark registrado)
- Copiar um episódio novo pra pasta faz ele aparecer sem scan manual
- Pasta de teste **toda bagunçada** (tudo na raiz, nomes de release) gera uma biblioteca correta em ≥ 95% dos arquivos; o resto cai em Revisar
- Uma correção manual persiste após rescan e após renomear o arquivo
- Nenhum arquivo do usuário é alterado (teste verifica `mtime` e hash antes/depois)

---

## Sprint 2 — API e direct play

**Objetivo:** dá pra listar a biblioteca e assistir um arquivo compatível no navegador.

- [x] `GET /api/movies`, `GET /api/series`, `GET /api/series/{id}` (temporadas + episódios), `GET /api/animes`
- [x] `GET /api/media/{id}`: detalhes técnicos e faixas
- [x] `GET /api/movies/{id}/extras` e `GET /api/series/{id}/extras`: trailers e extras (tocam pelo mesmo endpoint de stream)
- [x] `GET /api/media/{id}/stream`: direct play com `http.ServeContent` (Range)
- [x] Proteção contra path traversal (nenhum caminho fora de `MEDIA_DIR`)
- [x] Pipeline de imagens (`AGENTS.md` §7): pôster/fanart local recortado em 16:9 ou **melhor frame extraído** (sem frames pretos), em WebP 480/960/1920, versão desfocada pré-renderizada e cor dominante
- [x] `GET /api/images/{id}?w=960&variant=card|backdrop|blur` com cache em `DATA_DIR`
- [x] Paginação e ordenação (título, data de adição, ano)
- [x] Headers de cache (`ETag` / `Cache-Control`) para imagens e listagens
- [x] Testes de integração dos handlers (`httptest`)

**Aceite**
- Um `.mp4` H.264/AAC toca no navegador e o *seek* é instantâneo
- Listagem da biblioteca < 50 ms p95 com 10 mil itens
- Requisição com `../` no caminho retorna 400/404

---

## Sprint 3 — Frontend web

**Objetivo:** interface navegável em qualquer navegador de PC, pronta pra controle remoto.

- [x] Tokens visuais do `AGENTS.md` §7 no `uno.config.ts` (cores, tipografia Inter auto-hospedada, espaçamento, raios, área segura)
- [x] Layout base estilo Apple TV+: menu (Início, Filmes, Séries, Animes), tema escuro
- [x] Tela inicial com *hero* + fileiras (Continuar assistindo, Próximos episódios, Adicionados recentemente, Filmes, Séries, Animes) — as duas primeiras entraram com o progresso (Sprint 4)
- [x] Cards 16:9 com estado de foco (escala 1.08 + sombra + contorno) e card tipográfico quando não houver imagem
- [x] Navegação espacial com `lrud` + memória de foco por fileira + Voltar em níveis
- [x] Fileiras virtualizadas, imagens lazy com placeholder na cor dominante
- [x] Página de detalhes (filme) e de série/anime (temporadas → episódios)
- [x] Botão **"Assistir trailer"** quando houver trailer, e aba **"Extras"** quando houver extras
- [x] Trailer tocando **sem som, em loop, no fundo da página de detalhes** (desligável nas configurações; desligado automaticamente em TVs com pouca memória)
- [x] Player ~~Video.js 10~~ próprio no `<video>` (ADR 004) com sobreposição mínima (some após 3 s), barra de progresso grande, ±10 s e menu de áudio (troca de faixa só no Safari até a Sprint 5)
- [x] Navegação por teclado: setas, Enter, Esc/Backspace = voltar (**base para a TV**)
- [x] Busca local (no título) com debounce
- [x] Tela **Revisar**: corrigir tipo/título/temporada/episódio, mesclar e separar séries
- [x] Tela de configurações: disparar scan, ver status

**Aceite**
- Todo o fluxo (início → série → episódio → assistir → voltar) funciona **só com o teclado**
- Bundle inicial < 150 KB gzip
- Navegação por controle a 60 fps numa TV de 2021 (ou emulador Chromium 76)
- Nenhuma tela usa `filter: blur()` ou `backdrop-filter`
- Lighthouse Performance ≥ 90 no desktop
- Nenhuma propriedade da lista de proibidos no CSS final (checagem automática no `make check`)
- Testado em Chrome, Edge, Firefox e Safari (Windows, Linux e macOS)

---

## Sprint 4 — Perfis e progresso

**Objetivo:** cada pessoa da casa tem o seu "continuar assistindo".

- [x] Perfis locais (nome, avatar com cor/inicial, PIN opcional de 4 dígitos)
- [x] Tela "Quem está assistindo?"
- [x] Salvar progresso a cada 10 s e ao pausar/sair (`PUT /api/progress/{mediaId}`)
- [x] Marcar como assistido automaticamente a partir de 90%
- [x] Fileira "Continuar assistindo" e "Próximo episódio" na tela inicial
- [x] Autoplay do próximo episódio com contagem regressiva (cancelável)
- [x] Marcar/desmarcar como assistido manualmente (episódio, temporada, série)

**Aceite**
- Parar um episódio no PC e retomar na TV (ou em outro navegador) no mesmo ponto
- O progresso de um perfil não aparece em outro

---

## Sprint 5 — Transcodificação HLS

**Objetivo:** qualquer arquivo toca em qualquer cliente.

- [x] Detecção de capacidade do cliente (o frontend envia codecs suportados via `MediaSource.isTypeSupported`)
- [x] Decisão: direct play → remux (só troca o contêiner) → transcodificação (áudio e/ou vídeo)
- [x] Detecção de hardware na inicialização: NVENC, QSV, AMF, VAAPI ou VideoToolbox, com CPU como fallback (`HWACCEL=auto`)
- [x] Geração de HLS **fMP4** sob demanda, segmentos de 4–6 s
- [x] *Seek* para trechos ainda não transcodificados (reiniciar o FFmpeg no ponto certo)
- [x] Cache de segmentos em `DATA_DIR/transcode`, com limite de tamanho e limpeza por LRU
- [x] Encerrar o FFmpeg quando o cliente para de pedir segmentos (timeout)
- [x] Limite de transcodificações simultâneas (configurável)
- [x] Escolha de faixa de áudio na transcodificação (ex.: áudio japonês em anime)

**Aceite**
- MKV HEVC 10-bit + áudio DTS toca no Chrome, no Firefox e no Safari
- Cada cliente recebe direct play quando suporta o arquivo e transcodificação só quando precisa
- Primeiro segmento em < 2 s com aceleração por hardware
- Fechar a aba encerra o processo do FFmpeg em até 30 s
- Nenhum processo órfão após 50 inícios/paradas seguidos

---

## Sprint 6 — Legendas

**Objetivo:** legendas funcionando, inclusive as ASS estilizadas de anime.

- [ ] Legendas laterais SRT → convertidas para WebVTT sob demanda
- [ ] Extração de legendas embutidas no MKV (cache em `DATA_DIR`)
- [ ] ASS/SSA renderizadas com **JASSUB** no cliente
- [ ] Extração das **fontes anexadas** no MKV (attachments) para o JASSUB
- [ ] Seleção de legenda no player + legenda padrão por perfil (ex.: pt-BR)
- [ ] Fallback: *burn-in* de ASS na transcodificação quando o cliente não aguenta o JASSUB (TV antiga)

**Aceite**
- Episódio de anime com ASS estilizado (karaokê, posicionamento) exibe igual ao mpv
- Legenda SRT externa com acento aparece corretamente (UTF-8 e Latin-1)

---

## Sprint 7 — Apps de TV

**Objetivo:** assistir em qualquer TV com o controle remoto, usando o mesmo frontend.

- [ ] Camada de plataforma em `web/src/lib/platform/` (teclas do controle, armazenamento local, sair do app) com implementações para navegador, Tizen, webOS e Android
- [ ] Tela inicial de configuração: IP/porta do servidor (salvo localmente), com descoberta automática na rede quando possível
- [ ] Mapeamento das teclas do controle (setas, OK, Voltar, Play/Pause, Avançar/Retroceder, teclas coloridas)
- [ ] Foco visível e área de toque grande (layout 10-foot), testado em 720p, 1080p e 4K
- [ ] **Samsung (Tizen):** empacotamento `.wgt` em `tv/tizen/`
- [ ] **LG (webOS):** empacotamento `.ipk` em `tv/webos/`
- [ ] **Android TV / Google TV / Fire TV:** wrapper WebView `.apk` em `tv/android/`, com *leanback launcher*
- [ ] Teste de codecs nativos de cada plataforma e ajuste da decisão de direct play
- [ ] Guias de instalação por plataforma (modo desenvolvedor / sideload) em `docs/tv/`

**Aceite**
- Navegação completa só com o controle em Tizen, webOS e Android TV
- Arquivo H.264/HEVC + AAC toca em direct play nas três plataformas, sem transcodificar
- Nenhum `if (plataforma)` fora de `web/src/lib/platform/`
- Interface sem quebra de layout numa TV (ou emulador) com **Chromium 76** (Tizen 6.0 / 2021)

---

## Sprint 8 — Polimento e release

**Objetivo:** versão 1.0 estável para uso diário.

- [ ] Rodar como serviço com início automático: **Windows Service**, **systemd** (Linux) e **launchd** (macOS)
- [ ] Backup e restauração do banco (`/api/admin/backup`)
- [ ] Página de status: uso de CPU/GPU, transcodificações ativas, tamanho do cache
- [ ] Tratamento de erros amigável no frontend (arquivo sumiu, codec não suportado)
- [ ] Testes de carga: 3 streams simultâneos (2 direct play + 1 transcodificado)
- [ ] `README.md` completo: instalação, configuração, convenção de pastas
- [ ] Release `v1.0.0` com binários Windows, Linux e macOS + pacotes das TVs (`.wgt`, `.ipk`, `.apk`)

**Aceite**
- 7 dias de uso contínuo sem reiniciar e sem vazamento de memória
- Todas as metas do `AGENTS.md` §8 batidas e registradas

---

## Backlog (sem sprint)

- Acesso externo via **Tailscale** (`tailscale serve`)
- Metadados externos opcionais (TMDB, AniList)
- Pular abertura/encerramento (detecção por *fingerprint* de áudio)
- *Trickplay* (miniaturas na barra de progresso)
- Múltiplas bibliotecas / múltiplas pastas raiz
- Coleções (ex.: filmes de uma franquia)
- Pré-transcodificação noturna da biblioteca
- **Organizador físico** (renomear/mover para o formato padrão), com dry-run, desfazer e modo hardlink
- Apps nativos para **Apple TV (tvOS)** e **Roku**, que não rodam apps web
- *Cast* (Chromecast / AirPlay) a partir do navegador do PC
- Escolha do nome do projeto e troca do placeholder `mediaserver`
- Scanner: arquivo ainda "em cópia" no scan de inicialização só é reprocessado quando chega outro evento do disco; reagendar um scan quando `pending > 0`
- Imagens: pré-gerar após o scan (cor dominante disponível já na 1ª listagem) e limpar o cache de itens removidos
- Frontend: migrar os estilos dos componentes (hoje em `<style>`) para classes UnoCSS com os tokens do tema (AGENTS.md §9)
- Transcodificação: *tone mapping* HDR→SDR (hoje um HDR convertido para H.264 fica desbotado)
- Transcodificação: manter 5.1 quando o cliente suporta (hoje o áudio convertido é AAC estéreo)
- Transcodificação: pausar o FFmpeg quando ele está muito à frente do player (*throttling*)
- Transcodificação: matar os FFmpeg filhos se o servidor cair à força (Job Object no Windows, `Pdeathsig` no Linux); no encerramento normal nada sobra