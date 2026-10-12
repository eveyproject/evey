package grpcapi

import (
	"context"
	"fmt"
	"strconv"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

type adminLibraryLister interface {
	ListLibraries(ctx context.Context) ([]*domainlibrary.Library, error)
}

type adminScheduler interface {
	ListSchedules(ctx context.Context) ([]*domainlibrary.ScanSchedule, error)
	SetSchedule(ctx context.Context, libraryID, cronExpr string, enabled bool) (*domainlibrary.ScanSchedule, error)
}

type adminLibraryScanner interface {
	ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error)
}

func stringFromInterface(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
