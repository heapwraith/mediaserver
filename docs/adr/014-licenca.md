# ADR 014 — Licença do projeto: GPL-3.0-or-later

Status: aceito (02/11/2026)

## Contexto
O projeto precisa de uma licença open source. Três fatos pesam:
1. O build do frontend embute `rvfc-polyfill`, **GPL-3.0**, que vem do JASSUB.
2. O JASSUB (WebAssembly) é LGPL-2.1+ e carrega FreeType, HarfBuzz e FriBidi.
3. A imagem Docker traz FFmpeg compilado com `--enable-gpl --enable-version3` (x264, x265).

## Decisão
**GPL-3.0-or-later** para o código do repositório (`LICENSE`).
- Não tem conflito com nenhuma dependência atual.
- O que se distribui junto (binário com o frontend embutido) já precisa ser GPL-3 por causa do `rvfc-polyfill`.
- O FFmpeg é chamado como processo separado (`exec`), então não contamina o código; mesmo assim a imagem Docker é uma redistribuição de software GPL e tem de seguir a GPL.

## Alternativas descartadas
- **MIT / Apache-2.0:** exigiriam trocar o `rvfc-polyfill` por uma implementação própria. Fica como opção futura, com ADR.
- **AGPL-3.0:** o servidor é de uso local; a cláusula de rede não traz ganho agora.

## Consequências
- Quem receber o binário, a imagem ou os pacotes de TV precisa poder obter o código-fonte correspondente (o repositório público e a tag da versão cumprem isso).
- Contribuições entram sob a mesma licença.
- Dependência nova passa pela checagem de licença: nada proprietário, nada "non-commercial", nada incompatível com a GPL-3 (ex.: GPL-2-only, SSPL, CC-NC).
- Detalhes e pendências em [`docs/conformidade.md`](../conformidade.md).
