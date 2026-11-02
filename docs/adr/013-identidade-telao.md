# ADR 013 — Identidade visual "Telão"

- **Status:** aceito
- **Data:** 01/11/2026
- **Substitui:** a direção visual do AGENTS.md §7 (inspiração Apple TV+/tvOS, fonte Inter, destaque azul `#4C8DFF`).

## Contexto

O app não tinha marca, tinha um menu horizontal de site com 7 itens (inclusive telas de administração), destaques vazios quando não havia frame e cards com o título repetido. O usuário pediu uma identidade no nível dos grandes streamings. Foram feitas duas rodadas de protótipos com a skill impeccable:

1. Três mundos temáticos (cordel, videolocadora, azulejo), recusados por parecerem conceito, não produto.
2. Três variações do padrão premium com cenas reais (filmes abertos da Blender, CC-BY): Cinema, **Telão** e Sessão. O usuário escolheu o Telão, com fundo preto e destaque laranja.

## Decisões

1. **Nome na interface: Telão.** O módulo, o binário e os pacotes continuam `mediaserver` até a troca do placeholder (Backlog).
2. **Tema escuro preto** (`#000`), painéis `#121212`/`#1c1c1e`, texto `#f5f5f4` / `#a8a29e` / `#8f8882`, **um destaque laranja `#ff4d2e`** para estado e progresso (nunca para texto corrido).
3. **Tipografia:** Bebas Neue para títulos (condensada, lida bem a 3 m) e Archivo para o resto, ambas auto-hospedadas via Fontsource (OFL). A Inter sai.
4. **Menu lateral de ícones** fixo à esquerda, que abre com os rótulos quando recebe o foco. Só o que é para assistir fica no menu (Buscar, Início, Filmes, Séries, Animes); Configurações e o perfil ficam no rodapé do menu; **Revisar** passa a ser acessado por Configurações.
5. **Destaque (hero):** a cena ocupa a direita da tela e esmaece no preto; título grande em Bebas; selos de qualidade e áudio; estado em laranja ("Continuar · Ep. 4", "Novo na biblioteca").
6. **Cards:** o título vai sobre a imagem (com sombra embaixo) e não se repete fora dela; sem imagem, o card mostra o título grande sobre o degradê da cor dominante.
7. **Foco:** anel branco de 4 px, escala 1.08–1.1 e sombra. Nunca só cor.

## O que continua valendo

Piso Chromium 76 e a lista de proibidos (§6), sem `filter: blur()`/`backdrop-filter` (fundos desfocados são pré-renderizados no servidor), animar só `transform`/`opacity`, 10-foot UI (texto ≥ 24 px a 1080p na TV), área segura de 5%, orçamento de 150 KB gzip de JS inicial e sem recursos de CDN.

## Consequências

- Duas famílias de fonte a mais no bundle de CSS (woff2, sub-conjunto latin), sem impacto no orçamento de JS.
- O §7 do AGENTS.md passa a descrever o Telão; o ADR 004 (player próprio) e o 003 (imagens) seguem válidos.
