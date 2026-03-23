# Project Manager skill prototype

Reviews your JJ stack and ties work to problems using machine-readable trailers.

## Trailer format

Use commit-description trailers:

```text
PM-Problem: <problem_id_or_unique_prefix>
PM-Relation: addresses
PM-Progress: short progress note
```

## Commands

```bash
assistant project_manager review --pretty
assistant project_manager review --pretty --apply
assistant project_manager review --pretty --create-problem
assistant project_manager trailer --problem-id <problem_id> --relation addresses --progress "implemented picker"
```

## Notes

- `review` inspects `jj diff -r "trunk()..@" --summary` and stack commit descriptions.
- If one bound problem is found and `--apply` is set, a `progress_update` event is logged to that problem.
- If no binding is found, it suggests a problem statement and trailer block.
