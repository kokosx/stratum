package authz

import "testing"

func TestAuthorOnlyEditsOwnEntries(t *testing.T) {
	if !CanAccessEntry(string(RoleAuthor), "author", "author", EditOwnEntry) {
		t.Fatal("author should edit their own entry")
	}
	if CanAccessEntry(string(RoleAuthor), "author", "other", EditOwnEntry) {
		t.Fatal("author should not edit another user's entry")
	}
	if Allows(string(RoleAuthor), PublishEntries) || Allows(string(RoleAuthor), ManageSite) {
		t.Fatal("author received an administrative permission")
	}
}

func TestEditorCannotManageUsersOrSite(t *testing.T) {
	if !Allows(string(RoleEditor), EditAnyEntry) {
		t.Fatal("editor should edit entries")
	}
	if Allows(string(RoleEditor), ManageUsers) || Allows(string(RoleEditor), ManageSite) {
		t.Fatal("editor received an administrator permission")
	}
}
