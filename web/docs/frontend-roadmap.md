# Frontend rewrite roadmap

## Routing map

- `/` – Landing marketing page. Static data only, uses `landing` feature module.
- `/login` / `/register` – Auth flow (nanostores + Connect `AccountService`). Needs mutation hooks for `Login`, `Register`, `RefreshTokens`.
- `/docs` – Placeholder for documentation listing. Replace with MDX-powered docs after new content pipeline is ready.
- `/app/dashboard` – Panel shell home. Requires aggregated metrics from `RunService.ListTopRuns` and future `StroppyService`.
- `/app/configurator` – Workload presets editor. Connects to `AutomateService` for template CRUD and `Crossplane` preview.
- `/app/runs` – Full run table with filters, pagination, and run actions built on top of `RunService.ListRuns` + `AddRun`.

## Data requirements

| Feature | RPC sources | Stores / cache |
| --- | --- | --- |
| Auth/session | `AccountService.Login`, `RefreshTokens`, `Profile` | `sessionStore`, `authStore`, React Query mutations |
| Dashboard metrics | `RunService.ListTopRuns`, `StroppyService.GetStats` (TBD) | React Query queries keyed by filters |
| Configurator | `AutomateService.ListPresets`, `AutomateService.UpdatePreset` | Derived nanostore for dirty state, Connect mutations |
| Runs table | `RunService.ListRuns`, `RunService.AddRun` | React Query infinite queries, optimistic cache updates |

## Next implementation chunks

1. Create feature folders (`features/auth`, `features/runs`, etc.) with hooks that wrap Connect Query helpers.
2. Replace placeholder landing + panel cards with real shadcn components wired to store/query data.
3. Introduce optimistic navigation for `/app` shell (keep panel layout loaded, swap outlet content only).
4. Add error boundaries and toast system driven by nanostores for cross-cutting notifications.
