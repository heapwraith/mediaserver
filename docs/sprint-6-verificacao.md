# Verificação da Sprint 6

Executada em 31/10/2026 no Windows. Implementação e verificações automáticas/Edge concluídas, sem commit. A sprint permanece ativa para o aceite manual reservado ao usuário no plano.

## Qualidade e testes

`make fmt`, `make check`, `make test` e `make bench` passaram com FFmpeg/ffprobe 9.0.2 no PATH, depois de aprovar a execução fora do sandbox. O lint Go apontou o fechamento de uma consulta sem `defer`; corrigido antes da execução final, que terminou com **0 issues**.

- Go: suíte completa aprovada, incluindo extração real de SRT/ASS/fontes e burn-in libass. Sem `-race`, pois não há gcc acessível; o próprio Makefile usa essa alternativa.
- Frontend: **91 testes**, formatação, lint e svelte-check sem erros/avisos.
- API: preferência na criação, atualização, listagem e sessão; manutenção com campo omitido/null; desativação; rejeição de idioma inválido; conteúdo SRT UTF-8/Latin-1 e ASS; offset e respostas de erro.
- Parser: NOTE/STYLE/REGION com timestamps não viram cues. A API rejeita offsets negativos, não finitos e fora de `time.Duration`.
- Cache: pedidos concorrentes compartilham a preparação; uma nova versão do arquivo preserva o cache anterior, que pode ter leitores ativos.
- JASSUB **2.5.23** instalado e lockfile atualizado. `npm run check` sincroniza os tipos SvelteKit antes do lint.
- Build: **55,6 KB gzip de JS inicial**, abaixo do limite de 150 KB. Inclui WASM normal (2.110.107 bytes), SIMD (2.189.919 bytes), fonte padrão (145.972 bytes) e workers, carregados ao selecionar ASS.
- Binário Go recompilado e assets de runtime servidos com HTTP 200, bytes idênticos ao build e MIME correto para WASM/JavaScript.
- `go vet ./...` e `git diff --check` aprovados.

## E2E no Edge headless

Servidor na porta 8096, MEDIA_DIR/DATA_DIR próprios em `.dev/sprint6-media` e `.dev/sprint6-data`, perfil Edge isolado e sem alterar o banco de uso normal. Origem entrou por hardlink; os arquivos originais não foram alterados. Os caps dos testes HLS omitiram os contêineres para garantir o caminho de remux/conversão.

- [x] **Origem, legenda embutida pt:** 684 cues no `<track>`, incluindo “Sua mãe precisa de você.”. Com áudio português e preferência `pt-BR`, iniciou desligada corretamente, pois não havia faixa forçada; a faixa completa foi selecionada no menu.
- [x] **Seek a 90% em Origem:** após seek e troca de áudio para inglês, a sessão retornou `start=2744.742`; o `<track>` foi recriado com `?offset=2744.742` e carregou 45 cues restantes.
- [x] **SRT externa UTF-8/Latin-1:** arquivos gerados `.en.srt` e `.es.srt`; os cues exibiram respectivamente “ação, você UTF-8” e “ação, você Latin-1”.
- [x] **Offset com cue ativo:** arquivo gerado de 120 s, sessão iniciada em 108 s. O cue original de 105–120 s foi cortado para 0–12 s e ficou ativo com “ação, você final”.
- [x] **ASS com `\k`, `\pos` e fonte real anexada:** canvas JASSUB visível e texto no topo na posição esperada. A fonte Arial foi extraída como `4.ttf`, sem usar o nome `../../Arial.ttf` armazenado no MKV. A fonte e o ASS responderam HTTP 200.
- [x] **Comparação JASSUB/libass:** screenshot a 2,5 s em 640×360 comparado com frame do próprio FFmpeg usando o ASS e a fonte extraídos. SSIM global **0,999174**. O texto e o posicionamento foram conferidos visualmente. Artefatos: [JASSUB](sprint-6/jassub.png) e [libass](sprint-6/libass.png).
- [x] **Sem suporte ASS:** OffscreenCanvas indisponível no cliente de teste; os caps não continham `ass`. Ao selecionar ASS, resposta `burned: true`, vídeo H.264 tocando (`readyState=4`) e nenhum canvas. Desligar a legenda iniciou uma sessão sem burn-in.
- [x] **Perfil com `pt-BR`:** no arquivo gerado com áudio japonês, a legenda pt iniciou ligada antes de qualquer interação.
- [x] **Teclado:** setas alteraram apenas o destaque do menu; Enter aplicou ASS e depois retornou para texto, removendo o canvas.

Os bloqueios iniciais do sandbox foram resolvidos com aprovação para executar as ferramentas e o Edge fora dele. A primeira tentativa de vídeo no Edge ficou em aba de fundo; trazer a aba de teste à frente permitiu o carregamento e a reprodução.

## Benchmarks

Última execução, Ryzen 7 5700X/Windows:

| Benchmark | Resultado |
|---|---:|
| Rescan sem mudanças | 187,0 ms/scan |
| Range | 1.867,57 MB/s |
| Listagem por título | 1,29 ms/op |
| Listagem por adição | 0,67 ms/op |
| Listagem por ano | 10,37 ms/op |
| Home | 18,64 ms/op |

A medição anterior desta continuação registrou 231 ms/scan e 1.876 MB/s em Range. São medições locais sem controle suficiente para afirmar ganho ou regressão.

## Aceite manual restante

- [ ] Anime real com ASS pesado comparado no mpv.
- [ ] Firefox e Safari.

Esses testes foram reservados ao usuário no plano aprovado. Não foram declarados concluídos nem substituídos pelo teste sintético. A sprint ativa continua sendo a 6 até esse aceite.
