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
	ManageTaxonomies Permission = "taxonomies.manage"
	ManageMedia      Permission = "media.manage"
	ReadEntries      Permission = "entries.read"
	CreateEntries    Permission = "entries.create"
	EditAnyEntry     Permission = "entries.edit_any"
	EditOwnEntry     Permission = "entries.edit_own"
	PublishEntries   Permission = "entries.publish"
	DeleteEntries    Permission = "entries.delete"
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
		case ManageMedia, ManageTaxonomies, ReadEntries, CreateEntries, EditAnyEntry, EditOwnEntry, PublishEntries, DeleteEntries:
			return true
		}
	case RoleAuthor:
		switch permission {
		case ReadEntries, CreateEntries, EditOwnEntry:
			return true
		}
	}
	return false
}

func CanAccessEntry(role, userID, authorID string, permission Permission) bool {
	if permission == EditOwnEntry {
		return userID != "" && userID == authorID
	}
	if Allows(role, permission) {
		return true
	}
	return false
}
