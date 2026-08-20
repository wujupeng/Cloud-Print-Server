package domain

import (
	"encoding/json"

	"time"
)

type Protocol string

const (
	ProtocolRAW     Protocol = "RAW"
	ProtocolLPR     Protocol = "LPR"
	ProtocolIPP     Protocol = "IPP"
	ProtocolUnknown Protocol = "UNKNOWN"
)

func (p Protocol) String() string { return string(p) }

func (p Protocol) MarshalJSON() ([]byte, error) { return json.Marshal(string(p)) }

func (p *Protocol) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*p = Protocol(s)
	return nil
}

type DeviceStatus string

const (
	DeviceStatusOnline      DeviceStatus = "ONLINE"
	DeviceStatusOffline     DeviceStatus = "OFFLINE"
	DeviceStatusProbeFailed DeviceStatus = "PROBE_FAILED"
)

func (s DeviceStatus) String() string { return string(s) }

func (s DeviceStatus) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

func (s *DeviceStatus) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = DeviceStatus(v)
	return nil
}

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "PENDING"
	TaskStatusRunning   TaskStatus = "RUNNING"
	TaskStatusSuccess   TaskStatus = "SUCCESS"
	TaskStatusFailed    TaskStatus = "FAILED"
	TaskStatusRetrying  TaskStatus = "RETRYING"
	TaskStatusCancelled TaskStatus = "CANCELLED"
)

func (s TaskStatus) String() string { return string(s) }

func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusSuccess || s == TaskStatusFailed || s == TaskStatusCancelled
}

func (s TaskStatus) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

func (s *TaskStatus) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = TaskStatus(v)
	return nil
}

type Role string

const (
	RoleAdmin Role = "ADMIN"
	RoleUser  Role = "USER"
)

func (r Role) String() string { return string(r) }

func (r Role) IsAdmin() bool { return r == RoleAdmin }

func (r Role) MarshalJSON() ([]byte, error) { return json.Marshal(string(r)) }

func (r *Role) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = Role(v)
	return nil
}

type UserStatus string

const (
	UserStatusActive   UserStatus = "ACTIVE"
	UserStatusDisabled UserStatus = "DISABLED"
)

func (s UserStatus) String() string { return string(s) }

func (s UserStatus) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

func (s *UserStatus) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = UserStatus(v)
	return nil
}

type NetClass string

const (
	NetClassOK                      NetClass = "OK"
	NetClassLocalNetFail            NetClass = "LOCAL_NET_FAIL"
	NetClassCloudGatewayUnreachable NetClass = "CLOUD_GATEWAY_UNREACHABLE"
	NetClassDNSResolveFail          NetClass = "DNS_RESOLVE_FAIL"
)

func (c NetClass) String() string { return string(c) }

func (c NetClass) IsOK() bool { return c == NetClassOK }

func (c NetClass) IsLocalFail() bool { return c == NetClassLocalNetFail }

func (c NetClass) MarshalJSON() ([]byte, error) { return json.Marshal(string(c)) }

func (c *NetClass) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*c = NetClass(v)
	return nil
}

type CloudProtocol string

const (
	CloudProtocolWSS   CloudProtocol = "wss"
	CloudProtocolHTTPS CloudProtocol = "https"
)

func (p CloudProtocol) String() string { return string(p) }

type User struct {
	UserID      string     `json:"user_id" db:"user_id"`
	Username    string     `json:"username" db:"username"`
	PasswordHash string    `json:"-" db:"password_hash"`
	PasswordSalt string    `json:"-" db:"password_salt"`
	Role        Role       `json:"role" db:"role"`
	Status      UserStatus `json:"status" db:"status"`
	DisplayName string     `json:"display_name,omitempty" db:"display_name"`
	CreatedAt   time.Time  `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at,omitempty" db:"updated_at"`
}

type Factory struct {
	FactoryID  string    `json:"factory_id" db:"factory_id"`
	Name       string    `json:"name" db:"name"`
	Code       string    `json:"code,omitempty" db:"code"`
	Location   string    `json:"location,omitempty" db:"location"`
	CreatedAt  time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at,omitempty" db:"updated_at"`
}

type Agent struct {
	AgentID         string    `json:"agent_id" db:"agent_id"`
	Name            string    `json:"name" db:"name"`
	FactoryID       string    `json:"factory_id" db:"factory_id"`
	DeviceTokenEnc  []byte    `json:"-" db:"device_token_enc"`
	Version         string    `json:"version,omitempty" db:"version"`
	Online          bool      `json:"online" db:"online"`
	OnlineDevices   int       `json:"online_devices,omitempty" db:"online_devices"`
	PendingTasks    int       `json:"pending_tasks,omitempty" db:"pending_tasks"`
	NetClass        NetClass  `json:"net_class,omitempty" db:"net_class"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at,omitempty" db:"last_heartbeat_at"`
	CreatedAt       time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at,omitempty" db:"updated_at"`
}

type Device struct {
	DeviceID    string       `json:"device_id" db:"device_id"`
	Name        string       `json:"name" db:"name"`
	IP          string       `json:"ip" db:"ip"`
	Hostname    string       `json:"hostname,omitempty" db:"hostname"`
	Model       string       `json:"model" db:"model"`
	Protocol    Protocol     `json:"protocol" db:"protocol"`
	Status      DeviceStatus `json:"status" db:"status"`
	FactoryID   string       `json:"factory_id" db:"factory_id"`
	AgentID     string       `json:"agent_id" db:"agent_id"`
	Port        int          `json:"port,omitempty" db:"port"`
	LastProbeAt time.Time    `json:"last_probe_at,omitempty" db:"last_probe_at"`
	LastStatusAt time.Time   `json:"last_status_at,omitempty" db:"last_status_at"`
	CreatedAt   time.Time    `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at,omitempty" db:"updated_at"`
}

func (d *Device) DefaultPort() int {
	if d.Port > 0 {
		return d.Port
	}
	switch d.Protocol {
	case ProtocolRAW:
		return 9100
	case ProtocolLPR:
		return 515
	case ProtocolIPP:
		return 631
	default:
		return 0
	}
}

type PrintParams struct {
	Copies      int               `json:"copies,omitempty" db:"copies"`
	Orientation string            `json:"orientation,omitempty" db:"orientation"`
	Extra       map[string]string `json:"extra,omitempty" db:"-"`
}

type Document struct {
	DocID        string    `json:"doc_id" db:"doc_id"`
	UserID       string    `json:"user_id" db:"user_id"`
	Filename     string    `json:"filename" db:"filename"`
	ContentType  string    `json:"content_type,omitempty" db:"content_type"`
	Size         int64     `json:"size" db:"size"`
	Checksum     string    `json:"checksum" db:"checksum"`
	StoragePath  string    `json:"-" db:"storage_path"`
	CleanupAt    time.Time `json:"cleanup_at,omitempty" db:"cleanup_at"`
	CreatedAt    time.Time `json:"created_at,omitempty" db:"created_at"`
}

type PrintTask struct {
	TaskID       string      `json:"task_id" db:"task_id"`
	UserID       string      `json:"user_id,omitempty" db:"user_id"`
	DeviceID     string      `json:"device_id" db:"device_id"`
	AgentID      string      `json:"agent_id,omitempty" db:"agent_id"`
	DocID        string      `json:"doc_id,omitempty" db:"doc_id"`
	DocumentRef  string      `json:"document_ref,omitempty" db:"document_ref"`
	Checksum     string      `json:"checksum,omitempty" db:"checksum"`
	Params       PrintParams `json:"params,omitempty" db:"-"`
	Status       TaskStatus  `json:"status" db:"status"`
	RetryCount   int         `json:"retry_count" db:"retry_count"`
	TraceID      string      `json:"trace_id,omitempty" db:"trace_id"`
	SubmittedAt  time.Time   `json:"submitted_at,omitempty" db:"submitted_at"`
	DispatchedAt time.Time   `json:"dispatched_at,omitempty" db:"dispatched_at"`
	ReceivedAt   time.Time   `json:"received_at,omitempty" db:"received_at"`
	StartedAt    time.Time   `json:"started_at,omitempty" db:"started_at"`
	FinishedAt   time.Time   `json:"finished_at,omitempty" db:"finished_at"`
	NextRetryAt  time.Time   `json:"next_retry_at,omitempty" db:"next_retry_at"`
	ErrorCode    string      `json:"error_code,omitempty" db:"error_code"`
	ErrorMsg     string      `json:"error_msg,omitempty" db:"error_msg"`
}

type UserPermission struct {
	PermissionID string    `json:"permission_id" db:"permission_id"`
	UserID       string    `json:"user_id" db:"user_id"`
	DeviceID     string    `json:"device_id" db:"device_id"`
	GrantedAt    time.Time `json:"granted_at,omitempty" db:"granted_at"`
}

type AgentLog struct {
	LogID     string    `json:"log_id" db:"log_id"`
	AgentID   string    `json:"agent_id" db:"agent_id"`
	Level     string    `json:"level" db:"level"`
	Event     string    `json:"event" db:"event"`
	Message   string    `json:"message,omitempty" db:"message"`
	TraceID   string    `json:"trace_id,omitempty" db:"trace_id"`
	TS        time.Time `json:"ts" db:"ts"`
}

type AuditLog struct {
	AuditID  string    `json:"audit_id" db:"audit_id"`
	UserID   string    `json:"user_id,omitempty" db:"user_id"`
	Action   string    `json:"action" db:"action"`
	Target   string    `json:"target,omitempty" db:"target"`
	Detail   string    `json:"detail,omitempty" db:"detail"`
	IP       string    `json:"ip,omitempty" db:"ip"`
	TS       time.Time `json:"ts" db:"ts"`
}

type ConfigDispatch struct {
	DispatchID    string    `json:"dispatch_id" db:"dispatch_id"`
	AgentID       string    `json:"agent_id" db:"agent_id"`
	ConfigVersion int       `json:"config_version" db:"config_version"`
	Patch         string    `json:"patch,omitempty" db:"patch"`
	Applied       bool      `json:"applied" db:"applied"`
	Reason        string    `json:"reason,omitempty" db:"reason"`
	DispatchedAt  time.Time `json:"dispatched_at,omitempty" db:"dispatched_at"`
	AckedAt       time.Time `json:"acked_at,omitempty" db:"acked_at"`
}

const Version = "0.1.0"
