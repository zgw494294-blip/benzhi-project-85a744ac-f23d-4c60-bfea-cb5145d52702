package rigging

import "time"

type PlanStatus string

const (
	StatusDraft       PlanStatus = "draft"
	StatusTesting     PlanStatus = "testing"
	StatusRemediation PlanStatus = "remediation"
	StatusApproved    PlanStatus = "approved"
	StatusReturned    PlanStatus = "returned"
)

type RigPlan struct {
	ID               string                `json:"id"`
	VenueName        string                `json:"venueName"`
	PerformanceDate  string                `json:"performanceDate"`
	RatedTotalLoadKg float64               `json:"ratedTotalLoadKg"`
	OwnerName        string                `json:"ownerName"`
	Status           PlanStatus            `json:"status"`
	Version          int64                 `json:"version"`
	CreatedAt        time.Time             `json:"createdAt"`
	FrozenAt         *time.Time            `json:"frozenAt,omitempty"`
	FrozenDigest     string                `json:"frozenDigest,omitempty"`
	Points           []SuspensionPoint     `json:"points"`
	Tests            []LoadTest            `json:"tests"`
	Issues           []SafetyIssue         `json:"issues"`
	Revisions        []RemediationRevision `json:"revisions"`
	PlanRevisions    []PlanRevision        `json:"planRevisions"`
	RetestTasks      []RetestTask          `json:"retestTasks"`
	Reviews          []ReviewDecision      `json:"reviews"`
	Credentials      []ClearanceCredential `json:"credentials"`
	FrozenPoints     []SuspensionPoint     `json:"frozenPoints,omitempty"`
	FrozenTests      []LoadTest            `json:"frozenTests,omitempty"`
}

type SuspensionPoint struct {
	ID                   string  `json:"id"`
	PlanID               string  `json:"planId"`
	Label                string  `json:"label"`
	RatedLoadKg          float64 `json:"ratedLoadKg"`
	PlannedLoadKg        float64 `json:"plannedLoadKg"`
	DeviceModel          string  `json:"deviceModel"`
	CableSpec            string  `json:"cableSpec"`
	PrimaryPointID       string  `json:"primaryPointId,omitempty"`
	RedundantPointID     string  `json:"redundantPointId,omitempty"`
	CertificateExpiresOn string  `json:"certificateExpiresOn"`
	ConfigRevision       int64   `json:"configRevision"`
	ConfigDigest         string  `json:"configDigest"`
}

type TestKind string
type TestOutcome string

const (
	TestInitial TestKind    = "initial"
	TestRetest  TestKind    = "retest"
	OutcomePass TestOutcome = "pass"
	OutcomeFail TestOutcome = "fail"
)

type LoadTest struct {
	ID                   string      `json:"id"`
	PlanID               string      `json:"planId"`
	PointID              string      `json:"pointId"`
	TestKind             TestKind    `json:"testKind"`
	TargetLoadKg         float64     `json:"targetLoadKg"`
	MeasuredLoadKg       float64     `json:"measuredLoadKg"`
	HoldSeconds          int         `json:"holdSeconds"`
	DeformationMm        float64     `json:"deformationMm"`
	Outcome              TestOutcome `json:"outcome"`
	EvidenceDigest       string      `json:"evidenceDigest"`
	PerformedBy          string      `json:"performedBy"`
	PerformedAt          time.Time   `json:"performedAt"`
	PointConfigRevision  int64       `json:"pointConfigRevision"`
	PointConfigDigest    string      `json:"pointConfigDigest"`
	RetestTaskID         string      `json:"retestTaskId,omitempty"`
	CurrentConfiguration bool        `json:"currentConfiguration"`
}

type PlanRevision struct {
	ID        string         `json:"id"`
	Before    map[string]any `json:"before"`
	After     map[string]any `json:"after"`
	RevisedBy string         `json:"revisedBy"`
	RevisedAt time.Time      `json:"revisedAt"`
}

type IssueStatus string

const (
	IssueOpen          IssueStatus = "open"
	IssueRetestPending IssueStatus = "retest_pending"
	IssueClosed        IssueStatus = "closed"
)

type SafetyIssue struct {
	ID              string      `json:"id"`
	PlanID          string      `json:"planId"`
	PointID         string      `json:"pointId,omitempty"`
	RuleCode        string      `json:"ruleCode"`
	Severity        string      `json:"severity"`
	Description     string      `json:"description"`
	Status          IssueStatus `json:"status"`
	RemediationNote string      `json:"remediationNote,omitempty"`
	OpenedAt        time.Time   `json:"openedAt"`
	ClosedAt        *time.Time  `json:"closedAt,omitempty"`
}

type RemediationRevision struct {
	ID        string         `json:"id"`
	IssueID   string         `json:"issueId"`
	PointID   string         `json:"pointId"`
	Before    map[string]any `json:"before"`
	After     map[string]any `json:"after"`
	Note      string         `json:"note"`
	RevisedBy string         `json:"revisedBy"`
	RevisedAt time.Time      `json:"revisedAt"`
}

type RetestTaskStatus string

const (
	RetestTaskPending  RetestTaskStatus = "pending"
	RetestTaskClosed   RetestTaskStatus = "closed"
	RetestTaskReplaced RetestTaskStatus = "replaced"
)

type RetestConditions struct {
	MinimumTargetLoadKg  float64     `json:"minimumTargetLoadKg"`
	MinimumHoldSeconds   int         `json:"minimumHoldSeconds"`
	MaximumDeformationMm float64     `json:"maximumDeformationMm"`
	RequiredOutcome      TestOutcome `json:"requiredOutcome"`
}

type RetestTask struct {
	ID                string           `json:"id"`
	PlanID            string           `json:"planId"`
	IssueID           string           `json:"issueId"`
	PointID           string           `json:"pointId"`
	RevisionID        string           `json:"revisionId"`
	PointConfigDigest string           `json:"pointConfigDigest"`
	Conditions        RetestConditions `json:"conditions"`
	Status            RetestTaskStatus `json:"status"`
	CreatedAt         time.Time        `json:"createdAt"`
	ClosedAt          *time.Time       `json:"closedAt,omitempty"`
	ReplacedAt        *time.Time       `json:"replacedAt,omitempty"`
	AttemptTestIDs    []string         `json:"attemptTestIds"`
	UnmetConditions   []string         `json:"unmetConditions,omitempty"`
	ClosureBasis      string           `json:"closureBasis,omitempty"`
}

type ReviewDecision struct {
	Decision string    `json:"decision"`
	Reviewer string    `json:"reviewer"`
	Note     string    `json:"note"`
	At       time.Time `json:"at"`
}

type ClearanceCredential struct {
	ID               string    `json:"id"`
	PlanID           string    `json:"planId"`
	Sequence         uint64    `json:"sequence"`
	FrozenDigest     string    `json:"frozenDigest"`
	IssuedBy         string    `json:"issuedBy"`
	IssuedAt         time.Time `json:"issuedAt"`
	CredentialDigest string    `json:"credentialDigest"`
}

type AuditRecord struct {
	Sequence       uint64         `json:"sequence"`
	PlanID         string         `json:"planId"`
	Action         string         `json:"action"`
	Actor          string         `json:"actor"`
	At             time.Time      `json:"at"`
	Payload        map[string]any `json:"payload"`
	PreviousDigest string         `json:"previousDigest"`
	Digest         string         `json:"digest"`
}

type Verification struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

type CredentialChecks struct {
	CredentialDigest bool `json:"credentialDigest"`
	FrozenManifest   bool `json:"frozenManifest"`
	FrozenReference  bool `json:"frozenReference"`
	Sequence         bool `json:"sequence"`
	AuditChain       bool `json:"auditChain"`
}

type GlobalCredentialVerification struct {
	Valid           bool             `json:"valid"`
	Message         string           `json:"message"`
	CredentialID    string           `json:"credentialId"`
	Sequence        uint64           `json:"sequence"`
	IssuedAt        time.Time        `json:"issuedAt"`
	IssuedBy        string           `json:"issuedBy"`
	VenueName       string           `json:"venueName"`
	PerformanceDate string           `json:"performanceDate"`
	Checks          CredentialChecks `json:"checks"`
}

type PointRemovalCheck struct {
	Allowed    bool     `json:"allowed"`
	References []string `json:"references"`
	History    []string `json:"history"`
	Message    string   `json:"message"`
}
