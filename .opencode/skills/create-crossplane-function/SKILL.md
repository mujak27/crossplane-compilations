---
name: create-crossplane-function
description: Design a new Crossplane function, approve its plan, and publish implementation tickets; dispatch child agents only on request.
disable-model-invocation: true
---

# Create Crossplane Function

Coordinate the existing skills rather than repeating their workflows. Default to
design and tickets, not implementation. Resume from the last approved checkpoint;
check existing plans, issues, and worktrees before creating duplicates.

## 1. Establish context and workspace

- Read the repository and ancestor `AGENTS.md` files, `docs/agents/domain.md`,
  the glossary and relevant ADRs when present, `docs/agents/issue-tracker.md`,
  `docs/agents/triage-labels.md`, and the root README's naming conventions.
  Repository paths here are relative to the Git root, not this skill directory.
- Establish the proposed function's purpose and a short filesystem-safe slug.
  Inspect the existing examples and `function-template/` without modifying them.
- Resolve the original checkout with Git worktree metadata. Use its sibling
  `<repository>-worktrees/<function-slug>` directory and a task branch. For this
  repository that container is `crossplane-compilations-worktrees/`, not a nested
  directory and not the singular `-worktree` suffix.
- Before creating a worktree, fetch `origin main`; stop on failure. Create the
  new branch from updated `origin/main` and move the session into the worktree
  before any file edits, including glossary or ADR updates. Use an explicit
  `git worktree add` destination if the worktree tool cannot honor this location.
- Reuse an existing worktree only for continuation of this same function task,
  after inspecting its branch and changes. Preserve unfamiliar changes. If the
  starting checkout has another task's uncommitted work or an open PR, offer
  `session.moveToWorktree` and wait for approval unless the user already explicitly
  requested a worktree. Configure the destination to honor the sibling convention.

Done when the function's context is understood and the session has a safe task
workspace. The primary checkout and other sessions' worktrees remain untouched.

## 2. Grill with docs

Load `grill-with-docs`, which composes `grilling` and `domain-modeling`. If the
router is unavailable to the skill tool, read its installed `SKILL.md` and load
those two skills directly. Use their frontier rounds, recommended answers, and
approval rules; investigate facts rather than asking the user to look them up.

Cover the function's behavior, typed XR contract, API identity and project
directory, composed resources, namespace and naming rules, desired-state
preservation, errors, readiness semantics, test seams, and explicit non-goals.
Adapt questions to the actual function rather than asking an invariant checklist.
Record resolved domain terms and qualifying ADRs using domain-modeling.

Apply the repository's Crossplane v2 namespaced guardrails throughout. Distinguish
namespaced resource instances from inherently cluster-scoped infrastructure
definitions. Use the template's typed Go → CRD → XRD generation pipeline rather
than hand-editing generated XRDs.

Done only when the design frontier is empty and the user confirms shared
understanding. Approval of one round does not approve unanswered branches.

## 3. Save and approve the plan

Save the implementation plan in OpenChamber Project knowledge, with a title,
coarse step checklist, approved decisions, acceptance checks, template dependency,
and non-goals. Show it for review and wait for plan approval. Keep temporary
research or handoff files in the ancestor instructions' ephemeral-doc location.
Do not scaffold the function at this stage.

The template is ready only when its implementation is merged into `main` and
its generation, tests, drift checks, and local render checks pass. Inspect current
template issues and merged changes; ticket closure alone is insufficient. Never
copy another session's uncommitted template. If unavailable, planning and ticket
publication may continue, but mark scaffolding blocked by template readiness and
report the dependency. Record missing or failed checks honestly.

Done when the saved plan has explicit user approval.

## 4. Approve and publish tickets

Load `to-tickets` (or read its installed `SKILL.md` if unavailable to the tool).
Draft verifiable vertical slices with acceptance criteria and genuine blocking
edges. Make the first implementation slice scaffold from the approved template
and deliver a minimal working path for this function. Carry every plan constraint
into the relevant tickets, including template-readiness blockers.

Present the numbered breakdown and wait for separate breakdown approval before
publishing. Publish to this repository's GitHub Issues with the configured
`ready-for-agent` label and native blocking relationships, using the documented
fallback when native relationships are unavailable. Blocked tickets remain
ineligible for dispatch regardless of their label. Preserve existing parent
issues and link tickets to the approved plan or a durable decision summary.

As soon as an issue or change becomes part of this work, link it to the session
with OpenChamber `session.link`; do the same for issues or PRs created or resolved.

Done when approved tickets exist, their dependencies are recorded, and the user
has their links. **Stop here unless the user explicitly requests dispatch.**
Ticket or plan approval alone does not authorize implementation agents.

## 5. Dispatch only on request

Use native child subagents attached to this session, not independent top-level
sessions. Dispatch only tickets whose blockers are complete and verified,
including template readiness. Claim tickets per the tracker conventions.

- Sequential implementation uses the function's worktree with one writer at a
  time. Parallel file-changing tickets each use an isolated ticket worktree in
  the same sibling container. Parallelize only genuinely independent work.
- New ticket branches also start from freshly fetched `origin/main`. Before
  dispatch, integrate the required verified prerequisite commits into the target
  branch; an issue marked closed does not put its code in a worktree. Stop and
  report conflicts instead of overwriting changes.
- Give each child the issue body/comments, relevant approved decisions, exact
  worktree and branch, prerequisite commits, scope, and acceptance checks. Require
  it to work only there and return its commits, actual validation results, and
  unresolved blockers. Use `tdd` at agreed test seams where appropriate.
- Require `code-review` before completion, resolve actionable findings, and keep
  tested implementation commits local. Pushes, PR creation, merges, package
  publication, and cluster deployment require an explicit user request.
- Verify returned work against acceptance criteria before resolving its ticket
  and unblocking dependents. Integrate isolated ticket commits serially into the
  function branch and rerun relevant combined checks. Tick approved plan steps as
  completed without rewriting their constraints.

Finish with the function branch/worktree, completed and remaining issues,
validation results, and blockers. Leave an unfinished ticket open with a clear
status; do not claim checks ran when tools were unavailable.
