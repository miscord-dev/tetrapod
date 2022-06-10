package signalserver

import (
	"github.com/miscord-dev/toxfu/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	proto.UnimplementedNodeAPIServer
}

var _ proto.NodeAPIServer = (*Server)(nil)

func (s *Server) Refresh(svr proto.NodeAPI_RefreshServer) error {
	return status.Errorf(codes.Unimplemented, "method Refresh not implemented")
}
