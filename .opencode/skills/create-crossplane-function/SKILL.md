---
name: create-crossplane-function
description: Crossplane design, template scaffolding, and validation requirements to use alongside plan-to-tickets.
disable-model-invocation: true
---

# Create Crossplane Function

Use this skill as the Crossplane domain supplement to `plan-to-tickets`, not a
second orchestrator. Apply its requirements at the corresponding coordinator
stages and carry them into the approved plan, tickets, and worker context.

## Coordinator handoff

Ask the user to invoke `@plan-to-tickets` alongside this skill if it is not
already active for the task. Both skills are user-invoked; reading an installed
skill file is inspection, not permission to execute it. If the coordinator is
missing or cannot be invoked, report the blocker and wait rather than substituting
an inline workflow. It is supplied by `mujak27/skills`.

`plan-to-tickets` owns dependency preflight, companion-skill invocation, session
placement and anchoring, delegation boundaries, approval gates, ticket publication,
implementation authorization, TDD, review, and result tracking. Follow its
workspace policy subject to repository and harness instructions. This supplement
adds no session moves or dispatch permissions. In particular, a function's ticket
worktrees are not new parent locations or reasons for top-level sessions.

Proceed when the user has invoked the coordinator and its preflight and initial
placement requirements are satisfied. Resume its existing checkpoint rather than
starting a duplicate design, plan, or ticket set.

## Context and design requirements

At context discovery, read repository and ancestor `AGENTS.md`,
`docs/agents/domain.md`, the glossary and relevant ADRs when present,
`docs/agents/issue-tracker.md`, `docs/agents/triage-labels.md`, and the root README's
naming conventions. These paths are relative to the Git root. Inspect the existing
examples and `function-template/` without modifying them; establish the function's
purpose and a short filesystem-safe slug.

During the coordinator's design stage, resolve the function's behavior, typed XR
contract, API identity and project directory, composed resources, namespace and
naming rules, desired-state preservation, errors, readiness semantics, test seams,
and explicit non-goals. Adapt questions to the actual function. Distinguish
provisioning readiness from application health as defined in the glossary.

Apply the repository's Crossplane v2 namespaced guardrails throughout. Keep
composed resources in the XR namespace and distinguish namespaced instances from
inherently cluster-scoped infrastructure definitions. Use the template's typed
Go → CRD → XRD generation pipeline rather than hand-editing generated XRDs.

Design coverage is complete when these decisions are resolved and recorded under
the coordinator's design approval gate.

## Plan and ticket requirements

Include the approved Crossplane decisions, template dependency, acceptance checks,
and non-goals in the coordinator's plan and relevant tickets. Make the first
implementation slice scaffold from the approved template and deliver a minimal
working path for the function.

The template is ready only when its implementation is merged into `main` and
generation, tests, drift checks, and local render checks pass. Verify implementation
commits and actual checks, not ticket closure alone. Use the freshly fetched task
baseline, preserving the repository's worktree rules; never copy another session's
uncommitted template. If unavailable, planning and ticket publication may continue,
but mark scaffolding blocked by template readiness and report the evidence.

For template-readiness checks and scaffolding acceptance criteria, read
[Template integration](references/template-integration.md). Carry its generated-
artifact commit ordering and namespace checks into the scaffolding ticket.

These requirements are accounted for when every relevant plan constraint has an
owning ticket and unavailable prerequisites have explicit blocking dependencies.
Use the coordinator's approval and publication gates, not an additional set here.

## Authorized implementation requirements

When implementation is explicitly requested through the coordinator, give each
assigned worker the Crossplane decisions and template-integration requirements
with its ticket context. Keep the template and existing numbered examples
unchanged unless the approved scope includes them.

Verify actual generation, tests, drift, render, API identity, namespace isolation,
and chosen readiness behavior before accepting the function. Package builds,
publishing, and cluster deployment are separate from local render validation and
remain subject to explicit authorization. Return validation evidence and blockers
to the coordinator; it owns review, integration, ticket completion, and reporting.
