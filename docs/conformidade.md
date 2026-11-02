# Conformidade: licenças, direitos autorais, privacidade e segurança

Análise de 02/11/2026 sobre o código, as dependências, a imagem Docker e os apps de TV. É uma revisão técnica, **não é parecer jurídico**. Itens marcados "verificar" dependem de consulta a um advogado ou de uma busca que não dá para fazer daqui.

Resumo: **não há nada que impeça o uso pessoal e o código aberto**. Os pontos que pedem ação antes de publicar binários ou a imagem Docker estão em "Fazer antes de publicar".

## 1. Licenças das dependências

| Área | Resultado |
|---|---|
| Go (14 módulos) | Todos MIT ou BSD-3. Nada a fazer além de manter [`THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md) (BSD/MIT exigem manter o aviso de copyright ao redistribuir). |
| npm que vai no build | MIT, ISC, BSD, Apache-2.0, OFL-1.1 (fontes), **LGPL-2.1+ (JASSUB)** e **GPL-3.0 (`rvfc-polyfill`)**. |
| npm só de build | MPL-2.0 (Lightning CSS) e CC-BY-4.0 (caniuse-lite): não vão no que se distribui. |
| Fontes | Bebas Neue e Archivo, OFL-1.1: uso, embutir e redistribuir liberados; não vender a fonte sozinha. Auto-hospedadas, sem CDN (sem vazar IP para o Google Fonts, bom também para a LGPD). |

**Achado principal:** o `rvfc-polyfill` (GPL-3.0, vem do JASSUB) está no bundle servido (`web/build/_app/immutable/chunks/…`). Com licença MIT/Apache, o conjunto distribuído continuaria sendo GPL-3 na prática. Por isso o projeto é **GPL-3.0-or-later** ([ADR 014](adr/014-licenca.md)). Se um dia quiser licença permissiva, é preciso trocar esse polyfill por código próprio.

Pendente: a fonte padrão embutida no JASSUB (`default.woff2`) não declara licença no pacote. Antes do release, confirmar no repositório do JASSUB de qual fonte se trata e se a licença permite redistribuir.

## 2. FFmpeg e codecs

- A imagem Docker compila o FFmpeg 9.0.2 com `--enable-gpl --enable-version3` (x264, x265) e **sem** `--enable-nonfree`. Isso é correto: um binário `nonfree` não pode ser redistribuído, e este pode (com a GPL).
- O servidor só chama `ffmpeg`/`ffprobe` por `exec.CommandContext`: processo separado, sem ligação com o código. A GPL do FFmpeg não se estende ao servidor por isso, e o servidor já é GPL de qualquer forma.
- **Decisão: não publicar imagem** (ver seção 8). Se isso mudar, publicar a imagem (Docker Hub, GHCR) ou um pacote com o FFmpeg dentro é redistribuição de GPL. Obrigações: incluir a licença, informar de onde vem o código-fonte exato (versão e URL do `Dockerfile` bastam, junto com um `--build-arg` fixo) e oferecer o código por 3 anos ou manter público.
- **Patentes:** H.264, HEVC (H.265) e AAC têm *pools* de patentes. Quem compila para uso próprio não costuma ter problema; quem **distribui binários** com x264/x265 fica mais exposto em países que aceitam patente de software (EUA, por exemplo). No Brasil, programa de computador "em si" não é patenteável (Lei 9.279/96, art. 10, V). Por isso o projeto distribui só o código e o `Dockerfile`, não uma imagem pronta.
- **NVENC:** só os cabeçalhos MIT entram; o driver da NVIDIA é carregado do host e não é redistribuído. Sem problema.
- O release `v1.0.0` (binários Go + pacotes de TV) **não leva FFmpeg**: o usuário instala o dele. Isso evita a questão acima nos binários.

## 3. Direitos autorais e conteúdo

- O projeto **não baixa, não busca e não distribui mídia**, não tem integração com torrent e não contorna DRM (nenhum código de quebra de proteção; o que for criptografado simplesmente não toca). Isso mantém o projeto fora do alcance de normas contra contorno de proteção (Lei 9.610/98, art. 107, e DMCA §1201).
- O servidor só serve arquivos de `MEDIA_DIR` ao próprio usuário, na rede local. Compartilhar o acesso com terceiros fora da casa (Tailscale, abrir porta) pode virar "comunicação ao público" de obra protegida. Por isso o acesso externo está fora de escopo no `AGENTS.md`.
- Nomes de release no código: `WEB-DL`, `BluRay` e `[Grupo]` aparecem só como **padrões de reconhecimento**. Num repositório público isso pode parecer "ferramenta de pirataria". Mitigações feitas: aviso de uso responsável no README e **troca de todos os títulos, episódios e grupos de release reais** (obras, séries, animes, grupos de fansub e de scene) nas fixtures de teste e na documentação por nomes fictícios.
- Imagens do repositório: ícones, banner e logo são próprios; os dois prints em `docs/sprint-6/` são frames sintéticos. Não há vídeo, legenda ou pôster de terceiros versionado (`/media/` e `/dist/` estão no `.gitignore`).
- **Não commitar** mídia de teste real. Para a prévia, usar só obras livres (ex.: filmes da Blender, CC-BY).

## 4. Marcas e nome

- **Samsung, Tizen, LG, webOS, Android, Google TV, Fire TV, Amazon, TMDB, Netflix, Apple TV, Max:** citadas só de forma descritiva (compatibilidade e *princípios* de design no `AGENTS.md`). Isso é uso nominativo. Adicionado aviso de "sem afiliação" no README e no `THIRD_PARTY_NOTICES.md`. Nenhum logo, ícone ou fonte desses serviços é usado.
- **Nome "Telão":** palavra comum do português. Não achei nada no repositório que indique conflito, mas **não dá para consultar o INPI daqui**. Verificar a base do INPI (classes 9, 38, 41 e 42) antes de publicar o nome em loja de apps ou fazer divulgação. Alternativa barata: usar "Telão" como nome de exibição e manter o repositório com um nome mais específico.
- **Apps de TV:** a loja da Samsung (Seller Office), da LG e a Google Play têm regras próprias (ex.: não usar "TV", "Samsung" ou "LG" no nome, política de conteúdo, política de privacidade). Hoje o app só é instalado por sideload; ao enviar a uma loja, ler as regras de cada uma. O `.wgt` assinado com o certificado do autor/distribuidor Samsung **é ligado ao DUID da sua TV**: não publicar o `.wgt` assinado nem o `.p12` (o `.gitignore` já barra; a checagem de `git ls-files` não achou certificado ou chave versionados).

## 5. TMDB (opcional, desligado por padrão)

- Desligado: nenhuma chamada de rede. Ligado: só envia título/ano/ID para a API do TMDB com a chave do próprio usuário.
- Os termos do TMDB pedem **atribuição visível** ("This product uses the TMDB API but is not endorsed or certified by TMDB") e, para apps, o **logotipo do TMDB** na tela de créditos/sobre. O texto e o logotipo estão em Configurações. O uso é **não comercial** por padrão: se um dia o projeto virar produto pago, é preciso pedir licença comercial ao TMDB.
- Cache: os termos limitam guardar os dados por muito tempo sem atualizar; hoje só gêneros e coleções são guardados. Verificar os termos vigentes ao ampliar para sinopse, nota e elenco (Backlog).
- A chave é gravada no banco local e nunca volta pela API nem vai para o log (aceite da Sprint 11). Ela **está no backup** (ver item de segurança abaixo).

## 6. Privacidade (LGPD e similares)

- O servidor guarda nome e cor do perfil, PIN (hash PBKDF2-SHA256 com sal), histórico de reprodução e o caminho dos arquivos. Tudo local, **sem telemetria, sem conta externa, sem fonte/CDN externo**.
- Uso doméstico por pessoa física, sem fim econômico, está fora do alcance da LGPD (art. 4º, I). Se alguém operar o Telão para **terceiros** (empresa, hotel, condomínio), passa a ser controlador de dados e precisa de base legal, aviso de privacidade e canal do titular. Fica registrado aqui e deve entrar no README se o projeto crescer.
- O log JSON registra o play (`playback`) com o ID da mídia e os codecs que o aparelho declarou, e cada pedido da API com método, caminho (sem a query, onde ficam os tokens), status e duração. **Não registra IP, perfil, token nem chave do TMDB.** Mesmo assim, não envie logs a terceiros sem revisar.

## 7. Segurança (risco de responsabilidade e de reputação)

O servidor escuta em `0.0.0.0` sem autenticação de rede. Para uso em casa é aceitável. Em 02/11/2026 uma auditoria de segurança ([auditoria-seguranca.md](auditoria-seguranca.md)) corrigiu o vazamento de segredos no backup, CSRF, DNS rebinding e o bloqueio de PIN; o PIN de administrador agora existe, mas é opcional (desligado por padrão): recomende defini-lo. Tabela original:

| Prioridade | Item | Risco |
|---|---|---|
| Parcial | `/api/admin/backup` aceita qualquer perfil (o backup já sai sem segredos e sem PINs; falta o administrador) e o backup levava o segredo que assina as sessões (e a chave do TMDB) | Qualquer pessoa na rede pode forjar a sessão de um perfil com PIN |
| Alta | Revisar, scan e mesclar/separar séries sem autenticação | Qualquer aparelho da rede altera a biblioteca (não os arquivos) |
| Média | Subpasta ilegível apaga progresso | Perda de dados do usuário |
| Média | `usesCleartextTraffic="true"` no app Android | Necessário para HTTP na LAN; documentar que o tráfego não é cifrado e não expor à internet |
| Baixa | `Access-Control-Allow-Origin: *` só em `/api/health` | Só devolve nome, status e versão; sem dado sensível |

Pontos já corretos: PIN com PBKDF2 e bloqueio por tentativas; sessão assinada por HMAC; validação contra *path traversal* e `MEDIA_DIR` somente leitura no Docker; container sem root; nenhum segredo, chave ou `.env` versionado (varredura de `git ls-files`); IP e DUID da sua TV **não** estão no repositório (só exemplos genéricos nos guias).

Aviso de garantia: a GPL já traz "sem garantia"; o README repete que o projeto não tem garantia e que não deve ser exposto à internet sem uma camada de proteção.

## 8. Decisões e pendências

**Decididas em 02/11/2026**
- **Docker:** o projeto distribui **só o `Dockerfile`**. Não se publica imagem pronta em Docker Hub, GHCR ou similar. Cada pessoa compila a sua (`make docker`), o que deixa a redistribuição do FFmpeg GPL e as patentes de codec fora do que o projeto entrega.
- **Fixtures sem obras reais:** os testes do scanner (`internal/scanner/testdata/`, testes Go e `cards.spec.ts`) e a documentação usam só títulos, episódios e grupos de release fictícios. Os padrões que o parser reconhece (`WEB-DL`, `BluRay`, `x265`, `[Grupo]`) continuam; o filtro de propaganda (`1xbet`, `promo`, `www`) também.

**Feitas**
1. [x] `LICENSE` (GPL-3.0-or-later), `THIRD_PARTY_NOTICES.md`, ADR 014, aviso de uso responsável e de marcas no README, `license` no `web/package.json`.
2. [x] Logotipo do TMDB junto da atribuição em Configurações (`web/src/lib/assets/tmdb.svg`, logo oficial; mantê-lo menos destacado que a marca do Telão).
3. [x] Cabeçalho `SPDX-License-Identifier: GPL-3.0-or-later` em todos os `.go`. Arquivo Go novo deve começar com ele.
4. [x] Checagem de licenças no `make check` (`web/scripts/check-licenses.ts`): barra dependência de produção do frontend fora da lista compatível com a GPL-3. Para o Go, conferir a licença de módulo novo à mão (todos os atuais são MIT/BSD).

**Ainda abertas**
- [x] Backup sem segredos, `guard` (Host/Origin/Content-Type), sessão com prazo e PIN por endereço ([ADR 015](adr/015-seguranca-rede.md)).
- [x] PIN de administrador opcional (ADR 015). Recomendado definir na instalação.
- [ ] Confirmar a licença da fonte padrão do JASSUB (`default.woff2`): o pacote não a identifica.
- [ ] Buscar "Telão" no INPI (classes 9, 38, 41 e 42).
- [ ] Opcional: cabeçalho SPDX também nos `.ts` e `.svelte`.
