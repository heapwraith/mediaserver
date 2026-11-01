# mediaserver

Servidor de streaming **local** para filmes, séries e animes. Um binário só (Go, sem CGO) com o frontend embutido, para assistir no navegador do PC e nas TVs da mesma rede.

- Aponte uma pasta com seus vídeos, organizada ou não: o servidor entende nomes de release e monta a biblioteca sozinho.
- **Nunca** renomeia, move ou apaga seus arquivos: toda a organização é virtual, no banco.
- Toca direto quando o aparelho suporta o formato e converte (HLS, com GPU quando houver) só quando precisa.
- Perfis com "continuar assistindo", legendas SRT/ASS (inclusive as estilizadas de anime) e apps para TVs Samsung, LG e Android.
- Só rede local, sem contas, sem internet: nada é buscado fora (TMDB etc.).

> `mediaserver` é um nome provisório. Regras do projeto em [`AGENTS.md`](AGENTS.md); planejamento em [`SPRINTS.md`](SPRINTS.md).

**Sumário:** [Instalação](#instalação) · [Rodar como serviço](#rodar-como-serviço) · [Primeiro uso](#primeiro-uso) · [Organização das pastas](#organização-das-pastas) · [Assistindo](#assistindo) · [TVs](#tvs) · [Configuração](#configuração) · [Backup e restauração](#backup-e-restauração) · [Status do servidor](#status-do-servidor) · [Problemas comuns](#problemas-comuns) · [API](#api) · [Desenvolvimento](#desenvolvimento)

---

## Instalação

Ainda não há binários publicados (a versão 1.0 sai na Sprint 8). Por enquanto, gere o binário a partir do código.

### 1. Ferramentas

| | Windows | macOS | Linux (Debian/Ubuntu) |
|---|---|---|---|
| Go 1.27 | `winget install GoLang.Go` | `brew install go` | [go.dev/dl](https://go.dev/dl) |
| Node.js 24+ | `winget install OpenJS.NodeJS` | `brew install node` | [nodejs.org](https://nodejs.org) ou `nvm install 24` |
| make + git | Git for Windows + `winget install ezwinports.make` (use o **Git Bash**) | `xcode-select --install` | `sudo apt install make git` |
| **FFmpeg 9+** | `winget install Gyan.FFmpeg` | `brew install ffmpeg` | `sudo apt install ffmpeg` (ou um build estático recente) |

O **FFmpeg** (com `ffprobe`) é o que lê os codecs, a duração e as faixas, gera as imagens dos cards e converte o que o aparelho não toca. Sem ele o servidor funciona, mas só pelo nome dos arquivos, sem imagens e sem conversão. Confira com `ffmpeg -version` e `ffprobe -version`; se não estiverem no PATH, use `FFMPEG_PATH` / `FFPROBE_PATH`.

### 2. Gerar o binário

```bash
git clone https://github.com/heapwraith/mediaserver.git
cd mediaserver
make build          # frontend + binários em dist/
```

Saem 6 binários em `dist/` (`mediaserver-<sistema>-<arquitetura>`): `windows-amd64.exe`, `windows-arm64.exe`, `linux-amd64`, `linux-arm64`, `darwin-amd64` (Mac Intel) e `darwin-arm64` (Mac M1+). Cada um é um arquivo só: copie para onde quiser.

### 3. Rodar

```bash
# Linux
./dist/mediaserver-linux-amd64 -media-dir "/srv/midias"

# macOS (Apple Silicon)
./dist/mediaserver-darwin-arm64 -media-dir "$HOME/Movies"

# Windows (Git Bash ou PowerShell)
./dist/mediaserver-windows-amd64.exe -media-dir "D:\Midias"
```

Abra **http://localhost:8080** no próprio PC, ou **http://IP-DO-PC:8080** em outro aparelho da rede (o IP aparece em `ipconfig` no Windows, `ip addr` no Linux, Ajustes → Rede no macOS).

O banco, as imagens e o cache de conversão ficam em `./data` (mude com `-data-dir`). Para não repetir as opções, crie um [`config.toml`](#configuração) na pasta onde roda o binário.

> Para o servidor iniciar sozinho com o computador, veja [Rodar como serviço](#rodar-como-serviço).

### 4. Liberar na rede

Outros aparelhos (celular, TV) precisam alcançar a porta 8080 do PC:

- **Windows:** na primeira execução, permita o acesso em **Redes privadas** na janela do Firewall. Se passou, libere manualmente: `New-NetFirewallRule -DisplayName mediaserver -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow -Profile Private` (PowerShell como administrador).
- **macOS:** Ajustes → Rede → Firewall → Opções → permitir o `mediaserver`.
- **Linux com ufw:** `sudo ufw allow 8080/tcp`.
- **WSL 2:** o WSL fica numa rede interna (172.x). Rode o binário Windows, ou ative `networkingMode=mirrored` no `%UserProfile%\.wslconfig`.

Teste abrindo `http://IP-DO-PC:8080` no celular, conectado ao mesmo Wi-Fi.

---

## Docker (opcional)

Sem instalar Go, Node nem FFmpeg: a imagem já traz tudo.

```bash
MEDIA_DIR=/caminho/das/midias docker compose up -d --build
```

Abra `http://IP-DO-PC:8080`. As mídias entram como somente leitura e o banco fica no volume `mediaserver-data` (sobrevive a `docker compose down`). Sem `MEDIA_DIR`, usa a pasta `./media`.

- **GPU NVIDIA (NVENC), no Linux ou no Docker Desktop com WSL 2:** `docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up -d --build`. Sem isso, a conversão usa a CPU.
- **Pasta no Windows (WSL):** use o caminho `/mnt/c/...`, por exemplo `MEDIA_DIR=/mnt/c/Users/SEU-USUARIO/Videos/mediaStorage` num arquivo `.env`. Arquivos novos podem não ser percebidos sozinhos nessa pasta; use **Configurações → scan**.
- **Primeira imagem:** o build compila o FFmpeg 9 (NVENC) e leva alguns minutos; os próximos usam o cache.
- **TVs:** na rede padrão do Docker a descoberta automática pode falhar; digite o IP na TV. No Linux, `network_mode: host` resolve.
- Só a imagem: `make docker`. Detalhes no [ADR 011](docs/adr/011-docker.md).

---

## Rodar como serviço

Para o servidor ligar junto com o computador e voltar sozinho se cair, instale como serviço. Rode **na pasta onde você roda o servidor** (é dela que vêm `./data` e o `config.toml`) e com as mesmas opções:

```bash
# Linux: com sudo, vira serviço do sistema (roda como o seu usuário).
# Sem sudo, vira serviço do seu usuário: inicia quando você entra.
sudo ./mediaserver-linux-amd64 service install -media-dir "/srv/midias"

# macOS: inicia quando você entra (log em ~/Library/Logs/mediaserver.log)
./mediaserver-darwin-arm64 service install -media-dir "$HOME/Movies"

# Windows: abra o terminal como administrador (log em DATA_DIR\mediaserver.log)
.\mediaserver-windows-amd64.exe service install -media-dir "D:\Midias"
```

| Comando | O que faz |
|---|---|
| `service install [opções]` | Instala e inicia. Confere a configuração na hora; variáveis de ambiente como `MEDIA_DIR` são gravadas como opções |
| `service status` | `running`, `stopped`, `not installed`… |
| `service stop` / `service start` | Para / inicia agora |
| `service uninstall` | Remove o serviço (não apaga dados) |

- **Atualizar o binário:** `service stop`, troque o arquivo, `service start`.
- **Mudar as opções:** `service uninstall` e `install` de novo. Opções no `config.toml` valem no próximo `start`, sem reinstalar.
- **Linux sem sudo:** para iniciar no boot mesmo sem você entrar, rode `sudo loginctl enable-linger $USER`. Os logs ficam em `journalctl --user -u mediaserver` (ou `journalctl -u mediaserver` com sudo).

---

## Primeiro uso

1. **Escolha quem está assistindo.** Na primeira vez, crie um perfil: nome, cor e, se quiser, um **PIN de 4 dígitos** (5 erros seguidos bloqueiam o perfil por 30 s). Cada perfil tem seu próprio "continuar assistindo" e sua legenda padrão.
2. **Espere o primeiro scan.** Ele começa sozinho ao iniciar o servidor. Acompanhe em **Configurações → Biblioteca**. Arquivos copiados depois aparecem sozinhos, assim que a cópia termina.
3. **Confira a fila Revisar.** O que o servidor não conseguiu classificar com certeza vai para **Revisar**, nunca some. Ali você corrige tipo (filme, série, anime ou ignorar), título, ano, temporada e episódio, e pode **juntar** séries duplicadas ou **separar** episódios. As correções valem para os próximos scans e acompanham o arquivo se ele for renomeado ou movido.
4. **Ajuste o perfil** (Configurações → Editar perfil): idioma padrão das legendas (ex.: Português). Com áudio no mesmo idioma, só legendas forçadas aparecem sozinhas.

---

## Organização das pastas

Qualquer organização funciona: tudo numa pasta só, subpastas aleatórias, nomes de release (`Show.S01E01.1080p.WEB-DL.x265.mkv`, `[SubsPlease] Anime - 01 (1080p).mkv`, `Filme.2020.BluRay.mkv`). Episódios da mesma série em pastas diferentes viram uma série só.

Para 100% de acerto, este é o formato recomendado:

```
Midias/
├── Filmes/
│   └── Nome do Filme (2020)/
│       ├── Nome do Filme (2020).mkv
│       ├── Nome do Filme (2020).pt-BR.srt     ← legenda com idioma
│       ├── Nome do Filme (2020)-trailer.mkv   ← trailer
│       ├── poster.jpg / fanart.jpg            ← imagens (opcionais)
│       └── extras/                            ← making of, cenas extras
├── Series/
│   └── Nome da Série/
│       └── Temporada 01/
│           └── Nome da Série - S01E01.mkv
└── Animes/
    └── Nome do Anime/
        └── Nome do Anime - 01.mkv
```

- **Vídeo:** `.mkv .mp4 .m4v .avi .webm .mov .ts`.
- **Legendas ao lado do vídeo:** `.srt .ass .ssa .vtt`, com idioma opcional (`.pt-BR.srt`, `.en.ass`). SRT em UTF-8 ou Latin-1 (Windows-1252). Legendas embutidas no MKV e as fontes anexadas (anime) são lidas automaticamente.
- **Imagens:** `poster`, `folder`, `cover`, `fanart`, `backdrop` (`.jpg`, `.png`, `.webp`) na pasta; também `Nome do Filme-poster.jpg` e `season01-poster.jpg`. Sem elas, o servidor escolhe um frame do vídeo (sem cenas pretas). Sem FFmpeg, o card é tipográfico.
- **Ignorados:** arquivos `sample`. **Trailers** (`-trailer`, pasta `trailers/`) e **extras** (`extras/`, `featurettes/`, `-behindthescenes`…) ficam vinculados ao filme/série.

---

## Assistindo

- **Início:** destaque, *Continuar assistindo*, *Próximos episódios*, *Adicionados recentemente* e fileiras de Filmes, Séries e Animes.
- O progresso é salvo a cada 10 s e ao pausar/sair; a partir de **90%** conta como assistido. Marque/desmarque à mão um episódio, uma temporada ou a série toda.
- No fim do episódio, o próximo começa sozinho após uma contagem (cancelável; desligue em Configurações → Reprodução).
- **Legendas e áudio:** menu no topo do player. ASS/SSA estilizadas (karaokê, posicionamento) são desenhadas no navegador; em aparelhos que não aguentam, o servidor grava a legenda no vídeo.
- **Direct play primeiro:** se o aparelho toca o arquivo, ele vai direto (seek instantâneo). Senão, o servidor troca só o contêiner (remux) ou converte o que for preciso, com GPU quando disponível (NVENC, QSV, AMF, VAAPI, VideoToolbox).
- **Configurações → Este aparelho** mostra os formatos que o aparelho atual toca direto.

### Teclado (PC) e controle remoto

| Tecla | Fora do player | No player |
|---|---|---|
| Setas | Navegar | ←/→: −10 s/+10 s · ↑: barra de ferramentas · ↓: mostra os controles |
| Enter / OK | Abrir | Pausar/continuar |
| Esc / Backspace / Voltar | Voltar um nível (no Início das TVs: sair do app) | Sair do player |
| Espaço / Play-Pause | — | Pausar/continuar |
| Play · Pause · Stop | — | Tocar · pausar · sair |
| Avançar / Retroceder | — | +10 s / −10 s |
| Vermelho / Verde | — | Legendas / áudio |
| Amarelo / Azul | Buscar / Início | Buscar / Início |

O mouse também funciona: passar por cima foca, clicar abre.

---

## TVs

| TV | Como |
|---|---|
| Samsung (Tizen, 2021+) | App `.wgt` instalado pelo modo desenvolvedor: [docs/tv/samsung.md](docs/tv/samsung.md) |
| LG (webOS, 2021+) | App `.ipk` pelo Developer Mode: [docs/tv/lg.md](docs/tv/lg.md) |
| Android TV, Google TV, Fire TV | App `.apk` por sideload: [docs/tv/android.md](docs/tv/android.md) |
| Outras (Apple TV, Roku…) | Navegador da TV, quando houver |

O app da TV é leve: ele acha o servidor na rede (ou você digita o IP) e mostra a mesma interface do navegador, servida pelo servidor. Atualizar o servidor atualiza as TVs. Visão geral em [docs/tv](docs/tv/README.md).

```bash
make tv-tizen      # dist/tv/tizen/mediaserver.wgt  (Tizen Studio + certificado Samsung)
make tv-webos      # dist/tv/webos/*.ipk            (npm i -g @webos-tools/cli)
make tv-android    # dist/tv/mediaserver-tv.apk     (Android SDK)
```

---

## Gêneros e coleções (TMDB, opcional)

Nenhum arquivo de vídeo costuma trazer gênero, então essa parte vem do [TMDB](https://www.themoviedb.org), pela internet. **É opcional e vem desligada:** sem ligar, o servidor não faz nenhuma chamada externa.

1. Crie uma conta gratuita no TMDB e pegue a chave em **Configurações → API** do site.
2. Em **Configurações** do mediaserver (no navegador do PC, para colar a chave), salve a chave e ligue o TMDB.
3. O servidor busca um título por vez, em segundo plano. Depois disso, a tela inicial ganha fileiras por gênero e de coleções, e Filmes, Séries e Animes ganham filtro por gênero.

Só entra o que é certo: pelo código do TMDB ou do IMDb gravado no arquivo ou, na busca por nome, quando há um único título igual com ano compatível. O resto fica em **Revisar**, em "Títulos do TMDB", para você escolher.

A chave fica no banco de dados, nunca volta pela API e entra no backup do banco.

Este produto usa a API do TMDB, mas não é endossado nem certificado pelo TMDB.

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
| `-max-transcodes` | `MAX_TRANSCODES` | `max_transcodes` | `2` (conversões de vídeo simultâneas; remux não conta) |
| `-transcode-cache-mb` | `TRANSCODE_CACHE_MB` | `transcode_cache_mb` | `10240` (cache de HLS em `DATA_DIR/transcode`) |

Exemplo de `config.toml`:

```toml
media_dir = "/srv/midias"
data_dir = "/var/lib/mediaserver"
port = 8080
hwaccel = "auto"
```

No Windows, use barras normais ou dobradas no TOML: `media_dir = "D:/Midias"`.

### O que fica em `DATA_DIR`

| Caminho | Conteúdo | Pode apagar? |
|---|---|---|
| `mediaserver.db` (+ `-wal`, `-shm`) | Biblioteca, correções da Revisar, perfis e progresso | **Não** (perde perfis, progresso e correções). Faça cópia com o servidor parado |
| `images/` | Imagens dos cards | Sim, são geradas de novo |
| `subtitles/` | Legendas e fontes extraídas dos MKV | Sim |
| `transcode/` | Cache das conversões (limitado por `TRANSCODE_CACHE_MB`) | Sim, com o servidor parado |

---

## Backup e restauração

O banco guarda perfis, progresso e as correções da Revisar. A biblioteca em si é refeita por um scan, e seus vídeos nunca ficam no banco.

- **Backup:** **Configurações → Baixar backup** (no navegador do PC). Ele baixa `mediaserver-AAAAMMDD-HHMM.db`, uma cópia consistente feita com o servidor rodando.
- **Restaurar:** com o servidor **parado**, na mesma pasta e com as mesmas opções do servidor:

  ```bash
  ./mediaserver service stop                  # se for serviço
  ./mediaserver restore mediaserver-20261101-0930.db
  ./mediaserver service start
  ```

  O arquivo é conferido antes (precisa ser um banco do mediaserver, íntegro e de uma versão igual ou anterior). O banco atual fica guardado ao lado como `mediaserver.db.antes-<data>`.

---

## Status do servidor

**Configurações → Status do servidor** mostra, atualizando sozinha:

- CPU do servidor e do computador, memória e tempo ligado;
- a aceleração de vídeo em uso (e o uso da GPU e do NVENC em placas NVIDIA);
- as conversões em andamento e as vagas livres (`MAX_TRANSCODES`);
- o tamanho dos caches e do banco;
- o tamanho da biblioteca.

---

## Problemas comuns

| Sintoma | Causa provável e solução |
|---|---|
| Biblioteca sem duração, sem imagens, "ffprobe not available" no log | FFmpeg fora do PATH: instale-o ou use `FFPROBE_PATH`/`FFMPEG_PATH` |
| Celular/TV não abre `http://IP:8080` | Firewall ou WSL: veja [Liberar na rede](#4-liberar-na-rede) |
| Arquivo não aparece | Ainda copiando (entra quando terminar), é um `sample`, ou caiu na **Revisar** |
| Série duplicada ou episódio no lugar errado | Revisar → juntar/separar, ou corrigir temporada/episódio |
| Vídeo demora a começar | Está sendo convertido (aparelho não suporta o codec). Veja a linha `playback` no log (`direct=false`) e **Configurações → Este aparelho** |
| Conversão lenta / CPU a 100% | Sem GPU detectada: confira o log de inicialização e o driver; force com `-hwaccel nvenc\|qsv\|amf\|vaapi\|videotoolbox` |
| "limite de conversões simultâneas atingido" | Aumente `MAX_TRANSCODES` (cada conversão pesa na CPU/GPU) |
| Legenda com acento quebrado | SRT em codificação rara: salve em UTF-8 |
| HDR convertido parece desbotado | Ainda sem *tone mapping* (Backlog); prefira direct play num aparelho com HDR |
| Servidor não liga como serviço | `mediaserver service status` e o log (`journalctl --user -u mediaserver`; macOS `~/Library/Logs/mediaserver.log`; Windows `DATA_DIR\mediaserver.log`) |
| Esqueci o PIN | Ainda não há como redefinir (está no Backlog). Crie outro perfil; um perfil só pode ser editado ou apagado por ele mesmo |

Logs: no terminal onde o servidor roda (`-env dev` para texto legível, `-log-level debug` para mais detalhes).

---

## API

JSON em `/api`. Rotas de perfil e progresso exigem o cabeçalho `X-Profile: <token>` (devolvido ao escolher o perfil).

### Biblioteca e Revisar

| Endpoint | O que faz |
|---|---|
| `GET /api/health` | Status e versão (`name: "mediaserver"`; aberto a outras origens para a descoberta das TVs) |
| `POST /api/library/scan` | Inicia um scan manual (202) |
| `GET /api/library/scan` | Status do scan atual/último |
| `GET /api/review` | Fila Revisar |
| `PUT /api/review/{fileId}` | Corrige um arquivo: `{"kind":"movie\|series\|anime\|ignored","title":"…","year":2020,"season":1,"episode":5}` |
| `POST /api/series/merge` | Junta séries: `{"source_ids":[…],"target_id":"…"}` |
| `POST /api/series/split` | Separa episódios numa série nova: `{"series_id":"…","file_ids":[…],"title":"…"}` |

### Leitura

| Endpoint | O que faz |
|---|---|
| `GET /api/movies`, `/api/series`, `/api/animes` | Listagens paginadas: `?sort=title\|added\|year&order=asc\|desc&offset=0&limit=100` (máx. 500); `&q=` busca no título (sem acento/caixa) |
| `GET /api/movies/{id}` | Um filme |
| `GET /api/series/{id}` | Série com temporadas e episódios |
| `GET /api/movies/{id}/extras`, `/api/series/{id}/extras` | Trailers e extras (tocam pelo `media_id`) |
| `GET /api/media/{id}` | Detalhes técnicos, faixas de áudio e legendas |
| `GET /api/media/{id}/next` | Próximo episódio |
| `GET /api/images/{id}?w=480\|960\|1920&variant=card\|backdrop\|blur` | Imagem 16:9 em WebP, gerada na 1ª vez e guardada em `DATA_DIR/images` |

Listagens e detalhes respondem com `ETag` (o cliente revalida e recebe 304).
| `GET /api/genres?kind=movie\|series\|anime` | Gêneros de um tipo, com contagem (vazio com o TMDB desligado) |
| `GET /api/genres/rows` | Fileiras por gênero da tela inicial |
| `GET /api/collections`, `GET /api/collections/{id}` | Coleções com 2 ou mais filmes na biblioteca |
| `GET /api/movies?genre=ID`, `/api/series?genre=ID`, `/api/animes?genre=ID` | Listagens filtradas por gênero |

### Reprodução e legendas

| Endpoint | O que faz |
|---|---|
| `GET /api/media/{id}/stream` | Direct play com Range (seek) |
| `POST /api/media/{id}/playback` | Decide direct play, remux ou conversão: `{"caps":["mp4","h264","aac",…],"audio":1,"start":0,"subtitle":"…"}` → `{"direct":false,"url":"…/index.m3u8","start":0,"video":"copy","audio":"aac"}` |
| `GET /api/transcode/{key}/{file}` | Playlist e segmentos HLS (fMP4) |
| `DELETE /api/transcode/{key}` | Encerra uma conversão |
| `GET /api/subtitles/{id}/vtt?offset=S` | Legenda em WebVTT |
| `GET /api/subtitles/{id}/ass` | Legenda ASS/SSA em UTF-8 |
| `GET /api/media/{id}/fonts`, `/api/media/{id}/fonts/{name}` | Fontes anexadas ao MKV |

### Perfis e progresso

| Endpoint | O que faz |
|---|---|
| `GET /api/profiles` | Lista os perfis |
| `POST /api/profiles` | Cria: `{"name":"Sala","color":"#4c8dff","pin":"1234","subtitle_language":"pt-BR"}` |
| `POST /api/profiles/{id}/session` | Escolhe o perfil (`{"pin":"1234"}` se tiver) → `{"profile":…,"token":"…"}` |
| `PUT /api/profiles/{id}`, `DELETE /api/profiles/{id}` | Edita / apaga o próprio perfil (com `X-Profile` dele) |
| `GET /api/home` | Fileiras do Início do perfil |
| `GET`/`PUT /api/progress/{mediaId}` | Posição: `{"position_ms":…,"duration_ms":…}` |
| `PUT /api/watched` | Marca/desmarca episódio, temporada ou série como assistido |
| `GET /api/series/{id}/progress` | Progresso de cada episódio |

### Administração (com `X-Profile`)

| Endpoint | O que faz |
|---|---|
| `GET /api/admin/backup` | Baixa uma cópia consistente do banco (`mediaserver-AAAAMMDD-HHMM.db`) |
| `GET /api/admin/status` | CPU, memória, encoder, GPU NVIDIA, conversões ativas, caches e biblioteca |
| `GET /api/metadata/status` | TMDB: ligado, tem chave, títulos reconhecidos e para revisar (nunca a chave) |
| `PUT /api/metadata/settings` | Liga ou desliga o TMDB e salva a chave (`{"enabled": true, "key": "..."}`) |
| `POST /api/metadata/sync` | Busca agora os títulos ainda sem resposta |
| `GET /api/metadata/review` | Títulos sem certeza, com as opções |
| `PUT /api/metadata/{id}` | Resposta do usuário: `{"tmdb_id": 123}` ou `{"skip": true}` |

### Erros

Toda resposta de erro é `{"error": "texto para logs", "code": "…"}`. Códigos: `file_missing` (410, o arquivo sumiu do disco), `no_ffmpeg` (503), `busy` (503, limite de conversões), `shutting_down` (503), `not_found` (404), `invalid` (400), `conflict` (409), `no_profile` (401), `wrong_pin` / `forbidden` (403), `pin_locked` (429), `unavailable` (503), `internal` (500).

---

## Desenvolvimento

### Requisitos

- Go 1.27, Node.js 24+, make; FFmpeg/ffprobe 9+ opcional (sem ele, os testes com vídeo real são pulados).
- gcc (ou clang) só para `go test -race`; sem ele, `make test` roda sem `-race` e avisa.
- O `golangci-lint` não precisa ser instalado: a versão fixada em `tools.mod` roda via `go tool`.

**Windows:** rode os comandos no **Git Bash**. Para o `-race`, instale o gcc (MSYS2 + `pacman -S mingw-w64-ucrt-x86_64-gcc`, com a pasta `bin` no PATH). O repositório força LF (`.gitattributes`); não precisa mexer no `core.autocrlf`.

**macOS:** `xcode-select --install` traz `make`, `git` e o `clang` do `-race`.

### Comandos

```bash
make dev        # backend com reload (:8080) + Vite (http://localhost:5173)
make build      # frontend + 6 binários em dist/ (Windows, Linux, macOS × amd64, arm64)
make test       # go test -race + vitest
make fmt        # formata Go e web
make check      # lint + formatação + tipos + build do frontend (orçamento de 150 KB e CSS proibido)
make bench      # benchmarks
make tv         # tela de conexão dos apps de TV (web/build-tv)
make load       # teste de carga: 2 direct play + 1 conversão (a conversão precisa de FFmpeg)
make clean      # apaga as saídas de build
```

O `make dev` usa a pasta `./media` (mude com `make dev MEDIA_DIR=/caminho`) e guarda o banco em `.dev/data`.

O CI (`.github/workflows/ci.yml`) roda `make check` e `make test` em Linux, Windows e macOS a cada push.

### Estrutura

```
cmd/mediaserver/   bootstrap e encerramento
internal/          config, db, library, scanner, probe, stream, subtitles, api
web/               SvelteKit (interface) e web/src/shell (tela de conexão das TVs)
tv/                empacotamento Samsung, LG e Android
migrations/        SQL versionado
docs/              ADRs, guias de TV e verificações das sprints
```

Decisões de arquitetura em [`docs/adr/`](docs/adr/).
