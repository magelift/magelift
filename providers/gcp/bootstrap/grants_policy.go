package bootstrap

import (
	artifactregistry "google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
	secretmanager "google.golang.org/api/secretmanager/v1"
	storagev1 "google.golang.org/api/storage/v1"
)

func cloneCRMPolicy(policy *cloudresourcemanager.Policy) *cloudresourcemanager.Policy {
	if policy == nil {
		return &cloudresourcemanager.Policy{}
	}
	out := &cloudresourcemanager.Policy{
		Etag:    policy.Etag,
		Version: policy.Version,
	}
	if len(policy.AuditConfigs) > 0 {
		out.AuditConfigs = make([]*cloudresourcemanager.AuditConfig, len(policy.AuditConfigs))
		for i, cfg := range policy.AuditConfigs {
			if cfg == nil {
				continue
			}
			cloned := &cloudresourcemanager.AuditConfig{
				Service:         cfg.Service,
				AuditLogConfigs: make([]*cloudresourcemanager.AuditLogConfig, len(cfg.AuditLogConfigs)),
			}
			for j, logCfg := range cfg.AuditLogConfigs {
				if logCfg == nil {
					continue
				}
				cloned.AuditLogConfigs[j] = &cloudresourcemanager.AuditLogConfig{
					LogType:         logCfg.LogType,
					ExemptedMembers: append([]string(nil), logCfg.ExemptedMembers...),
				}
			}
			out.AuditConfigs[i] = cloned
		}
	}
	if len(policy.Bindings) > 0 {
		out.Bindings = make([]*cloudresourcemanager.Binding, len(policy.Bindings))
		for i, binding := range policy.Bindings {
			if binding == nil {
				continue
			}
			cloned := &cloudresourcemanager.Binding{
				Role:    binding.Role,
				Members: append([]string(nil), binding.Members...),
			}
			if binding.Condition != nil {
				cloned.Condition = &cloudresourcemanager.Expr{
					Title:       binding.Condition.Title,
					Description: binding.Condition.Description,
					Expression:  binding.Condition.Expression,
					Location:    binding.Condition.Location,
				}
			}
			out.Bindings[i] = cloned
		}
	}
	return out
}

func mergeUnconditionalCRMMember(policy *cloudresourcemanager.Policy, role, member string) (*cloudresourcemanager.Policy, bool) {
	if policy == nil {
		policy = &cloudresourcemanager.Policy{}
	}
	for _, binding := range policy.Bindings {
		if binding == nil || binding.Role != role {
			continue
		}
		if binding.Condition != nil {
			continue
		}
		if containsString(binding.Members, member) {
			return policy, false
		}
		binding.Members = append(binding.Members, member)
		return policy, true
	}
	policy.Bindings = append(policy.Bindings, &cloudresourcemanager.Binding{
		Role:    role,
		Members: []string{member},
	})
	return policy, true
}

func cloneARPolicy(policy *artifactregistry.Policy) *artifactregistry.Policy {
	if policy == nil {
		return &artifactregistry.Policy{}
	}
	out := &artifactregistry.Policy{Etag: policy.Etag, Version: policy.Version}
	if len(policy.Bindings) > 0 {
		out.Bindings = make([]*artifactregistry.Binding, len(policy.Bindings))
		for i, binding := range policy.Bindings {
			if binding == nil {
				continue
			}
			cloned := &artifactregistry.Binding{
				Role:    binding.Role,
				Members: append([]string(nil), binding.Members...),
			}
			if binding.Condition != nil {
				cloned.Condition = &artifactregistry.Expr{
					Title:       binding.Condition.Title,
					Description: binding.Condition.Description,
					Expression:  binding.Condition.Expression,
					Location:    binding.Condition.Location,
				}
			}
			out.Bindings[i] = cloned
		}
	}
	return out
}

func mergeUnconditionalARMember(policy *artifactregistry.Policy, role, member string) (*artifactregistry.Policy, bool) {
	if policy == nil {
		policy = &artifactregistry.Policy{}
	}
	for _, binding := range policy.Bindings {
		if binding == nil || binding.Role != role {
			continue
		}
		if binding.Condition != nil {
			continue
		}
		if containsString(binding.Members, member) {
			return policy, false
		}
		binding.Members = append(binding.Members, member)
		return policy, true
	}
	policy.Bindings = append(policy.Bindings, &artifactregistry.Binding{
		Role:    role,
		Members: []string{member},
	})
	return policy, true
}

func cloneSecretPolicy(policy *secretmanager.Policy) *secretmanager.Policy {
	if policy == nil {
		return &secretmanager.Policy{}
	}
	out := &secretmanager.Policy{Etag: policy.Etag, Version: policy.Version}
	if len(policy.AuditConfigs) > 0 {
		out.AuditConfigs = make([]*secretmanager.AuditConfig, len(policy.AuditConfigs))
		for i, cfg := range policy.AuditConfigs {
			if cfg == nil {
				continue
			}
			cloned := &secretmanager.AuditConfig{
				Service:         cfg.Service,
				AuditLogConfigs: make([]*secretmanager.AuditLogConfig, len(cfg.AuditLogConfigs)),
			}
			for j, logCfg := range cfg.AuditLogConfigs {
				if logCfg == nil {
					continue
				}
				cloned.AuditLogConfigs[j] = &secretmanager.AuditLogConfig{
					LogType:         logCfg.LogType,
					ExemptedMembers: append([]string(nil), logCfg.ExemptedMembers...),
				}
			}
			out.AuditConfigs[i] = cloned
		}
	}
	if len(policy.Bindings) > 0 {
		out.Bindings = make([]*secretmanager.Binding, len(policy.Bindings))
		for i, binding := range policy.Bindings {
			if binding == nil {
				continue
			}
			cloned := &secretmanager.Binding{
				Role:    binding.Role,
				Members: append([]string(nil), binding.Members...),
			}
			if binding.Condition != nil {
				cloned.Condition = &secretmanager.Expr{
					Title:       binding.Condition.Title,
					Description: binding.Condition.Description,
					Expression:  binding.Condition.Expression,
					Location:    binding.Condition.Location,
				}
			}
			out.Bindings[i] = cloned
		}
	}
	return out
}

func mergeUnconditionalSecretMember(policy *secretmanager.Policy, role, member string) (*secretmanager.Policy, bool) {
	if policy == nil {
		policy = &secretmanager.Policy{}
	}
	for _, binding := range policy.Bindings {
		if binding == nil || binding.Role != role {
			continue
		}
		if binding.Condition != nil {
			continue
		}
		if containsString(binding.Members, member) {
			return policy, false
		}
		binding.Members = append(binding.Members, member)
		return policy, true
	}
	policy.Bindings = append(policy.Bindings, &secretmanager.Binding{
		Role:    role,
		Members: []string{member},
	})
	return policy, true
}

func cloneStoragePolicy(policy *storagev1.Policy) *storagev1.Policy {
	if policy == nil {
		return &storagev1.Policy{}
	}
	out := &storagev1.Policy{
		Etag:       policy.Etag,
		Version:    policy.Version,
		Kind:       policy.Kind,
		ResourceId: policy.ResourceId,
	}
	if len(policy.Bindings) > 0 {
		out.Bindings = make([]*storagev1.PolicyBindings, len(policy.Bindings))
		for i, binding := range policy.Bindings {
			if binding == nil {
				continue
			}
			cloned := &storagev1.PolicyBindings{
				Role:    binding.Role,
				Members: append([]string(nil), binding.Members...),
			}
			if binding.Condition != nil {
				cloned.Condition = &storagev1.Expr{
					Title:       binding.Condition.Title,
					Description: binding.Condition.Description,
					Expression:  binding.Condition.Expression,
					Location:    binding.Condition.Location,
				}
			}
			out.Bindings[i] = cloned
		}
	}
	return out
}

func mergeUnconditionalStorageMember(policy *storagev1.Policy, role, member string) (*storagev1.Policy, bool) {
	if policy == nil {
		policy = &storagev1.Policy{}
	}
	for _, binding := range policy.Bindings {
		if binding == nil || binding.Role != role {
			continue
		}
		if binding.Condition != nil {
			continue
		}
		if containsString(binding.Members, member) {
			return policy, false
		}
		binding.Members = append(binding.Members, member)
		return policy, true
	}
	policy.Bindings = append(policy.Bindings, &storagev1.PolicyBindings{
		Role:    role,
		Members: []string{member},
	})
	return policy, true
}
