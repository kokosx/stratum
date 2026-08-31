package authz

// ActorKind distinguishes human users from service identities.
type ActorKind string

const (
	ActorUser   ActorKind = "user"
	ActorAgent  ActorKind = "agent"
	ActorSystem ActorKind = "system"
)

// Actor is the authenticated identity performing an operation.
// It unifies human users and service agents behind one abstraction.
type Actor struct {
	ID          string
	Kind        ActorKind
	Role        string // human role when Kind==user; empty for agent/system
	AgentID     string // agent.id when Kind==agent
	AgentName   string // display name when Kind==agent
	DisplayName string // email for user, name for agent
}

// IsUser reports whether this actor is a human user.
func (a Actor) IsUser() bool { return a.Kind == ActorUser }

// IsAgent reports whether this actor is a service agent.
func (a Actor) IsAgent() bool { return a.Kind == ActorAgent }

// IsSystem reports whether this actor is trusted internal system.
func (a Actor) IsSystem() bool { return a.Kind == ActorSystem }

// Resource identifies the target of an authorization check.
type Resource struct {
	ContentTypeID string
	EntryID       string
	OwnerID       string
}

// AgentGrant is a persisted permission grant for an agent.
type AgentGrant struct {
	Permission string
	Scope      string // "*" or "content_type:<id>"
}

// Scope constants
const (
	ScopeAll = "*"
)

// ScopeCovers reports whether scope covers the given content type.
func ScopeCovers(scope, contentTypeID string) bool {
	if scope == ScopeAll || scope == "*" {
		return true
	}
	if len(scope) > len("content_type:") && scope[:13] == "content_type:" {
		return scope[13:] == contentTypeID
	}
	return false
}

// Agent permission constants (scoped, fine-grained)
const (
	PermEntriesRead    Permission = "entries.read"
	PermEntriesCreate  Permission = "entries.create"
	PermEntriesEdit    Permission = "entries.edit"
	PermEntriesPublish Permission = "entries.publish"
	PermEntriesTrash   Permission = "entries.trash"

	PermMediaRead   Permission = "media.read"
	PermMediaUpload Permission = "media.upload"
	PermMediaEdit   Permission = "media.edit"

	PermTaxonomiesRead   Permission = "taxonomies.read"
	PermTaxonomiesAssign Permission = "taxonomies.assign"

	PermContentTypesRead Permission = "content_types.read"
	PermBlocksRead       Permission = "blocks.read"
	PermSiteRead         Permission = "site.read"
	PermAgentsManage     Permission = "agents.manage"
)

// AllAgentPermissions is the allowlist of permissions that agents may hold.
// MCP must never expose dangerous operations; this list is the enforcement point.
var AllAgentPermissions = []Permission{
	PermEntriesRead,
	PermEntriesCreate,
	PermEntriesEdit,
	PermEntriesPublish,
	PermEntriesTrash,
	PermMediaRead,
	PermMediaUpload,
	PermMediaEdit,
	PermTaxonomiesRead,
	PermTaxonomiesAssign,
	PermContentTypesRead,
	PermBlocksRead,
	PermSiteRead,
	PermAgentsManage,
}

// IsValidAgentPermission reports whether permission may be granted to an agent.
func IsValidAgentPermission(p Permission) bool {
	for _, allowed := range AllAgentPermissions {
		if p == allowed {
			return true
		}
	}
	return false
}

// Allowed is the unified authorization boundary used by application services.
// Human actors are evaluated via role policy; agent actors via explicit grants.
// System actors are trusted for internal operations where explicitly used.
func Allowed(actor Actor, perm Permission, res Resource, grants []AgentGrant) bool {
	switch actor.Kind {
	case ActorSystem:
		return true
	case ActorUser:
		return allowedForUser(actor.Role, perm, res)
	case ActorAgent:
		return allowedForAgent(perm, res, grants)
	default:
		return false
	}
}

func allowedForUser(role string, perm Permission, res Resource) bool {
	// Map new fine-grained perms onto existing role policy.
	switch perm {
	case PermSiteRead, PermContentTypesRead, PermBlocksRead:
		// All authenticated users can read site/catalog.
		// Editors/admins get more; authors limited to posts.
		if Role(role) == RoleAdmin || Role(role) == RoleEditor {
			return true
		}
		if Role(role) == RoleAuthor {
			// Authors can read entries/content types for their own contentType
			return Allows(role, ReadEntries)
		}
		return false
	case PermEntriesRead:
		if Allows(role, ReadEntries) {
			return true
		}
		// Author read is ownership-scoped; handled via CanAccessEntry elsewhere for specific entries.
		return CanAccessEntry(role, "", res.OwnerID, res.ContentTypeID, EntryRead)
	case PermEntriesCreate:
		if Allows(role, CreateEntries) {
			// Author CreateEntries is global but CanAccessEntry restricts to post
			if Role(role) == RoleAuthor {
				return CanAccessEntry(role, res.OwnerID, res.OwnerID, res.ContentTypeID, EntryCreate)
			}
			return true
		}
		return CanAccessEntry(role, res.OwnerID, res.OwnerID, res.ContentTypeID, EntryCreate)
	case PermEntriesEdit:
		if Allows(role, EditAnyEntry) {
			return true
		}
		if Allows(role, EditOwnEntry) {
			// For author, ownership check is needed; if OwnerID unknown, allow create path
			if Role(role) == RoleAuthor {
				return res.OwnerID == "" || CanAccessEntry(role, res.OwnerID, res.OwnerID, res.ContentTypeID, EntryEdit)
			}
			return true
		}
		return false
	case PermEntriesPublish:
		if Allows(role, PublishAnyEntry) {
			return true
		}
		if Allows(role, PublishOwnEntry) {
			if Role(role) == RoleAuthor {
				return res.OwnerID == "" || CanAccessEntry(role, res.OwnerID, res.OwnerID, res.ContentTypeID, EntryPublish)
			}
			return true
		}
		return false
	case PermEntriesTrash:
		if Allows(role, DeleteAnyEntry) {
			return true
		}
		if Allows(role, DeleteOwnEntry) {
			if Role(role) == RoleAuthor {
				return res.OwnerID == "" || CanAccessEntry(role, res.OwnerID, res.OwnerID, res.ContentTypeID, EntryDelete)
			}
			return true
		}
		return false
	case PermMediaRead, PermMediaUpload, PermMediaEdit:
		return Allows(role, ManageMedia)
	case PermTaxonomiesRead:
		return Allows(role, ManageTaxonomies) || Allows(role, ReadEntries)
	case PermTaxonomiesAssign:
		return Allows(role, ManageTaxonomies) || Allows(role, ReadEntries)
	case PermAgentsManage:
		return Role(role) == RoleAdmin
	default:
		// Fall back to direct permission check for legacy perms
		return Allows(role, perm)
	}
}

func allowedForAgent(perm Permission, res Resource, grants []AgentGrant) bool {
	if !IsValidAgentPermission(perm) {
		return false
	}
	for _, g := range grants {
		if Permission(g.Permission) != perm {
			continue
		}
		if ScopeCovers(g.Scope, res.ContentTypeID) {
			return true
		}
		// For non-entry resources (site, blocks, media) content type may be empty;
		// only "*" should grant those.
		if res.ContentTypeID == "" && g.Scope == ScopeAll {
			return true
		}
	}
	return false
}
