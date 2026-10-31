# CLAUDE.md

@AGENTS.md
@SPRINTS.md

---

## Instruções específicas do Claude Code

As regras do projeto estão no `AGENTS.md` (importado acima). Esta seção só acrescenta o modo de trabalho do Claude Code.

### Antes de codar
- Identifique a **sprint ativa** e a próxima tarefa não marcada no `SPRINTS.md`.
- Pra tarefas com mais de 3 arquivos ou que tocam `stream/` ou `scanner/`, use o **Plan Mode** e apresente o plano antes de editar.
- Na dúvida sobre requisito, pergunte. Não invente comportamento que não está no `AGENTS.md` ou no `SPRINTS.md`.

### Durante
- Rode `make fmt`, `make check` e `make test` antes de considerar a tarefa pronta.
- Mexeu em caminho crítico? Rode `make bench` e informe o resultado comparado ao anterior.
- Não instale dependência nova sem justificar em uma linha (o que faz, por que a stdlib não resolve, tamanho).
- Não crie arquivos fora da estrutura definida no `AGENTS.md` §3.
- Não faça chamadas de rede externas no código (a única exceção é baixar dependências na build).

### Ao terminar uma tarefa
1. Marque `[x]` no `SPRINTS.md`.
2. Resuma em até 5 linhas: o que mudou, como testar e pendências.
3. Sugira a mensagem de commit (Conventional Commits, em inglês).
4. **Não faça commit nem push** sem pedido explícito.

### Estilo das respostas
- Direto e curto. Sem repetir o que já está no código.
- Comandos prontos pra copiar quando houver algo pra rodar.
- Português do Brasil nas respostas; inglês no código e nos commits.