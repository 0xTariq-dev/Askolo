package authorization

import (
	"sync"
	"testing"
)

func TestEvaluateDeniesByDefault(t *testing.T) {
	input := Input{
		ActorUserID: "user-1",
		WorkspaceID: "workspace-1",
		Action:      ActionResourceRead,
	}
	decision := Evaluate(input, "active", "active", nil, nil)
	if decision.Allowed || decision.Reason != "membership_missing" {
		t.Fatalf("decision = %+v, want membership_missing denial", decision)
	}
}

func TestEvaluateRejectsRevokedAndInactiveActors(t *testing.T) {
	input := Input{ActorUserID: "user-1", WorkspaceID: "workspace-1", Action: ActionWorkspaceRead}
	membership := &Membership{Status: "active", Permissions: []string{string(ActionWorkspaceRead)}}
	if decision := Evaluate(input, "suspended", "active", membership, nil); decision.Allowed || decision.Reason != "account_inactive" {
		t.Fatalf("suspended account decision = %+v", decision)
	}
	membership.Status = "revoked"
	if decision := Evaluate(input, "active", "active", membership, nil); decision.Allowed || decision.Reason != "membership_revoked" {
		t.Fatalf("revoked membership decision = %+v", decision)
	}
}

func TestEvaluateRequiresCapabilityAndOwnership(t *testing.T) {
	input := Input{
		ActorUserID:  "user-1",
		WorkspaceID:  "workspace-1",
		ResourceType: "note",
		ResourceID:   "note-1",
		Action:       ActionResourceUpdate,
	}
	membership := &Membership{Status: "active", Permissions: []string{string(ActionResourceRead)}}
	resource := &Resource{WorkspaceID: "workspace-1", ResourceType: "note", ResourceID: "note-1", OwnerUserID: "user-1", Status: "active"}
	if decision := Evaluate(input, "active", "active", membership, resource); decision.Allowed || decision.Reason != "action_not_allowed" {
		t.Fatalf("missing capability decision = %+v", decision)
	}
	membership.Permissions = append(membership.Permissions, string(ActionResourceUpdate))
	resource.OwnerUserID = "user-2"
	if decision := Evaluate(input, "active", "active", membership, resource); decision.Allowed || decision.Reason != "resource_not_owned" {
		t.Fatalf("cross-owner decision = %+v", decision)
	}
	membership.Permissions = append(membership.Permissions, string(ActionResourceUpdate)+".any")
	if decision := Evaluate(input, "active", "active", membership, resource); !decision.Allowed {
		t.Fatalf("explicit cross-owner capability decision = %+v", decision)
	}
}

func TestEvaluateRejectsCrossWorkspaceResource(t *testing.T) {
	input := Input{
		ActorUserID: "user-1", WorkspaceID: "workspace-1",
		ResourceType: "goal", ResourceID: "goal-1", Action: ActionResourceRead,
	}
	membership := &Membership{Status: "active", Permissions: []string{string(ActionResourceRead)}}
	resource := &Resource{WorkspaceID: "workspace-2", ResourceType: "goal", ResourceID: "goal-1", OwnerUserID: "user-1", Status: "active"}
	decision := Evaluate(input, "active", "active", membership, resource)
	if decision.Allowed || decision.Reason != "resource_workspace_mismatch" {
		t.Fatalf("cross-workspace decision = %+v", decision)
	}
}

func TestEvaluateConcurrentDecisionsRemainIndependent(t *testing.T) {
	input := Input{ActorUserID: "user-1", WorkspaceID: "workspace-1", Action: ActionWorkspaceRead}
	membership := &Membership{Status: "active", Permissions: []string{string(ActionWorkspaceRead)}}
	const workers = 64
	var wait sync.WaitGroup
	wait.Add(workers)
	results := make(chan Decision, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wait.Done()
			results <- Evaluate(input, "active", "active", membership, nil)
		}()
	}
	wait.Wait()
	close(results)
	for decision := range results {
		if !decision.Allowed || decision.Reason != "allowed" {
			t.Fatalf("concurrent decision = %+v", decision)
		}
	}
}
