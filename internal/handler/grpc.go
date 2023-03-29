package handler

import (
	"context"

	pb "github.com/denisdubovitskiy/blackhole/internal/api"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Blacklist interface {
	Add(domains ...string) (count int)
	Remove(domains ...string) (count int)
}

func New(blacklist Blacklist) pb.BlackholeServer {
	return &Handler{
		blacklist: blacklist,
	}
}

type Handler struct {
	pb.UnimplementedBlackholeServer
	blacklist Blacklist
}

var ok = &emptypb.Empty{}

func (h Handler) Block(ctx context.Context, request *pb.DomainsRequest) (*emptypb.Empty, error) {
	h.blacklist.Add(request.GetDomains()...)
	return ok, nil
}

func (h Handler) Unblock(ctx context.Context, request *pb.DomainsRequest) (*emptypb.Empty, error) {
	h.blacklist.Remove(request.GetDomains()...)
	return ok, nil
}
