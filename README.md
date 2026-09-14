# Terraform Provider for Cursor

Manage [Cursor Automations](https://cursor.com) and Enterprise administration settings with Terraform or OpenTofu.

The provider talks to the Cursor Automations API over Connect RPC and to the documented Team Admin and Organization REST APIs.

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
- `CURSOR_TEAM_API_KEY` - Team Admin API key for team-member lookups and per-user spend limits. A raw `CURSOR_TOKEN` is used as a fallback.
- `CURSOR_ORGANIZATION_API_KEY` - Organization API key with `members:*` access for organization groups and membership.
- `CURSOR_ADMIN_API_ENDPOINT` - Team Admin and Organization API base URL. Optional, defaults to `https://api.cursor.com`.

Configure at least one credential. A configuration that manages automations and spending policy can use all three:

```hcl
provider "cursor" {
  token                = var.cursor_token
  team_api_key         = var.cursor_team_api_key
  organization_api_key = var.cursor_organization_api_key
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

`cursor_user_spend_limit` manages a monthly override for one team member. `cursor_organization_group` manages a flat per-member limit for a cohort, and `cursor_organization_group_membership` manages manual group membership. Cursor applies the highest applicable team, group, or user limit.

Import existing live limits before applying policy so Terraform adopts them instead of treating them as new. Protect imported SCIM-backed groups with Terraform's native `lifecycle.prevent_destroy`, and keep their membership in the identity provider.

Removing a user-limit resource clears its override; it does not set a $0 limit. Removing a group's limit clears only that setting. Deleting a group requires it to be empty and have no active SCIM mapping. Keep protected group resources in configuration: Terraform's lifecycle protection does not apply after the resource block is removed.

These resources use the documented [Team Admin API](https://cursor.com/docs/account/teams/admin-api) and [Organization API](https://cursor.com/docs/account/organizations/organization-admin-api). Group routes are limited to 20 requests per minute per organization; rate-limit errors are returned as diagnostics, and the next apply can resume from refreshed state. Team-wide budgets and notification thresholds are outside this resource set.

## Development

```bash
make build   # go build
make test    # go test ./...
make docs    # regenerate docs/ with tfplugindocs
```

Regenerate `docs/` with `make docs` after changing any schema `Description` or the files under `examples/`.

## License

[Apache-2.0](./LICENSE). "Cursor" is a trademark of Anysphere, Inc.
