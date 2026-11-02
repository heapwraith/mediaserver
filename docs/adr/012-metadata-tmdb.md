# ADR 012 — Gêneros e coleções do TMDB

- **Status:** aceito
- **Data:** 01/11/2026

## Contexto

O usuário quer categorizar a biblioteca por gênero e agrupar franquias. Na biblioteca real (67 vídeos) nenhum arquivo traz gênero: não há `.nfo`, nenhum vídeo tem tag de gênero e os `.txt` dos releases são propaganda. Sem fonte local, o dado só pode vir de fora, o que muda a regra "sem APIs externas" do AGENTS.md §1. O usuário pediu essa mudança.

## Decisões

1. **Opcional e desligado por padrão.** Sem o usuário ligar e salvar a chave dele, o servidor não faz nenhuma chamada de rede (coberto por teste). Com o TMDB desligado, gêneros e coleções não aparecem.
2. **Só gêneros e coleções.** Sinopse, nota, elenco e imagens ficam de fora: o resto do projeto segue funcionando só com dados locais.
3. **A chave fica na tela de Configurações**, na tabela `app_state`, e não em flag: dá para trocar sem reinstalar o serviço.
   - A API nunca devolve a chave, e os erros não a carregam (o `*url.Error` do Go traria a URL; por isso uma chave v3, que só pode ir na URL, é removida da mensagem).
   - **Limite conhecido:** ela entra no backup do banco, como o segredo das sessões. Está no item de segurança do Backlog.
   - **Limite conhecido:** qualquer perfil da rede pode trocar a chave ou desligar o TMDB (não pode lê-la), como as outras telas de administração. Também está no Backlog.
4. **Identificação, da mais certa para a menos certa:**
   1. tag `TMDB` que o grupo do release gravou no arquivo (`tv/136315`), lida do `probe_json` que o scanner já guarda, sem abrir o arquivo;
   2. tag `IMDB` (`tt14452776`), pelo `/find` do TMDB;
   3. busca por título, aceita **só com um único resultado** de título igual (normalizado, o traduzido ou o original) e ano compatível (±1).
5. **Sem certeza, vai para Revisar** com até 5 opções, um campo para o código do TMDB e "Não é nenhum destes". Um gênero errado é pior que nenhum.
6. **Quando roda:** em segundo plano depois de cada scan, um título por vez, com 250 ms entre eles.
   - Cada título é consultado uma vez; só volta a ser consultado se o título ou o ano mudarem (`lookup_key`).
   - Sem rede ou com o TMDB fora do ar, nada é gravado, e o próximo scan tenta os mesmos títulos. Uma chave recusada para a rodada e aparece em Configurações.
7. **As tabelas não têm chave estrangeira para `movies` e `series`.** O `materialize` apaga e recria esses registros (com ids estáveis), e um `ON DELETE CASCADE` apagaria tudo a cada scan. O que é de títulos que saíram da biblioteca é limpo depois de cada rodada, como faz o `image_cache`.
8. **Coleção só aparece com 2 ou mais filmes na biblioteca.** Uma coleção de um filme só não ajuda ninguém.
9. **Gêneros que só as séries têm viram os gêneros dos filmes.** O TMDB os devolve em inglês mesmo em pt-BR, e juntos: "Sci-Fi & Fantasy" vira Ficção científica + Fantasia, e "Action & Adventure" vira Ação + Aventura. Assim uma fileira traz filmes e séries juntos. Só apareceu com dados reais.
10. **Aviso do TMDB:** Configurações mostra "Este produto usa a API do TMDB, mas não é endossado nem certificado pelo TMDB", exigido pelos termos.

## Consequências

- Nenhuma dependência nova: só `net/http`.
- O idioma é sempre pt-BR, a língua da interface.
- A exceção à regra de rede é uma só, e fica em `internal/metadata`.
