---
name: send-to-reader
description: Use when the user asks an agent to draft, save, or send an existing URL or generated Markdown article through the shared Reading Relay to Instapaper and Kobo. Enforces explicit authorization before external delivery.
version: 1.2.0
author: Hermes Agent
license: MIT
platforms: [linux]
metadata:
  hermes:
    tags: [reading-relay, readerctl, instapaper, kobo, publishing, drafting]
    category: workflow
    related_skills: []
---

# Send to Reader Relay

Use this skill to hand off content to the Reading Relay through the
`readerctl` CLI. This skill is intentionally thin:

- no API keys
- no embedded credentials
- no browser sign-in steps
- no direct network calls from the agent

The agent prepares content and invokes `readerctl`; the relay handles the
Instapaper write. Kobo delivery occurs through Kobo's Instapaper sync; this
skill does not call a Kobo API directly.

## Trigger phrases

Use this skill when the user asks to:

- save, draft, or queue an article for reading later
- send a URL to the reader relay
- turn a URL into a readable item
- publish generated Markdown into the relay
- push to Kobo or Instapaper

## Policy gate for external writes

**Important:** sending to Instapaper is an external write.

- Only use `--send` when **the user explicitly asks** to send, save, push,
  or otherwise deliver to **Kobo/Instapaper**.
- If the request is only to prepare a draft, queue, or stage content,
  use the non-sending form and keep the item in draft mode.
- If the content may be sensitive, personal, private, regulated, or
  otherwise high-risk, ask for explicit confirmation before sending.

## Required input before calling readerctl

Before publishing or saving, ensure the draft includes:

1. **Agent identity** via `--agent`
2. **Title** via `--title`
3. **Source citations** embedded in the Markdown body when using
   `publish`
4. **No secrets**: never include tokens, passwords, private keys,
   session cookies, or confidential credentials
5. **Safe content**: do not send sensitive content without explicit
   confirmation

If the user provides only a URL, save it with a clear title. If the user
provides generated Markdown, keep citations inline in the Markdown body.

## Workflow

### A. Save an existing URL

Use this when the source is already a URL and you do not need to rewrite
it into Markdown.

Draft only:

```bash
readerctl save-url --url "https://example.com/article" --title "Article Title" --agent "agent-name"
```

Send only when the user explicitly asks to deliver it to the reader service
or downstream platforms:

```bash
readerctl save-url --url "https://example.com/article" --title "Article Title" --agent "agent-name" --send
```

If a short description helps the relay, include it only if the command
supports it in the current environment; otherwise omit it and rely on the
title plus URL.

### B. Publish generated Markdown

Use this when the agent has generated a reading-ready Markdown article.
The Markdown should include source citations as descriptive inline links or in a
simple Sources section.

Draft only:

```bash
readerctl publish --title "Article Title" --file /tmp/article.md --agent "agent-name" --description "Optional summary"
```

Send only with explicit user authorization to write externally:

```bash
readerctl publish --title "Article Title" --file /tmp/article.md --agent "agent-name" --description "Optional summary" --send
```

### C. Choosing draft vs send

- **Draft**: default when the user is asking for preparation, staging, or
  relay intake
- **Send**: only when the user explicitly asks to send/save/push to Kobo or
  Instapaper, or otherwise clearly authorizes the external write

When in doubt, stop at draft and ask for confirmation.

## Reader-friendly Markdown best practices

Choose the structure that fits the automation and the content. Do not force every
article into one template. For generated Markdown, optimize for semantic HTML and
a narrow e-reader screen:

- Pass the article title through `--title`; do **not** repeat it as a top-level
  `# H1` in the Markdown body. Begin with the introduction or the first `##`
  section instead.
- Prefer short paragraphs, generally two to four sentences, over long blocks of
  uninterrupted text.
- Use descriptive `##` and `###` headings to create clear navigation. Avoid deep
  heading nesting.
- Use short bullet or numbered lists when they improve scanning; do not turn
  ordinary prose into excessive lists.
- Avoid Markdown tables. The current renderer does not support table syntax, and
  wide tables are difficult to read on an e-reader. Convert tabular information
  into bullets, labeled sections, or concise prose.
- Use blockquotes sparingly and only for meaningful attributed excerpts.
- Keep code examples and preformatted text short, with narrow lines that do not
  require horizontal scrolling.
- Avoid raw HTML and layout-oriented markup. The relay sanitizes HTML, and
  Instapaper/Kobo—not the agent—controls final typography and spacing.
- Use images sparingly. Provide useful alt text and absolute, externally reachable
  image URLs when an image is necessary to understand the content.
- Use descriptive inline links or a simple Sources section for citations. The
  renderer does not currently produce true formatted footnotes from `[^1]`
  syntax, so do not rely on Markdown footnotes.
- Place citations at natural reading breaks and use descriptive source names
  instead of exposing long raw URLs in prose.
- Reflect sources accurately, avoid fabricated quotations or unsupported claims,
  and keep quoted material minimal and attributed.

Examples of reliable citation formatting:

```md
According to the [original report](https://example.com/source), the release
ships in July 2026.
```

```md
## Sources

- [Original report](https://example.com/source)
- [Supporting documentation](https://example.com/documentation)
```

## Pitfalls

- **Missing agent identity**: always populate `--agent`
- **Missing title**: never publish an untitled item
- **No citations**: generated Markdown should include source links or
  notes
- **Secrets in content**: scrub sensitive material before writing a file
- **Sensitive or personal content**: ask before sending
- **Accidental external write**: do not add `--send` unless the user
  explicitly requested a write to Kobo/Instapaper or equivalent delivery

## Verification

After running `readerctl`, report back:

- the article ID, if returned
- the status (`draft`, `queued`, `saved`, `sent`, or equivalent)
- whether the operation used `--send` or stayed draft-only
- any relay warnings or validation issues

If `readerctl` returns an error, surface the exact failure and do not
claim success.

## Minimal operating checklist

1. Confirm whether the user wants draft-only or an external send.
2. Prepare the URL or Markdown with citations.
3. Include `--agent` and `--title`.
4. Add `--send` only with explicit authorization for Kobo/Instapaper or
   other external delivery.
5. Verify the returned article ID and status.
