package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/ui"
)

type createDraft struct {
	Name            string
	Source          string
	ServiceType     string
	Framework       string
	RegionID        string
	CPU             float64
	Memory          float64
	RepoName        string
	RepoBranch      string
	InstallationID  int
	InstallCommand  string
	BuildCommand    string
	StartCommand    string
	OutputDirectory string
	RootDirectory   string
	PreStartCommand string
	RegistryUser    string
	RegistryToken   string
	CronEnabled     bool
	CronExpression  string
	CronTimezone    string
	AuthEnabled     bool
	EnvVars         []map[string]string
	Deploy          bool
	TeamID          string
}

func runCreateQuestionnaire(ctx context.Context, teamID string) (map[string]any, error) {
	if !ui.IsInteractive() {
		return nil, fmt.Errorf("not a terminal; pass --file with a JSON body")
	}

	ui.Title("Create project")

	name, err := ui.Text("Project name", "my-app", "")
	if err != nil {
		return nil, err
	}
	name = slugifyName(name)
	if len(name) < 3 {
		return nil, fmt.Errorf("project name must be at least 3 characters")
	}

	source, err := ui.Select("Source", []ui.Option{
		{Label: "Docker image", Value: "docker"},
		{Label: "GitHub", Value: "github"},
		{Label: "GitLab", Value: "gitlab"},
		{Label: "Bitbucket", Value: "bitbucket"},
	})
	if err != nil {
		return nil, err
	}

	serviceType, err := ui.Select("Service type", []ui.Option{
		{Label: "Web Service — APIs and dynamic apps", Value: "web-service"},
		{Label: "Static Site — HTML/CSS/JS, no server", Value: "static"},
		{Label: "Worker — background jobs / cron", Value: "worker"},
		{Label: "MCP — AI tool servers", Value: "mcp"},
	})
	if err != nil {
		return nil, err
	}

	draft := createDraft{
		Name:        name,
		Source:      source,
		ServiceType: serviceType,
		CPU:         0.5,
		Memory:      0.5,
		Deploy:      true,
		TeamID:      teamID,
	}

	if source == "docker" {
		if err := fillDockerDraft(&draft); err != nil {
			return nil, err
		}
	} else {
		if err := fillGitDraft(ctx, &draft); err != nil {
			return nil, err
		}
	}

	if err := fillSharedDraft(ctx, &draft); err != nil {
		return nil, err
	}

	if serviceType == "worker" {
		if err := fillWorkerDraft(&draft); err != nil {
			return nil, err
		}
	}
	if serviceType == "mcp" {
		auth, err := ui.Confirm("Enable MCP auth?", false)
		if err != nil {
			return nil, err
		}
		draft.AuthEnabled = auth
	}

	if err := fillEnvVars(&draft); err != nil {
		return nil, err
	}

	deploy, err := ui.Confirm("Deploy immediately after create?", true)
	if err != nil {
		return nil, err
	}
	draft.Deploy = deploy

	return draft.toPayload(), nil
}

func fillDockerDraft(d *createDraft) error {
	d.Framework = "docker"
	image, err := ui.Text("Docker image", "nginx:latest", "")
	if err != nil {
		return err
	}
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("docker image is required")
	}
	tag := "latest"
	if i := strings.LastIndex(image, ":"); i > 0 && !strings.Contains(image[i:], "/") {
		tag = image[i+1:]
	}
	d.RepoName = image
	d.RepoBranch = tag

	needCreds, err := ui.Confirm("Private registry credentials?", false)
	if err != nil {
		return err
	}
	if needCreds {
		d.RegistryUser, err = ui.Text("Registry username", "", "")
		if err != nil {
			return err
		}
		d.RegistryToken, err = ui.Password("Registry token / password", "")
		if err != nil {
			return err
		}
	}

	d.PreStartCommand, err = ui.Text("Pre-start command (optional)", "", "")
	return err
}

func fillGitDraft(ctx context.Context, d *createDraft) error {
	accounts, err := loadGitAccounts(ctx, d.Source, d.TeamID)
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		return fmt.Errorf("%s is not connected — link it in the Brimble dashboard under account settings, then try again", d.Source)
	}

	account, err := pickGitAccount(accounts)
	if err != nil {
		return err
	}
	d.InstallationID = account.InstallationID

	repos, err := loadGitRepos(ctx, d.Source, d.InstallationID, d.TeamID)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repositories found for this %s account", d.Source)
	}

	repo, err := pickGitRepo(repos)
	if err != nil {
		return err
	}
	d.RepoName = repo.FullName
	d.RepoBranch = repo.Branch
	if d.RepoBranch == "" {
		d.RepoBranch = "main"
	}
	if repo.InstallationID > 0 {
		d.InstallationID = repo.InstallationID
	}

	branch, err := ui.Text("Branch", d.RepoBranch, d.RepoBranch)
	if err != nil {
		return err
	}
	if strings.TrimSpace(branch) != "" {
		d.RepoBranch = strings.TrimSpace(branch)
	}

	if repo.Framework != "" {
		d.Framework = repo.Framework
	}

	frameworks, err := loadFrameworkSlugs(ctx)
	if err != nil || len(frameworks) == 0 {
		frameworks = []string{"nodejs", "next", "docker", "html", "static", "python", "golang"}
	}
	fwOpts := make([]ui.Option, 0, len(frameworks)+1)
	if d.Framework != "" {
		fwOpts = append(fwOpts, ui.Option{Label: d.Framework + " (detected)", Value: d.Framework})
	}
	seen := map[string]bool{d.Framework: true}
	for _, s := range frameworks {
		if seen[s] {
			continue
		}
		seen[s] = true
		fwOpts = append(fwOpts, ui.Option{Label: s, Value: s})
	}
	d.Framework, err = ui.Select("Framework", fwOpts)
	if err != nil {
		return err
	}

	if isNoBuildFramework(d.Framework) {
		return nil
	}

	d.RootDirectory, err = ui.Text("Root directory", ".", "")
	if err != nil {
		return err
	}
	if d.RootDirectory == "." || d.RootDirectory == "./" {
		d.RootDirectory = ""
	}

	d.InstallCommand, err = ui.Text("Install command", firstNonEmptyStr(repo.InstallCommand, "npm install"), firstNonEmptyStr(repo.InstallCommand, "npm install"))
	if err != nil {
		return err
	}
	d.BuildCommand, err = ui.Text("Build command", firstNonEmptyStr(repo.BuildCommand, "npm run build"), firstNonEmptyStr(repo.BuildCommand, "npm run build"))
	if err != nil {
		return err
	}
	if d.ServiceType != "static" {
		d.StartCommand, err = ui.Text("Start command", firstNonEmptyStr(repo.StartCommand, "npm start"), firstNonEmptyStr(repo.StartCommand, "npm start"))
		if err != nil {
			return err
		}
	}
	d.OutputDirectory, err = ui.Text("Output directory (optional)", repo.OutputDirectory, repo.OutputDirectory)
	return err
}

type gitAccount struct {
	Label          string
	InstallationID int
}

type gitRepo struct {
	FullName        string
	Branch          string
	InstallationID  int
	Framework       string
	InstallCommand  string
	BuildCommand    string
	StartCommand    string
	OutputDirectory string
}

func loadGitAccounts(ctx context.Context, source, teamID string) ([]gitAccount, error) {
	var data any
	err := ui.WithSpinner("Loading "+source+" accounts…", func() error {
		var err error
		data, err = coreClient.ListGitAccounts(ctx, source, teamID)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Shape: { accounts: [...], authenticated: bool } or bare list
	var items []any
	if m, ok := data.(map[string]any); ok {
		if auth, ok := m["authenticated"].(bool); ok && !auth {
			return nil, nil
		}
		items = asSlice(m["accounts"])
		if items == nil {
			items = asSlice(unwrapList(data))
		}
	} else {
		items = asSlice(unwrapList(data))
	}

	out := make([]gitAccount, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		label := firstString(m, "name", "username", "login")
		id := parseInstallationID(m)
		if label == "" && id == 0 {
			continue
		}
		if label == "" {
			label = fmt.Sprintf("account %d", id)
		}
		out = append(out, gitAccount{Label: label, InstallationID: id})
	}
	return out, nil
}

func pickGitAccount(accounts []gitAccount) (gitAccount, error) {
	if len(accounts) == 1 {
		return accounts[0], nil
	}
	opts := make([]ui.Option, 0, len(accounts))
	byLabel := map[string]gitAccount{}
	for _, a := range accounts {
		opts = append(opts, ui.Option{Label: a.Label, Value: a.Label})
		byLabel[a.Label] = a
	}
	picked, err := ui.Select("Git account / org", opts)
	if err != nil {
		return gitAccount{}, err
	}
	return byLabel[picked], nil
}

func loadGitRepos(ctx context.Context, source string, installationID int, teamID string) ([]gitRepo, error) {
	q := client.InstallationQuery(installationID)
	// GitLab / Bitbucket query keys used by Core in some paths
	if source == "gitlab" && installationID > 0 {
		q.Set("groupId", strconv.Itoa(installationID))
	}
	if source == "bitbucket" && installationID > 0 {
		q.Set("workspaceId", strconv.Itoa(installationID))
	}
	q.Set("limit", "50")

	var data any
	err := ui.WithSpinner("Loading repositories…", func() error {
		var err error
		data, err = coreClient.ListRepos(ctx, source, q, teamID)
		return err
	})
	if err != nil {
		return nil, err
	}

	items := asSlice(unwrapList(data))
	if m, ok := data.(map[string]any); ok {
		if repos := asSlice(m["repositories"]); len(repos) > 0 {
			items = repos
		}
	}

	out := make([]gitRepo, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fullName := firstString(m, "full_name", "fullName", "path_with_namespace", "name")
		if fullName == "" {
			continue
		}
		r := gitRepo{
			FullName:       fullName,
			Branch:         firstString(m, "branch", "default_branch", "defaultBranch"),
			InstallationID: parseInstallationID(m),
		}
		if fw, ok := m["framework"].(map[string]any); ok {
			r.Framework = firstString(fw, "slug", "name")
			r.InstallCommand = firstString(fw, "installCommand")
			r.BuildCommand = firstString(fw, "buildCommand")
			r.StartCommand = firstString(fw, "startCommand")
			r.OutputDirectory = firstString(fw, "outputDirectory")
		} else if s := firstString(m, "framework"); s != "" {
			r.Framework = s
		}
		out = append(out, r)
	}
	return out, nil
}

func pickGitRepo(repos []gitRepo) (gitRepo, error) {
	opts := make([]ui.Option, 0, len(repos))
	byName := map[string]gitRepo{}
	for _, r := range repos {
		opts = append(opts, ui.Option{Label: r.FullName, Value: r.FullName})
		byName[r.FullName] = r
	}
	picked, err := ui.Select("Repository", opts)
	if err != nil {
		return gitRepo{}, err
	}
	return byName[picked], nil
}

func parseInstallationID(m map[string]any) int {
	for _, key := range []string{"installationId", "installation_id", "groupId", "workspaceId", "id"} {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			if t > 0 {
				return int(t)
			}
		case string:
			if n, err := strconv.Atoi(t); err == nil && n > 0 {
				return n
			}
		case int:
			if t > 0 {
				return t
			}
		}
	}
	return 0
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func fillSharedDraft(ctx context.Context, d *createDraft) error {
	regions, err := loadRegionOptions(ctx)
	if err != nil || len(regions) == 0 {
		d.RegionID, err = ui.Text("Region id", "", "")
		return err
	}
	d.RegionID, err = ui.Select("Region", regions)
	if err != nil {
		return err
	}

	cpuStr, err := ui.Text("CPU cores", "0.5", "0.5")
	if err != nil {
		return err
	}
	memStr, err := ui.Text("Memory (GB)", "0.5", "0.5")
	if err != nil {
		return err
	}
	if v, e := strconv.ParseFloat(strings.TrimSpace(cpuStr), 64); e == nil && v > 0 {
		d.CPU = v
	}
	if v, e := strconv.ParseFloat(strings.TrimSpace(memStr), 64); e == nil && v > 0 {
		d.Memory = v
	}
	return nil
}

func fillWorkerDraft(d *createDraft) error {
	cron, err := ui.Confirm("Run on a cron schedule?", false)
	if err != nil {
		return err
	}
	d.CronEnabled = cron
	if !cron {
		return nil
	}
	d.CronExpression, err = ui.Text("Cron expression", "0 * * * *", "0 * * * *")
	if err != nil {
		return err
	}
	d.CronTimezone, err = ui.Text("Timezone", "UTC", "UTC")
	return err
}

func fillEnvVars(d *createDraft) error {
	add, err := ui.Confirm("Add environment variables?", false)
	if err != nil {
		return err
	}
	for add {
		key, err := ui.Text("Variable name", "API_URL", "")
		if err != nil {
			return err
		}
		key = strings.TrimSpace(key)
		if key == "" {
			break
		}
		val, err := ui.Password("Value for "+key, "")
		if err != nil {
			return err
		}
		d.EnvVars = append(d.EnvVars, map[string]string{"name": key, "value": val})
		add, err = ui.Confirm("Add another variable?", false)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d createDraft) toPayload() map[string]any {
	git := strings.ToUpper(d.Source)
	if d.Source == "docker" {
		git = "DOCKER"
	}

	body := map[string]any{
		"name":        d.Name,
		"git":         git,
		"type":        "import",
		"serviceType": d.ServiceType,
		"framework":   d.Framework,
		"repo": map[string]any{
			"name":   d.RepoName,
			"branch": d.RepoBranch,
			"git":    git,
		},
		"configurations": map[string]any{
			"region":  d.RegionID,
			"cpu":     d.CPU,
			"memory":  d.Memory,
			"storage": 3,
		},
		"environments": d.EnvVars,
		"deploy":       d.Deploy,
		"authEnabled":  d.AuthEnabled,
	}

	if d.TeamID != "" {
		body["teamId"] = d.TeamID
	}
	if d.InstallationID > 0 {
		body["installationId"] = d.InstallationID
	}
	if d.InstallCommand != "" {
		body["installCommand"] = d.InstallCommand
	}
	if d.BuildCommand != "" {
		body["buildCommand"] = d.BuildCommand
	}
	if d.StartCommand != "" {
		body["startCommand"] = d.StartCommand
	}
	if d.OutputDirectory != "" {
		body["outputDirectory"] = d.OutputDirectory
	}
	if d.RootDirectory != "" {
		body["rootDirectory"] = d.RootDirectory
	}
	if d.PreStartCommand != "" {
		body["preStartCommand"] = d.PreStartCommand
	}
	if d.RegistryUser != "" && d.RegistryToken != "" {
		body["registry_credentials"] = map[string]string{
			"username": d.RegistryUser,
			"token":    d.RegistryToken,
		}
	}
	if d.CronEnabled {
		body["cronEnabled"] = true
		body["cronExpression"] = d.CronExpression
		body["cronTimezone"] = d.CronTimezone
	}
	return body
}

func loadFrameworkSlugs(ctx context.Context) ([]string, error) {
	var data any
	err := ui.WithSpinner("Loading frameworks…", func() error {
		var err error
		data, err = coreClient.GetFrameworks(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return slugsOnly(data), nil
}

func loadRegionOptions(ctx context.Context) ([]ui.Option, error) {
	var data any
	err := ui.WithSpinner("Loading regions…", func() error {
		var err error
		data, err = coreClient.GetRegions(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}

	items := asSlice(unwrapList(data))
	opts := make([]ui.Option, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := firstString(m, "_id", "id")
		name := firstString(m, "name", "slug", "code", "region")
		if id == "" && name == "" {
			continue
		}
		label := name
		if label == "" {
			label = id
		}
		value := id
		if value == "" {
			value = name
		}
		opts = append(opts, ui.Option{Label: label, Value: value})
	}
	return opts, nil
}

func isNoBuildFramework(framework string) bool {
	f := strings.ToLower(strings.TrimSpace(framework))
	return f == "html" || f == "static"
}

var nonSlug = regexp.MustCompile(`[^a-z0-9-]+`)

func slugifyName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}
