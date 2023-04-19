package handler

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/denisdubovitskiy/blackhole/internal/api"
	"google.golang.org/protobuf/types/known/emptypb"
)

type SourcesProvider interface {
	AddSource(ctx context.Context, url string) error
	RefreshSources(ctx context.Context) error
}

type Blacklist interface {
	Add(ctx context.Context, domains ...string) (count int)
	Remove(ctx context.Context, domains ...string) (count int)
}

func New(blacklist Blacklist, sourcesProvider SourcesProvider) pb.BlackholeServer {
	return &Handler{
		blacklist:       blacklist,
		sourcesProvider: sourcesProvider,
	}
}

type Handler struct {
	pb.UnimplementedBlackholeServer
	blacklist       Blacklist
	sourcesProvider SourcesProvider
}

var ok = &emptypb.Empty{}

func (h Handler) Block(ctx context.Context, request *pb.DomainsRequest) (*emptypb.Empty, error) {
	h.blacklist.Add(ctx, request.GetDomains()...)
	return ok, nil
}

func (h Handler) Unblock(ctx context.Context, request *pb.DomainsRequest) (*emptypb.Empty, error) {
	h.blacklist.Remove(ctx, request.GetDomains()...)
	return ok, nil
}

func (h Handler) AddSource(ctx context.Context, request *pb.AddSourceRequest) (*emptypb.Empty, error) {
	if err := h.sourcesProvider.AddSource(ctx, request.GetUrl()); err != nil {
		return nil, status.Errorf(codes.Internal, "unable to add source: %v", err)
	}
	return ok, nil
}

func (h Handler) RefreshSources(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if err := h.sourcesProvider.RefreshSources(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "unable to refresh sources: %v", err)
	}
	return ok, nil
}
