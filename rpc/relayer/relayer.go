package relayer

import (
	"fmt"

	"github.com/republicprotocol/republic-go/order"
	"github.com/republicprotocol/republic-go/orderbook"
	"google.golang.org/grpc"
)

// Relayer implements the gRPC Relay service.
type Relayer struct {
	orderbook *orderbook.Orderbook
}

// NewRelayer returns a Relayer that will use an orderbook.Orderbook for
// synchronization (see Relay.Sync).
func NewRelayer(orderbook *orderbook.Orderbook) Relayer {
	return Relayer{
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
	entries := make(chan orderbook.Entry)
	defer close(entries)

	if err := relayer.orderbook.Subscribe(entries); err != nil {
		return fmt.Errorf("cannot subscribe to orderbook: %v", err)
	}
	defer relayer.orderbook.Unsubscribe(entries)

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
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
				Epoch:     request.Epoch,
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
				return err
			}
		}
	}
}