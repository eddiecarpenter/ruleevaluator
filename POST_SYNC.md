Improves pipeline resilience with intra-task recovery checkpoints and extracts reusable composite actions to reduce workflow duplication.

## Features
- Adds intra-task recovery checkpoints to the dev session, allowing interrupted tasks to resume from the last committed unit of work rather than restarting from scratch
- Commits completed units of work immediately within a task so progress is preserved on failure
- Extracts `setup-claude-auth` into a reusable local composite action, eliminating duplicated auth configuration across workflows
- Extracts `install-system-deps` into a reusable local composite action with apt caching, reducing setup duplication in pipeline workflows

## Chores
- Removes `build-and-test.yml` from the template distribution directory — it is now maintained as a distribution-only workflow in `.ai/` and no longer copied to `.github/workflows/`

## Downstream Actions
- New composite actions `setup-claude-auth` and `install-system-deps` are added under `.ai/.github/actions/`. Run `gh agentic sync` to pull these into downstream repos.
- The `agentic-pipeline.yml` and `release.yml` workflows have been significantly refactored to use the new composite actions. Run `gh agentic sync` to update downstream workflow files.
