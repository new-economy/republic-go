package network

import (
	"github.com/republicprotocol/go-do"
	"github.com/republicprotocol/republic-go/network/rpc"
	"github.com/republicprotocol/republic-go/order"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// A GossipDelegate is used as a callback interface to inject behavior into the
// Gossip service.
type GossipDelegate interface {
	OnGossip(order.ID, order.ID)
	OnFinalize(order.ID, order.ID)
}

// GossipService implements the gRPC Gossip service.
type GossipService struct {
	GossipDelegate
}

// NewGossipService returns a new GossipService
func NewGossipService(delegate GossipDelegate) *GossipService {
	return &GossipService{
		GossipDelegate: delegate,
	}
}

// Register the gRPC service.
func (service *GossipService) Register(server *grpc.Server) {
	rpc.RegisterGossipServer(server, service)
}

// Gossip handles an rpc.GossipRequest
func (service *GossipService) Gossip(ctx context.Context, gossipRequest *rpc.GossipRequest) (*rpc.Rumor, error) {
	wait := do.Process(func() do.Option {
		rumor, err := service.gossip(gossipRequest)
		if err != nil {
			return do.Err(err)
		}
		return do.Ok(rumor)
	})

	select {
	case val := <-wait:
		if val, ok := val.Ok.(*rpc.Rumor); ok {
			return val, nil
		}
		return &rpc.Rumor{}, val.Err

	case <-ctx.Done():
		return &rpc.Rumor{}, ctx.Err()
	}
}

func (service *GossipService) gossip(gossipRequest *rpc.GossipRequest) (*rpc.Rumor, error) {
	service.GossipDelegate.OnGossip(order.ID(gossipRequest.Rumor.BuyOrderId), order.ID(gossipRequest.Rumor.SellOrderId))
	return nil, nil
}

// Finalize handles an rpc.FinalizeRequest
func (service *GossipService) Finalize(ctx context.Context, finalizeRequest *rpc.FinalizeRequest) (*rpc.Rumor, error) {
	wait := do.Process(func() do.Option {
		rumor, err := service.finalize(finalizeRequest)
		if err != nil {
			return do.Err(err)
		}
		return do.Ok(rumor)
	})

	select {
	case val := <-wait:
		if val, ok := val.Ok.(*rpc.Rumor); ok {
			return val, nil
		}
		return &rpc.Rumor{}, val.Err

	case <-ctx.Done():
		return &rpc.Rumor{}, ctx.Err()
	}
}

func (service *GossipService) finalize(finalizeRequest *rpc.FinalizeRequest) (*rpc.Rumor, error) {
	service.GossipDelegate.OnFinalize(order.ID(finalizeRequest.Rumor.BuyOrderId), order.ID(finalizeRequest.Rumor.SellOrderId))
	return nil, nil
}
