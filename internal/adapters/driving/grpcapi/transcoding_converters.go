package grpcapi

import (
	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

var hwAccelTypeToProto = map[string]eveyv1.HWAccelType{
	"none":  eveyv1.HWAccelType_HW_ACCEL_TYPE_NONE,
	"vaapi": eveyv1.HWAccelType_HW_ACCEL_TYPE_VAAPI,
	"nvenc": eveyv1.HWAccelType_HW_ACCEL_TYPE_NVENC,
	"qsv":   eveyv1.HWAccelType_HW_ACCEL_TYPE_QSV,
	"rkmpp": eveyv1.HWAccelType_HW_ACCEL_TYPE_RKMPP,
}

var hwAccelTypeFromProto = map[eveyv1.HWAccelType]string{
	eveyv1.HWAccelType_HW_ACCEL_TYPE_NONE:  "none",
	eveyv1.HWAccelType_HW_ACCEL_TYPE_VAAPI: "vaapi",
	eveyv1.HWAccelType_HW_ACCEL_TYPE_NVENC: "nvenc",
	eveyv1.HWAccelType_HW_ACCEL_TYPE_QSV:   "qsv",
	eveyv1.HWAccelType_HW_ACCEL_TYPE_RKMPP: "rkmpp",
}

var transcodeStateToProto = map[string]eveyv1.TranscodeState{
	"queued":    eveyv1.TranscodeState_TRANSCODE_STATE_QUEUED,
	"running":   eveyv1.TranscodeState_TRANSCODE_STATE_RUNNING,
	"completed": eveyv1.TranscodeState_TRANSCODE_STATE_COMPLETED,
	"failed":    eveyv1.TranscodeState_TRANSCODE_STATE_FAILED,
	"cancelled": eveyv1.TranscodeState_TRANSCODE_STATE_CANCELLED,
}

func transcodeJobToProto(j *transcoding.TranscodeJob) *eveyv1.TranscodeJob {
	pb := &eveyv1.TranscodeJob{
		Id:          j.ID,
		MediaItemId: string(j.MediaItemID),
		State:       transcodeStateToProto[string(j.Status)],
		VideoCodec:  j.OutputCodec,
		AudioCodec:  j.AudioCodec,
		HwAccel:     hwAccelTypeToProto[j.HWAccelType],
		Bitrate:     j.BitrateOut,
		Speed:       j.Speed,
		Error:       j.ErrorMessage,
		StartedAt:   toTimestamppb(j.StartedAt),
	}
	if j.EndedAt != nil {
		pb.CompletedAt = toTimestamppb(*j.EndedAt)
	}
	return pb
}

func transcodeProfileToProto(p *transcoding.TranscodeProfile) *eveyv1.TranscodeProfile {
	return &eveyv1.TranscodeProfile{
		Id:         p.ID,
		Name:       p.Name,
		VideoCodec: p.VideoCodec,
		AudioCodec: p.AudioCodec,
		Container:  p.Container,
		MaxWidth:   int32(p.MaxWidth),
		MaxHeight:  int32(p.MaxHeight),
		MaxBitrate: p.VideoBitrate,
	}
}

func hwDeviceToProto(d transcoding.HWDevice) *eveyv1.HWDevice {
	return &eveyv1.HWDevice{
		Type:            hwAccelTypeToProto[d.Type],
		Name:            d.Name,
		SupportedCodecs: d.SupportedCodecs,
	}
}
