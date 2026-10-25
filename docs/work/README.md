# Non-developer deliverables

This directory holds persistent output from roles that do not write code:

| Folder | Owner | Typical contents |
|---|---|---|
| `manager/` | `manager` | Plans, delegation notes, DoD reviews |
| `product/` | `product` | Requirements, acceptance criteria, business cases |
| `ux/` | `ux-designer` | Interaction specs, layouts, state matrices |
| `database/` | `database` | Schema reviews, index proposals, query analyses |
| `security/` | `security` | Audit reports, threat models, remediation plans |

Rules:

- Markdown only. No code, migrations, configuration, or build files.
- Link to the owning document instead of duplicating it.
- Name files with a stable prefix, for example `2026-10-24-product-credit-limits.md`.
- Developer agents read these files as input; they do not edit them.
- Developers implement the resulting work and update the technical docs.
