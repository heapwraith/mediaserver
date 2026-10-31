# mediaserver

Servidor de streaming **local** para filmes, séries e animes. Um binário só (Go, sem CGO) com o frontend embutido, para assistir no navegador do PC e nas TVs da mesma rede.

> `mediaserver` é um nome provisório. Regras do projeto em [`AGENTS.md`](AGENTS.md); planejamento em [`SPRINTS.md`](SPRINTS.md).

## Requisitos (desenvolvimento)

- Go 1.27
- Node.js 24 ou mais novo
- make
- gcc (só para `go test -race`)

O `golangci-lint` não precisa ser instalado: a versão fixada em `tools.mod` roda via `go tool`.

FFmpeg/ffprobe 9+ é opcional para desenvolver: sem ele, os testes com vídeo real são pulados, o servidor indexa só pelo nome e não gera imagens. No Windows: `winget install Gyan.FFmpeg`. Coloque a pasta `bin` do FFmpeg no PATH (ou use `FFPROBE_PATH`/`FFMPEG_PATH`).

### Windows

- Rode os comandos `make` no **Git Bash** (vem com o [Git for Windows](https://git-scm.com/download/win)). O `make` pode ser instalado com `winget install ezwinports.make`.
- Para o `go test -race`, instale o gcc (ex.: MSYS2 + `pacman -S mingw-w64-ucrt-x86_64-gcc`, com a pasta `bin` no PATH). Sem gcc, o `make test` roda os testes **sem** `-race` e avisa.
- O repositório força fim de linha LF (`.gitattributes`); não precisa mexer no `core.autocrlf`.

### macOS

- Instale as ferramentas de linha de comando: `xcode-select --install` (traz `make`, `git` e o `clang` usado pelo `-race`).
- Go e Node pelo instalador oficial ou Homebrew (`brew install go node`).

O CI (`.github/workflows/ci.yml`) roda `make check` e `make test` em Linux, Windows e macOS a cada push.

## Comandos

```bash
make dev      # backend com reload (:8080) + Vite (http://localhost:5173)
make build    # frontend + 6 binários em dist/ (Windows, Linux, macOS × amd64, arm64)
make test     # go test -race + vitest
make fmt      # formata Go e web
make check    # lint + formatação + tipos + build do frontend (orçamento de 150 KB e CSS proibido)
make bench    # benchmarks
```

## Rodando

```bash
make build
./dist/mediaserver-linux-amd64 -media-dir "/caminho/das/midias"
# abra http://IP-DO-PC:8080
```

## Configuração

Prioridade: **flags > variáveis de ambiente > `config.toml`** (na pasta atual, ou o caminho em `-config` / `CONFIG_FILE`).

| Flag | Variável | `config.toml` | Padrão |
|---|---|---|---|
| `-media-dir` | `MEDIA_DIR` | `media_dir` | — (obrigatório) |
| `-data-dir` | `DATA_DIR` | `data_dir` | `./data` |
| `-host` | `HOST` | `host` | `0.0.0.0` |
| `-port` | `PORT` | `port` | `8080` |
| `-ffmpeg` | `FFMPEG_PATH` | `ffmpeg_path` | `ffmpeg` |
| `-ffprobe` | `FFPROBE_PATH` | `ffprobe_path` | `ffprobe` |
| `-hwaccel` | `HWACCEL` | `hwaccel` | `auto` (`auto`, `nvenc`, `qsv`, `amf`, `vaapi`, `videotoolbox`, `none`) |
| `-env` | `APP_ENV` | `env` | `prod` (`dev` = logs em texto; `prod` = JSON) |
| `-log-level` | `LOG_LEVEL` | `log_level` | `info` |
| `-scan-workers` | `SCAN_WORKERS` | `scan_workers` | `0` (= nº de CPUs) |
| `-watch` | `WATCH` | `watch` | `true` (detecta arquivos novos sozinho) |
| `-scan-on-start` | `SCAN_ON_START` | `scan_on_start` | `true` |

Exemplo de `config.toml`:

```toml
media_dir = "/srv/midias"
port = 8080
hwaccel = "auto"
```

O servidor **nunca** renomeia, move ou apaga seus arquivos: toda a organização é virtual, no banco (`DATA_DIR/mediaserver.db`).

## Biblioteca

A pasta de mídias pode estar organizada ou não: o scanner entende nomes de release (`Show.S01E01.1080p.WEB-DL.mkv`, `[SubsPlease] Anime - 01 (1080p).mkv`, `Filme (2020).mkv`), agrupa episódios espalhados em pastas diferentes e manda o que for ambíguo para a fila **Revisar**. Arquivos novos aparecem sozinhos (watcher); um arquivo ainda sendo copiado só entra quando a cópia termina.

Sem `ffprobe` o servidor funciona, mas indexa só pelo nome (sem codecs, duração e faixas).

| Endpoint | O que faz |
|---|---|
| `POST /api/library/scan` | Inicia um scan manual (202) |
| `GET /api/library/scan` | Status do scan atual/último |
| `GET /api/review` | Fila Revisar |
| `PUT /api/review/{fileId}` | Corrige um arquivo: `{"kind":"movie\|series\|anime\|ignored","title":"…","year":2020,"season":1,"episode":5}` |
| `POST /api/series/merge` | Junta séries: `{"source_ids":[…],"target_id":"…"}` |
| `POST /api/series/split` | Separa episódios numa série nova: `{"series_id":"…","file_ids":[…],"title":"…"}` |

As correções valem para os próximos scans e acompanham o arquivo se ele for renomeado ou movido.

### Leitura e reprodução

| Endpoint | O que faz |
|---|---|
| `GET /api/movies`, `/api/series`, `/api/animes` | Listagens paginadas: `?sort=title\|added\|year&order=asc\|desc&offset=0&limit=100` (máx. 500); `&q=` busca no título (sem acento/caixa) |
| `GET /api/movies/{id}` | Um filme |
| `GET /api/series/{id}` | Série com temporadas e episódios |
| `GET /api/movies/{id}/extras`, `/api/series/{id}/extras` | Trailers e extras (tocam pelo `media_id`) |
| `GET /api/media/{id}` | Detalhes técnicos, faixas de áudio e legendas |
| `GET /api/media/{id}/stream` | Direct play com suporte a Range (seek) |
| `GET /api/images/{id}?w=480\|960\|1920&variant=card\|backdrop\|blur` | Imagem 16:9 em WebP (arte local ou melhor frame do vídeo), gerada na 1ª vez e guardada em `DATA_DIR/images` |

As listagens e detalhes respondem com `ETag` (o cliente revalida e recebe 304). Sem FFmpeg, `/api/images` responde 404 e o frontend mostra o card tipográfico.
