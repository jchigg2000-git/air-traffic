# TODO — per-vendor auth/config schemas (remaining 10 vendors)

> Status: **PLANNED**. Six Tier-1 vendors have real proxy-config schemas today
> (`web/src/lib/authSchemas.ts`): OpenAI, Anthropic, AWS Bedrock, Azure OpenAI,
> Google Vertex, GitHub Copilot — and they ship **enabled** by default. The ten
> vendors below ship **disabled** (`internal/store/store.go` → `defaultRoster`)
> and fall back to URL-only config in the modal until their schema is built.

**Reminder:** the roster (who's enabled) is the user's to set — do **not** auto-toggle
`enabled` (see auto-memory `feedback-no-auto-toggle-vendors`). Building a schema here
does **not** imply enabling the vendor.

Also note: proxy mode itself is still a Phase-1 stub (no real outbound calls). These
schemas capture config for the future proxy implementation; none of them authenticate
against a live vendor yet.

## Schema to add per vendor

Add an entry to `AUTH_SCHEMAS` with `authType`, optional `note`, and `fields`
(`url` | `text` | `secret_ref` | `select`; secrets are reference-only). Suggested shape:

| Vendor (id) | Auth type | Key fields |
|---|---|---|
| Mistral (`mistral`) | Bearer admin API key | `upstream_url` (https://api.mistral.ai), `admin_key_ref` |
| Databricks (`databricks`) | Workspace PAT | `upstream_url` (https://{workspace}.cloud.databricks.com), `token_ref` |
| Perplexity (`perplexity`) | Bearer API key + webhook secret | `upstream_url` (https://api.perplexity.ai), `api_key_ref`, `webhook_secret_ref` |
| Cohere (`cohere`) | Bearer API key | `upstream_url` (https://api.cohere.com), `api_key_ref` |
| Together (`together`) | Bearer API key (project-scoped) | `upstream_url` (https://api.together.xyz), `api_key_ref` |
| Groq (`groq`) | Bearer API key | `upstream_url` (https://api.groq.com), `api_key_ref` |
| xAI (`xai`) | Bearer API key | `upstream_url` (https://api.x.ai), `api_key_ref` |
| Amazon Q (`amazon_q`) | AWS SigV4 + IAM Identity Center | `upstream_url`, `region`, `access_key_id_ref`, `secret_access_key_ref`, `identity_store_id` |
| IBM watsonx (`watsonx`) | IAM apikey | `upstream_url` (https://{region}.ml.cloud.ibm.com), `apikey_ref`, `project_id` |
| M365 Copilot (`m365_copilot`) | Entra (AAD) app → Graph token | `tenant_id`, `client_id`, `client_secret_ref` (host is graph.microsoft.com) |

## Also add to `web/src/lib/endpoints.ts`
Each already has an `EXPECTED` host entry for URL validation — confirm hosts when
building (especially Databricks/Azure/watsonx region subdomains).

## When building these
1. Add `AUTH_SCHEMAS[id]` in `authSchemas.ts`.
2. Verify the `EXPECTED` host suffix in `endpoints.ts`.
3. Leave `defaultRoster` alone unless the user asks to enable the vendor.
4. (Future) Wire `endpoint_config` into a real proxy implementation — out of current scope.
