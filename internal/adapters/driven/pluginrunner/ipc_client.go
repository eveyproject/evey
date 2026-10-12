package pluginrunner

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	pluginv1 "github.com/eveyproject/evey/gen/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// Plafond d'attente du bind du socket plugin. Généreux VOLONTAIREMENT :
	// l'attente est coupée dès que le process meurt (waitForSocket surveille
	// procDone), donc le plafond ne joue que pour un process bien vivant mais
	// lent à démarrer. À 10 s, un node lancé pendant le scan de boot +
	// le backfill keyframe et déprioritisé (nice 10) expirait avant son bind,
	// et le plugin était parqué en Error alors qu'il tournait (prod 07-22).
	ipcConnectTimeout = 90 * time.Second
	ipcCallTimeout    = 5 * time.Second
	sockPollInterval  = 100 * time.Millisecond
)

type ipcClient struct {
	conn   *grpc.ClientConn
	plugin pluginv1.PluginServiceClient
}

// newIPCClient attend le bind du socket puis ouvre la connexion gRPC. procDone
// est le canal fermé quand le process du plugin meurt : il permet d'arrêter
// l'attente tout de suite au lieu de brûler le plafond sur un process disparu
// (passer nil = pas de surveillance, utilisé par les tests).
func newIPCClient(sockPath string, procDone <-chan struct{}) (*ipcClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ipcConnectTimeout)
	defer cancel()

	if err := waitForSocket(ctx, sockPath, procDone); err != nil {
		return nil, fmt.Errorf("wait for plugin socket: %w", err)
	}

	conn, err := grpc.NewClient(
		"unix://"+sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*1024*1024),
			grpc.MaxCallSendMsgSize(10*1024*1024),
		),
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

// waitForSocket sonde le socket jusqu'à ce qu'un dial aboutisse. Deux sorties
// d'échec, à ne pas confondre côté admin : le process est mort avant de binder
// (cause réelle, son code de sortie et son stdout sont exploitables), ou il est
// toujours vivant et n'a pas bindé dans le plafond (bien plus rare depuis que le
// plafond est large - cf ipcConnectTimeout).
func waitForSocket(ctx context.Context, sockPath string, procDone <-chan struct{}) error {
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
		case <-procDone:
			// Un dernier essai : le process peut avoir bindé puis rendu l'âme
			// entre deux tics, et dans ce cas le socket est exploitable.
			if conn, err := net.DialTimeout("unix", sockPath, sockPollInterval); err == nil {
				conn.Close()
				return nil
			}
			return fmt.Errorf("plugin process exited before binding %s", sockPath)
		case <-ticker.C:
			// Try to actually dial, not just check file existence
			conn, err := net.DialTimeout("unix", sockPath, sockPollInterval)
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}
