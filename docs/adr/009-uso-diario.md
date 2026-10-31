# ADR 009 — Serviço, backup, status e erros para o uso diário

- **Status:** aceito; serviço testado no Linux (systemd de usuário); Windows e macOS compilados, com lint e testes unitários.
- **Data:** 31/10/2026

## Contexto

A Sprint 8 prepara a v1.0 para ficar ligada o tempo todo num PC de casa:

- iniciar com o sistema;
- proteger perfis, progresso e correções;
- mostrar se o servidor está bem;
- explicar falhas para quem não lê logs.

O app não tem login de administrador, só perfis locais. A decisão de como fazer foi do usuário.

## Decisões

1. **Serviço por subcomando** (`internal/service`): `mediaserver service install|uninstall|start|stop|status`.
   - **Linux (systemd):** como root, cria uma unit de sistema com `User=$SUDO_USER`; sem root, uma unit de usuário, com a dica do `loginctl enable-linger` para iniciar no boot sem login. Usa `Restart=on-failure`.
   - **macOS (launchd):** LaunchAgent do usuário, com `KeepAlive.SuccessfulExit=false` (reinicia só após falha) e log em `~/Library/Logs/mediaserver.log`. Um LaunchDaemon poderia não ler as mídias nas pastas do usuário (TCC).
   - **Windows (Service Manager):** usa `golang.org/x/sys/windows/svc`, que já era dependência indireta (sem CGO). Configuração: início automático atrasado e reinício após falha. O serviço roda `mediaserver service run <pasta> <flags>`: entra na pasta de instalação, traduz Stop/Shutdown no cancelamento do contexto (o mesmo encerramento gracioso do Ctrl+C) e grava o log em `DATA_DIR/mediaserver.log`, que recomeça quando passa de 10 MB.
   - O `install` valida a configuração na hora, e não no boot. Também converte as variáveis de ambiente definidas em flags (`config.EnvAsFlags`), porque o serviço não herda o ambiente do terminal. A pasta atual vira o diretório de trabalho, então `./data` e `config.toml` continuam valendo.
   - As units e o plist saem de funções puras com testes. Os comandos externos ficam isolados e substituídos nos testes.
2. **Backup pela interface, restauração pelo terminal.**
   - `GET /api/admin/backup` exige perfil e faz `VACUUM INTO` (cópia consistente com o servidor rodando, sem WAL).
   - Em **Configurações → Baixar backup**, o download é feito via `fetch` + blob, porque precisa do cabeçalho `X-Profile`. Nas TVs o botão fica escondido.
   - `mediaserver restore <arquivo>` recusa se um mediaserver responder na porta configurada. Ele copia o arquivo para perto do banco e valida a cópia antes de tocar no original: `integrity_check`, `schema_migrations` e versão ≤ a do binário (as mais antigas migram no próximo início). Depois guarda o banco atual como `mediaserver.db.antes-<data>`.
   - Restaurar pela rede foi descartado: qualquer aparelho da casa poderia trocar o banco.
3. **Status** (`GET /api/admin/status`, exige perfil; página `/status`, atualizada a cada 2 s). Campos:
   - versão e tempo ligado;
   - CPU do processo (`getrusage` / `GetProcessTimes`) e do computador (`/proc/stat` / `GetSystemTimes` da kernel32; no macOS, só o processo, porque o total exigiria CGO);
   - memória pelas métricas do runtime Go (sem C, fica próxima da memória residente);
   - encoder detectado e, com `nvidia-smi`, o uso de GPU e NVENC;
   - sessões de FFmpeg e vagas (`Transcoder.Stats/Slots`);
   - tamanho dos caches e do banco, recalculado a cada 10 s;
   - contagem da biblioteca.
4. **Erros com código.** Toda resposta de erro tem `code`: `file_missing` (410), `no_ffmpeg`, `busy`, `shutting_down` (503), `not_found`, `invalid`, `conflict`, `internal`… O arquivo é verificado também no direct play, então "sumiu do disco" aparece antes do player tentar.
   - No cliente, `ApiError.code` (`offline` quando não há resposta) passa por `lib/api/errors.ts`, que devolve título, dica e ação (Tentar de novo / Escanear biblioteca).
   - Usado no `ErrorNotice` (páginas) e no player.
5. **Teste de carga** (`make load`, fora do `make test`): 2 direct play (256 MB, seeks) + 1 conversão real quando houver FFmpeg. Mede o tempo até o primeiro byte, a vazão, o 1º segmento e a memória, e confere que o FFmpeg termina ao encerrar a sessão.
6. **Release preparada, não publicada** (`.github/workflows/release.yml`):
   - uma tag `v*` cria um rascunho com os binários, o `.apk`, o `.ipk` e `SHA256SUMS`;
   - a execução manual só guarda um *artifact*;
   - o `.wgt` é anexado à mão (certificado Samsung);
   - o APK é assinado com a chave do projeto quando os secrets existem;
   - `versionCode` vem da tag.

   Passos em [`docs/release.md`](../release.md).

## Consequências

- Atualizar o binário instalado como serviço: `service stop`, trocar o arquivo, `service start`. Mudar flags exige `service uninstall` + `install`.
- Esqueci o PIN: continua sem solução pela interface (Backlog).
- O 1º segmento com NVENC < 2 s e os 7 dias de uso contínuo dependem do hardware do usuário e ficam para o aceite.
