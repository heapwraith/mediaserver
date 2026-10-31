# mediaserver

Servidor de streaming **local** para filmes, séries e animes. Um binário só (Go, sem CGO) com o frontend embutido, para assistir no navegador do PC e nas TVs da mesma rede.

> `mediaserver` é um nome provisório. Regras do projeto em [`AGENTS.md`](AGENTS.md); planejamento em [`SPRINTS.md`](SPRINTS.md).

## Requisitos (desenvolvimento)

- Go 1.27
- Node.js 24 ou mais novo
- make
- gcc (só para `go test -race`)

O `golangci-lint` não precisa ser instalado: a versão fixada em `tools.mod` roda via `go tool`.

## Comandos

```bash
make dev      # backend com reload (:8080) + Vite (http://localhost:5173)
make build    # frontend + 6 binários em dist/ (Windows, Linux, macOS × amd64, arm64)
make test     # go test -race + vitest
make fmt      # formata Go e web
make check    # lint + formatação + tipos (não altera arquivos)
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

Exemplo de `config.toml`:

```toml
media_dir = "/srv/midias"
port = 8080
hwaccel = "auto"
```

O servidor **nunca** renomeia, move ou apaga seus arquivos: toda a organização é virtual, no banco (`DATA_DIR/mediaserver.db`).
