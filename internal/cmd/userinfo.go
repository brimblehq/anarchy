package cmd

import (
	"fmt"
	"strings"
)

func userDisplayName(data any) string {
	m := asUserMap(data)
	if m == nil {
		return ""
	}
	for _, key := range []string{"email", "name", "username", "fullName", "full_name"} {
		if s := stringField(m, key); s != "" {
			return s
		}
	}
	return ""
}

func whoamiSummary(data any, teamID string) map[string]any {
	m := asUserMap(data)
	out := map[string]any{}
	if m == nil {
		return out
	}

	first := stringField(m, "first_name")
	last := stringField(m, "last_name")
	name := strings.TrimSpace(first + " " + last)
	if name == "" {
		name = stringField(m, "name")
	}

	setIf := func(key, value string) {
		if value != "" {
			out[key] = value
		}
	}
	setIf("name", name)
	setIf("email", stringField(m, "email"))
	setIf("username", stringField(m, "username"))
	setIf("company", stringField(m, "company"))
	setIf("team", teamID)
	out["auth"] = "api key"
	return out
}

func asUserMap(data any) map[string]any {
	m, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	for _, nest := range []string{"user", "account", "data", "profile"} {
		if nested, ok := m[nest].(map[string]any); ok {
			return nested
		}
	}
	return m
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func projectOptions(data any) []ProjectRef {
	items := asSlice(data)
	out := make([]ProjectRef, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := firstString(m, "name", "slug")
		if name == "" {
			continue
		}
		out = append(out, ProjectRef{
			ID:     firstString(m, "_id", "id"),
			Name:   name,
			Status: firstString(m, "status"),
		})
	}
	return out
}

func teamOptions(data any) []struct{ ID, Name string } {
	items := asSlice(data)
	out := make([]struct{ ID, Name string }, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := firstString(m, "name", "slug")
		if name == "" {
			continue
		}
		out = append(out, struct{ ID, Name string }{
			ID:   firstString(m, "_id", "id"),
			Name: name,
		})
	}
	return out
}

func asSlice(data any) []any {
	if data == nil {
		return nil
	}
	if s, ok := data.([]any); ok {
		return s
	}
	if m, ok := data.(map[string]any); ok {
		for _, key := range []string{"docs", "items", "projects", "teams", "data", "results", "envs", "logs", "frameworks", "regions", "plans"} {
			if v, ok := m[key].([]any); ok {
				return v
			}
		}
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := stringField(m, k); s != "" {
			return s
		}
	}
	return ""
}

func unwrapList(data any) any {
	m, ok := data.(map[string]any)
	if !ok {
		return data
	}
	for _, key := range []string{"docs", "items", "projects", "teams", "data", "results", "envs", "logs", "frameworks", "regions", "plans"} {
		if v, ok := m[key]; ok {
			if _, isSlice := v.([]any); isSlice {
				return v
			}
		}
	}
	return data
}
