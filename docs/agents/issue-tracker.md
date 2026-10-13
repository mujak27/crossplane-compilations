# Issue tracker: GitHub

Issues and specs live in GitHub Issues for `mujak27/crossplane-compilations`.
Use the `gh` CLI; it infers the repository from the Git remote.

## Conventions

- Create: `gh issue create --title "..." --body-file <file>`.
- Read: `gh issue view <number> --comments`; fetch labels with `--json labels`.
- List: `gh issue list --state open --json number,title,body,labels,comments`.
  Add label and state filters as needed.
- Comment: `gh issue comment <number> --body "..."`.
- Label: `gh issue edit <number> --add-label "..."` or `--remove-label "..."`.
- Close: `gh issue close <number> --comment "..."`.

When a skill says "publish to the issue tracker", create a GitHub issue.
When it says "fetch the relevant ticket", read the issue and its comments.

## Pull requests as a triage surface

**PRs as a request surface: no.**

GitHub issues and PRs share a number space. For an ambiguous reference,
try `gh pr view <number>`, then `gh issue view <number>`.

## Wayfinding operations

- Map: one issue labelled `wayfinder:map`, containing Notes,
  Decisions-so-far, and Fog.
- Child ticket: link as a GitHub sub-issue. If unavailable, use a task list
  in the map and put `Part of #<map>` in the child's body.
  Labels: `wayfinder:research`, `wayfinder:prototype`,
  `wayfinder:grilling`, or `wayfinder:task`.
- Blocking: use native issue dependencies:
  `gh api --method POST repos/mujak27/crossplane-compilations/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`.
  Get the database ID with
  `gh api repos/mujak27/crossplane-compilations/issues/<blocker> --jq .id`.
  If dependencies are unavailable, put `Blocked by: #<n>` at the top
  of the child body. All blockers must be closed before work starts.
- Frontier: choose the first open child in map order with no open
  blockers and no assignee. Native dependency status is
  `issue_dependencies_summary.blocked_by`; otherwise check the body references.
- Claim: `gh issue edit <number> --add-assignee @me`, before other writes.
- Resolve: comment with the answer, close the child, and append a gist
  and link to the map's Decisions-so-far.
