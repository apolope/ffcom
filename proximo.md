# Prompt para retomar o TODO do FFCom

Cole este prompt sempre que quiser voltar a processar o backlog do projeto.

---

Estamos retomando o projeto FFCom (`D:\Dev\ffcom`), alternativa self-hosted ao Discord. Antes de qualquer coisa, leia `docs/architecture.md` (decisões técnicas já tomadas e por quê) e `TODO.md` (backlog por tema) direto do repositório, não confie em memória de conversas antigas.

Faça o seguinte:

1. Abra `TODO.md` e identifique os itens ainda não marcados (`- [ ]`).
2. Se a seção "Decisões de design pendentes" ainda tiver itens em aberto que bloqueiam o que eu pedir, avise antes de começar a codar e proponha a decisão em vez de assumir em silêncio.
3. Se eu não indicar qual item trabalhar, priorize nesta ordem: decisões de design pendentes → infraestrutura/scaffolding básico do componente envolvido → o resto do tema.
4. Ao concluir um item, marque como feito (`- [x]`) em `TODO.md` e, se a decisão tomada durante o trabalho não estiver documentada, registre em `docs/architecture.md` no mesmo formato usado (decisão + alternativas + razão).
5. Se algo relevante para conversas futuras não for óbvio pelo código nem pelos docs do repo (motivação, prazo, decisão de negócio), grave na memória do Claude para este projeto.

Item(s) que eu quero atacar agora: [descreva aqui, ou deixe em branco para você sugerir o próximo mais lógico do TODO]
