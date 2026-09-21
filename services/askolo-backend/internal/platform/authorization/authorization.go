package authorization

import "strings"

type Action string

const (
	ActionWorkspaceRead    Action = "workspace.read"
	ActionResourceRead     Action = "resource.read"
	ActionResourceCreate   Action = "resource.create"
	ActionResourceUpdate   Action = "resource.update"
	ActionResourceDelete   Action = "resource.delete"
	ActionProviderRead     Action = "provider.read"
	ActionProviderWrite    Action = "provider.write"
	ActionJobExecute       Action = "job.execute"
	ActionWebSocketConnect Action = "websocket.connect"
	ActionAIExecute        Action = "ai.execute"
)

var PersonalWorkspaceCapabilities = []string{
	string(ActionWorkspaceRead),
	string(ActionResourceRead),
	string(ActionResourceCreate),
	string(ActionResourceUpdate),
	string(ActionResourceDelete),
	string(ActionProviderRead),
	string(ActionProviderWrite),
	string(ActionJobExecute),
	string(ActionWebSocketConnect),
	string(ActionAIExecute),
}

type Input struct {
	ActorUserID  string
	WorkspaceID  string
	ResourceType string
	ResourceID   string
	Action       Action
}

type Membership struct {
	Status      string
	Role        string
	Permissions []string
	Revoked     bool
}

type Resource struct {
	WorkspaceID  string
	ResourceType string
	ResourceID   string
	OwnerUserID  string
	Status       string
}

type Decision struct {
	Allowed bool
	Reason  string
}

func Evaluate(input Input, accountStatus, workspaceStatus string, membership *Membership, resource *Resource) Decision {
	if strings.TrimSpace(input.ActorUserID) == "" {
		return Decision{Reason: "missing_actor"}
	}
	if strings.TrimSpace(input.WorkspaceID) == "" {
		return Decision{Reason: "missing_workspace_scope"}
	}
	if strings.TrimSpace(string(input.Action)) == "" {
		return Decision{Reason: "missing_action"}
	}
	if accountStatus != "active" {
		return Decision{Reason: "account_inactive"}
	}
	if workspaceStatus == "" {
		return Decision{Reason: "workspace_missing"}
	}
	if workspaceStatus != "active" {
		return Decision{Reason: "workspace_inactive"}
	}
	if membership == nil {
		return Decision{Reason: "membership_missing"}
	}
	if membership.Status != "active" || membership.Revoked {
		return Decision{Reason: "membership_revoked"}
	}
	if !hasPermission(membership.Permissions, string(input.Action)) {
		return Decision{Reason: "action_not_allowed"}
	}
	if input.ResourceID == "" && input.ResourceType == "" {
		return Decision{Allowed: true, Reason: "allowed"}
	}
	if input.ResourceID == "" || input.ResourceType == "" {
		return Decision{Reason: "incomplete_resource_scope"}
	}
	if resource == nil {
		return Decision{Reason: "resource_missing"}
	}
	if resource.Status != "" && resource.Status != "active" {
		return Decision{Reason: "resource_inactive"}
	}
	if resource.WorkspaceID != input.WorkspaceID {
		return Decision{Reason: "resource_workspace_mismatch"}
	}
	if resource.ResourceType != input.ResourceType || resource.ResourceID != input.ResourceID {
		return Decision{Reason: "resource_scope_mismatch"}
	}
	if resource.OwnerUserID != input.ActorUserID &&
		!hasPermission(membership.Permissions, resourcePermissionFor(input.Action)+".any") &&
		!hasPermission(membership.Permissions, "resource.manage") {
		return Decision{Reason: "resource_not_owned"}
	}
	return Decision{Allowed: true, Reason: "allowed"}
}

func hasPermission(permissions []string, target string) bool {
	for _, permission := range permissions {
		if strings.TrimSpace(permission) == target {
			return true
		}
	}
	return false
}

func resourcePermissionFor(action Action) string {
	switch action {
	case ActionResourceRead:
		return string(ActionResourceRead)
	case ActionResourceCreate:
		return string(ActionResourceCreate)
	case ActionResourceUpdate:
		return string(ActionResourceUpdate)
	case ActionResourceDelete:
		return string(ActionResourceDelete)
	default:
		return string(action)
	}
}
