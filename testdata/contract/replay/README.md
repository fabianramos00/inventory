# Replay recordings

Recordings are generated, not checked in. Create one with
`CONTRACT_RECORD_DIR=<dir>` during a disposable legacy run, then compare the Go
server against it with `CONTRACT_REPLAY_FILE=<dir>/replay.json`.

Token values are redacted by the harness. Still treat recordings as test data:
do not commit credentials, real user data, or production responses.
