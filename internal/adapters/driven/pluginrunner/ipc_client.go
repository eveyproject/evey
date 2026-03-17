package pluginrunner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	pluginv1 "github.com/gravity-zero/evey/gen/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ipcConnectTimeout   = 10 * time.Second
	ipcCallTimeout      = 5 * time.Second
	sockPollInterval    = 100 * time.Millisecond
)

type ipcClient struct {
	conn   *grpc.ClientConn
	plugin pluginv1.PluginServiceClient
}

func newIPCClient(sockPath string) (*ipcClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ipcConnectTimeout)
	defer cancel()

	if err := waitForSocket(ctx, sockPath); err != nil {
		return nil, fmt.Errorf("wait for plugin socket: %w", err)
	}

	conn, err := grpc.NewClient(
		"unix://"+sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial plugin socket: %w", err)
	}

	return &ipcClient{
		conn:   conn,
		plugin: pluginv1.NewPluginServiceClient(conn),
	}, nil
}

func (c *ipcClient) Initialize(ctx context.Context, pluginID string, config map[string]string, hostVersion string) error {
	callCtx, cancel := context.WithTimeout(ctx, ipcCallTimeout)
	defer cancel()

	resp, err := c.plugin.Initialize(callCtx, &pluginv1.InitRequest{
		PluginId:    pluginID,
		Config:      config,
		HostVersion: hostVersion,
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("initialize failed: %s", resp.Error)
	}
	return nil
}

func (c *ipcClient) Shutdown(ctx context.Context, reason string) error {
	callCtx, cancel := context.WithTimeout(ctx, ipcCallTimeout)
	defer cancel()
	_, err := c.plugin.Shutdown(callCtx, &pluginv1.ShutdownRequest{Reason: reason})
	return err
}

func (c *ipcClient) OnEvent(ctx context.Context, eventType string, payload map[string]any, source string) error {
	callCtx, cancel := context.WithTimeout(ctx, ipcCallTimeout)
	defer cancel()

	pbPayload, err := structpb.NewStruct(payload)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}

	_, err = c.plugin.OnEvent(callCtx, &pluginv1.EventNotification{
		EventType: eventType,
		Payload:   pbPayload,
		Timestamp: timestamppb.Now(),
		Source:    source,
	})
	return err
}

func (c *ipcClient) HealthCheck(ctx context.Context) (bool, error) {
	callCtx, cancel := context.WithTimeout(ctx, ipcCallTimeout)
	defer cancel()

	resp, err := c.plugin.HealthCheck(callCtx, &pluginv1.HealthCheckRequest{})
	if err != nil {
		return false, err
	}
	return resp.Healthy, nil
}

func (c *ipcClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func waitForSocket(ctx context.Context, sockPath string) error {
	dir := filepath.Dir(sockPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}

	ticker := time.NewTicker(sockPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for socket %s", sockPath)
		case <-ticker.C:
			if _, err := os.Stat(sockPath); err == nil {
				return nil
			}
		}
	}
}
