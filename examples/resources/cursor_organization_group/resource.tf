resource "cursor_organization_group" "engineering" {
  name                           = "Engineering"
  monthly_spending_limit_dollars = 300
}

# Adopt SCIM-backed groups with an import block and guard them from deletion.
resource "cursor_organization_group" "contractors" {
  name                           = "Contractors"
  monthly_spending_limit_dollars = 100

  lifecycle {
    prevent_destroy = true
  }
}
