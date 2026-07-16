# Reading Relay Agent Instructions

This repository contains the shared Reading Relay service and its thin Hermes
skill client.

- Keep Instapaper credentials out of the repository, logs, environment files,
  and skill content.
- Preserve draft-by-default behavior. External delivery requires the explicit
  `send` field or `--send` CLI flag.
- Keep the agent API on the Unix socket; do not expose it through the public
  HTTP listener.
- Generated article HTML must pass through the Markdown renderer and sanitizer.
- Add or update tests before changing behavior.
- Do not install, enable, or restart the systemd service without the user's
  explicit approval.
- Do not install the skill into agent profiles until its reviewed rollout is
  approved.
