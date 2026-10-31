# Verificação da Sprint 8

Executada em 31/10/2026 no WSL 2 (Linux, Ryzen 7 5700X, RTX 5070), sem FFmpeg no PATH: baixar um FFmpeg estático foi bloqueado pela política de segurança da sessão. Os itens que dependem de FFmpeg ou de outro sistema estão marcados como pendentes.

## Metas do AGENTS.md §8

| Métrica | Meta | Medido | Como |
|---|---|---|---|
| Inicialização do servidor | < 500 ms | **36–42 ms** (5 inícios) | `startup_ms` no log `server ready`, biblioteca de 28 arquivos |
| RAM em repouso | < 80 MB | **15,8 MB** | `VmRSS` 20 s após iniciar (biblioteca pequena) |
| Scan incremental, 10 mil arquivos sem mudança | < 3 s | **79 ms** | `BenchmarkRescanNoChanges` |
| Início do direct play | < 300 ms até o 1º byte | **p95 2,5 ms** (localhost) · **p95 1,3 ms** com 2 streams em paralelo | 50 pedidos com Range; `make load` |
| Início com conversão (NVENC) | < 2 s até o 1º segmento | **pendente** | precisa de FFmpeg com NVENC: `make load` no seu PC |
| Listagem da biblioteca | < 50 ms p95 | **0,7–8,5 ms/op** com 10 mil itens (bench); p95 2,2 ms pela HTTP | `BenchmarkListMovies` (título/adição/ano), 300 pedidos |
| Bundle JS inicial | < 150 KB gzip | **58,5 KB** | `make check` (`check-build`) |

Outros números do `make bench`: Range 1,63 GB/s; Início (home) 17,6 ms/op.

## Qualidade e testes

- `make fmt`, `make check` e `make test` aprovados.
  - O golangci-lint também passou com `GOOS=windows` e `GOOS=darwin` em `cmd/` e `internal/service`.
  - `go vet` passou para Windows, macOS e FreeBSD.
- **Go:**
  - códigos de erro (arquivo apagado → 410 `file_missing` no direct play, na conversão e no stream; FFmpeg ausente → 503 `no_ffmpeg`);
  - backup (`VACUUM INTO`) e restauração (recusa servidor no ar, arquivo que não é banco, banco sem nossas tabelas e schema mais novo; mantém o banco anterior; caminho com acento);
  - units do systemd e plist do launchd (aspas, `%`, `$`, `&`);
  - gerenciador systemd com `systemctl` falso (usuário e root);
  - `EnvAsFlags`, `installSpec` e `restoreCmd` de ponta a ponta;
  - leitura de `/proc/stat` e do `nvidia-smi`;
  - `Transcoder.Stats`;
  - endpoints `/api/admin/backup` e `/api/admin/status` (401 sem perfil).
- **Web:** 203 testes, incluindo o mapeamento de erros e a formatação da página de status.

## Serviço (Linux real, systemd de usuário)

Sequência testada: `MEDIA_DIR=… mediaserver service install -data-dir ./svc-data -port 8099`.

- A unit foi criada com a variável de ambiente convertida em `-media-dir=…` e com `WorkingDirectory` na pasta atual.
- `status` mostrou `running` e o `/api/health` respondeu.
- `stop` levou a `inactive`, com o servidor fora do ar.
- `start` voltou a `running`, e os logs JSON apareceram no `journalctl --user`.
- `uninstall` levou a `not installed`, sem sobrar arquivo.

Windows (Service Manager) e macOS (launchd) compilam e passam no lint; falta instalar numa máquina real.

## E2E (Chromium 156 headless)

- **Configurações → Baixar backup:** o arquivo `mediaserver-20261031-1856.db` começa com `SQLite format 3`, e a página mostra o comando de restauração.
- **Status do servidor:**
  - mostra versão e tempo ligado e atualiza sozinha;
  - mostra processador (servidor e computador), memória, aceleração (com a RTX 5070 via `nvidia-smi`), conversões e vagas, disco e biblioteca;
  - foram feitas capturas em 1080p no PC e no modo TV.
- **Arquivo apagado depois do scan:** o player mostra "Arquivo não encontrado" com a dica de escanear.
- **Sem FFmpeg e formato que o navegador não toca:** "Conversão indisponível".
- **Servidor fora do ar** (API bloqueada no navegador): "Servidor indisponível". "Tentar de novo" recarrega a página quando ele volta.
- Nenhum erro de página.

## Teste de carga (`make load`)

2 direct play simultâneos de 256 MB, cada um com 3 seeks e leitura completa:

- 512 MB lidos em 137 ms (~3,7 GB/s, localhost);
- tempo até o primeiro byte: p95 1,3 ms;
- a memória não cresce com o tamanho dos arquivos (são servidos em streaming).

A conversão foi pulada (sem FFmpeg). No seu PC, com FFmpeg, o mesmo comando sobe uma conversão real ao mesmo tempo, mede o 1º segmento e confere que o FFmpeg termina ao encerrar a sessão.

## Encontrado durante a verificação

- **Arquivos copiados para a pasta segundos antes de iniciar o servidor** só entraram na biblioteca depois de um scan manual. É o item do Backlog sobre arquivos "em cópia" no scan de inicialização.

## Aceite manual restante

- [ ] `make load` com FFmpeg (e `HWACCEL=nvenc`): 1º segmento < 2 s, sem FFmpeg órfão.
- [ ] `mediaserver service install` no Windows (terminal de administrador) e, se houver, no macOS.
- [ ] 7 dias de uso contínuo com a memória estável (Configurações → Status do servidor).
- [ ] Publicar a v1.0.0 seguindo [docs/release.md](release.md).
