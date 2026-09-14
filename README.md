# Terraform Provider for Cursor

Manage [Cursor Automations](https://cursor.com) and Enterprise team administration settings with Terraform or OpenTofu.

The provider talks to the Cursor Automations API over Connect RPC and to the documented Team Admin REST API.

Full documentation for the provider, resources, and data sources lives in [`docs/`](./docs) and on the Terraform Registry.

## Installation

```hcl
terraform {
  required_providers {
    cursor = {
      source  = "cursor/cursor"
      version = "~> 0.1"
    }
  }
}
```

## Provider Configuration

Use explicit configuration or environment variables:

- `CURSOR_TOKEN` - Cursor Automations API token. Raw `key_` and `crsr_` API keys are exchanged for a session token automatically.
- `CURSOR_ENDPOINT` - Cursor API base URL. Optional, defaults to `https://api2.cursor.sh`.
- `CURSOR_TEAM_API_KEY` - Team Admin API key for per-user spend limits.
- `CURSOR_TEAM_API_ENDPOINT` - Team Admin API base URL. Optional, defaults to `https://api.cursor.com`.

Configure at least one credential. A configuration that manages automations and user spend limits uses separate credentials:

```hcl
provider "cursor" {
	token        = var.cursor_token
	team_api_key = var.cursor_team_api_key
}
```

## Example

```hcl
resource "cursor_platform_workflow" "example_review" {
  name    = "Example review automation"
  scope   = "team"
  enabled = true

  prompt = file("prompt.md")

  trigger = [
    {
      git_pull_request = {
        repos            = ["example-org/example-repo"]
        pr_action        = "opened"
        ignore_draft_prs = true
      }
    }
  ]

  action = [
    {
      pr_comment = {
        allow_inline_comments = true
      }
    }
  ]
}
```

Supported triggers: `git_pull_request`, `git_push`, `git_ci_completed`, `cron`, `slack`, `linear`, `webhook`, `microsoft_teams`, and `microsoft_teams_channel_created`.

Supported actions: `pr_comment`, `git_pr`, `request_reviewers`, `mcp`, `slack`, `read_slack`, `microsoft_teams`, and `read_microsoft_teams`.

See [`examples/`](./examples) for more, including the data source and import syntax. The resource and data source docs in [`docs/`](./docs) describe every trigger and action type.

## Enterprise spending policy

`cursor_user_spend_limit` manages a monthly override for one team member. Cursor applies the highest applicable team, group, or user limit, so the resource also exports the effective limit reported by Cursor.

Import existing live limits before applying policy so Terraform adopts them instead of treating them as new.

Removing a user-limit resource clears its override; it does not set a $0 limit. Setting `spend_limit_dollars = 0` creates an explicit $0 limit.

This resource uses the documented [Team Admin API](https://cursor.com/docs/account/teams/admin-api). API, authentication, rate-limit, malformed-response, and unsuccessful mutation outcomes are returned as diagnostics without treating the resource as absent. Team-wide budgets and notification thresholds are outside this resource.

## Development

```bash
make build   # go build
make test    # go test ./...
make docs    # regenerate docs/ with tfplugindocs
```

Regenerate `docs/` with `make docs` after changing any schema `Description` or the files under `examples/`.

## License

[Apache-2.0](./LICENSE). "Cursor" is a trademark of Anysphere, Inc.
