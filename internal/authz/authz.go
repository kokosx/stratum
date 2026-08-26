// Package authz owns the role-to-capability policy used by the admin surface.
package authz

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleAuthor Role = "author"
)

type Permission string

const (
	ManageUsers      Permission = "users.manage"
	ManageSite       Permission = "site.manage"
	ManageNavigation Permission = "navigation.manage"
	ManageTaxonomies Permission = "taxonomies.manage"
	ManageMedia      Permission = "media.manage"
	ReadEntries      Permission = "entries.read"
	CreateEntries    Permission = "entries.create"
	EditAnyEntry     Permission = "entries.edit_any"
	EditOwnEntry     Permission = "entries.edit_own"
	PublishAnyEntry  Permission = "entries.publish_any"
	PublishOwnEntry  Permission = "entries.publish_own"
	DeleteAnyEntry   Permission = "entries.delete_any"
	DeleteOwnEntry   Permission = "entries.delete_own"
	ModerateComments Permission = "comments.moderate"
	ReadComments     Permission = "comments.read"
	DeleteComments   Permission = "comments.delete"
)

type EntryAction string

const (
	EntryRead    EntryAction = "read"
	EntryCreate  EntryAction = "create"
	EntryEdit    EntryAction = "edit"
	EntryPublish EntryAction = "publish"
	EntryDelete  EntryAction = "delete"
)

func ValidRole(role string) bool {
	return Role(role) == RoleAdmin || Role(role) == RoleEditor || Role(role) == RoleAuthor
}

func Allows(role string, permission Permission) bool {
	switch Role(role) {
	case RoleAdmin:
		return true
	case RoleEditor:
		switch permission {
		case ManageMedia, ManageTaxonomies, ManageNavigation, ReadEntries, CreateEntries, EditAnyEntry, EditOwnEntry, PublishAnyEntry, DeleteAnyEntry, ModerateComments, ReadComments, DeleteComments:
			return true
		}
	case RoleAuthor:
		switch permission {
		case ManageMedia, ReadEntries, CreateEntries, EditOwnEntry, PublishOwnEntry, DeleteOwnEntry:
			return true
		}
	}
	return false
}

// CanAccessEntry is the single ownership decision for entry operations.
func CanAccessEntry(role, userID, authorID, contentType string, action EntryAction) bool {
	if Role(role) == RoleAdmin {
		return true
	}
	if Role(role) == RoleEditor {
		return action == EntryRead || action == EntryCreate || action == EntryEdit || action == EntryPublish || action == EntryDelete
	}
	if Role(role) != RoleAuthor || contentType != "post" {
		return false
	}
	if action == EntryCreate {
		return true
	}
	return userID != "" && userID == authorID && (action == EntryRead || action == EntryEdit || action == EntryPublish || action == EntryDelete)
}
