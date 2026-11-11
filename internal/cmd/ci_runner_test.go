package cmd

import (
	"testing"

	"github.com/brimblehq/anarchy/internal/client"
)

func TestPickRunnerProviderFromFlags(t *testing.T) {
	cases := []struct {
		provider, resourceClass, want string
	}{
		{"github", "", providerGitHub},
		{"GitHub", "", providerGitHub},
		{"circleci", "", providerCircleCI},
		{" CircleCI ", "", providerCircleCI},
		{"", "acme/brimble", providerCircleCI},
	}
	for _, tc := range cases {
		got, err := pickRunnerProvider(tc.provider, tc.resourceClass)
		if err != nil || got != tc.want {
			t.Fatalf("pickRunnerProvider(%q, %q) = %q, %v; want %q", tc.provider, tc.resourceClass, got, err, tc.want)
		}
	}
}

func TestPickRunnerProviderRejectsUnknown(t *testing.T) {
	if _, err := pickRunnerProvider("jenkins", ""); err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}

func TestRunnerTarget(t *testing.T) {
	if got := runnerTarget(client.CiRunner{Provider: providerCircleCI, ResourceClass: "acme/brimble"}); got != "acme/brimble" {
		t.Fatalf("circleci target = %q", got)
	}
	if got := runnerTarget(client.CiRunner{Provider: providerGitHub, AccountLogin: "acme"}); got != "@acme" {
		t.Fatalf("github target = %q", got)
	}
	if got := runnerTarget(client.CiRunner{Provider: providerGitHub}); got != "" {
		t.Fatalf("empty github target = %q", got)
	}
}
