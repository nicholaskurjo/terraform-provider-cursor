data "cursor_team_member" "developer" {
  email = "developer@example.com"
}

data "cursor_organization_group" "engineering" {
  name = "Engineering"
}

resource "cursor_organization_group_membership" "engineering_developer" {
  group_id = data.cursor_organization_group.engineering.id
  user_id  = data.cursor_team_member.developer.user_id
}
