# Verificação da Sprint 7

Executada em 31/10/2026 no WSL 2 (Linux), sem commit. Não houve teste em TV real: a Samsung do usuário está disponível, mas exige Tizen Studio + certificado Samsung (veja [docs/tv/samsung.md](tv/samsung.md)).

## Qualidade e testes

- `make fmt`, `make check` e `make test` aprovados.
  - golangci-lint: 0 issues.
  - `go test -race ./...`: ok. Os testes com FFmpeg real são pulados no WSL, porque o FFmpeg não está no PATH.
  - Web: **177 testes**; oxfmt, oxlint e svelte-check sem avisos.
- Orçamento: JS inicial do app **56,8 KB gzip**. A página da TV (`web/build-tv/index.html`, arquivo único) tem 75,4 KB gzip e passa na checagem de CSS proibido do Chromium 76.
- Testes novos:
  - teclas da TV (códigos Samsung/LG e nomes DOM);
  - detecção de plataforma;
  - protocolo `postMessage`;
  - adaptadores Tizen/webOS/Android com APIs falsas;
  - descoberta (sub-rede, concorrência, timeout, cancelamento);
  - endereço do servidor;
  - montagem do HTML único;
  - `/api/health` com CORS e `name`.
- `make bench` não foi rodado: nenhum caminho crítico (scanner, stream, listagens) mudou. O log `playback` é uma linha por sessão.

## Pacotes

- **Android:** `make tv-android` gera `dist/tv/mediaserver-tv.apk` (105 KB). O `aapt2 dump badging` confirma:
  - `leanback-launchable-activity` e `launchable-activity`;
  - banner;
  - `INTERNET`;
  - `leanback`/`touchscreen` não obrigatórios;
  - `assets/index.html`.

  Assinatura: chave debug.
- **LG:** `make tv-webos` gera `com.mediaserver.tv_1.0.0_all.ipk` com `@webos-tools/cli` 3.2.6, instalado só fora do repositório. `requiredACG: ["network.query"]` foi declarado depois do aviso do `ares-package`. O nome do serviço luna foi corrigido para `com.palm.connectionmanager`, conforme a referência da LG.
- **Samsung:** sem Tizen CLI aqui. `make tv-tizen` para com a instrução. O `config.xml` é XML válido.

## E2E (Chromium 156 headless, `playwright-core` fora do repositório)

O shell foi aberto via `file://`. As APIs de cada TV foram simuladas (`tizen`, `PalmServiceBridge`, `MediaserverAndroid`) contra o servidor real na porta 8080. O resultado foi **30/30, estável em 6 rodadas seguidas**.

- **Tizen:**
  - 20 teclas registradas;
  - a descoberta acha o servidor na /24 da "TV" e põe o foco nele;
  - OK abre o iframe com `?tv=tizen`; o servidor fica salvo; o app entra em modo TV;
  - perfil → Início → Séries → série → episódio → **player só com teclas**;
  - Pause/Play/Avançar/Retroceder mantêm o player, e Stop sai dele;
  - Voltar (10009) sobe até o Início; Amarelo → Buscar e Azul → Início;
  - tecla com o frame sem foco é repassada pelo shell;
  - "Este aparelho" mostra Samsung (Tizen) com `mkv`/`ac3`;
  - "Trocar servidor" volta à tela de conexão, e Voltar retorna ao servidor;
  - Voltar no Início abre "Sair do mediaserver?"; Voltar fecha o diálogo; Sair chama o `exit` da Tizen;
  - escala em 720p (10,67 px/rem), 1080p (16) e 4K (32);
  - nenhum erro de página.
- **webOS:** o servidor salvo abre direto com `?tv=webos`; Voltar (461) + Sair chama `window.close()`.
- **Android:**
  - endereço digitado, Conectar navega o WebView até o servidor;
  - modo TV com viewport 960×540 (1 rem = 8 px);
  - o mesmo JS que a `MainActivity` injeta para Voltar abre o diálogo, e Sair chama `exit()` da ponte.
- **Texto na TV:** auditoria de 11 telas em 1080p. Todo texto ficou ≥ 24 px, exceto a inicial do avatar, que é gráfica.

## Problemas encontrados e corrigidos

- Campo de texto da tela de conexão: o `blur()` antes do `focus()` encerrava a edição na hora, e o texto digitado não entrava.
- `nav.svelte.ts` (Sprint 3): numa troca de página, o lrud podia lançar "trying to assign focus to a non focusable node". Agora cai no primeiro item registrado.

## Aceite manual restante

- [ ] Samsung: instalar o `.wgt`, navegar só com o controle, direct play H.264/HEVC + AAC (log `playback direct=true`), reabrir o app mantendo o perfil, layout no Chromium 76.
- [ ] LG e Android TV/Fire TV: sem aparelho disponível.
