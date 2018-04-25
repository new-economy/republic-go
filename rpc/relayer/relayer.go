package relayer

import (
	"fmt"
	"io"

	"github.com/republicprotocol/republic-go/identity"
	"github.com/republicprotocol/republic-go/order"
	"github.com/republicprotocol/republic-go/orderbook"
	"google.golang.org/grpc"
)

// Relayer implements the gRPC Relay service.
type Relayer struct {
	client    *Client
	orderbook *orderbook.Orderbook
}

// NewRelayer returns a Relayer that will use an orderbook.Orderbook for
// synchronization (see Relay.Sync).
func NewRelayer(client *Client, orderbook *orderbook.Orderbook) Relayer {
	// TODO: Implement a max connection parameter to prevent too many clients
	// from syncing and slowing down the Relay service
	return Relayer{
		client:    client,
		orderbook: orderbook,
	}
}

// Register the gRPC service to a grpc.Server.
func (relayer *Relayer) Register(server *grpc.Server) {
	RegisterRelayServer(server, relayer)
}

// Sync is an RPC used to synchronize the entries of an orderbook.Orderbook. In
// the SyncRequest, the client stores the epoch that they are interested in
// synchronizing. Existing entries in the orderbook.Orderbook will be streamed
// by new updates to the orderbook.Orderbook will also be streamed, without
// waiting for the existing entries to finish. The client must manage the
// merging of conflicting entries.
func (relayer *Relayer) Sync(request *SyncRequest, stream Relay_SyncServer) error {
	addrSignature := request.GetSignature()
	addr := identity.Address(request.GetAddress())
	if err := relayer.client.crypter.Verify(addr, addrSignature); err != nil {
		return err
	}

	entries := make(chan orderbook.Entry)
	defer close(entries)

	errs := make(chan error, 1)
	go func() {
		defer close(errs)
		defer relayer.orderbook.Unsubscribe(entries)
		if err := relayer.orderbook.Subscribe(entries); err != nil {
			errs <- fmt.Errorf("cannot subscribe to orderbook: %v", err)
			return
		}
	}()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case err, ok := <-errs:
			if !ok {
				return nil
			}
			return err
		case entry, ok := <-entries:
			if !ok {
				return nil
			}

			orderStatus := OrderStatus_Open
			switch entry.Status {
			case order.Open:
				orderStatus = OrderStatus_Open
			case order.Canceled:
				orderStatus = OrderStatus_Canceled
			case order.Unconfirmed:
				orderStatus = OrderStatus_Unconfirmed
			case order.Confirmed:
				orderStatus = OrderStatus_Confirmed
			case order.Settled:
				orderStatus = OrderStatus_Settled
			}

			syncResponse := &SyncResponse{
				Signature: []byte{},
				Entry: &OrderbookEntry{
					Order: &Order{
						OrderId: entry.Order.ID,
						Expiry:  entry.Order.Expiry.Unix(),
						Type:    int32(entry.Order.Type),
						Tokens:  int32(0), // TODO: Use the correct token pair encoding
					},
					OrderStatus: orderStatus,
				},
			}
			if err := stream.Send(syncResponse); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
		}
	}
}