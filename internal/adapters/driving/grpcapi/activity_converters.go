package grpcapi

import (
	"encoding/json"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/activity"
)

func loginEntryToProto(e *activity.LoginEntry) *eveyv1.LoginEntry {
	pb := &eveyv1.LoginEntry{
		Id:        e.ID,
		UserId:    string(e.UserID),
		IpAddress: e.ClientIP,
		UserAgent: e.UserAgent,
		Success:   e.Success,
		Timestamp: toTimestamppb(e.CreatedAt),
	}
	if e.FailureReason != nil {
		pb.FailureReason = *e.FailureReason
	}
	return pb
}

func serverMetricToProto(m *activity.ServerMetric) *eveyv1.ServerMetric {
	memPercent := float64(0)
	if m.MemoryTotalBytes > 0 {
		memPercent = float64(m.MemoryUsedBytes) / float64(m.MemoryTotalBytes) * 100
	}
	return &eveyv1.ServerMetric{
		Timestamp:         toTimestamppb(m.RecordedAt),
		CpuPercent:        m.CPUPercent,
		MemoryPercent:     memPercent,
		MemoryUsedBytes:   m.MemoryUsedBytes,
		MemoryTotalBytes:  m.MemoryTotalBytes,
		ActiveStreams:     int32(m.ActiveStreams),
		ActiveTranscodes:  int32(m.ActiveTranscodes),
		BandwidthBps:      m.BandwidthBps,
		StorageUsedBytes:  m.StorageUsedBytes,
		StorageTotalBytes: m.StorageTotalBytes,
		IowaitPercent:     m.IOWaitPercent,
		DiskAwaitMs:       m.DiskAwaitMs,
		DiskUtilPercent:   m.DiskUtilPercent,
		DiskBusiest:       m.DiskBusiest,
		DiskReadBps:       m.DiskReadBps,
		DiskWriteBps:      m.DiskWriteBps,
		NetworkRxBps:      m.NetRxBps,
		NetworkTxBps:      m.NetTxBps,
	}
}

func playHistoryEntryToProto(e *activity.PlayHistoryEntry) *eveyv1.PlayHistoryEntry {
	pb := &eveyv1.PlayHistoryEntry{
		Id:          e.ID,
		ProfileId:   string(e.ProfileID),
		MediaItemId: string(e.MediaItemID),
		MediaTitle:  e.MediaTitle,
		MediaType:   mediaItemTypeToProto[e.MediaType],
		DurationMs:  e.Duration.Milliseconds(),
		PositionMs:  e.Position.Milliseconds(),
		Completed:   e.Completed,
		DeviceName:  e.DeviceName,
		ClientName:  e.ClientName,
		PlayMethod:  e.PlayMethod,
		StartedAt:   toTimestamppb(e.StartedAt),
		EndedAt:     toTimestamppbPtr(e.EndedAt),
		ProfileName: e.ProfileName,
		UserId:      string(e.UserID),
		Username:    e.Username,
	}
	return pb
}

func auditLogEntryToProto(e *activity.AuditLogEntry) *eveyv1.AuditLogEntry {
	pb := &eveyv1.AuditLogEntry{
		Id:           e.ID,
		UserId:       string(e.UserID),
		Username:     e.Username,
		ProfileId:    string(e.ProfileID),
		ProfileName:  e.ProfileName,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceName: e.ResourceName,
		CreatedAt:    toTimestamppb(e.CreatedAt),
	}
	if e.ResourceID != nil {
		pb.ResourceId = *e.ResourceID
	}
	if e.ClientIP != nil {
		pb.IpAddress = *e.ClientIP
	}
	if e.UserAgent != nil {
		pb.UserAgent = *e.UserAgent
	}
	if e.DeviceName != nil {
		pb.DeviceName = *e.DeviceName
	}
	if len(e.Details) > 0 {
		if b, err := json.Marshal(e.Details); err == nil {
			pb.Details = string(b)
		}
	}
	return pb
}
