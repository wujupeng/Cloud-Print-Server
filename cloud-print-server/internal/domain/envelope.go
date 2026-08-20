package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

type Envelope struct {
	Type    string          `json:"type"`
	TraceID string          `json:"trace_id,omitempty"`
	TS      time.Time       `json:"ts"`
	Payload json.RawMessage `json:"payload"`
}

type DownEnvelope struct {
	Type    string          `json:"type"`
	MsgID   string          `json:"msg_id"`
	TraceID string          `json:"trace_id,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

func NewEnvelope(msgType string, payload interface{}) (*Envelope, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return &Envelope{
		Type:    msgType,
		TS:      time.Now().UTC(),
		Payload: b,
	}, nil
}

func NewDownEnvelope(msgType, msgID, traceID string, payload interface{}) (*DownEnvelope, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return &DownEnvelope{
		Type:    msgType,
		MsgID:   msgID,
		TraceID: traceID,
		Payload: b,
	}, nil
}

const (
	MsgHeartbeat      = "heartbeat"
	MsgTaskAck        = "task_ack"
	MsgTaskResult     = "task_result"
	MsgDeviceStatus   = "device_status"
	MsgLog            = "log"
	MsgUpgradeResult  = "upgrade_result"
	MsgConfigAck      = "config_ack"
	MsgNetEvent       = "net_event"

	MsgTask           = "task"
	MsgDeviceAdd      = "device_add"
	MsgDeviceUpdate   = "device_update"
	MsgDeviceRemove   = "device_remove"
	MsgControl        = "control"
	MsgUpgrade        = "upgrade"
	MsgConfigUpdate   = "config_update"
)

var UpstreamMessageTypes = map[string]struct{}{
	MsgHeartbeat:     {},
	MsgTaskAck:       {},
	MsgTaskResult:    {},
	MsgDeviceStatus:  {},
	MsgLog:           {},
	MsgUpgradeResult: {},
	MsgConfigAck:     {},
	MsgNetEvent:      {},
}

var DownstreamMessageTypes = map[string]struct{}{
	MsgTask:         {},
	MsgDeviceAdd:    {},
	MsgDeviceUpdate: {},
	MsgDeviceRemove: {},
	MsgControl:      {},
	MsgUpgrade:      {},
	MsgConfigUpdate: {},
}

func IsUpstream(msgType string) bool {
	_, ok := UpstreamMessageTypes[msgType]
	return ok
}

func IsDownstream(msgType string) bool {
	_, ok := DownstreamMessageTypes[msgType]
	return ok
}

type HeartbeatPayload struct {
	AgentID       string    `json:"agent_id"`
	Version       string    `json:"version"`
	OnlineDevices int       `json:"online_devices"`
	PendingTasks  int       `json:"pending_tasks"`
	CloudEndpoint string    `json:"cloud_endpoint"`
	NetClass      NetClass  `json:"net_class"`
	Timestamp     time.Time `json:"timestamp"`
}

type TaskAckPayload struct {
	TaskID   string `json:"task_id"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

type TaskResultPayload struct {
	TaskID     string     `json:"task_id"`
	DeviceID   string     `json:"device_id"`
	Status     TaskStatus `json:"status"`
	RetryCount int        `json:"retry_count"`
	ErrorCode  string     `json:"error_code,omitempty"`
	ErrorMsg   string     `json:"error_msg,omitempty"`
	FinishedAt time.Time  `json:"finished_at"`
}

type DeviceStatusPayload struct {
	DeviceID    string       `json:"device_id"`
	Status      DeviceStatus `json:"status"`
	Protocol    Protocol     `json:"protocol,omitempty"`
	LastProbeAt time.Time    `json:"last_probe_at,omitempty"`
}

type LogPayload struct {
	Level   string `json:"level"`
	Event   string `json:"event"`
	Message string `json:"message,omitempty"`
}

type UpgradeResultPayload struct {
	Success  bool   `json:"success"`
	FromVer  string `json:"from_ver,omitempty"`
	ToVer    string `json:"to_ver,omitempty"`
	Rollback bool   `json:"rollback,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type ConfigAckPayload struct {
	Applied bool   `json:"applied"`
	Reason  string `json:"reason,omitempty"`
	Field   string `json:"field,omitempty"`
}

type NetEventPayload struct {
	Class    NetClass  `json:"class"`
	Endpoint string    `json:"endpoint,omitempty"`
	Detail   string    `json:"detail,omitempty"`
	TS       time.Time `json:"ts"`
}

type TaskPayload struct {
	TaskID      string      `json:"task_id"`
	DeviceID    string      `json:"device_id"`
	DocumentRef string      `json:"document_ref,omitempty"`
	Content     []byte      `json:"content,omitempty"`
	Checksum    string      `json:"checksum"`
	Params      PrintParams `json:"params,omitempty"`
}

type DeviceAddPayload struct {
	DeviceID string   `json:"device_id"`
	Name     string   `json:"name"`
	IP       string   `json:"ip"`
	Port     int      `json:"port,omitempty"`
	Protocol Protocol `json:"protocol"`
	Model    string   `json:"model,omitempty"`
}

type DeviceUpdatePayload struct {
	DeviceID string   `json:"device_id"`
	Name     string   `json:"name,omitempty"`
	IP       string   `json:"ip,omitempty"`
	Port     int      `json:"port,omitempty"`
	Protocol Protocol `json:"protocol,omitempty"`
	Model    string   `json:"model,omitempty"`
}

type DeviceRemovePayload struct {
	DeviceID string `json:"device_id"`
}

type ControlPayload struct {
	Action   string `json:"action"`
	DeviceID string `json:"device_id,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
}

type UpgradePayload struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256,omitempty"`
}

type ConfigUpdatePayload struct {
	ConfigVersion int                    `json:"config_version"`
	Patch         map[string]interface{} `json:"patch"`
}