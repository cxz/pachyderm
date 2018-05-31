package grpcutil

import (
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path"
	"time"

	"github.com/pachyderm/pachyderm/src/client/version"
	"github.com/pachyderm/pachyderm/src/client/version/versionpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

const (
	// TLSVolumePath is the path at which the tls cert and private key (if any)
	// will be mounted in the pachd pod
	TLSVolumePath = "/pachd-tls-cert"

	// TLSCertFile is the name of the mounted file containing a TLS certificate
	// that identifies pachd
	TLSCertFile = "tls.crt"

	// TLSKeyFile is the name of the mounted file containing a private key
	// corresponding to the public certificate in TLSCertFile
	TLSKeyFile = "tls.key"
)

var (
	// ErrMustSpecifyRegisterFunc is used when a register func is nil.
	ErrMustSpecifyRegisterFunc = errors.New("must specify registerFunc")
)

// ServeOptions represent optional fields for serving.
type ServeOptions struct {
	Version    *versionpb.Version
	MaxMsgSize int
	Cancel     chan struct{}
}

// ServeEnv are environment variables for serving.
type ServeEnv struct {
	// Default is 7070.
	GRPCPort uint16 `env:"GRPC_PORT,default=7070"`
	PeerPort uint16 // Port where pachd serves traffic to peer pachd nodes
}

// runServer is a helper function for Serve(). It takes a slice of
// grpc.ServerOption and
func runServer(grpcServer *grpc.Server, options ServeOptions, port uint16) error {
	if options.Version != nil {
		versionpb.RegisterAPIServer(grpcServer, version.NewAPIServer(options.Version, version.APIServerOptions{}))
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	if options.Cancel != nil {
		go func() {
			<-options.Cancel
			if err := listener.Close(); err != nil {
				fmt.Printf("listener.Close(): %v\n", err)
			}
		}()
	}
	return grpcServer.Serve(listener)
}

// Serve serves stuff.
func Serve(
	registerFunc func(*grpc.Server),
	options ServeOptions,
	serveEnv ServeEnv,
) (retErr error) {
	if registerFunc == nil {
		return ErrMustSpecifyRegisterFunc
	}
	if serveEnv.GRPCPort == 0 {
		serveEnv.GRPCPort = 7070
	}
	peerOpts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(math.MaxUint32),
		grpc.MaxRecvMsgSize(options.MaxMsgSize),
		grpc.MaxSendMsgSize(options.MaxMsgSize),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	}
	var publicOpts []grpc.ServerOption
	if _, err := os.Stat(TLSVolumePath); err == nil {
		certPath := path.Join(TLSVolumePath, TLSCertFile)
		keyPath := path.Join(TLSVolumePath, TLSKeyFile)
		transportCreds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
		if err != nil {
			return fmt.Errorf("couldn't build transport creds: %v", err)
		}
		publicOpts = append(peerOpts, grpc.Creds(transportCreds))
	} else {
		publicOpts = peerOpts
	}
	// No TLS
	peerServer := grpc.NewServer(peerOpts...)
	registerFunc(peerServer)
	if err := runServer(peerServer, options, serveEnv.PeerPort); err != nil {
		return err
	}
	// Might additionally contain TLS option
	// publicServer := grpc.NewServer(publicOpts...)
	publicServer := grpc.NewServer(peerOpts...)
	registerFunc(publicServer)
	return runServer(publicServer, options, serveEnv.GRPCPort)
}
