terraform {
  required_providers {
    cursor = {
      source  = "cursor/cursor"
      version = "~> 0.1"
    }
  }
}

provider "cursor" {
  # Each value can also be supplied through its CURSOR_* environment variable.
  token                = var.cursor_token
  team_api_key         = var.cursor_team_api_key
  organization_api_key = var.cursor_organization_api_key
}
