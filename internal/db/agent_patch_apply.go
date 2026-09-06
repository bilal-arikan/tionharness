package db

// applyInheritablePatch writes every INHERITABLE unit the patch touches onto a,
// calling mark with the unit's override key for each one it writes.
//
// Shared by the two write paths so they can never drift: a normal agent update
// (where mark pins the field on a child) and a built-in system agent edit (where
// mark collects the keys pinned in the app-global override layer). Identity
// fields — Name, Disabled, ParentID, ResetFields — are NOT handled here: they
// are not inheritable, and the two callers treat them differently.
func applyInheritablePatch(a *Agent, p AgentProfilePatch, mark func(key string)) {
	if p.Soul != nil {
		a.Soul = *p.Soul
		mark("soul")
	}
	if p.Identity != nil {
		a.Identity = *p.Identity
		mark("identity")
	}
	if p.Provider != nil {
		a.Provider = *p.Provider
		mark("provider")
	}
	if p.ProviderInstanceID != nil {
		a.ProviderInstanceID = *p.ProviderInstanceID
		mark("provider")
	}
	if p.Model != nil {
		a.Model = *p.Model
		mark("model")
	}
	if p.ThinkingLevel != nil {
		a.ThinkingLevel = *p.ThinkingLevel
		mark("thinkingLevel")
	}
	if p.NativeWebSearch != nil {
		v := *p.NativeWebSearch
		a.NativeWebSearch = &v
		mark("nativeWebSearch")
	}
	if p.PermissionMode != nil {
		a.PermissionMode = *p.PermissionMode
		mark("permissionMode")
	}
	if p.Avatar != nil {
		a.Avatar = *p.Avatar
		mark("avatar")
	}
	if p.Color != nil {
		a.Color = *p.Color
		mark("color")
	}
	if p.Skills != nil {
		a.Skills = *p.Skills
		mark("skills")
	}
	if p.CoordinatorMode != nil {
		a.CoordinatorMode = *p.CoordinatorMode
		mark("coordinatorMode")
	}
	if p.CoordinatorWorkflow != nil {
		a.CoordinatorWorkflow = *p.CoordinatorWorkflow
		mark("coordinatorWorkflow")
	}
	if p.CoordinatorPrompt != nil {
		a.CoordinatorPrompt = *p.CoordinatorPrompt
		mark("coordinatorPrompt")
	}
}
