package expression

import "testing"

func TestEvaluateGithubLabelFilter(t *testing.T) {
	vars := map[string]any{
		"method": "POST",
		"path":   "/github/issues",
		"query":  map[string]any{},
		"headers": map[string]any{
			"X-GitHub-Event": "issues",
		},
		"body": map[string]any{
			"action": "labeled",
			"label":  map[string]any{"name": "deploy-approved"},
			"sender": map[string]any{"login": "alice"},
		},
		"config": map[string]any{
			"required_label":   "deploy-approved",
			"authorized_users": []any{"alice", "bob"},
		},
	}
	if err := Validate(`headers["X-GitHub-Event"] == "issues" && body.action == "labeled" && body.label.name == config.required_label && body.sender.login in config.authorized_users`); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got, err := Evaluate(`headers["X-GitHub-Event"] == "issues" && body.action == "labeled" && body.label.name == config.required_label && body.sender.login in config.authorized_users`, vars)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !got {
		t.Fatal("expected expression to be true")
	}
}
