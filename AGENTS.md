# Repository working rules

- Unless the user explicitly requests an exception, start each task from the latest `main` on a separate `feature/*`, `fix/*`, `chore/*`, or `release/*` branch. Do not develop, commit, or push directly on `main`.
- Push task branches and prepare pull requests with the problem, resulting behavior, validation, and deployment impact. Do not merge or publish to production without the required review and authorization.
- `main` is the production baseline watched by Argo CD. A change to production image references can trigger deployment; handle it through a release/fix PR after verifying that the image exists.
- Keep credentials, `基础设施*.md`, `.env`, environment-specific files, data backups, and build context archives out of Git.
- Read `CONTRIBUTING.md` for the development and release process. Follow the scoped instructions in `frontend/AGENTS.md` when editing frontend code.
