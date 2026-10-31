# ADR 005 — Perfis locais, sessão e progresso

- **Status:** aceito
- **Data:** 31/10/2026

## Contexto

A Sprint 4 dá a cada pessoa da casa o seu "continuar assistindo". Perfis são locais e o PIN de 4 dígitos é opcional. Multiusuário com senha forte está fora de escopo (AGENTS.md §12), mas o PIN precisa valer de verdade: sem ele, ninguém lê nem grava o progresso daquele perfil, nem pela API.

## Decisões

1. **Sessão por token assinado**, sem tabela de sessões: `<profileId>.<HMAC-SHA256(segredo, profileId | pin_hash)>`, enviado no header `X-Profile`.
   - O segredo (32 bytes de `crypto/rand`) é criado no primeiro uso e guardado na tabela `app_state` (migração 004). Um backup do banco leva as sessões junto.
   - O hash do PIN entra no dado assinado: trocar ou remover o PIN invalida as sessões antigas. Excluir o perfil também, porque o perfil some.
   - Header em vez de cookie: o mesmo mecanismo funciona no navegador e nos apps de TV (Sprint 7), e não há CSRF a tratar.
2. **PIN com PBKDF2-SHA256** (`crypto/pbkdf2` da stdlib, 100 mil iterações, salt de 16 bytes), comparado em tempo constante. Com só 10 mil PINs possíveis, quem protege de verdade é o **limite de tentativas**: 5 erros bloqueiam o perfil por 30 s (em memória; reiniciar o servidor zera).
3. **Listagens da biblioteca continuam sem perfil** e com ETag. Os dados do perfil têm rotas próprias com `Cache-Control: no-store` (`/api/home`, `/api/progress/{id}`, `/api/series/{id}/progress`). Nada de `Vary` nem cache por perfil.
4. **Regras de progresso no servidor:** assistido a partir de 90%, e "pegajoso" (rever do início não desmarca; só o desmarcar manual limpa). A duração do `ffprobe` vale mais que a enviada pelo cliente. "Continuar assistindo" pede ≥ 30 s e mostra uma série uma vez só.
5. **Próximo episódio** segue a ordem da página da série (temporada, episódio, número absoluto, caminho). Especiais (temporada 0) só encadeiam entre si.

## Consequências

- Não é autenticação forte: quem tem acesso à rede e conhece o PIN entra; quem não conhece faz no máximo 10 tentativas por minuto.
- A sessão fica salva no aparelho (`localStorage`) até trocar de perfil, trocar o PIN ou excluir o perfil.
- `GET /api/home` faz duas consultas por série com atividade (no máximo 40): ~17 ms com 10 mil filmes, 200 séries e 500 linhas de progresso (`BenchmarkHome`).
