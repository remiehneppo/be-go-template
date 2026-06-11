# Domain Docs

Engineering skills should consume this repo's domain documentation before making architectural or implementation changes.

## Layout

This repo uses a multi-context layout.

- Read `CONTEXT-MAP.md` at the repo root first.
- Follow the map to the context-specific `CONTEXT.md` file relevant to the task.
- Read ADRs in `docs/adr/` for system-wide decisions.
- If a context has its own `docs/adr/`, read ADRs relevant to that context.

If a referenced context or ADR does not exist yet, proceed silently. Producer skills can create them later when terms and decisions become stable.

## Vocabulary

Use domain terms as defined in the relevant `CONTEXT.md`. If a needed concept is missing, note it as a documentation gap instead of inventing a parallel vocabulary.

## ADR conflicts

If an implementation or proposal contradicts an existing ADR, surface the conflict explicitly before proceeding.
