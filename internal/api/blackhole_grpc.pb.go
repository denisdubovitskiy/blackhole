// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.21.12
// source: blackhole.proto

package api

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// BlackholeClient is the client API for Blackhole service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type BlackholeClient interface {
	Block(ctx context.Context, in *DomainsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	Unblock(ctx context.Context, in *DomainsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type blackholeClient struct {
	cc grpc.ClientConnInterface
}

func NewBlackholeClient(cc grpc.ClientConnInterface) BlackholeClient {
	return &blackholeClient{cc}
}

func (c *blackholeClient) Block(ctx context.Context, in *DomainsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/denisdubovitskiy.blackhole.api.Blackhole/Block", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *blackholeClient) Unblock(ctx context.Context, in *DomainsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/denisdubovitskiy.blackhole.api.Blackhole/Unblock", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BlackholeServer is the server API for Blackhole service.
// All implementations must embed UnimplementedBlackholeServer
// for forward compatibility
type BlackholeServer interface {
	Block(context.Context, *DomainsRequest) (*emptypb.Empty, error)
	Unblock(context.Context, *DomainsRequest) (*emptypb.Empty, error)
	mustEmbedUnimplementedBlackholeServer()
}

// UnimplementedBlackholeServer must be embedded to have forward compatible implementations.
type UnimplementedBlackholeServer struct {
}

func (UnimplementedBlackholeServer) Block(context.Context, *DomainsRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Block not implemented")
}
func (UnimplementedBlackholeServer) Unblock(context.Context, *DomainsRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Unblock not implemented")
}
func (UnimplementedBlackholeServer) mustEmbedUnimplementedBlackholeServer() {}

// UnsafeBlackholeServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to BlackholeServer will
// result in compilation errors.
type UnsafeBlackholeServer interface {
	mustEmbedUnimplementedBlackholeServer()
}

func RegisterBlackholeServer(s grpc.ServiceRegistrar, srv BlackholeServer) {
	s.RegisterService(&Blackhole_ServiceDesc, srv)
}

func _Blackhole_Block_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DomainsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BlackholeServer).Block(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/denisdubovitskiy.blackhole.api.Blackhole/Block",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BlackholeServer).Block(ctx, req.(*DomainsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Blackhole_Unblock_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DomainsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BlackholeServer).Unblock(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/denisdubovitskiy.blackhole.api.Blackhole/Unblock",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BlackholeServer).Unblock(ctx, req.(*DomainsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Blackhole_ServiceDesc is the grpc.ServiceDesc for Blackhole service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Blackhole_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "denisdubovitskiy.blackhole.api.Blackhole",
	HandlerType: (*BlackholeServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Block",
			Handler:    _Blackhole_Block_Handler,
		},
		{
			MethodName: "Unblock",
			Handler:    _Blackhole_Unblock_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "blackhole.proto",
}
