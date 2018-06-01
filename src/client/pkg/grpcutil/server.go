package grpcutil

import (
	"errors"
	"fmt"
	"math"
	"net"
	// "os"
	// "path"
	"time"

	"github.com/pachyderm/pachyderm/src/client/version"
	"github.com/pachyderm/pachyderm/src/client/version/versionpb"

	"google.golang.org/grpc"
	// "google.golang.org/grpc/credentials"
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

	// ErrMustSpecifyPort is used when a port is 0
	ErrMustSpecifyPort = errors.New("must specify port on which to serve")
)

// ServerSpec represent optional fields for serving.
type ServerSpec struct {
	Port         uint16
	MaxMsgSize   int
	Cancel       chan struct{}
	RegisterFunc func(*grpc.Server)
}

// grpc.ServerOption and
func runServer(grpcServer *grpc.Server, options ServeOptions, port uint16) error {
	if options.Version != nil {
		versionpb.RegisterAPIServer(grpcServer, version.NewAPIServer(options.Version, version.APIServerOptions{}))
	}
}

// Serve serves stuff.
func Serve(
	servers ...ServerSpec,
) (retErr error) {
	for _, server := range servers {
		if server.registerFunc == nil {
			return ErrMustSpecifyRegisterFunc
		}
		if server.Port == 0 {
			return ErrMustSpecifyPort
		}
		peerOpts := []grpc.ServerOption{
			grpc.MaxConcurrentStreams(math.MaxUint32),
			grpc.MaxRecvMsgSize(server.MaxMsgSize),
			grpc.MaxSendMsgSize(server.MaxMsgSize),
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             5 * time.Second,
				PermitWithoutStream: true,
			}),
		}
		// var publicOpts []grpc.ServerOption
		// fmt.Printf(">>> Deciding whether to serve over TLS\n")
		// if _, err := os.Stat(TLSVolumePath); err == nil {
		// 	certPath := path.Join(TLSVolumePath, TLSCertFile)
		// 	keyPath := path.Join(TLSVolumePath, TLSKeyFile)
		// 	fmt.Printf(">>> Serving over TLS\n")
		// 	transportCreds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
		// 	if err != nil {
		// 		fmt.Printf(">>> Could not build transport creds: %v\n", err)
		// 		return fmt.Errorf("couldn't build transport creds: %v", err)
		// 	}
		// 	fmt.Printf(">>> transport creds built successfully\n", err)
		// 	publicOpts = append(peerOpts, grpc.Creds(transportCreds))
		// } else {
		// 	publicOpts = peerOpts
		// }

		// Might additionally contain TLS option
		// publicServer := grpc.NewServer(publicOpts...)
		GRPCServer := grpc.NewServer(peerOpts...)
		server.RegisterFunc(GRPCServer)
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", server.Port))
		if err != nil {
			return err
		}
		if server.Cancel != nil {
			go func() {
				<-server.Cancel
				if err := listener.Close(); err != nil {
					fmt.Printf("listener.Close(): %v\n", err)
				}
			}()
		}
		if err := GRPCServer.Serve(listener); err != nil {
			return err
		}
	}
	return nil
}
