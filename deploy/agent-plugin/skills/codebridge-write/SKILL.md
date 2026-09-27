---
name: codebridge-write
description: Safely modify files in a CodeBridge workspace using write, edit, apply_patch, and rollback_patch. Use when the user asks to create, edit, replace, delete, patch, or roll back local project files. Prefer preview before confirm, verify writable workspace capability first, and preserve the checkpoint id for rollback.
---

# Modifying local files safely

CodeBridge write operations are local, workspace-scoped, and explicitly
opted-in by the Client.

## Preflight

Before changing anything:

1. call `list_devices`;
2. select the target online device;
3. verify the target workspace is advertised with `writable: true`;
4. inspect `tool_policy` and confirm the needed write tool is enabled;
5. read the current file(s) before constructing an edit unless the operation
   intentionally creates a new file.

If a workspace is not writable, do not try to bypass the local policy with
`bash`.

## Pick the right primitive

- `edit` — replace exactly one known text occurrence in an existing file.
- `write` — create a new text file. The current Client rejects `write` when
  the target already exists; use `edit` or `apply_patch` for existing files.
- `apply_patch` — atomically modify several files, or combine
  create/replace/delete edits in one operation. For a full-file replacement,
  use the current complete file text as `old_text` and the desired complete
  content as `new_text`.
- `rollback_patch` — restore files from a checkpoint returned by a successful
  write/edit/apply_patch.

Prefer `edit` for small precise changes. Prefer `apply_patch` when several
files form one logical change.

## Preview then confirm

Use the two-step flow for every non-trivial modification:

1. call the write tool with `preview: true`;
2. inspect the returned diff and affected file list;
3. if it matches the intended change, repeat with `confirm: true`.

Do not claim a preview changed files. If the preview differs from the user's
intent, do not confirm it; rebuild the edit instead.

A successful confirmed write returns a `checkpoint_id`. Preserve it until
the change has been validated.

## Exact-match semantics

For `edit` and replacement entries in `apply_patch`:

- `old_text` must match the current content exactly;
- it must identify a unique occurrence;
- if the file changed since you read it, read it again and rebuild the edit;
- never make the match artificially broad just to force an update through.

For deletion through `apply_patch`, use the current full file content as the
expected `old_text` so deletion remains concurrency-safe.

## Validation

After applying a change:

1. re-read the changed area or inspect the diff;
2. run focused tests/build checks when relevant;
3. if validation fails and the safest action is to undo the whole change, call
   `rollback_patch` with the returned checkpoint.

Do not use rollback as a substitute for understanding a partially correct
change. If only part of the change is wrong, prepare a new precise patch.

## Safety boundaries

- Sensitive paths and binary content can be rejected by the Client. Respect
  that policy.
- Never use `bash` to write around a rejected CodeBridge write operation.
- Do not expose physical host paths.
- Keep edits scoped to the user's requested task.
- Do not modify unrelated dirty worktree files.
- When the working tree already has changes, inspect the relevant diff before
  editing and preserve existing work.

## Multi-file implementation pattern

For a normal coding task:

1. discover device/workspace and writable capability;
2. read the relevant files;
3. inspect existing local changes when necessary;
4. preview one coherent `apply_patch`;
5. confirm it;
6. capture `checkpoint_id`;
7. run focused validation;
8. roll back only if the applied change should be reverted as a unit.
