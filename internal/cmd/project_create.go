package cmd

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
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
	EnvVars         []client.EnvVar
	Deploy          bool
	TeamID          string
	// UploadDir is set for the "upload a folder" source (static drop project).
	UploadDir string
}

const sourceUpload = "upload"

type createOptions struct {
	offerLink bool
	detach    bool
	noLogs    bool
}

// runCreate creates a project from body (a Core JSON payload) or, when body
// is nil, from the interactive questionnaire, then follows the first deploy.
func runCreate(ctx context.Context, body json.RawMessage, opts createOptions) error {
	var draft *createDraft
	if len(body) == 0 {
		d, err := runCreateQuestionnaire(ctx, cfg.TeamID)
		if err != nil {
			return err
		}
		draft = d
	}

	var ref ProjectRef
	var logID string
	if draft != nil && draft.Source == sourceUpload {
		var err error
		logID, err = uploadDrop(ctx, client.DropOpts{Name: draft.Name, TeamID: cfg.TeamID}, draft.UploadDir, draft.Name)
		if err != nil {
			return err
		}
		ref, err = fetchProject(ctx, draft.Name)
		if err != nil {
			return err
		}
	} else {
		payload := body
		if draft != nil {
			var err error
			payload, err = json.Marshal(draft.toPayload())
			if err != nil {
				return err
			}
		}
		var created client.CreateResult
		err := ui.WithSpinner("Creating project…", func() error {
			var err error
			created, err = coreClient.CreateProject(ctx, payload, cfg.TeamID)
			return err
		})
		if err != nil {
			return err
		}
		if jsonOutput() {
			return printResult(created)
		}
		ref = ProjectRef{ID: created.ID, Name: created.Name, Project: created.Project}
		if ref.Name == "" && draft != nil {
			ref.Name = draft.Name
		}
		if ref.Name == "" {
			var named struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(body, &named)
			ref.Name = named.Name
		}
		logID = created.LogID
	}

	ui.Success("Created %s", ref.Display())
	if opts.offerLink || draft != nil {
		offerLink(ref)
	}
	if logID == "" {
		ui.Hint("Deploy it with `brimble deploy %s`", ref.Name)
		return nil
	}
	return afterDeployStarted(ctx, ref, logID, opts.detach, !opts.noLogs)
}

func runCreateQuestionnaire(ctx context.Context, teamID string) (*createDraft, error) {
	if !ui.IsInteractive() {
		return nil, fmt.Errorf("not a terminal; pass --file with a JSON body")
	}

	ui.Title("Create project")

	defaultName := ""
	if wd, err := os.Getwd(); err == nil {
		defaultName = slugifyName(lastPathPart(wd))
	}
	name, err := ui.Text("Project name", cmp.Or(defaultName, "my-app"), defaultName)
	if err != nil {
		return nil, err
	}
	name = slugifyName(name)
	if len(name) < 3 {
		return nil, fmt.Errorf("project name must be at least 3 characters (letters, numbers, dashes)")
	}

	source, err := ui.Select("Deploy from", []ui.Option{
		{Label: "GitHub repository", Value: "github"},
		{Label: "GitLab repository", Value: "gitlab"},
		{Label: "Bitbucket repository", Value: "bitbucket"},
		{Label: "Docker image", Value: "docker"},
		{Label: "Upload a local folder (static site)", Value: sourceUpload},
	})
	if err != nil {
		return nil, err
	}

	draft := &createDraft{
		Name:   name,
		Source: source,
		CPU:    0.5,
		Memory: 0.5,
		Deploy: true,
		TeamID: teamID,
	}

	if source == sourceUpload {
		draft.ServiceType = "static"
		draft.UploadDir, err = ui.Text("Folder to upload", ".", ".")
		if err != nil {
			return nil, err
		}
		if st, err := os.Stat(draft.UploadDir); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("%s is not a folder", draft.UploadDir)
		}
		return draft, nil
	}

	serviceOpts := []ui.Option{
		{Label: "Web service — APIs and server-rendered apps", Value: "web-service"},
		{Label: "Static site — HTML/CSS/JS, no server", Value: "static"},
		{Label: "Worker — background jobs or cron, no public URL", Value: "worker"},
		{Label: "MCP server — tools for AI agents", Value: "mcp"},
	}
	if source == "docker" {
		serviceOpts = append(serviceOpts[:1], serviceOpts[2:]...)
	}
	draft.ServiceType, err = ui.Select("Service type", serviceOpts)
	if err != nil {
		return nil, err
	}

	if source == "docker" {
		err = fillDockerDraft(ctx, draft)
	} else {
		err = fillGitDraft(ctx, draft)
	}
	if err != nil {
		return nil, err
	}

	if err := fillSharedDraft(ctx, draft); err != nil {
		return nil, err
	}

	if draft.ServiceType == "worker" {
		if err := fillWorkerDraft(draft); err != nil {
			return nil, err
		}
	}
	if draft.ServiceType == "mcp" {
		draft.AuthEnabled, err = ui.Confirm("Require a Brimble API key to call this MCP server?", false)
		if err != nil {
			return nil, err
		}
	}

	if err := fillEnvVars(draft); err != nil {
		return nil, err
	}

	draft.Deploy, err = ui.Confirm("Deploy now?", true)
	if err != nil {
		return nil, err
	}
	return draft, nil
}

func fillDockerDraft(ctx context.Context, d *createDraft) error {
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

	private, err := ui.Confirm("Is the image in a private registry?", false)
	if err != nil {
		return err
	}
	if private {
		d.RegistryUser, err = ui.Text("Registry username", "", "")
		if err != nil {
			return err
		}
		d.RegistryToken, err = ui.Password("Registry token or password", "")
		if err != nil {
			return err
		}
	}

	var valid bool
	err = ui.WithSpinner("Checking image…", func() error {
		var err error
		valid, err = coreClient.ValidateImage(ctx, image, d.RegistryUser, d.RegistryToken, d.TeamID)
		return err
	})
	if err != nil || !valid {
		reason := "the image could not be found or pulled"
		if err != nil {
			reason = err.Error()
		}
		ui.Warn("Could not verify %s: %s", image, reason)
		ok, err := ui.Confirm("Continue anyway?", false)
		if err != nil {
			return err
		}
		if !ok {
			return errAborted
		}
	}

	d.PreStartCommand, err = ui.Text("Pre-start command, e.g. migrations (optional)", "", "")
	return err
}

func fillGitDraft(ctx context.Context, d *createDraft) error {
	accounts, err := loadGitAccounts(ctx, d.Source, d.TeamID)
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		return fmt.Errorf("%s is not connected to Brimble — connect it at https://app.brimble.io, then try again", gitLabel(d.Source))
	}

	account, err := pickGitAccount(accounts)
	if err != nil {
		return err
	}
	d.InstallationID = account.InstallationID

	search, err := ui.Text("Search repositories (leave empty to list recent)", "", "")
	if err != nil {
		return err
	}
	repos, err := loadGitRepos(ctx, d.Source, d.InstallationID, strings.TrimSpace(search), d.TeamID)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repositories found for this %s account", gitLabel(d.Source))
	}

	repo, err := pickGitRepo(repos)
	if err != nil {
		return err
	}
	d.RepoName = repo.FullName
	d.RepoBranch = cmp.Or(repo.Branch, "main")
	if repo.InstallationID > 0 {
		d.InstallationID = repo.InstallationID
	}

	branch, err := ui.Text("Production branch", d.RepoBranch, d.RepoBranch)
	if err != nil {
		return err
	}
	if strings.TrimSpace(branch) != "" {
		d.RepoBranch = strings.TrimSpace(branch)
	}

	frameworks, err := loadFrameworks(ctx)
	if err != nil {
		return err
	}
	detected := ""
	if repo.Framework != nil {
		detected = repo.Framework.Slug
	}
	fw, err := pickFramework(frameworks, detected)
	if err != nil {
		return err
	}
	d.Framework = fw.Slug

	if isNoBuildFramework(d.Framework) {
		return nil
	}

	install, build, start, output := fw.InstallCommand, fw.BuildCommand, fw.StartCommand, fw.OutputDirectory
	if repo.Framework != nil {
		install = cmp.Or(repo.Framework.InstallCommand, install)
		build = cmp.Or(repo.Framework.BuildCommand, build)
		start = cmp.Or(repo.Framework.StartCommand, start)
		output = cmp.Or(repo.Framework.OutputDirectory, output)
	}

	d.RootDirectory, err = ui.Text("Root directory (for monorepos)", ".", "")
	if err != nil {
		return err
	}
	if d.RootDirectory == "." || d.RootDirectory == "./" {
		d.RootDirectory = ""
	}
	if d.Framework == "docker" {
		return nil
	}

	if d.InstallCommand, err = ui.Text("Install command", install, install); err != nil {
		return err
	}
	if d.BuildCommand, err = ui.Text("Build command", build, build); err != nil {
		return err
	}
	if d.ServiceType == "static" {
		d.OutputDirectory, err = ui.Text("Output directory", cmp.Or(output, "dist"), cmp.Or(output, "dist"))
		return err
	}
	d.StartCommand, err = ui.Text("Start command", start, start)
	return err
}

func gitLabel(source string) string {
	switch source {
	case "github":
		return "GitHub"
	case "gitlab":
		return "GitLab"
	case "bitbucket":
		return "Bitbucket"
	}
	return source
}

func loadGitAccounts(ctx context.Context, source, teamID string) ([]client.GitAccount, error) {
	var data client.GitAccounts
	err := ui.WithSpinner("Loading "+gitLabel(source)+" accounts…", func() error {
		var err error
		data, err = coreClient.ListGitAccounts(ctx, source, teamID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !data.Authenticated && len(data.Accounts) == 0 {
		return nil, nil
	}
	out := make([]client.GitAccount, 0, len(data.Accounts))
	for _, a := range data.Accounts {
		if a.Username != "" || a.Name != "" {
			out = append(out, a)
		}
	}
	return out, nil
}

func pickGitAccount(accounts []client.GitAccount) (client.GitAccount, error) {
	if len(accounts) == 1 {
		return accounts[0], nil
	}
	opts := make([]ui.Option, 0, len(accounts))
	byKey := map[string]client.GitAccount{}
	for _, a := range accounts {
		label := a.Username
		if label == "" {
			label = a.Name
		} else if a.Name != "" && a.Name != a.Username {
			label = a.Name + " (@" + a.Username + ")"
		}
		opts = append(opts, ui.Option{Label: label, Value: label})
		byKey[label] = a
	}
	picked, err := ui.Select("Git account / org", opts)
	if err != nil {
		return client.GitAccount{}, err
	}
	return byKey[picked], nil
}

func loadGitRepos(ctx context.Context, source string, installationID int, search, teamID string) ([]client.GitRepo, error) {
	q := url.Values{}
	q.Set("limit", "50")
	if installationID > 0 {
		q.Set("installationId", strconv.Itoa(installationID))
	}
	if search != "" {
		q.Set("q", search)
	}

	var data client.RepoList
	err := ui.WithSpinner("Loading repositories…", func() error {
		var err error
		data, err = coreClient.ListRepos(ctx, source, q, teamID)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]client.GitRepo, 0, len(data.Repositories))
	for _, r := range data.Repositories {
		if r.FullName != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

func pickGitRepo(repos []client.GitRepo) (client.GitRepo, error) {
	opts := make([]ui.Option, 0, len(repos))
	byName := map[string]client.GitRepo{}
	for _, r := range repos {
		opts = append(opts, ui.Option{Label: r.FullName, Value: r.FullName})
		byName[r.FullName] = r
	}
	picked, err := ui.Select("Repository", opts)
	if err != nil {
		return client.GitRepo{}, err
	}
	return byName[picked], nil
}

func fillSharedDraft(ctx context.Context, d *createDraft) error {
	regions, defaultRegion, err := loadRegionOptions(ctx)
	if err != nil {
		return err
	}
	switch {
	case len(regions) == 0 && d.ServiceType == "static":
		// Region is optional for static sites.
	case len(regions) == 0:
		return fmt.Errorf("no regions are available for your plan")
	default:
		// Put the plan's default region first so Enter picks it.
		for i, r := range regions {
			if r.Value == defaultRegion && i > 0 {
				regions[0], regions[i] = regions[i], regions[0]
			}
		}
		if d.RegionID, err = ui.Select("Region", regions); err != nil {
			return err
		}
	}

	if d.ServiceType == "static" {
		return nil
	}
	cpuStr, err := ui.Text("CPU (vCPUs)", "0.5", "0.5")
	if err != nil {
		return err
	}
	memStr, err := ui.Text("Memory (GB)", "0.5", "0.5")
	if err != nil {
		return err
	}
	if d.CPU, err = parsePositive(cpuStr, "CPU"); err != nil {
		return err
	}
	if d.Memory, err = parsePositive(memStr, "memory"); err != nil {
		return err
	}
	return nil
}

func parsePositive(s, label string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive number, got %q", label, s)
	}
	return v, nil
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
	d.CronTimezone, err = ui.Text("Timezone (IANA)", "UTC", "UTC")
	return err
}

func fillEnvVars(d *createDraft) error {
	if vars, err := readDotenv(".env"); err == nil && len(vars) > 0 {
		use, err := ui.Confirm(fmt.Sprintf("Add the %d variables from ./.env?", len(vars)), true)
		if err != nil {
			return err
		}
		if use {
			d.EnvVars = append(d.EnvVars, vars...)
		}
	}

	add, err := ui.Confirm("Add environment variables manually?", false)
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
		d.EnvVars = append(d.EnvVars, client.EnvVar{Name: key, Value: val})
		add, err = ui.Confirm("Add another variable?", false)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d createDraft) toPayload() client.CreateProjectRequest {
	git := strings.ToUpper(d.Source)
	envs := d.EnvVars
	if envs == nil {
		envs = []client.EnvVar{}
	}
	req := client.CreateProjectRequest{
		Name:        d.Name,
		Git:         git,
		Type:        "import",
		ServiceType: d.ServiceType,
		Framework:   d.Framework,
		Repo: client.CreateRepo{
			Name:           d.RepoName,
			Branch:         d.RepoBranch,
			Git:            git,
			InstallationID: d.InstallationID,
		},
		Configurations: client.CreateConfigurations{
			CPU:     d.CPU,
			Memory:  d.Memory,
			Storage: 3,
			Region:  d.RegionID,
		},
		Environments:    envs,
		Deploy:          d.Deploy,
		TeamID:          d.TeamID,
		InstallationID:  d.InstallationID,
		InstallCommand:  d.InstallCommand,
		BuildCommand:    d.BuildCommand,
		StartCommand:    d.StartCommand,
		OutputDirectory: d.OutputDirectory,
		RootDirectory:   d.RootDirectory,
		PreStartCommand: d.PreStartCommand,
		CronEnabled:     d.CronEnabled,
		CronExpression:  d.CronExpression,
		CronTimezone:    d.CronTimezone,
	}
	if d.ServiceType == "mcp" {
		req.AuthEnabled = &d.AuthEnabled
	}
	if d.RegistryUser != "" && d.RegistryToken != "" {
		req.RegistryCredentials = &client.RegistryCredentials{
			Username: d.RegistryUser,
			Token:    d.RegistryToken,
		}
	}
	return req
}

func loadFrameworks(ctx context.Context) ([]client.Framework, error) {
	var data []client.Framework
	err := ui.WithSpinner("Loading frameworks…", func() error {
		var err error
		data, err = coreClient.GetFrameworks(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]client.Framework, 0, len(data))
	for _, f := range data {
		if f.Slug != "" {
			out = append(out, f)
		}
	}
	return out, nil
}

func pickFramework(frameworks []client.Framework, detected string) (client.Framework, error) {
	if len(frameworks) == 0 {
		return client.Framework{}, fmt.Errorf("no frameworks available from Brimble")
	}
	bySlug := map[string]client.Framework{}
	opts := make([]ui.Option, 0, len(frameworks))
	for _, f := range frameworks {
		bySlug[f.Slug] = f
		label := cmp.Or(f.Name, f.Slug)
		if f.Slug == detected {
			opts = append([]ui.Option{{Label: label + " (detected)", Value: f.Slug}}, opts...)
			continue
		}
		opts = append(opts, ui.Option{Label: label, Value: f.Slug})
	}
	slug, err := ui.Select("Framework", opts)
	if err != nil {
		return client.Framework{}, err
	}
	return bySlug[slug], nil
}

func loadRegionOptions(ctx context.Context) ([]ui.Option, string, error) {
	var catalog client.RegionCatalog
	err := ui.WithSpinner("Loading regions…", func() error {
		var err error
		catalog, err = coreClient.GetRegions(ctx)
		return err
	})
	if err != nil {
		return nil, "", err
	}

	var opts []ui.Option
	defaultID := ""
	for _, r := range catalog.Regions() {
		if r.ID == "" || r.Name == "" || r.Type != "web" {
			continue
		}
		if !r.Enabled || !r.Available {
			continue
		}
		if r.Eligible != nil && !*r.Eligible {
			continue
		}
		label := r.Name
		if r.Country != "" {
			label += " · " + r.Country
		}
		if r.Default {
			label += " (default)"
			defaultID = r.ID
		}
		opts = append(opts, ui.Option{Label: label, Value: r.ID})
	}
	return opts, defaultID, nil
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

func lastPathPart(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
