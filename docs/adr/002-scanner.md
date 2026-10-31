# ADR 002 — Scanner e biblioteca virtual

- **Status:** aceito
- **Data:** 31/10/2026

## Contexto

A pasta de mídias pode estar organizada ou não, e o servidor nunca pode alterar os arquivos (AGENTS.md §4). A biblioteca precisa ser estável (IDs não mudam, §5.6), o rescan sem mudanças precisa levar < 3 s com 10 mil arquivos, e as correções manuais precisam sobreviver a rescans e renomeações.

## Decisões

1. **Duas etapas: arquivos → biblioteca.** O scan só mantém `media_files` em dia (caminho, tamanho, mtime, hash parcial, ffprobe). A **materialização** recalcula filmes/séries/temporadas/episódios/extras a partir de todos os arquivos + `overrides`, numa transação, reaproveitando os IDs existentes. Ela só roda quando algo mudou (arquivos, legendas/artes, correções ou versão das regras).
2. **Parser sem estado.** O parse do nome é refeito na materialização (barato: microssegundos por arquivo), então melhorar o parser não exige migração: basta subir `materializeVersion`.
3. **Incremental por caminho + tamanho + mtime.** Arquivos iguais nem são abertos. Mediu-se 77 ms para 10 mil arquivos sem mudança.
4. **Arquivo movido = mesmo tamanho + hash parcial** (SHA-256 dos primeiros e últimos 64 KB). O registro antigo é reaproveitado, preservando ID, progresso e correções.
5. **Correções como overrides** (chave caminho + tamanho, com hash parcial como fallback). Merge e split de séries também viram overrides de título, então sobrevivem ao rescan.
6. **Na dúvida, Revisar.** Sinais conflitantes ou ausentes nunca viram uma classificação silenciosa: o arquivo vai para a fila com o palpite e o motivo.
7. **Cópia em andamento:** arquivos modificados há menos de 2 s ficam para o próximo scan; o watcher espera a pasta ficar quieta e o tamanho parar de mudar.

## Dependências novas

- `github.com/fsnotify/fsnotify`: watcher multiplataforma (inotify/kqueue/ReadDirectoryChangesW); a stdlib não tem.
- `golang.org/x/text/unicode/norm`: decomposição Unicode para tirar acentos do título normalizado sem estragar japonês (dakuten).

## Consequências

- Cada correção ou scan com mudanças reescreve as tabelas da biblioteca inteira (rápido até dezenas de milhares de arquivos; revisar se a biblioteca crescer muito além disso).
- Dois arquivos do mesmo episódio: a versão maior (v2) vence; empates vão para Revisar.
- Arquivos com "sample" no nome são ignorados mesmo quando a palavra faz parte do título (limitação aceita pela regra do §4).
