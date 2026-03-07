package pluginapi

import (
	"log"
	"net"
	"os"
	"path/filepath"

	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	"google.golang.org/grpc"
)

type PluginAPIServer struct {
	server   *grpc.Server
	library  *applibrary.Service
	metadata *appmetadata.Service
	sockPath string
}

func NewPluginAPIServer(
	library *applibrary.Service,
	metadata *appmetadata.Service,
	sockDir string,
) *PluginAPIServer {
	return &PluginAPIServer{
		server:   grpc.NewServer(),
		library:  library,
		metadata: metadata,
		sockPath: filepath.Join(sockDir, "evey-host.sock"),
	}
}

func (s *PluginAPIServer) Start() error {
	os.Remove(s.sockPath)
	lis, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return err
	}
	// TODO: register HostService gRPC handler that delegates to app services
	log.Printf("plugin API server listening on %s", s.sockPath)
	return s.server.Serve(lis)
}

func (s *PluginAPIServer) Stop() {
	s.server.GracefulStop()
	os.Remove(s.sockPath)
}
