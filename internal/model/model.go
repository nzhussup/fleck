package model

import "time"

type SourceLocation struct {
	File   string `json:"file,omitempty" yaml:"file,omitempty"`
	Line   int    `json:"line,omitempty" yaml:"line,omitempty"`
	Column int    `json:"column,omitempty" yaml:"column,omitempty"`
}

type Evidence struct {
	Kind        string            `json:"kind"`
	Description string            `json:"description"`
	Location    SourceLocation    `json:"location,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Path        []string          `json:"path,omitempty"`
}

type Requirement string

const (
	RequirementRequired Requirement = "required"
	RequirementOptional Requirement = "optional"
	RequirementUnknown  Requirement = "unknown"
)

type EnvRead struct {
	ID          string         `json:"id"`
	Application string         `json:"application"`
	Name        string         `json:"name,omitempty"`
	Resolved    bool           `json:"resolved"`
	Requirement Requirement    `json:"requirement"`
	Evidence    []Evidence     `json:"evidence,omitempty"`
	Source      SourceLocation `json:"source"`
}

type SupplyState string

const (
	SupplyDefinite         SupplyState = "definite"
	SupplyUnverified       SupplyState = "unverified"
	SupplyMissingOptional  SupplyState = "missing_optional"
	SupplyMissingMandatory SupplyState = "missing_mandatory"
)

type SupplySource string

const (
	SupplyKubernetes SupplySource = "kubernetes"
	SupplyConfigMap  SupplySource = "configmap"
	SupplySecret     SupplySource = "secret"
	SupplyDockerfile SupplySource = "dockerfile"
)

type EnvSupply struct {
	ID         string         `json:"id"`
	Container  string         `json:"container"`
	Name       string         `json:"name,omitempty"`
	Value      string         `json:"value,omitempty"`
	ValueKnown bool           `json:"valueKnown"`
	State      SupplyState    `json:"state"`
	SourceKind SupplySource   `json:"sourceKind"`
	Reference  string         `json:"reference,omitempty"`
	Optional   bool           `json:"optional,omitempty"`
	Location   SourceLocation `json:"location,omitempty"`
	Sensitive  bool           `json:"sensitive,omitempty"`
}

type ListenPort struct {
	ID          string         `json:"id"`
	Application string         `json:"application"`
	Port        int            `json:"port,omitempty"`
	Resolved    bool           `json:"resolved"`
	EnvName     string         `json:"envName,omitempty"`
	DefaultPort int            `json:"defaultPort,omitempty"`
	Expression  string         `json:"expression,omitempty"`
	Source      SourceLocation `json:"source"`
}

type ContainerPort struct {
	Name     string `json:"name,omitempty"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type Container struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Workload string          `json:"workload"`
	Supplies []EnvSupply     `json:"supplies"`
	Ports    []ContainerPort `json:"ports,omitempty"`
}

type Workload struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels,omitempty"`
	Containers []Container       `json:"containers"`
}

type ServicePort struct {
	Name            string         `json:"name,omitempty"`
	Port            int            `json:"port"`
	TargetPortName  string         `json:"targetPortName,omitempty"`
	TargetPort      int            `json:"targetPort,omitempty"`
	TargetResolved  bool           `json:"targetResolved"`
	TargetContainer string         `json:"targetContainer,omitempty"`
	TargetAmbiguous bool           `json:"targetAmbiguous,omitempty"`
	Protocol        string         `json:"protocol"`
	Location        SourceLocation `json:"location,omitempty"`
}

type Service struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Selector  map[string]string `json:"selector,omitempty"`
	Ports     []ServicePort     `json:"ports"`
}

type DockerFacts struct {
	Environment  []EnvSupply    `json:"environment,omitempty"`
	ExposedPorts []int          `json:"exposedPorts,omitempty"`
	Entrypoint   []string       `json:"entrypoint,omitempty"`
	Command      []string       `json:"command,omitempty"`
	Location     SourceLocation `json:"location,omitempty"`
}

type Facts struct {
	ApplicationID     string             `json:"applicationId,omitempty"`
	ApplicationPath   string             `json:"applicationPath,omitempty"`
	EnvReads          []EnvRead          `json:"envReads"`
	ListenPorts       []ListenPort       `json:"listenPorts"`
	Workload          Workload           `json:"workload"`
	Container         Container          `json:"container"`
	Services          []Service          `json:"services"`
	Docker            DockerFacts        `json:"docker"`
	Diagnostics       []Diagnostic       `json:"diagnostics,omitempty"`
	InvalidReferences []InvalidReference `json:"invalidReferences,omitempty"`
	Layers            LayerAvailability  `json:"layers"`
	Capabilities      CapabilitySet      `json:"capabilities"`
}

type LayerAvailability struct {
	Application bool `json:"application"`
	Deployment  bool `json:"deployment"`
	Dockerfile  bool `json:"dockerfile"`
}

type Capability string

const (
	CapabilityEnvReads       Capability = "env_reads"
	CapabilityEnvSupplies    Capability = "env_supplies"
	CapabilityDeploymentEnv  Capability = "deployment_env"
	CapabilityAppListenPorts Capability = "app_listen_ports"
	CapabilityContainerPorts Capability = "container_ports"
	CapabilityServicePorts   Capability = "service_ports"
	CapabilityDockerEnv      Capability = "docker_env"
	CapabilityDockerPorts    Capability = "docker_ports"
)

type CapabilitySet map[Capability]bool

func (set CapabilitySet) Has(capability Capability) bool { return set[capability] }

type SkippedCheck struct {
	RuleID   string       `json:"ruleId"`
	RuleName string       `json:"ruleName"`
	Reason   string       `json:"reason"`
	Missing  []Capability `json:"missingCapabilities"`
}

type Diagnostic struct {
	Level    string         `json:"level"`
	Message  string         `json:"message"`
	Location SourceLocation `json:"location,omitempty"`
}

type InvalidReference struct {
	Kind      string         `json:"kind"`
	Subject   string         `json:"subject"`
	Reference string         `json:"reference"`
	Message   string         `json:"message"`
	Location  SourceLocation `json:"location,omitempty"`
}

type Severity string
type Confidence string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

func SeverityRank(s Severity) int {
	switch s {
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	default:
		return 1
	}
}

type Finding struct {
	RuleID         string           `json:"ruleId"`
	RuleName       string           `json:"ruleName"`
	Severity       Severity         `json:"severity"`
	Confidence     Confidence       `json:"confidence"`
	CorrelationKey string           `json:"correlationKey,omitempty"`
	Message        string           `json:"message"`
	Subject        string           `json:"subject,omitempty"`
	Location       SourceLocation   `json:"location,omitempty"`
	Related        []SourceLocation `json:"related,omitempty"`
	Evidence       []Evidence       `json:"evidence,omitempty"`
	Suggestion     string           `json:"suggestion,omitempty"`
}

type Timings struct {
	GoExtraction         time.Duration `json:"goExtraction"`
	HelmRendering        time.Duration `json:"helmRendering"`
	KubernetesExtraction time.Duration `json:"kubernetesExtraction"`
	DockerExtraction     time.Duration `json:"dockerExtraction"`
	GraphConstruction    time.Duration `json:"graphConstruction"`
	Detection            time.Duration `json:"detection"`
	Total                time.Duration `json:"total"`
}
