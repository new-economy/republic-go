package darknode

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	"github.com/republicprotocol/go-do"

	"github.com/republicprotocol/republic-go/logger"
	"github.com/republicprotocol/republic-go/order"
	"github.com/republicprotocol/republic-go/rpc"
	"google.golang.org/grpc"

	"github.com/republicprotocol/republic-go/smpc"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/republicprotocol/republic-go/ethereum/client"
	"github.com/republicprotocol/republic-go/ethereum/contracts"
	"github.com/republicprotocol/republic-go/identity"
)

type EpochRoute struct {
	Epoch          [32]byte
	OrderFragments chan<- order.Fragment
	DeltaFragments chan<- smpc.DeltaFragment
}

type Darknodes []Darknode

type Darknode struct {
	Config
	Logger *logger.Logger

	id           identity.ID
	address      identity.Address
	multiAddress identity.MultiAddress

	darknodeRegistry contracts.DarkNodeRegistry
	ocean            Ocean

	relayer rpc.RelayService
	smpcer  rpc.ComputerService

	epochRoutes    chan EpochRoute
	orderFragments chan order.Fragment
	deltaFragments chan smpc.DeltaFragment
}

// NewDarknode returns a new Darknode.
func NewDarknode(config Config) (Darknode, error) {
	node := Darknode{
		Config: config,
		Logger: logger.StdoutLogger,
	}

	// Get identity information from the Config
	key, err := identity.NewKeyPairFromPrivateKey(node.Config.Key.PrivateKey)
	if err != nil {
		return node, fmt.Errorf("cannot get ID from private key: %v", err)
	}
	node.id = key.ID()
	node.address = key.Address()
	node.multiAddress = config.Network.MultiAddress

	// Open a connection to the Ethereum network
	transactOpts := bind.NewKeyedTransactor(config.Key.PrivateKey)
	client, err := client.Connect(
		config.Ethereum.URI,
		config.Ethereum.Network,
		config.Ethereum.RepublicTokenAddress,
		config.Ethereum.DarkNodeRegistryAddress,
		config.Ethereum.HyperdriveRegistryAddres,
	)
	if err != nil {
		println("ERR!", err)
		return node, err
	}

	// Create bindings to the DarknodeRegistry and Ocean
	darknodeRegistry, err := contracts.NewDarkNodeRegistry(context.Background(), client, transactOpts, &bind.CallOpts{})
	if err != nil {
		return Darknode{}, err
	}
	node.darknodeRegistry = darknodeRegistry
	node.ocean = NewOcean(darknodeRegistry)

	// Create a channel for notifying the Darknode about new epochs
	node.epochRoutes = make(chan EpochRoute, 2)
	node.orderFragments = make(chan order.Fragment)
	node.deltaFragments = make(chan smpc.DeltaFragment)

	return node, nil
}

// ServeRPC services, using a gRPC server, until the done channel is closed or
// an error is encountered.
func (node *Darknode) ServeRPC(done <-chan struct{}) <-chan error {
	errs := make(chan error, 1)

	go func() {
		defer close(errs)

		server := grpc.NewServer()
		go func() {
			defer server.Stop()
			<-done
		}()

		node.relayer = rpc.NewRelayService(rpc.Options{}, node, node.Logger)
		node.relayer.Register(server)

		node.smpcer = rpc.NewComputerService()
		node.smpcer.Register(server)

		listener, err := net.Listen("tcp", fmt.Sprintf("%v:%v", node.Config.Host, node.Config.Port))
		if err != nil {
			node.Logger.Network(logger.Error, err.Error())
			errs <- err
			return
		}

		node.Logger.Network(logger.Info, fmt.Sprintf("listening on %v:%v", node.Config.Host, node.Config.Port))
		if err := server.Serve(listener); err != nil {
			node.Logger.Network(logger.Error, err.Error())
			errs <- err
			return
		}
	}()

	time.Sleep(2 * time.Second)
	return errs
}

// RunWatcher will watch for changes to the Ocean and run the secure
// multi-party computation with new Pools. Stops when the done channel is
// closed, and will attempt to recover from errors encountered while
// interacting with the Ocean.
func (node *Darknode) RunWatcher(done <-chan struct{}) {
	go func() {

		// Maintain multiple done channels so that multiple epochs can be running
		// in parallel
		var prevDone chan struct{}
		var currDone chan struct{}
		defer func() {
			if prevDone != nil {
				close(prevDone)
			}
			if currDone != nil {
				close(currDone)
			}
		}()

		// Looping until the done channel is closed will recover from errors
		// returned by watching the Ocean
		for {
			select {
			case <-done:
				return
			default:
			}

			epochs, errs := node.ocean.Watch(done)

			for quit := false; !quit; {
				select {

				case <-done:
					return

				case err, ok := <-errs:
					if !ok {
						quit = true
						break
					}
					node.Logger.Network(logger.Error, err.Error())

				case epoch, ok := <-epochs:
					if !ok {
						quit = true
						break
					}

					if prevDone != nil {
						close(prevDone)
					}
					prevDone = currDone
					currDone = make(chan struct{})
					node.RunEpoch(currDone, epoch)
				}
			}
		}
	}()
}

// RunEpoch until the done channel is closed. Order fragments will be received,
// computed, and broadcast in accordance to the epoch.
func (node *Darknode) RunEpoch(done <-chan struct{}, epoch contracts.Epoch) {

	// FIXME: Compute n and k from the epoch
	n := int64(5)
	k := (n + 1) * 2 / 3

	smpcerID := smpc.ComputerID{}
	copy(smpcerID[:], node.ID()[:])
	smpcer := smpc.NewComputer(smpcerID, n, k)

	// Run secure multi-party computer
	orderFragments, deltaFragments := make(chan order.Fragment), make(chan smpc.DeltaFragment)
	defer func() {
		close(orderFragments)
		close(deltaFragments)
	}()
	deltaFragmentsComputed, deltasComputed := smpcer.ComputeOrderMatches(done, orderFragments, deltaFragments)

	go func() {
		for delta := range deltasComputed {
			if delta.IsMatch(smpc.Prime) {
				node.Logger.OrderMatch(logger.Info, delta.ID.String(), delta.BuyOrderID.String(), delta.SellOrderID.String())
				node.OrderMatchToHyperdrive(delta)
			}
		}
	}()

	computationConns := []chan<- *rpc.Computation{}

	for _, peerMulti := range node.Config.Network.BootstrapMultiAddresses {
		peerID := peerMulti.ID()

		computationsIn := make(chan *rpc.Computation)
		computationConns = append(computationConns, computationsIn)

		var computations <-chan *rpc.Computation
		var errs <-chan error

		go func(peerMulti identity.MultiAddress) {
			if bytes.Compare(node.ID(), peerID) < 0 {
				computations, errs = node.smpcer.WaitForCompute(peerMulti, computationsIn)
				go func() {
					for err := range errs {
						node.Logger.Compute(logger.Error, "server error: "+err.Error())
					}
				}()
			} else {
				client, err := rpc.NewClient(context.Background(), peerMulti, node.MultiAddress())
				if err != nil {
					node.Logger.Network(logger.Error, err.Error())
					return
				}
				computations, errs = client.Compute(context.Background(), computationsIn)
				go func() {
					for err := range errs {
						node.Logger.Compute(logger.Error, "client error: "+err.Error())
					}
				}()

				multi := node.MultiAddress()
				computationsIn <- &rpc.Computation{MultiAddress: rpc.MarshalMultiAddress(&multi)}
			}

			go func() {
				for computation := range computations {
					if computation.DeltaFragment != nil {
						deltaFragment, err := rpc.UnmarshalDeltaFragment(computation.DeltaFragment)
						if err != nil {
							node.Logger.Compute(logger.Error, err.Error())
						}
						deltaFragments <- deltaFragment
					}
				}
			}()

		}(peerMulti)
	}

	go func() {
		for deltaFragment := range deltaFragmentsComputed {
			println("SENDING DELTA FRAGMENT")
			do.CoForAll(computationConns, func(i int) {
				computationConns[i] <- &rpc.Computation{
					DeltaFragment: rpc.MarshalDeltaFragment(&deltaFragment),
				}
			})
		}
	}()

	node.epochRoutes <- EpochRoute{
		Epoch:          epoch.Blockhash,
		OrderFragments: orderFragments,
		DeltaFragments: deltaFragments,
	}

	<-done
}

// RunEpochSwitch until the done channel is closed. EpochRoutes will be
// received and used to switch messages to the correct goroutine. This allows
// multiple goroutines to run in parallel, each processing a different epoch.
func (node *Darknode) RunEpochSwitch(done <-chan struct{}) {
	go func() {
		var currRoute EpochRoute
		var ok bool

		select {
		case <-done:
		case currRoute, ok = <-node.epochRoutes:
			if !ok {
				return
			}
		}

		for {
			select {
			case <-done:
				return
			case route, ok := <-node.epochRoutes:
				if !ok {
					return
				}
				currRoute = route
			case orderFragment, ok := <-node.orderFragments:
				if !ok {
					return
				}
				select {
				case <-done:
				case currRoute.OrderFragments <- orderFragment:
				}
			case deltaFragment, ok := <-node.deltaFragments:
				if !ok {
					return
				}
				select {
				case <-done:
				case currRoute.DeltaFragments <- deltaFragment:
				}
			}
		}
	}()
}

// OrderMatchToHyperdrive converts an order match into a hyperdrive.Tx and
// forwards it to the Hyperdrive.
func (node *Darknode) OrderMatchToHyperdrive(delta smpc.Delta) {
	if !delta.IsMatch(smpc.Prime) {
		return
	}

	// TODO: Implement
}

// OnOpenOrder implements the rpc.RelayDelegate interface.
func (node *Darknode) OnOpenOrder(from identity.MultiAddress, orderFragment *order.Fragment) {
	println("ORDER RECEIVED")
	node.orderFragments <- *orderFragment
	println("ORDER PROCESSED")
}

// Ocean returns the Ocean used by this Darknode for computing the Pools and
// its position in them.
func (node *Darknode) Ocean() Ocean {
	return node.ocean
}

// ID returns the ID of the Darknode.
func (node *Darknode) ID() identity.ID {
	return node.id
}

// Address returns the Address of the Darknode.
func (node *Darknode) Address() identity.Address {
	return node.address
}

// MultiAddress returns the MultiAddress of the Darknode.
func (node *Darknode) MultiAddress() identity.MultiAddress {
	return node.multiAddress
}
