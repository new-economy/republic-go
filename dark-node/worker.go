package node

import (
	"fmt"

	"github.com/republicprotocol/republic-go/compute"
	"github.com/republicprotocol/republic-go/dark"
	"github.com/republicprotocol/republic-go/logger"
	"github.com/republicprotocol/republic-go/network/rpc"
	"github.com/republicprotocol/republic-go/order"
)

// An OrderFragmentWorker consumes order fragments and computes all
// combinations of delta fragments.
type OrderFragmentWorker struct {
	logger              *logger.Logger
	deltaFragmentMatrix *compute.DeltaFragmentMatrix
	queue               chan *order.Fragment
}

// NewOrderFragmentWorker returns an OrderFragmentWorker that reads work from
// a queue and uses a DeltaFragmentMatrix to do computations.
func NewOrderFragmentWorker(logger *logger.Logger, deltaFragmentMatrix *compute.DeltaFragmentMatrix, queue chan *order.Fragment) *OrderFragmentWorker {
	return &OrderFragmentWorker{
		logger:              logger,
		deltaFragmentMatrix: deltaFragmentMatrix,
		queue:               queue,
	}
}

// Run the OrderFragmentWorker and write all delta fragments to an output
// queue.
func (worker *OrderFragmentWorker) Run(queues ...chan *compute.DeltaFragment) error {
	for orderFragment := range worker.queue {
		deltaFragments, err := worker.deltaFragmentMatrix.InsertOrderFragment(orderFragment)
		if err != nil {
			return err
		}
		if deltaFragments != nil {
			// Write to channels that might be closed
			func() {
				defer func() { recover() }()
				for _, deltaFragment := range deltaFragments {
					for _, queue := range queues {
						queue <- deltaFragment
					}
				}
			}()
		}
	}
	return nil
}

// A DeltaFragmentBroadcastWorker consumes delta fragments and broadcasts them
type DeltaFragmentBroadcastWorker struct {
	logger     *logger.Logger
	clientPool *rpc.ClientPool
	darkPool   *dark.Pool
	queue      chan *compute.DeltaFragment
}

// NewDeltaFragmentBroadcastWorker returns an DeltaFragmentBroadcastWorker that reads  fragments from
// a queue and forwards them to all nodes in the dark pool
func NewDeltaFragmentBroadcastWorker(logger *logger.Logger, clientPool *rpc.ClientPool, darkPool *dark.Pool, queue chan *compute.DeltaFragment) *DeltaFragmentBroadcastWorker {
	return &DeltaFragmentBroadcastWorker{
		logger:     logger,
		clientPool: clientPool,
		darkPool:   darkPool,
		queue:      queue,
	}
}

// Run the DeltaFragmentBroadcastWorker and forward all fragments to nodes in the dark pool
func (worker *DeltaFragmentBroadcastWorker) Run() {
	for deltaFragment := range worker.queue {
		serializedDeltaFragment := rpc.SerializeDeltaFragment(deltaFragment)
		worker.darkPool.CoForAll(func(node *dark.Node) {
			multiAddress := node.MultiAddress()
			if multiAddress == nil {
				return
			}
			_, err := worker.clientPool.BroadcastDeltaFragment(*multiAddress, serializedDeltaFragment)
			if err != nil {
				worker.logger.Error(err.Error())
			}
		})
	}
}

// A DeltaFragmentWorker consumes delta fragments and reconstructs deltas.
type DeltaFragmentWorker struct {
	logger       *logger.Logger
	deltaBuilder *compute.DeltaBuilder
	queue        chan *compute.DeltaFragment
}

// NewDeltaFragmentWorker returns an DeltaFragmentWorker that reads work from
// a queue and uses a DeltaBuilder to do reconstructions.
func NewDeltaFragmentWorker(logger *logger.Logger, deltaBuilder *compute.DeltaBuilder, queue chan *compute.DeltaFragment) *DeltaFragmentWorker {
	return &DeltaFragmentWorker{
		logger:       logger,
		deltaBuilder: deltaBuilder,
		queue:        queue,
	}
}

// Run the DeltaFragmentWorker and write all deltas to  an output queue.
func (worker *DeltaFragmentWorker) Run(queues ...chan *compute.Delta) {
	for deltaFragment := range worker.queue {
		delta := worker.deltaBuilder.InsertDeltaFragment(deltaFragment)
		if delta != nil {
			// Write to channels that might be closed
			func() {
				defer func() { recover() }()
				for _, queue := range queues {
					queue <- delta
				}
			}()
		}
	}
}

// A DeltaMatchWorker consumes reconstructed deltas and calculates whether or not
// the two orders can be matched
type DeltaMatchWorker struct {
	logger              *logger.Logger
	deltaFragmentMatrix *compute.DeltaFragmentMatrix
	queue               chan *compute.Delta
}

// NewDeltaMatchWorker returns a new DeltaMatchWorker consumes deltas from the queue
// and removes the fragments from the DeltaFragmentMatrix if the two orders match
func NewDeltaMatchWorker(logger *logger.Logger, deltaFragmentMatrix *compute.DeltaFragmentMatrix, queue chan *compute.Delta) *DeltaMatchWorker {
	return &DeltaMatchWorker{
		logger:              logger,
		deltaFragmentMatrix: deltaFragmentMatrix,
		queue:               queue,
	}
}

// Run the DeltaMatchWorker. When two orders match:
// 1) Remove the fragments for each order from the OrderFragmentMatrix and
// 2) Write the match to the output queues
func (worker *DeltaMatchWorker) Run(queues ...chan *compute.Delta) {
	for delta := range worker.queue {
		if delta.IsMatch(prime) {
			if err := worker.deltaFragmentMatrix.RemoveOrderFragment(delta.BuyOrderID); err != nil {
				worker.logger.Compute(logger.Error, fmt.Sprintf("cannot remove buy order fragment: %s", err.Error()))
			}
			if err := worker.deltaFragmentMatrix.RemoveOrderFragment(delta.SellOrderID); err != nil {
				worker.logger.Compute(logger.Error, fmt.Sprintf("cannot remove sell order fragment: %s", err.Error()))
			}
			worker.logger.OrderMatch(logger.Info, delta.ID.String(), delta.BuyOrderID.String(), delta.SellOrderID.String())
			for _, queue := range queues {
				queue <- delta
			}
		}
	}
}
