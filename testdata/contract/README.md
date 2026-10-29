# Contract parity fixtures

Fixtures for `tests/contract` (`go test -tags=contract ./tests/contract`).
`docs/TESTING.md` is the authoritative guide; this file is a quick reference.

- Files are grouped by legacy module and loaded in the order listed in
  `tests/contract/parity_test.go`.
- `PARITY-` names are stable natural keys. `on_conflict` reuses an existing row
  when a create returns a listed status.
- `{{variable}}` placeholders are captured per side, so generated IDs never
  leak into comparisons.
- `compare: "shape"` ignores scalar values for global aggregates;
  `strict_compare: "full"` is used when `CONTRACT_STRICT=1`.
- A failing run writes `report.md` here. Replay recordings go under `replay/`;
  tokens are redacted before writing.
- Live runs require `CONTRACT_ALLOW_WRITES=1` and a disposable legacy
  instance. Never run against production or the user's existing database.
