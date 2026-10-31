# AGENTS.md

Instruções para qualquer agente de IA (Claude Code, Codex, Cursor etc.) que trabalhe neste repositório.

> **Nome do projeto:** ainda não definido. Use `mediaserver` como placeholder no módulo Go, no binário e nos textos. A troca será feita de uma vez quando o nome for escolhido.

---

## 1. Visão geral

Servidor de streaming **local** e próprio para **filmes, séries e animes**, com foco em **máxima performance**.

- O usuário aponta uma **pasta local** com as mídias. O servidor indexa, organiza e transmite.
- Clientes: **qualquer PC e qualquer TV** na mesma rede.
  - **PC:** navegador moderno (Chrome, Edge, Firefox, Safari) no **Windows, Linux e macOS**.
  - **TVs com sistema web:** Samsung (Tizen) e LG (webOS), usando o mesmo frontend empacotado.
  - **Android TV / Google TV / Fire TV:** o mesmo frontend dentro de um *wrapper* Android (WebView).
  - **Qualquer outra TV** (Apple TV, Roku, TVs sem loja de apps): por enquanto via navegador da TV, quando houver, ou por *cast* (Chromecast/AirPlay) a partir do PC. Apps nativos ficam no Backlog.

  | Plataforma | App instalado na TV? | Formato | Instalação sem loja |
  |---|---|---|---|
  | PC (navegador) | Não, roda no navegador | — | — |
  | Samsung (Tizen) | **Sim** | `.wgt` | Modo desenvolvedor + Tizen Studio + certificado |
  | LG (webOS) | **Sim** | `.ipk` | Modo desenvolvedor (a sessão expira e precisa ser renovada) ou Homebrew Channel (TV com root) |
  | Android TV / Google TV / Fire TV (inclui TCL, Sony, Philips) | **Sim** | `.apk` | Sideload do `.apk` (ex.: app Downloader); fica instalado de vez |
  | Apple TV, Roku, TVs sem loja | Não, só navegador da TV ou cast | — | — |

- O **servidor** também é multiplataforma: binário para **Windows, Linux e macOS** (amd64 e arm64).
- **Escopo atual: somente rede local.** O acesso externo (Tailscale) fica pra depois e não deve exigir mudança de código.
- **Sem APIs externas.** Nada de TMDB, AniList ou qualquer outra chamada à internet. Todo metadado vem do nome do arquivo, da estrutura de pastas, do `ffprobe` e de arquivos locais (pôster, legendas).

## 2. Stack

| Camada | Tecnologia | Observação |
|---|---|---|
| Backend | **Go 1.27**, router `chi` v5 | Binário único, sem CGO. Usar `encoding/json/v2` e `uuid` da stdlib (novos no 1.27) |
| Banco | **SQLite** (`modernc.org/sqlite`), modo **WAL** | Migrações SQL versionadas e embutidas |
| Mídia | **FFmpeg / ffprobe 9.0+** (binários locais) | Detecta a GPU: NVENC (NVIDIA), QSV (Intel), AMF (AMD/Windows), VAAPI (Linux), VideoToolbox (macOS) → CPU |
| Frontend | **SvelteKit 3** (`adapter-static`) + **Svelte 5** + TypeScript | Config no `vite.config.ts` (sem `svelte.config.js`). Build embutido via `embed.FS` |
| Navegação por controle | **lrud** (BBC), via *action* do Svelte | Navegação espacial independente de framework, feita para TV |
| Player | **Player próprio** sobre o `<video>` nativo + **hls.js** (Sprint 5) | O Video.js 10 exige Chrome 111+ (ADR 004). **JASSUB** pra legendas ASS |
| TV | Mesmo frontend empacotado: `.wgt` (Tizen), `.ipk` (webOS), `.apk` (Android TV/Fire TV, WebView) | Navegação por controle remoto; um código só |
| Qualidade (Go) | **golangci-lint v2** (`run` + `fmt`) + `go fix` | Um só binário roda staticcheck, govet, gosec, modernize e formata com gofumpt + gci |
| Estilo | **UnoCSS 66.x** (`preset-wind3` + `preset-legacy-compat`) + **Lightning CSS** | Classes estilo Tailwind geradas só no build (zero JS na TV). O Lightning CSS converte o CSS final pros alvos do browserslist. **Não** usar Tailwind v4 nem `preset-wind4` (exigem Chrome 111+) |
| Qualidade (web) | **Oxlint** + **Oxfmt** + **svelte-check** (com tsgo) | Oxc (Rust): ~30x mais rápido que ESLint/Prettier. O svelte-check cobre os templates `.svelte`, que o Oxlint ainda não analisa |
| Testes (web) | **Vitest** | Mesmo pipeline do Vite |

Versões conferidas em 31/10/2026. Sempre instalar a última *patch* da versão indicada; subir *major* só com ADR.

**Não adicionar** sem discutir antes: ORMs pesados, frameworks de DI, Docker como requisito, dependências com CGO, bibliotecas de UI grandes (Material, Bootstrap etc.).

## 3. Estrutura do repositório

```
cmd/mediaserver/        → main.go (flags, bootstrap, graceful shutdown)
internal/config/        → leitura de config (flags > env > arquivo)
internal/db/            → conexão SQLite, migrações (embed), queries
internal/library/       → modelos de domínio (Movie, Series, Season, Episode, MediaFile)
internal/scanner/       → varredura da pasta, parser de nomes, watcher (fsnotify)
internal/probe/         → wrapper do ffprobe (codecs, duração, faixas)
internal/stream/        → direct play (Range) e transcodificação HLS
internal/subtitles/     → legendas externas e embutidas, SRT→VTT
internal/api/           → handlers HTTP, rotas, middlewares
web/                    → SvelteKit (o build vai para web/build, embutido)
tv/tizen/               → empacotamento Samsung (.wgt)
tv/webos/               → empacotamento LG (.ipk)
tv/android/             → wrapper Android TV / Google TV / Fire TV (.apk)
migrations/             → arquivos .sql numerados
docs/                   → decisões de arquitetura (ADRs)
```

## 4. Pasta de mídias e normalização virtual

O usuário configura `MEDIA_DIR` e pode organizar os arquivos **como quiser**: tudo numa pasta só, subpastas aleatórias, nomes de release bagunçados. **O servidor nunca renomeia, move ou apaga arquivos.** Toda a organização é **virtual**: o scanner interpreta cada arquivo e grava no banco a estrutura limpa (tipo, título, ano, temporada, episódio), e a interface mostra só essa versão limpa.

### Classificação automática
| Sinal no nome / caminho | Classificação |
|---|---|
| `S01E01`, `s1e1`, `1x01`, pasta `Temporada 01` / `Season 1` | Série |
| Episódio absoluto (` - 01`, `EP01`, `E01`) + tags de grupo (`[SubsPlease]`, `[Erai-raws]`) ou pasta com "anime" | Anime |
| Ano (`(2020)`, `.2020.`) sem marcação de episódio | Filme |
| Nenhum sinal claro ou sinais conflitantes | **Revisar** |

A pasta onde o arquivo está também conta como pista (ex.: uma pasta chamada `Animes`, `Filmes` ou `Series` reforça a classificação), mas **não é obrigatória**.

### O parser deve aceitar, no mínimo
- `S01E01`, `s1e1`, `1x01`, `S01E01-E02` (episódio duplo)
- Anime com episódio absoluto: ` - 01`, ` - 1080`, `EP01`, `E01`, `v2` (versão)
- Ano: `(2020)`, `.2020.`, `[2020]`
- Separadores `.`, `_` e `-` tratados como espaço
- Tags de release ignoradas no título: grupo (`[SubsPlease]`), resolução (`1080p`, `2160p`), codec (`x265`, `HEVC`, `AV1`), fonte (`WEB-DL`, `BluRay`), áudio (`DTS`, `AAC`), hash (`[ABC123EF]`), `REPACK`, `PROPER`
- Títulos com acento, Unicode e caracteres japoneses
- Extensões de vídeo: `.mkv .mp4 .m4v .avi .webm .mov .ts`
- Legendas laterais: `.srt .ass .ssa .vtt`, com idioma opcional (`.pt-BR.srt`, `.en.ass`)
- Arquivos que não são mídia principal **não entram como filme/episódio** (o arquivo nunca é tocado):
  - **Sample** (`sample` no nome ou na pasta, ou < 50 MB com essa marcação): ignorado.
  - **Trailer** (`-trailer`, `.trailer.`, `trailer.mkv`, pasta `trailers/`): vinculado ao filme/série/temporada da mesma pasta como **trailer**.
  - **Extras** (pastas `extras/`, `featurettes/`, `behind the scenes/`, `deleted scenes/`, `interviews/`, ou sufixos `-featurette`, `-behindthescenes`, `-deleted`, `-interview`): vinculados como **extras**, com o tipo identificado.

### Agrupamento
- Episódios do mesmo título normalizado (minúsculas, sem acento, sem pontuação) formam **uma série/anime**, mesmo espalhados em pastas diferentes.
- O título exibido é o mais "limpo" encontrado no grupo (preferindo o nome da pasta, quando existir).

### Fila "Revisar" e correções manuais
- Arquivo ambíguo ou não reconhecido **nunca é descartado em silêncio**: ele entra na fila **Revisar** da interface.
- Ali o usuário confirma ou corrige: tipo, título, ano, temporada e episódio, e pode **mesclar** ou **separar** séries.
- Cada correção é gravada como **override** no banco (tabela `overrides`, chave = caminho relativo + tamanho) e tem prioridade sobre o parser nos próximos scans.
- Os overrides sobrevivem a renomear ou mover o arquivo quando o tamanho e o hash parcial (primeiros e últimos 64 KB) baterem.

### Formato recomendado (opcional)
Quem quiser organizar manualmente pode seguir este padrão, que garante 100% de acerto:

```
<MEDIA_DIR>/
├── Filmes/Nome do Filme (2020)/Nome do Filme (2020).mkv
├── Series/Nome da Série/Temporada 01/Nome da Série - S01E01.mkv
└── Animes/Nome do Anime/Nome do Anime - 01.mkv
```

## 5. Regras de arquitetura

1. **Direct play primeiro.** Só transcodificar quando o cliente não suporta o contêiner, o codec de vídeo ou o de áudio. A decisão é **por cliente**, a partir dos codecs que ele declara: cada navegador e cada TV suporta coisas diferentes (ex.: Firefox sem HEVC, Safari com HLS nativo). Nunca decidir pelo *user-agent*.
2. **Um frontend só.** Nada de código específico de plataforma fora de `tv/` e de uma camada fina de adaptação (`web/src/lib/platform/`) para teclas do controle, armazenamento e saída do app.
3. **Servir arquivos com `http.ServeContent`** (suporte a Range e `sendfile`). Nunca carregar arquivo de vídeo inteiro em memória.
4. **Scan incremental:** comparar caminho + tamanho + `mtime`. Só rodar `ffprobe` em arquivos novos ou alterados.
5. **FFmpeg sempre via `exec.CommandContext`**, com cancelamento quando o cliente desconecta. Nenhum processo órfão.
6. **IDs estáveis:** o ID de uma mídia não pode mudar entre scans enquanto o arquivo não mudar de caminho.
7. **Caminhos:** sempre `filepath` (o servidor roda no Windows, Linux e macOS). Testar caminhos com espaço, acento e Unicode.
8. **Segurança local mínima:** nenhum endpoint pode servir arquivo fora de `MEDIA_DIR` (validar contra path traversal). O servidor escuta em `0.0.0.0` por padrão.
9. **Configuração** em ordem de prioridade: flags > variáveis de ambiente > `config.toml`. Nada hardcoded.
10. **Logs** com `log/slog` (JSON em produção, texto em dev).

## 6. Compatibilidade de navegadores

**Piso: Chromium 76** (Samsung Tizen 6.0 e LG webOS 6, ou seja, TVs de 2021 em diante). TVs mais antigas não são alvo oficial.

`web/.browserslistrc` (fonte única para CSS e JS):

```
chrome >= 76
safari >= 15
firefox >= 115
edge >= 115
```

- **Vite:** `build.target` derivado do browserslist (`chrome76`), para converter `?.`, `??` e outras sintaxes novas de JS.
- **Lightning CSS:** `css.transformer = 'lightningcss'` com os mesmos alvos. Pode escrever aninhamento, `oklch()`, `color-mix()` com valores fixos, media queries com intervalo, `@custom-media` e propriedades lógicas: tudo é convertido.
- **UnoCSS:** `preset-wind3` + `preset-legacy-compat({ commaStyleColorFunction: true, legacyColorSpace: true })`, com `outputToCssLayers: false`.

### Proibido (não tem conversão possível para o Chromium 76)

| Recurso | Precisa de | Usar no lugar |
|---|---|---|
| `gap` em **flexbox** | Chrome 84 | Grid com `gap`, ou margem nos filhos (`space-x-*`, `space-y-*`) |
| `aspect-ratio` | Chrome 88 | Proporção via padding ou grid |
| `inset` | Chrome 87 | `top` / `right` / `bottom` / `left` |
| `:where()` | Chrome 88 | Seletores simples |
| `@layer` | Chrome 99 | Ordem dos imports + escopo do Svelte |
| `:has()`, container queries | Chrome 105 | Media queries ou classe via JS |
| `subgrid` | Chrome 117 | Grid comum |

Esses recursos são bloqueados na **`blocklist` do `uno.config.ts`** (ex.: `aspect-*`, `inset-*`, `has-*`, `@container`, `gap-*` aplicado a flex). O `preset-typography` não é usado, porque gera `:where()`. Usar algo da lista só é permitido dentro de `@supports`, como melhoria opcional, e nunca para algo essencial.

## 7. Design e UX

**Inspiração: Apple TV+ / tvOS.** Visual cinematográfico e minimalista, com o conteúdo em primeiro plano. São só *princípios* de referência: nada de logos, ícones, fontes (SF Pro) ou outros assets da Apple.

**Tema: escuro**, único por enquanto.

### Princípios
1. **Conteúdo primeiro:** a imagem do título ocupa a tela, e a interface some ao redor dela.
2. **10-foot UI:** tudo precisa ser legível e navegável a 3 metros de distância, com controle remoto.
3. **O foco é a interface:** sempre existe exatamente um elemento em foco, e ele é óbvio.
4. **Movimento calmo:** animações curtas e suaves, nunca chamativas.
5. **Funciona sem pôster oficial:** o layout é pensado para frames extraídos do vídeo, não para pôsteres de divulgação.

### Estrutura das telas
- **Início:** *hero* no topo (frame grande do título em destaque + título + linha de metadados + `▶ Continuar/Assistir` + `Trailer`), seguido de fileiras horizontais: *Continuar assistindo*, *Próximos episódios*, *Adicionados recentemente*, *Filmes*, *Séries*, *Animes*.
- **Detalhes:** frame de fundo em tela cheia com gradiente escuro embaixo, título grande, metadados (tipo · ano · duração/nº de episódios · resolução), botões de ação e trailer opcional no fundo (§4). Séries/animes: abas de temporada e lista de episódios com miniatura, número, duração e barra de progresso.
- **Player:** sobreposição mínima que some após 3 s sem interação; barra de progresso grande com miniatura do ponto; ±10 s; menu de áudio e legenda; card "Próximo episódio" nos últimos segundos.
- **Perfis, Revisar e Configurações:** mesma linguagem visual, sem telas "de admin" com cara de outro app.

### Cards e imagens
- **Cards sempre 16:9 (paisagem)**, inclusive para filmes. Se existir `poster.jpg`/`fanart.jpg` local, ele é usado, recortado em 16:9.
- **Sem imagem nenhuma:** card tipográfico (título grande sobre um gradiente gerado a partir do nome), nunca um ícone genérico de "arquivo".
- **Pipeline de imagens no servidor** (FFmpeg, cache em `DATA_DIR`):
  - Extrair candidatos em ~10%, 30% e 50% da duração, descartar frames pretos ou com pouca variação (`blackdetect` / filtro `thumbnail`) e escolher o mais nítido.
  - Gerar em **WebP** nos tamanhos 480, 960 e 1920 px de largura (`srcset`).
  - Gerar também uma versão **desfocada pré-renderizada** para fundos. **Nada de `filter: blur()` ou `backdrop-filter` no CSS**, que pesam demais nas TVs.
  - Extrair a cor dominante para o placeholder enquanto a imagem carrega.

### Foco e navegação
- Elemento em foco: **escala 1.08 + sombra elevada + contorno claro de 3 px**. Nunca depender só de mudança de cor.
- **Navegação espacial com `lrud`** (setas movem o foco para o item mais próximo naquela direção), encapsulada numa *action* do Svelte em `web/src/lib/platform/`.
- A fileira rola para manter o item em foco visível; o foco lembra a última posição de cada fileira ao voltar.
- **Voltar** sempre fecha o nível atual (player → detalhes → início). No início, Voltar pergunta se quer sair do app (só nas TVs).
- **Mouse e toque no PC:** hover funciona como foco; clique = OK.

### Movimento
- Animar **só `transform` e `opacity`** (aceleração por GPU).
- Duração de 150–250 ms com `ease-out`; nada acima de 400 ms.
- Respeitar `prefers-reduced-motion` e ter opção "Reduzir animações" nas configurações (ligada por padrão em TVs com pouca memória).

### Tokens visuais (definidos no `theme` do `uno.config.ts`)
- **Cores:** fundo `#0B0B0F`, superfícies em 2–3 níveis um pouco mais claros, texto principal branco a 92%, secundário a 60%, terciário a 40%, **uma única cor de destaque** para ação principal e progresso. Nunca preto puro nem branco puro. Contraste mínimo WCAG AA.
- **Tipografia:** **Inter** (auto-hospedada, fonte variável). Escala de títulos grande e generosa; corpo com no mínimo **24 px em 1080p na TV** e 16 px no PC.
- **Espaçamento:** grade de 8 px.
- **Área segura na TV:** margem de 5% nas bordas (overscan).
- **Raio de borda:** 12 px nos cards, 999 px nos botões.

### Tamanhos de tela
- PC: de 1280 px até ultrawide; a interface escala sem esticar o conteúdo.
- TV: layout desenhado em 1920×1080 e escalado proporcionalmente para 720p e 4K (unidade base em `rem` ajustada pela largura da tela).
- Celular não é alvo agora, mas a interface não pode quebrar em telas estreitas.

### Desempenho visual
- Fileiras longas **virtualizadas** (só renderiza o que está perto da tela).
- Imagens com `loading="lazy"`, `decoding="async"` e tamanho fixo reservado (sem pulos de layout).
- 60 fps na navegação por controle numa TV de 2021 é critério de aceite.

## 8. Metas de performance

| Métrica | Meta |
|---|---|
| Inicialização do servidor | < 500 ms (sem contar o scan) |
| RAM em repouso | < 80 MB |
| Scan incremental, 10 mil arquivos sem mudança | < 3 s |
| Início do direct play (LAN) | < 300 ms até o primeiro byte |
| Início com transcodificação (NVENC) | < 2 s até o primeiro segmento |
| Listagem da biblioteca (API) | < 50 ms p95 |
| Bundle JS inicial do frontend | < 150 KB gzip |

Toda mudança em caminho crítico (scanner, stream, queries de listagem) precisa vir com benchmark (`go test -bench`) ou medição registrada no PR.

## 9. Convenções de código

**Go**
- `golangci-lint run` sem avisos, com `.golangci.yml` versionado. Linters mínimos: `staticcheck`, `govet`, `errcheck`, `gosec`, `modernize`, `revive`, `perfsprint`, `prealloc`, `bodyclose`, `contextcheck`.
- Formatação só via `golangci-lint fmt` (formatters `gofumpt` + `gci`). Nunca `gofmt` direto.
- Rodar `go fix ./...` ao subir a versão do Go (aplica os *modernizers*).
- Erros com contexto: `fmt.Errorf("probe %s: %w", path, err)`.
- `context.Context` como primeiro parâmetro em tudo que faz I/O.
- Sem estado global; dependências passadas por struct.
- Testes table-driven, principalmente no parser de nomes.

**Frontend**
- TypeScript estrito. Svelte 5 (runes).
- Lint com **Oxlint** (`.oxlintrc.json`, com *type-aware* ligado) e formatação com **Oxfmt** (suporte a `.svelte` exige o `svelte/compiler` instalado).
- **svelte-check** com tsgo para checar tipos e templates. Sem ESLint e sem Prettier no projeto.
- Todo componente navegável precisa funcionar com **setas + Enter + Voltar** (exigência das TVs).
- Estilo com classes do UnoCSS; CSS próprio só em `<style>` do componente quando a classe não resolver. Tokens de tema (cores, espaçamentos) no `theme` do `uno.config.ts`.
- Nada da lista de proibidos do §6 fora de `@supports`.
- Sem fontes ou recursos vindos de CDN: tudo local.

**Idioma**
- Código, identificadores e mensagens de commit: **inglês**.
- Interface do usuário e documentação: **português do Brasil**.

## 10. Comandos

> Criados na Sprint 0. Até lá, esta seção é a especificação.

```bash
make dev        # backend com reload + vite dev server
make build      # build do frontend + binários Go (Windows, Linux e macOS; amd64 e arm64)
make test       # go test -race ./... + vitest run
make fmt        # golangci-lint fmt + oxfmt
make check      # golangci-lint run + oxlint + svelte-check (não altera arquivos; usado no CI)
make bench      # benchmarks do scanner e do stream
```

Variáveis principais:

```
MEDIA_DIR=/caminho/das/midias
DATA_DIR=./data          # banco, cache de HLS, miniaturas
PORT=8080
FFMPEG_PATH=ffmpeg
FFPROBE_PATH=ffprobe
HWACCEL=auto             # auto | nvenc | qsv | amf | vaapi | videotoolbox | none
```

## 11. Fluxo de trabalho

1. Antes de começar, leia **`SPRINTS.md`** e trabalhe só na sprint ativa.
2. Uma tarefa por vez. Ao concluir, marque `[x]` no `SPRINTS.md`.
3. Não cumpriu o critério de aceite? A tarefa não está concluída.
4. Decisão de arquitetura nova ou mudança de stack: registre um ADR curto em `docs/adr/NNN-titulo.md`.
5. Commits pequenos, no padrão Conventional Commits (`feat(scanner): ...`, `fix(stream): ...`).

## 12. Fora de escopo (por enquanto)

- APIs externas de metadados (TMDB, AniList, OpenSubtitles)
- Acesso externo (Tailscale, domínio, HTTPS)
- Multiusuário com senha forte (por enquanto são só perfis locais)
- Download de mídia, integração com *arr, torrents
- Renomear, mover ou apagar arquivos do usuário (organizador físico está no Backlog)
- Apps nativos de celular