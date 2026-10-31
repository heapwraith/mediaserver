# ADR 008 — Apps de TV leves: a interface vem do servidor

- **Status:** aceito; validado em Chromium headless com Tizen, webOS e Android simulados. Falta o aceite em aparelho real (Samsung disponível; LG e Android TV sem aparelho).
- **Data:** 31/10/2026

## Contexto

A Sprint 7 leva o mediaserver para Samsung (Tizen), LG (webOS) e Android TV/Google TV/Fire TV. O AGENTS.md falava em "mesmo frontend empacotado". Empacotar a interface inteira em cada `.wgt`/`.ipk`/`.apk` traria alguns custos:

- reinstalar o app em cada TV a cada mudança de interface (modo desenvolvedor, certificado Samsung, sessão da LG que expira);
- o app da TV e o servidor ficariam em versões diferentes;
- toda a API precisaria de CORS, rotas com `#` e URLs absolutas para imagens, vídeo e legendas.

## Decisões

1. **O pacote traz só a página de conexão** (`web/src/shell`, mini app Svelte 5 sem SvelteKit). Ela acha ou recebe o endereço do servidor, salva no aparelho e abre `http://IP:PORTA/?tv=<plataforma>`. A interface é a mesma do navegador, servida pelo servidor: atualizar o servidor atualiza todas as TVs.
2. **Tizen e webOS: `<iframe>` em tela cheia.** A página local continua por trás com acesso às APIs da TV (registrar teclas, sair). O app e a página conversam por `postMessage` (`web/src/lib/platform/bridge.ts`):
   - o app manda `ready`, `exit` e `change-server`;
   - a página repassa uma tecla quando o frame perde o foco;
   - sem `ready` em 15 s, a tela de conexão volta.

   É o mesmo modelo do app do Jellyfin para LG.
3. **Android: navegação direta, sem iframe.** O WebView abre a página local e depois o servidor. A ponte `window.MediaserverAndroid` (`exit`, `openSetup`, `connect`, `localIp`) existe em qualquer página. A Activity:
   - repassa Voltar, mídia e cores como `keydown`, porque o WebView não os entrega;
   - bloqueia navegação para fora do servidor escolhido.

   Sem iframe, o storage não fica particionado e a tecla Voltar do Android funciona.
4. **A página é um único `index.html`** (`web/vite.tv.config.ts`): script clássico embrulhado numa função, CSS em `<style>` e a fonte Inter (só latin) como data URL. Isso porque, aberto via `file://`, o runtime da TV pode bloquear scripts de módulo, folhas `crossorigin` e arquivos de fonte. O `make check` aplica a mesma checagem de CSS proibido (Chromium 76) ao `<style>` dela.
5. **Descoberta:** apps web de TV não mandam UDP (SSDP/mDNS). A página pede `GET /api/health` a cada endereço da /24 da TV, 24 por vez, com timeout de 1,5 s (~16 s no pior caso), na porta do campo (padrão 8080).
   - O IP da TV vem de `tizen.systeminfo` (`ETHERNET_NETWORK`/`WIFI_NETWORK`), da luna `com.palm.connectionmanager/getStatus` via `PalmServiceBridge` (ACG `network.query`) ou da ponte Android.
   - Sem IP: varre a rede do servidor salvo, senão 192.168.0.x e 192.168.1.x.
   - `/api/health` é o único endpoint com `Access-Control-Allow-Origin: *` e devolve `"name": "mediaserver"` para identificar o servidor.
6. **Plataforma:** o app lê `?tv=` uma vez e guarda em `sessionStorage`. Tizen/webOS só valem dentro de um frame; Android, só com a ponte. Fora disso é `browser`. Nada de user-agent. `isTV` liga:
   - `html.tv`: escala sempre `100vw/120`; o Android TV informa viewport de 960×540;
   - texto ≥ 24 px em 1080p;
   - o diálogo "Sair do mediaserver?" no Voltar do Início;
   - "Trocar servidor" nas Configurações.
7. **Teclas** (`keys.ts`):
   - Play, Pause, Play/Pause, Stop, Avançar e Retroceder por nome DOM e pelos códigos Samsung/LG (415, 19, 10252, 413, 417, 412).
   - Cores 403–406. No player, Vermelho = legendas e Verde = áudio. Em qualquer tela, Amarelo = Buscar e Azul = Início.
   - A Tizen registra mídia, cores e 0–9 com `tvinputdevice.registerKey`.
8. **Codecs:** Tizen e webOS somam `mkv`, `ac3` e `eac3` ao que o navegador declara (a mesma tabela do Jellyfin). As TVs respondem `''` para esses formatos, mas o player nativo toca. Se o direct play falhar, o player tenta de novo sem os contêineres. "Configurações → Este aparelho" mostra os tokens para conferência.
9. **Android:** Gradle + AGP 8.13, Java, sem AndroidX e sem Kotlin, minSdk 21. Release assinado com a chave debug da máquina (sideload). `LEANBACK_LAUNCHER` + `LAUNCHER` e banner 320×180. Tela sempre ligada com o app aberto.

## Consequências

- O app da TV quase nunca precisa ser reinstalado; a interface acompanha o servidor.
- A TV precisa do servidor ligado para abrir. Sem ele, a tela de conexão mostra o erro e procura de novo.
- **Risco a verificar na TV:** a partir da Tizen 8 / webOS 23 (Chromium 108+), o storage do iframe pode ficar particionado pela origem `file://`. Se o perfil e as configurações não sobreviverem a fechar o app, mover o storage para a página local (via `postMessage`).
- Os pacotes só empacotam a página (`make tv-tizen`, `make tv-webos`, `make tv-android`). Ferramentas: Tizen Studio CLI + certificado Samsung; `@webos-tools/cli`; Android SDK. Nenhuma entra nas dependências do `web/`.
