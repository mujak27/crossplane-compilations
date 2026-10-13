# Domain Docs

## Before exploring

This repo uses a single-context layout:

- Read root `GLOSSARY.md`.
- Read relevant decisions in `docs/adr/`.

If these files are absent, proceed silently. Domain modeling creates
them lazily when terms or decisions are resolved.

## Use the glossary's vocabulary

Use defined domain terms in issue titles, proposals, hypotheses, and
test names. Avoid synonyms the glossary explicitly rejects.

If a concept is missing, reconsider whether it belongs to the domain;
note genuine gaps for domain modeling.

## Flag ADR conflicts

Explicitly surface proposals that contradict an existing ADR, naming
the decision and explaining why it may be worth reopening.
