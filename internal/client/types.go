package client

import (
	"bytes"
	"encoding/json"
	"strings"
)

type Project struct {
	ID                        string       `json:"id"`
	RawID                     string       `json:"_id"`
	Name                      string       `json:"name"`
	Status                    string       `json:"status"`
	ServiceType               string       `json:"serviceType"`
	Framework                 string       `json:"framework"`
	Domain                    string       `json:"domain"`
	GitLink                   string       `json:"gitLink"`
	Public                    bool         `json:"public"`
	Maintenance               bool         `json:"maintenance"`
	HasUpdates                bool         `json:"hasUpdates"`
	Disabled                  bool         `json:"disabled"`
	PasswordEnabled           bool         `json:"passwordEnabled"`
	PreviewDeploymentsEnabled bool         `json:"previewDeploymentsEnabled"`
	DeployedInCluster         bool         `json:"deployedInCluster"`
	ReplicaReady              bool         `json:"replica_ready"`
	DiskSize                  float64      `json:"diskSize"`
	CreatedAt                 string       `json:"createdAt"`
	UpdatedAt                 string       `json:"updatedAt"`
	InstallCommand            string       `json:"installCommand"`
	BuildCommand              string       `json:"buildCommand"`
	StartCommand              string       `json:"startCommand"`
	RootDirectory             string       `json:"rootDirectory"`
	OutputDirectory           string       `json:"outputDirectory"`
	Team                      *Named       `json:"team"`
	Repo                      *Repo        `json:"repo"`
	Specs                     *Specs       `json:"specs"`
	Log                       *CommitLog   `json:"log"`
	Health                    HealthStatus `json:"healthStatus"`
	Domains                   []Named      `json:"domains"`
	Environments              []Named      `json:"environments"`
}

func (p Project) ProjectID() string {
	if p.ID != "" {
		return p.ID
	}
	return p.RawID
}

type ProjectList struct {
	Projects      []Project `json:"projects"`
	TotalProjects int       `json:"totalProjects"`
	TotalPages    int       `json:"totalPages"`
	CurrentPage   int       `json:"currentPage"`
}

type Named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Repo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Branch   string `json:"branch"`
}

func (r Repo) Label() string {
	if r.FullName != "" {
		return r.FullName
	}
	if r.Name != "" {
		return r.Name
	}
	return "a repository"
}

type Specs struct {
	CPU    float64      `json:"cpu"`
	Memory float64      `json:"memory"`
	Region StringOrName `json:"region"`
}

type CommitLog struct {
	Message string `json:"message"`
}

// HealthStatus is a status string or {status, statusCode}.
type HealthStatus struct {
	Status     string
	StatusCode int
}

func (h *HealthStatus) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		return json.Unmarshal(b, &h.Status)
	}
	var obj struct {
		Status     string `json:"status"`
		StatusCode int    `json:"statusCode"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	h.Status = obj.Status
	h.StatusCode = obj.StatusCode
	return nil
}

func (h HealthStatus) MarshalJSON() ([]byte, error) {
	if h.StatusCode == 0 {
		return json.Marshal(h.Status)
	}
	return json.Marshal(struct {
		Status     string `json:"status"`
		StatusCode int    `json:"statusCode"`
	}{h.Status, h.StatusCode})
}

// StringOrName is a JSON string or {"name": "…"}.
type StringOrName struct {
	Name string
}

func (s *StringOrName) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		return json.Unmarshal(b, &s.Name)
	}
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	s.Name = obj.Name
	return nil
}

func (s StringOrName) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Name)
}

type CreateResult struct {
	ID      string   `json:"_id"`
	Name    string   `json:"name"`
	LogID   string   `json:"logId"`
	Project *Project `json:"project"`
}

type DeployResult struct {
	LogID string `json:"logId"`
}

type CreateProjectRequest struct {
	Name                string               `json:"name"`
	Git                 string               `json:"git"`
	Type                string               `json:"type"`
	ServiceType         string               `json:"serviceType"`
	Framework           string               `json:"framework"`
	Repo                CreateRepo           `json:"repo"`
	Configurations      CreateConfigurations `json:"configurations"`
	Environments        []EnvVar             `json:"environments"`
	Deploy              bool                 `json:"deploy"`
	AuthEnabled         *bool                `json:"authEnabled,omitempty"`
	TeamID              string               `json:"teamId,omitempty"`
	InstallationID      int                  `json:"installationId,omitempty"`
	InstallCommand      string               `json:"installCommand,omitempty"`
	BuildCommand        string               `json:"buildCommand,omitempty"`
	StartCommand        string               `json:"startCommand,omitempty"`
	OutputDirectory     string               `json:"outputDirectory,omitempty"`
	RootDirectory       string               `json:"rootDirectory,omitempty"`
	PreStartCommand     string               `json:"preStartCommand,omitempty"`
	RegistryCredentials *RegistryCredentials `json:"registry_credentials,omitempty"`
	CronEnabled         bool                 `json:"cronEnabled,omitempty"`
	CronExpression      string               `json:"cronExpression,omitempty"`
	CronTimezone        string               `json:"cronTimezone,omitempty"`
}

type CreateRepo struct {
	Name           string `json:"name"`
	Branch         string `json:"branch"`
	Git            string `json:"git"`
	InstallationID int    `json:"installationId,omitempty"`
}

type CreateConfigurations struct {
	CPU     float64 `json:"cpu"`
	Memory  float64 `json:"memory"`
	Storage int     `json:"storage"`
	Region  string  `json:"region,omitempty"`
}

type RegistryCredentials struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

type LinkRepoRequest struct {
	Type string       `json:"type"`
	Repo LinkRepoSpec `json:"repo"`
}

type LinkRepoSpec struct {
	Name           string `json:"name"`
	Branch         string `json:"branch"`
	Git            string `json:"git"`
	InstallationID int    `json:"installationId,omitempty"`
}

type RedeployRequest struct {
	StartOnly bool   `json:"startOnly,omitempty"`
	LogID     string `json:"logId,omitempty"`
}

type EnvList struct {
	Envs []Env `json:"envs"`
}

type Env struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Environment string `json:"environment"`
	Source      string `json:"source"`
	UpdatedAt   string `json:"updatedAt"`
}

type DeployLogList struct {
	Logs        []DeployLog `json:"logs"`
	TotalPages  int         `json:"totalPages"`
	CurrentPage int         `json:"currentPage"`
}

type DeployLog struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Branch      string `json:"branch"`
	Username    string `json:"username"`
	Environment string `json:"environment"`
	CreatedAt   string `json:"createdAt"`
}

type RequestLogList struct {
	Logs []RequestLog `json:"logs"`
}

type RequestLog struct {
	Status    int    `json:"status"`
	URL       string `json:"url"`
	Method    string `json:"method"`
	Timestamp string `json:"timestamp"`
}

type Team struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	Description  string  `json:"description"`
	TotalMembers float64 `json:"totalMembers"`
	CreatedAt    string  `json:"createdAt"`
}

type TeamMember struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Accepted  bool   `json:"accepted"`
	IsMe      bool   `json:"isMe"`
}

func (m TeamMember) DisplayName() string {
	return strings.TrimSpace(m.FirstName + " " + m.LastName)
}

type DomainList struct {
	Domains []Domain `json:"domains"`
}

type Domain struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Project string `json:"project"`
	Primary bool   `json:"primary"`
	Active  bool   `json:"active"`
	Custom  bool   `json:"isCustom"`
}

type Environment struct {
	ID        string `json:"_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	IsDefault bool   `json:"isDefault"`
}

type User struct {
	Email     string `json:"email"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Name      string `json:"name"`
	Company   string `json:"company"`
}

func (u User) DisplayName() string {
	if u.Email != "" {
		return u.Email
	}
	return u.Username
}

func (u User) FullName() string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name != "" {
		return name
	}
	return u.Name
}

type GitAccounts struct {
	Authenticated bool         `json:"authenticated"`
	Accounts      []GitAccount `json:"accounts"`
}

type GitAccount struct {
	Username       string `json:"username"`
	Name           string `json:"name"`
	InstallationID int    `json:"installationId"`
}

type RepoList struct {
	Repositories []GitRepo `json:"repositories"`
}

type GitRepo struct {
	FullName       string     `json:"full_name"`
	Branch         string     `json:"branch"`
	InstallationID int        `json:"installationId"`
	Framework      *Framework `json:"framework"`
}

type Framework struct {
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	InstallCommand  string `json:"installCommand"`
	BuildCommand    string `json:"buildCommand"`
	StartCommand    string `json:"startCommand"`
	OutputDirectory string `json:"outputDirectory"`
}

type RegionCatalog struct {
	Providers []RegionProvider `json:"providers"`
}

func (c RegionCatalog) Regions() []Region {
	var out []Region
	for _, p := range c.Providers {
		out = append(out, p.Regions...)
	}
	return out
}

type RegionProvider struct {
	Name    string   `json:"name"`
	Regions []Region `json:"regions"`
}

type Region struct {
	ID        string `json:"_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Country   string `json:"country"`
	Provider  string `json:"provider"`
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
	Eligible  *bool  `json:"eligible"`
	Default   bool   `json:"default"`
}

type ProjectStats struct {
	ServiceType     string          `json:"serviceType"`
	Live            bool            `json:"live"`
	Supported       bool            `json:"supported"`
	Range           StatsRange      `json:"range"`
	Summary         StatsSummary    `json:"summary"`
	WorkerExecution WorkerExecution `json:"workerExecution"`
}

type StatsRange struct {
	From string      `json:"from"`
	To   string      `json:"to"`
	Step json.Number `json:"step"`
}

type StatsSummary struct {
	CPU                      Metric  `json:"cpu"`
	Memory                   Metric  `json:"memory"`
	Network                  Metric  `json:"network"`
	Disk                     Metric  `json:"disk"`
	OOMKills                 Metric  `json:"oomKills"`
	RequestsPerSecond        Metric  `json:"requestsPerSecond"`
	ErrorRate                Metric  `json:"errorRate"`
	AverageResponseTimeMs    Metric  `json:"averageResponseTimeMs"`
	AverageResponseSizeBytes Metric  `json:"averageResponseSizeBytes"`
	ResponseTime             Latency `json:"responseTime"`
}

type Latency struct {
	Avg Metric `json:"avg"`
	P50 Metric `json:"p50"`
	P90 Metric `json:"p90"`
	P95 Metric `json:"p95"`
	P99 Metric `json:"p99"`
}

type WorkerExecution struct {
	Supported         bool   `json:"supported"`
	Executions        Metric `json:"executions"`
	Failures          Metric `json:"failures"`
	AverageDurationMs Metric `json:"averageDurationMs"`
}

// Metric is a stats point. Value is nil when the API omitted it.
type Metric struct {
	Value *float64 `json:"value"`
	Delta *float64 `json:"delta"`
}
