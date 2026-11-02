# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users
A família da casa, assistindo na TV da sala com o controle remoto, principalmente à noite, a uns 3 m da tela. O PC (navegador) é secundário: usado para assistir às vezes e para cuidar da biblioteca (Revisar, Configurações).

## Product Purpose
Servidor de streaming próprio e local para filmes, séries e animes. O usuário aponta uma pasta de vídeos bagunçada; o servidor organiza tudo virtualmente (sem tocar nos arquivos) e transmite para qualquer TV ou PC da rede. Sucesso: abrir a TV e chegar ao que se quer assistir em poucos cliques, como num serviço de streaming comercial, sem assinatura e sem depender da internet.

## Positioning
É a biblioteca da própria casa, não um catálogo de terceiros: só mostra o que está na pasta da família, funciona offline, cada pessoa tem seu "continuar assistindo", e tudo vem de dados locais (nome do arquivo, pastas, frames extraídos do vídeo). Nenhum serviço comercial pode copiar isso: o acervo é o da família.

## Operating Context
- Clientes: TVs Samsung (Tizen), LG (webOS), Android TV/Google TV/Fire TV (app leve que abre a interface do servidor) e navegador de PC (Windows, Linux, macOS).
- Navegação por controle remoto: setas, OK, Voltar, Play/Pause; mouse e teclado no PC.
- Perfis locais com PIN opcional ("Quem está assistindo?").
- Uso noturno, sala com pouca luz.

## Capabilities and Constraints
- Piso de compatibilidade: Chromium 76 (TVs de 2021+). Recursos CSS proibidos e regras técnicas em AGENTS.md §6 e §7 (sem `filter: blur()`/`backdrop-filter`, sem CDN, animar só `transform`/`opacity`, 60 fps numa TV de 2021, bundle inicial < 150 KB gzip).
- Imagens: sem pôster oficial na maioria dos casos; frames extraídos do vídeo (WebP 480/960/1920, versão desfocada pré-renderizada, cor dominante) ou pôster/fanart local quando existir. Sem imagem nenhuma: card tipográfico.
- Metadados: nome do arquivo, pastas e ffprobe. TMDB opcional e desligado por padrão, só gêneros e coleções. Sem sinopse, nota ou elenco.
- Telas: Perfis, Início (hero + fileiras), Filmes/Séries/Animes, Coleção, Detalhe de filme, Detalhe de série/anime (temporadas e episódios), Player (pular abertura, próximo episódio), Busca, Revisar, Configurações, Status.
- 10-foot UI: legível a 3 m; um elemento em foco sempre, e óbvio.

## Brand Commitments
- **Nome: Telão** (escolhido pelo usuário em 01/11/2026). Hoje o código usa o placeholder `mediaserver`; a troca do módulo, do binário e dos pacotes é uma tarefa à parte (Backlog). A interface já mostra "Telão".
- O usuário autorizou substituir a direção visual do AGENTS.md §7 (Apple TV+, só escuro, Inter, uma cor de destaque), registrando em ADR. As regras técnicas continuam.
- Nada de logos, ícones, fontes ou assets de serviços comerciais (Apple etc.).
- **Direção pinada pelo usuário (01/11/2026):** o padrão premium dos grandes streamings, executado no nível deles, com imagem grande de tela cheia. Mundos temáticos (cordel, locadora, azulejo) foram rejeitados.
- **Direção escolhida (01/11/2026):** "Telão" ([ADR 013](docs/adr/013-identidade-telao.md)): menu lateral de ícones que abre ao receber foco, título em letra condensada grande, cena de tela cheia à direita, fundo **preto**, destaque **laranja** (#ff4d2e).

## Evidence on Hand
- Biblioteca de teste em `media/` (arquivos fictícios ou de obras livres, fora do Git). Os arquivos de teste não são vídeos reais, então não há frames: demonstrações visuais precisam ser rotuladas como sintéticas.
- Sem depoimentos, números de uso ou imprensa; nada disso deve ser inventado.

## Product Principles
1. O conteúdo da família vem primeiro; a interface some ao redor dele.
2. Funciona do sofá: tudo se resolve com setas, OK e Voltar.
3. Bonito mesmo sem pôster: o visual não pode depender de arte oficial.
4. Leve em TV de 2021: nenhum efeito que derrube os 60 fps.
5. Administração é do mesmo produto, mas não atrapalha quem só quer assistir.

## Accessibility & Inclusion
Contraste mínimo WCAG AA; texto de corpo ≥ 24 px a 1080p na TV; foco nunca indicado só por cor; respeitar `prefers-reduced-motion` e a opção "Reduzir animações".
