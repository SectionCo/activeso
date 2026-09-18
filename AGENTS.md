# AGENTS.md

## Application overview

ActiveSo is a Go package that provides an active record-like persistence layer for application-defined Go types, backed by Turso's SQLite-compatible database. It aims to offer ergonomic model creation, querying, updates, and deletion while keeping database access explicit and lightweight.

## Data model and database

ActiveSo is built specifically to be used with Turso, an in-process SQLite-compatible database written in Rust.

## Project skills

Verified, load-bearing app knowledge lives in **`.skills/`** within the directory — read the relevant skill before touching the area it covers, and keep it current when behavior changes:

When you discover a new verified behavior in this repo (a wire format, an SDK limitation, a reactivity mechanism), record it as a new SKILL.md under `.skills/` rather than leaving it in conversation history.

## Code Documentation Rules

Every function must have a short 1-2 line documentation note explaining what the function does.

Inside each function, add concise comments for meaningful logic blocks.

At the top of every function body, include the exact comment:

`// Initialize Variables`

Immediately under that comment, declare the function's local variables/constants used by the function's main flow.
Do not leave the initialization section as a comment-only placeholder.
Only declare variables later when tighter scope is intentional (for example inside a branch/loop for correctness or clarity).

Keep comments practical and current whenever behavior changes.

## Code formatting

Use tabs for indentation in all code. Do not use spaces for indentation. Run `gofmt` on every Go source file after editing it; do not manually override its tab-based indentation.

## Human approval requirement

Any change to any file located in a directory named `types` requires explicit human approval before the change is made. This includes, but is not limited to, files under `types/` and any future nested `types/` directories. Do not edit, create, delete, rename, or mechanically rewrite those files without first obtaining that approval. Read-only inspection is allowed when needed to understand the application.

Any change to any `.sh` shell script requires explicit human approval before the change is made. This includes scripts in the repository root and scripts in nested directories.

Any change to `AGENTS.md` requires explicit human approval before the change is made.
