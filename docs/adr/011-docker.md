# ADR 011 — Docker como opção de instalação

- **Status:** aceito; build, execução, transcodificação (CPU e NVENC) e detecção de abertura verificados (01/11/2026).
- **Data:** 31/10/2026

## Contexto

Rodar a partir do código exige Go 1.27.3, Node 24, make e FFmpeg. Quem só quer usar o servidor precisa de um caminho mais curto. O `AGENTS.md` §2 proíbe Docker como **requisito**, mas não como opção.

## Decisões

1. **Opcional.** O binário continua sendo o caminho principal. Nenhum código Go ou web muda por causa do Docker.
2. **Imagem multi-stage**: Node 24 gera o frontend, Go 1.27 compila sem CGO (`GOTOOLCHAIN=auto` baixa o patch exato do `go.mod`), um stage compila o FFmpeg 9.0.2 do tarball oficial (SHA-256 fixado) com x264, x265, libass, libwebp, NVENC e Chromaprint (estático, com KissFFT, para o `internal/intro`; o `libchromaprint` do Debian puxaria o FFmpeg do Debian junto), e a imagem final `debian:bookworm-slim` (glibc, exigido pelo runtime NVIDIA) traz só o binário, `ffmpeg`, `ffprobe` e as bibliotecas.
3. **Configuração só por variáveis de ambiente** (`HOST`, `PORT`, `DATA_DIR`, `MEDIA_DIR`), como já prevê o `AGENTS.md` §5. Mídias em `/media` (somente leitura no compose) e dados em `/data` (volume nomeado).
4. **Usuário não-root** (UID 1000) e `HEALTHCHECK` em `/api/health`.
5. **GPU NVIDIA (NVENC)** por um arquivo à parte, `docker-compose.nvidia.yml`, que reserva a GPU (Linux com NVIDIA Container Toolkit, ou Docker Desktop com WSL 2). O NVENC carrega o driver em tempo de execução (`nv-codec-headers`), então a imagem não leva CUDA. Sem GPU, o servidor cai para CPU (`HWACCEL=auto`). VAAPI vem compilado, mas a imagem não traz drivers Intel/AMD; QSV não foi compilado.
6. **Rede:** na rede *bridge* a descoberta automática de TVs pode não funcionar. No Linux, `network_mode: host` resolve; no Docker Desktop (Windows/macOS) digite o IP na TV.

## Consequências

- O FFmpeg do Alpine (6.1 estável, 9.0 no edge) não tem NVENC, e o do Debian é 7.x; por isso o FFmpeg é compilado. O build leva alguns minutos na primeira vez e o resultado fica em cache.
- Versões e checksums do FFmpeg e do Chromaprint ficam em `ARG` no `Dockerfile`; atualizar é trocar os pares. Um recurso novo do servidor que dependa de um componente do FFmpeg precisa dele também no `./configure` da imagem (foi o caso do `-f chromaprint`).
- A imagem final tem ~283 MB.
- Em pasta do Windows montada via `/mnt/c`, o aviso de arquivo novo (inotify) pode não chegar; o scan manual resolve. Não verificado.
- Mais um artefato para manter; a imagem não faz parte do release v1.0.0.
